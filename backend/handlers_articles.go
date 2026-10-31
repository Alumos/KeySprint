package main

import (
	"database/sql"
	"net/http"
	"strings"
)

func scanArticleRows(rows *sql.Rows) []map[string]any {
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var teacher sql.NullInt64
		var title, content, source, created string
		_ = rows.Scan(&id, &teacher, &title, &content, &source, &created)
		out = append(out, map[string]any{"id": id, "teacher_id": teacher.Int64, "title": title, "content": content, "source": source, "created_at": created})
	}
	return out
}

func (a *app) getArticles(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query("SELECT id,teacher_id,title,content,source,created_at FROM articles WHERE teacher_id=? ORDER BY created_at DESC,id DESC", u.ID)
	if err != nil {
		writeError(w, 500, "读取文章失败")
		return
	}
	defer rows.Close()
	writeJSON(w, 200, scanArticleRows(rows))
}

func (a *app) getPublicArticle(w http.ResponseWriter, r *http.Request) {
	// Students read the article through their competition code, which keeps the
	// shared library from being scraped by walking article IDs.
	code := cleanCode(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, 404, "文章不存在")
		return
	}
	var id int64
	var title, content, source, created string
	var teacher sql.NullInt64
	err := a.db.QueryRow(`SELECT a.id,a.teacher_id,a.title,a.content,a.source,a.created_at FROM articles a
		WHERE a.id=? AND EXISTS(SELECT 1 FROM competitions c WHERE c.article_id=a.id AND c.access_code=? AND c.status IN ('waiting','running'))`,
		pathInt(r, "id"), code).Scan(&id, &teacher, &title, &content, &source, &created)
	if err != nil {
		writeError(w, 404, "文章不存在")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "teacher_id": teacher.Int64, "title": title, "content": content, "source": source, "created_at": created})
}

func articleInput(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	var in struct{ Title, Content string }
	if !decodeJSON(w, r, &in) {
		return "", "", false
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	if in.Title == "" || in.Content == "" {
		writeError(w, 400, "标题和内容不能为空")
		return "", "", false
	}
	return in.Title, in.Content, true
}

func (a *app) createArticle(w http.ResponseWriter, r *http.Request) {
	title, content, ok := articleInput(w, r)
	if !ok {
		return
	}
	u := currentUser(r)
	res, err := a.db.Exec("INSERT INTO articles(teacher_id,title,content,source) VALUES(?,?,?,'manual')", u.ID, title, content)
	if err != nil {
		writeError(w, 500, "创建失败")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 200, map[string]any{"success": true, "id": id})
}

func (a *app) canEditArticle(r *http.Request, id int64) bool {
	u := currentUser(r)
	var owner sql.NullInt64
	if a.db.QueryRow("SELECT teacher_id FROM articles WHERE id=?", id).Scan(&owner) != nil {
		return false
	}
	return u.IsAdmin || owner.Int64 == u.ID
}

func (a *app) updateArticle(w http.ResponseWriter, r *http.Request) {
	id := pathInt(r, "id")
	if !a.canEditArticle(r, id) {
		writeError(w, 403, "只能编辑自己的文章")
		return
	}
	title, content, ok := articleInput(w, r)
	if !ok {
		return
	}
	_, _ = a.db.Exec("UPDATE articles SET title=?,content=? WHERE id=?", title, content, id)
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) deleteArticle(w http.ResponseWriter, r *http.Request) {
	id := pathInt(r, "id")
	if !a.canEditArticle(r, id) {
		writeError(w, 403, "只能删除自己的文章")
		return
	}
	var n int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM competitions WHERE article_id=?", id).Scan(&n)
	if n > 0 {
		writeError(w, 400, "文章已被竞赛使用，不能删除")
		return
	}
	_, _ = a.db.Exec("DELETE FROM articles WHERE id=?", id)
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) getLibrary(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query(`SELECT a.id,a.title,a.content,a.source,a.created_at,COALESCE(t.display_name,''),CASE WHEN a.teacher_id=? THEN 1 ELSE 0 END,CASE WHEN ta.article_id IS NULL THEN 0 ELSE 1 END FROM articles a LEFT JOIN teachers t ON t.id=a.teacher_id LEFT JOIN teacher_articles ta ON ta.article_id=a.id AND ta.teacher_id=? ORDER BY a.created_at DESC,a.id DESC`, u.ID, u.ID)
	if err != nil {
		writeError(w, 500, "读取文章库失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var title, content, source, created, uploader string
		var mine, picked int
		_ = rows.Scan(&id, &title, &content, &source, &created, &uploader, &mine, &picked)
		out = append(out, map[string]any{"id": id, "title": title, "content": content, "source": source, "created_at": created, "uploader": uploader, "mine": mine, "picked": picked})
	}
	writeJSON(w, 200, out)
}

func (a *app) getSelectedArticles(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query(`SELECT DISTINCT a.id,a.title,a.content,a.created_at,COALESCE(t.display_name,'') FROM articles a LEFT JOIN teachers t ON t.id=a.teacher_id LEFT JOIN teacher_articles ta ON ta.article_id=a.id AND ta.teacher_id=? WHERE a.teacher_id=? OR ta.teacher_id=? ORDER BY a.created_at DESC`, u.ID, u.ID, u.ID)
	if err != nil {
		writeError(w, 500, "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var title, content, created, uploader string
		_ = rows.Scan(&id, &title, &content, &created, &uploader)
		out = append(out, map[string]any{"id": id, "title": title, "content": content, "created_at": created, "uploader": uploader})
	}
	writeJSON(w, 200, out)
}

func (a *app) pickArticle(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		ArticleID int64 `json:"articleId"`
		Picked    bool  `json:"picked"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	var n int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM articles WHERE id=?", in.ArticleID).Scan(&n)
	if n == 0 {
		writeError(w, 404, "文章不存在")
		return
	}
	if in.Picked {
		_, _ = a.db.Exec("INSERT OR IGNORE INTO teacher_articles(teacher_id,article_id) VALUES(?,?)", u.ID, in.ArticleID)
	} else {
		_, _ = a.db.Exec("DELETE FROM teacher_articles WHERE teacher_id=? AND article_id=?", u.ID, in.ArticleID)
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
