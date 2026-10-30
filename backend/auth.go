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

const (
	sessionScope   = "session"
	websocketScope = "ws"
	sessionTTL     = 7 * 24 * time.Hour
	websocketTTL   = 90 * time.Second
)

func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// signToken issues a HS256 token for a single purpose. Teachers use a session
// token in the x-admin-token header; the browser cannot set headers on a
// WebSocket upgrade, so the socket uses a short-lived, single-purpose ticket
// instead of putting the seven day session token in a URL.
func (a *app) signToken(id int64, scope string, ttl time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":   strconv.FormatInt(id, 10),
		"scope": scope,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(ttl).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.jwtSecret))
}

func (a *app) createToken(id int64) (string, error) {
	return a.signToken(id, sessionScope, sessionTTL)
}

func (a *app) signWebsocketTicket(id int64) (string, error) {
	return a.signToken(id, websocketScope, websocketTTL)
}

func (a *app) parseToken(raw string) (int64, error) {
	return a.parseScopedToken(raw, sessionScope)
}

func (a *app) parseScopedToken(raw, scope string) (int64, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Bearer "))
	if raw == "" {
		return 0, errors.New("missing token")
	}
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(a.jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return 0, errors.New("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("invalid claims")
	}
	if value, _ := claims["scope"].(string); value != "" {
		if value != scope {
			return 0, errors.New("invalid scope")
		}
	} else if scope != sessionScope {
		// Tokens issued before scopes existed stay valid as session tokens only.
		return 0, errors.New("missing scope")
	}
	sub, err := claims.GetSubject()
	if err != nil {
		return 0, err
	}
	var id int64
	if _, err = fmtSscan(sub, &id); err != nil || id < 1 {
		return 0, errors.New("invalid subject")
	}
	return id, nil
}

// requireAuth accepts the session token from the x-admin-token header only.
// The old ?token= fallback leaked long-lived tokens into access logs, browser
// history and Referer headers, so exports now download through an authenticated
// fetch instead.
func (a *app) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := a.parseToken(r.Header.Get("x-admin-token"))
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
