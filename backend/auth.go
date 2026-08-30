package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type authKey struct{}

type identity struct {
	ID      int64
	IsAdmin bool
}

func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (a *app) createToken(id int64) (string, error) {
	claims := jwt.MapClaims{"sub": strconv.FormatInt(id, 10), "iat": time.Now().Unix(), "exp": time.Now().Add(7 * 24 * time.Hour).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.jwtSecret))
}

func (a *app) parseToken(raw string) (int64, error) {
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(a.jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return 0, errors.New("invalid token")
	}
	sub, err := token.Claims.GetSubject()
	if err != nil {
		return 0, err
	}
	var id int64
	_, err = fmtSscan(sub, &id)
	return id, err
}

func (a *app) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("x-admin-token")
		if raw == "" {
			raw = r.URL.Query().Get("token")
		}
		if strings.HasPrefix(raw, "Bearer ") {
			raw = strings.TrimPrefix(raw, "Bearer ")
		}
		id, err := a.parseToken(raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "登录已过期，请重新登录")
			return
		}
		var isAdmin int
		if err := a.db.QueryRowContext(r.Context(), "SELECT is_admin FROM teachers WHERE id=?", id).Scan(&isAdmin); err != nil {
			writeError(w, http.StatusUnauthorized, "账号不存在")
			return
		}
		ctx := context.WithValue(r.Context(), authKey{}, identity{ID: id, IsAdmin: isAdmin == 1})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentUser(r *http.Request) identity { return r.Context().Value(authKey{}).(identity) }
