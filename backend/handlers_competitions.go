package main

import (
	"crypto/rand"
	"database/sql"
	"net/http"
	"strings"
	"time"
)

const compSelect = `SELECT c.id,c.teacher_id,c.article_id,c.access_code,c.status,c.time_limit,c.pinyin_enabled,c.started_at,c.stopped_at,c.created_at,a.title,c.text_options FROM competitions c JOIN articles a ON a.id=c.article_id`

func scanCompetitions(rows *sql.Rows) []map[string]any {
	out := []map[string]any{}
	for rows.Next() {
		var id, teacher, article, limit int64
		var pinyinEnabled int
		var code, status, created, title string
		var started, stopped, textRaw sql.NullString
		_ = rows.Scan(&id, &teacher, &article, &code, &status, &limit, &pinyinEnabled, &started, &stopped, &created, &title, &textRaw)
		options := parseTextOptions(textRaw.String, pinyinEnabled)
		out = append(out, map[string]any{"id": id, "teacher_id": teacher, "article_id": article, "access_code": code, "status": status, "time_limit": limit, "pinyin_enabled": options.Pinyin, "text_options": options, "started_at": started.String, "stopped_at": stopped.String, "created_at": created, "article_title": title})
	}
	return out
}

func (a *app) getCompetitions(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query(compSelect+" WHERE c.teacher_id=? ORDER BY c.created_at DESC,c.id DESC", u.ID)
	if err != nil {
		writeError(w, 500, "读取竞赛失败")
		return
	}
	defer rows.Close()
	writeJSON(w, 200, scanCompetitions(rows))
}
func (a *app) getActiveCompetitions(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query(compSelect+" WHERE c.teacher_id=? AND c.status IN ('waiting','running') ORDER BY c.created_at DESC", u.ID)
	if err != nil {
		writeError(w, 500, "读取竞赛失败")
		return
	}
	defer rows.Close()
	writeJSON(w, 200, scanCompetitions(rows))
}

func (a *app) competitionByCode(w http.ResponseWriter, r *http.Request) {
	row := a.db.QueryRow(compSelect+" WHERE c.access_code=? AND c.status IN ('waiting','running')", cleanCode(r.PathValue("code")))
	var id, teacher, article, limit int64
	var pinyinEnabled int
	var code, status, created, title string
	var started, stopped, textRaw sql.NullString
	if row.Scan(&id, &teacher, &article, &code, &status, &limit, &pinyinEnabled, &started, &stopped, &created, &title, &textRaw) != nil {
		writeError(w, 404, "比赛不存在或已结束")
		return
	}
	options := parseTextOptions(textRaw.String, pinyinEnabled)
	writeJSON(w, 200, map[string]any{"id": id, "teacher_id": teacher, "article_id": article, "access_code": code, "status": status, "time_limit": limit, "pinyin_enabled": options.Pinyin, "text_options": options, "started_at": started.String, "stopped_at": stopped.String, "created_at": created, "article_title": title})
}

func randomCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func (a *app) createCompetition(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		ArticleID     int64        `json:"articleId"`
		TimeLimit     int64        `json:"timeLimit"`
		CustomCode    string       `json:"customCode"`
		RequestID     string       `json:"requestId"`
		PinyinEnabled *bool        `json:"pinyinEnabled"`
		TextOptions   *textOptions `json:"textOptions"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.TimeLimit == 0 {
		in.TimeLimit = 300
	}
	if in.TimeLimit < 10 || in.TimeLimit > 86400 {
		writeError(w, 400, "限时应在10秒到24小时之间")
		return
	}
	var allowed int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM articles a LEFT JOIN teacher_articles ta ON ta.article_id=a.id AND ta.teacher_id=? WHERE a.id=? AND (a.teacher_id=? OR ta.teacher_id=?)`, u.ID, in.ArticleID, u.ID, u.ID).Scan(&allowed)
	if allowed == 0 {
		writeError(w, 403, "该文章未加入你的可用列表")
		return
	}
	code := cleanCode(in.CustomCode)
	if code != "" && (len(code) < 3 || len(code) > 20) {
		writeError(w, 400, "比赛编号应为3至20位")
		return
	}
	in.RequestID = strings.TrimSpace(in.RequestID)
	if !validRequestID(in.RequestID) {
		writeError(w, http.StatusBadRequest, "创建请求标识无效")
		return
	}
	if id, existingCode, ok := a.competitionByRequest(u.ID, in.RequestID); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true, "id": id, "accessCode": existingCode, "created": false,
		})
		return
	}
	// textOptions is the current contract; pinyinEnabled stays supported for
	// pages opened before the switch to a multi-option list.
	options := defaultTextOptions()
	if in.TextOptions != nil {
		options = *in.TextOptions
	} else if in.PinyinEnabled != nil {
		options.Pinyin = *in.PinyinEnabled
	}

	var id int64
	for attempt := 0; attempt < 10; attempt++ {
		if code == "" {
			code = randomCode()
		}
		res, err := a.db.Exec(`INSERT INTO competitions(
			teacher_id,article_id,access_code,request_id,time_limit,pinyin_enabled,text_options
		) VALUES(?,?,?,?,?,?,?)`, u.ID, in.ArticleID, code, nullableString(in.RequestID), in.TimeLimit, options.Pinyin, options.encode())
		if err == nil {
			id, _ = res.LastInsertId()
			break
		}
		if existingID, existingCode, ok := a.competitionByRequest(u.ID, in.RequestID); ok {
			writeJSON(w, http.StatusOK, map[string]any{
				"success": true, "id": existingID, "accessCode": existingCode, "created": false,
			})
			return
		}
		if !strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusInternalServerError, "创建竞赛失败")
			return
		}
		if cleanCode(in.CustomCode) != "" {
			writeError(w, http.StatusConflict, "该比赛编号已被使用")
			return
		}
		code = ""
	}
	if id == 0 {
		writeError(w, http.StatusInternalServerError, "暂时无法生成比赛编号，请重试")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "id": id, "accessCode": code, "created": true,
	})
}

func validRequestID(value string) bool {
	if value == "" { // Keep the API compatible with older open browser tabs.
		return true
	}
	if len(value) < 8 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func (a *app) competitionByRequest(teacherID int64, requestID string) (int64, string, bool) {
	if requestID == "" {
		return 0, "", false
	}
	var id int64
	var code string
	err := a.db.QueryRow(`SELECT id,access_code FROM competitions
		WHERE teacher_id=? AND request_id=?`, teacherID, requestID).Scan(&id, &code)
	return id, code, err == nil
}

func (a *app) ownCompetition(r *http.Request, id int64) (int64, string, bool) {
	u := currentUser(r)
	var limit int64
	var status string
	err := a.db.QueryRow("SELECT time_limit,status FROM competitions WHERE id=? AND teacher_id=?", id, u.ID).Scan(&limit, &status)
	return limit, status, err == nil
}

func (a *app) startCompetition(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID int64 `json:"id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u := currentUser(r)
	res, err := a.db.Exec(`UPDATE competitions
		SET status='running',started_at=datetime('now'),stopped_at=NULL
		WHERE id=? AND teacher_id=? AND status='waiting'`, in.ID, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "开始竞赛失败")
		return
	}
	updated, _ := res.RowsAffected()
	if updated == 0 {
		if _, _, ok := a.ownCompetition(r, in.ID); !ok {
			writeError(w, http.StatusNotFound, "竞赛不存在")
		} else {
			writeError(w, http.StatusConflict, "竞赛不能重复开始")
		}
		return
	}
	var limit int64
	if err := a.db.QueryRow("SELECT time_limit FROM competitions WHERE id=?", in.ID).Scan(&limit); err != nil {
		writeError(w, http.StatusInternalServerError, "读取竞赛失败")
		return
	}
	var data map[string]any
	if rows, err := a.db.Query(compSelect+" WHERE c.id=?", in.ID); err == nil {
		items := scanCompetitions(rows)
		rows.Close()
		if len(items) > 0 {
			data = items[0]
		}
	}
	a.hub.broadcast(in.ID, "competition:start", data)
	a.scheduleStop(in.ID, time.Duration(limit+10)*time.Second)
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) stopCompetition(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID int64 `json:"id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	_, _, ok := a.ownCompetition(r, in.ID)
	if !ok {
		writeError(w, 404, "竞赛不存在")
		return
	}
	_, _ = a.db.Exec("UPDATE competitions SET status='stopped',stopped_at=datetime('now') WHERE id=?", in.ID)
	a.cancelStop(in.ID)
	a.hub.broadcast(in.ID, "competition:stop", map[string]any{"id": in.ID, "reason": "manual"})
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) deleteCompetition(w http.ResponseWriter, r *http.Request) {
	id := pathInt(r, "id")
	_, status, ok := a.ownCompetition(r, id)
	if !ok {
		writeError(w, 404, "竞赛不存在")
		return
	}
	if status == "running" {
		writeError(w, 400, "请先停止进行中的竞赛")
		return
	}
	_, _ = a.db.Exec("DELETE FROM competitions WHERE id=?", id)
	a.cancelStop(id)
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) getParticipants(w http.ResponseWriter, r *http.Request) {
	id := pathInt(r, "id")
	if _, _, ok := a.ownCompetition(r, id); !ok {
		writeError(w, 404, "竞赛不存在")
		return
	}
	writeJSON(w, 200, map[string]any{"participants": a.hub.participants(id)})
}
