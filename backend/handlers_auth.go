package main

import (
	"database/sql"
	"net/http"
	"strings"
)

func (a *app) setupStatus(w http.ResponseWriter, r *http.Request) {
	var n int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM teachers").Scan(&n)
	writeJSON(w, 200, map[string]any{"done": n > 0})
}

func (a *app) setupInit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TeacherUsername    string `json:"teacherUsername"`
		TeacherPassword    string `json:"teacherPassword"`
		TeacherDisplayName string `json:"teacherDisplayName"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.TeacherUsername = strings.TrimSpace(in.TeacherUsername)
	in.TeacherDisplayName = strings.TrimSpace(in.TeacherDisplayName)
	if len(in.TeacherUsername) < 2 || len(in.TeacherPassword) < 6 {
		writeError(w, 400, "用户名至少2位，密码至少6位")
		return
	}
	if in.TeacherDisplayName == "" {
		in.TeacherDisplayName = in.TeacherUsername
	}
	h, err := hashPassword(in.TeacherPassword)
	if err != nil {
		writeError(w, 500, "初始化失败")
		return
	}
	// A conditional insert is evaluated atomically by SQLite, so two concurrent
	// first-run requests cannot both create an administrator.
	res, err := a.db.Exec(`INSERT INTO teachers(username,password_hash,display_name,is_admin)
		SELECT ?,?,?,1 WHERE NOT EXISTS (SELECT 1 FROM teachers)`,
		in.TeacherUsername, h, in.TeacherDisplayName)
	if err != nil {
		writeError(w, 400, "用户名已存在")
		return
	}
	if created, _ := res.RowsAffected(); created == 0 {
		writeError(w, 409, "系统已经初始化")
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password string }
	if !decodeJSON(w, r, &in) {
		return
	}
	var id int64
	var hash, name string
	var admin int
	if err := a.db.QueryRow("SELECT id,password_hash,display_name,is_admin FROM teachers WHERE username=?", strings.TrimSpace(in.Username)).Scan(&id, &hash, &name, &admin); err != nil || !checkPassword(hash, in.Password) {
		writeError(w, 401, "用户名或密码错误")
		return
	}
	token, err := a.createToken(id)
	if err != nil {
		writeError(w, 500, "登录失败")
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "token": token, "displayName": name, "isAdmin": admin == 1})
}

func (a *app) session(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var name, username string
	_ = a.db.QueryRow("SELECT username,display_name FROM teachers WHERE id=?", u.ID).Scan(&username, &name)
	writeJSON(w, 200, map[string]any{"id": u.ID, "username": username, "displayName": name, "isAdmin": u.IsAdmin})
}

func mustAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !currentUser(r).IsAdmin {
		writeError(w, 403, "仅系统管理员可执行此操作")
		return false
	}
	return true
}

func (a *app) getTeachers(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	rows, err := a.db.Query("SELECT id,username,display_name,is_admin,created_at FROM teachers ORDER BY id")
	if err != nil {
		writeError(w, 500, "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var username, name, created string
		var admin int
		_ = rows.Scan(&id, &username, &name, &admin, &created)
		out = append(out, map[string]any{"id": id, "username": username, "display_name": name, "is_admin": admin, "created_at": created})
	}
	writeJSON(w, 200, out)
}

func (a *app) createTeacher(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	var in struct{ Username, Password, DisplayName string }
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Username)) < 2 || len(in.Password) < 6 {
		writeError(w, 400, "用户名至少2位，密码至少6位")
		return
	}
	if in.DisplayName == "" {
		in.DisplayName = in.Username
	}
	h, err := hashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建失败")
		return
	}
	res, err := a.db.Exec("INSERT INTO teachers(username,password_hash,display_name) VALUES(?,?,?)", strings.TrimSpace(in.Username), h, strings.TrimSpace(in.DisplayName))
	if err != nil {
		writeError(w, 400, "用户名已存在")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 200, map[string]any{"success": true, "id": id})
}

func (a *app) deleteTeacher(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	u := currentUser(r)
	id := pathInt(r, "id")
	if id == u.ID {
		writeError(w, 400, "不能删除自己")
		return
	}
	var admin int
	if a.db.QueryRow("SELECT is_admin FROM teachers WHERE id=?", id).Scan(&admin) == sql.ErrNoRows {
		writeError(w, 404, "教师不存在")
		return
	}
	if admin == 1 {
		writeError(w, 400, "不能删除系统管理员")
		return
	}
	if _, err := a.db.Exec("DELETE FROM teachers WHERE id=?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除教师失败")
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.NewPassword) < 6 {
		writeError(w, 400, "新密码至少6位")
		return
	}
	u := currentUser(r)
	var h string
	_ = a.db.QueryRow("SELECT password_hash FROM teachers WHERE id=?", u.ID).Scan(&h)
	if !checkPassword(h, in.OldPassword) {
		writeError(w, 400, "原密码错误")
		return
	}
	nh, err := hashPassword(in.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "修改密码失败")
		return
	}
	if _, err = a.db.Exec("UPDATE teachers SET password_hash=? WHERE id=?", nh, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "修改密码失败")
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

// createWebsocketTicket issues the short-lived credential used by the teacher
// WebSocket. It keeps the seven day session token out of WebSocket URLs, which
// proxies and CDN logs tend to record.
func (a *app) createWebsocketTicket(w http.ResponseWriter, r *http.Request) {
	ticket, err := a.signWebsocketTicket(currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成连接凭证失败")
		return
	}
	writeJSON(w, 200, map[string]any{"ticket": ticket, "expires_in": int(websocketTTL.Seconds())})
}

func (a *app) resetPassword(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	var in struct {
		NewPassword string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.NewPassword) < 6 {
		writeError(w, 400, "新密码至少6位")
		return
	}
	nh, err := hashPassword(in.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "重置密码失败")
		return
	}
	res, err := a.db.Exec("UPDATE teachers SET password_hash=? WHERE id=?", nh, pathInt(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "重置密码失败")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "教师不存在")
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
