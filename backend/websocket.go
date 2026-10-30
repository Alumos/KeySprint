package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// originAllowed guards the WebSocket upgrade. Browsers always send Origin;
// non-browser clients may omit it. Behind a CDN (for example EdgeOne) the page
// origin and the origin host seen by the container can differ, so explicit
// ALLOWED_ORIGINS entries are accepted as well.
func (a *app) originAllowed(r *http.Request) bool {
	raw := strings.TrimSpace(r.Header.Get("Origin"))
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	for _, allowed := range a.allowedOrigins {
		if strings.EqualFold(allowed, parsed.Host) || strings.EqualFold(allowed, strings.TrimRight(raw, "/")) {
			return true
		}
	}
	return false
}

// websocket authenticates the connection before upgrading: a teacher presents a
// short-lived ticket from POST /api/ws-ticket, a student presents the
// competition access code they already typed. Unauthenticated sockets are
// rejected, which also stops cross-origin socket hijacking.
func (a *app) websocket(w http.ResponseWriter, r *http.Request) {
	if !a.originAllowed(r) {
		writeError(w, http.StatusForbidden, "连接来源不被允许")
		return
	}
	query := r.URL.Query()
	room, _ := strconv.ParseInt(query.Get("competitionId"), 10, 64)
	code := cleanCode(query.Get("code"))
	studentID := strings.TrimSpace(query.Get("studentId"))
	studentName := strings.TrimSpace(query.Get("studentName"))

	c := &client{send: make(chan []byte, 32), rooms: map[int64]bool{}}
	switch {
	case strings.TrimSpace(query.Get("ticket")) != "":
		id, err := a.parseScopedToken(query.Get("ticket"), websocketScope)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "连接凭证无效，请刷新页面")
			return
		}
		var exists int
		if err := a.db.QueryRow("SELECT COUNT(*) FROM teachers WHERE id=?", id).Scan(&exists); err != nil || exists == 0 {
			writeError(w, http.StatusUnauthorized, "账号不存在")
			return
		}
		c.teacher = true
		c.allowRoom = func(competitionID int64) bool {
			return a.teacherOwnsCompetition(id, competitionID)
		}
	case room > 0 && code != "" && studentID != "":
		status, err := a.competitionStatusByCode(room, code)
		if err != nil || (status != "waiting" && status != "running") {
			writeError(w, http.StatusUnauthorized, "比赛编号不正确或比赛已结束")
			return
		}
		c.rooms[room] = true
		c.studentID = studentID
		c.studentName = studentName
	default:
		writeError(w, http.StatusUnauthorized, "缺少连接凭证")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c.conn = conn
	a.hub.attach(c)
}

func (a *app) teacherOwnsCompetition(teacherID, competitionID int64) bool {
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM competitions WHERE id=? AND teacher_id=?", competitionID, teacherID).Scan(&count); err != nil {
		return false
	}
	return count == 1
}

func (a *app) competitionStatusByCode(competitionID int64, code string) (string, error) {
	var status string
	err := a.db.QueryRow("SELECT status FROM competitions WHERE id=? AND access_code=?", competitionID, code).Scan(&status)
	return status, err
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
