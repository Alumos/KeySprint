package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const resultSelect = `SELECT r.id,r.competition_id,r.student_id,r.student_name,r.class_id,
	COALESCE(cl.name,''),r.correct_count,r.typed_count,r.duration,r.created_at,c.access_code,a.title
	FROM results r
	JOIN competitions c ON c.id=r.competition_id
	JOIN articles a ON a.id=c.article_id
	LEFT JOIN classes cl ON cl.id=r.class_id`

const resultOrder = ` ORDER BY
	CASE WHEN r.duration>0 THEN r.correct_count*1.0/r.duration ELSE 0 END DESC,
	CASE WHEN r.typed_count>0 THEN r.correct_count*1.0/r.typed_count ELSE 0 END DESC,
	r.correct_count DESC,r.id ASC`

type resultRow struct {
	ID            int64   `json:"id"`
	CompetitionID int64   `json:"competition_id"`
	StudentID     string  `json:"student_id"`
	StudentName   string  `json:"student_name"`
	ClassID       *int64  `json:"class_id"`
	ClassName     string  `json:"class_name"`
	CorrectCount  int64   `json:"correct_count"`
	TypedCount    int64   `json:"typed_count"`
	Accuracy      float64 `json:"accuracy"`
	Speed         float64 `json:"speed"`
	Duration      int64   `json:"duration"`
	CreatedAt     string  `json:"created_at"`
	AccessCode    string  `json:"access_code"`
	ArticleTitle  string  `json:"article_title"`
}

func queryResults(db *sql.DB, query string, args ...any) ([]resultRow, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]resultRow, 0)
	for rows.Next() {
		var item resultRow
		var classID sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.CompetitionID, &item.StudentID, &item.StudentName,
			&classID, &item.ClassName, &item.CorrectCount, &item.TypedCount,
			&item.Duration, &item.CreatedAt, &item.AccessCode, &item.ArticleTitle,
		); err != nil {
			return nil, err
		}
		if classID.Valid {
			item.ClassID = &classID.Int64
		}
		if item.TypedCount > 0 {
			item.Accuracy = float64(item.CorrectCount) / float64(item.TypedCount)
		}
		if item.Duration > 0 {
			item.Speed = float64(item.CorrectCount) * 60 / float64(item.Duration)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *app) submitResult(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CompetitionID int64  `json:"competitionId"`
		ClassID       int64  `json:"classId"`
		StudentID     string `json:"studentId"`
		StudentName   string `json:"studentName"`
		Correct       int64  `json:"correctCount"`
		Typed         *int64 `json:"typedCount"`
		LegacyTotal   *int64 `json:"totalCount"`
		Duration      int64  `json:"duration"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.StudentID = strings.TrimSpace(in.StudentID)
	in.StudentName = strings.TrimSpace(in.StudentName)
	if in.CompetitionID < 1 || in.StudentID == "" {
		writeError(w, http.StatusBadRequest, "参数不完整")
		return
	}

	// totalCount is accepted only for compatibility with an already-open page
	// from an older release. New clients use the unambiguous typedCount name.
	typedCount := int64(0)
	if in.Typed != nil {
		typedCount = *in.Typed
	} else if in.LegacyTotal != nil {
		typedCount = *in.LegacyTotal
	}
	if typedCount <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "skipped": true})
		return
	}

	var teacherID, limit, articleLength int64
	var status string
	var started sql.NullString
	err := a.db.QueryRow(`SELECT c.teacher_id,c.time_limit,c.status,c.started_at,
		LENGTH(REPLACE(a.content,char(10),''))
		FROM competitions c JOIN articles a ON a.id=c.article_id WHERE c.id=?`, in.CompetitionID).
		Scan(&teacherID, &limit, &status, &started, &articleLength)
	if err != nil {
		writeError(w, http.StatusNotFound, "竞赛不存在")
		return
	}
	if !started.Valid || status == "waiting" {
		writeError(w, http.StatusConflict, "竞赛尚未开始")
		return
	}
	if typedCount > articleLength {
		writeError(w, http.StatusBadRequest, "已输入字数超出范文长度")
		return
	}
	if in.Correct < 0 {
		in.Correct = 0
	}
	if in.Correct > typedCount {
		in.Correct = typedCount
	}
	if in.Duration < 1 {
		in.Duration = 1
	}
	if in.Duration > limit {
		in.Duration = limit
	}
	if in.StudentName == "" {
		in.StudentName = in.StudentID
	}

	classValue := a.resultClassID(teacherID, in.ClassID, in.StudentID)
	_, err = a.db.Exec(`INSERT INTO results(
		competition_id,student_id,student_name,class_id,correct_count,typed_count,duration
	) VALUES(?,?,?,?,?,?,?)
	ON CONFLICT(competition_id,student_id) DO UPDATE SET
		student_name=excluded.student_name,
		class_id=COALESCE(excluded.class_id,results.class_id),
		correct_count=excluded.correct_count,
		typed_count=excluded.typed_count,
		duration=excluded.duration,
		created_at=datetime('now')
	WHERE excluded.correct_count*results.duration > results.correct_count*excluded.duration
		OR (excluded.correct_count*results.duration = results.correct_count*excluded.duration
			AND excluded.correct_count*results.typed_count > results.correct_count*excluded.typed_count)`,
		in.CompetitionID, in.StudentID, in.StudentName, classValue, in.Correct, typedCount, in.Duration)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存成绩失败")
		return
	}
	a.hub.broadcast(in.CompetitionID, "result:update", map[string]any{
		"competitionId": in.CompetitionID,
		"studentId":     in.StudentID,
		"studentName":   in.StudentName,
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *app) resultClassID(teacherID, requestedID int64, studentID string) any {
	if requestedID > 0 {
		var valid int
		_ = a.db.QueryRow(`SELECT COUNT(*) FROM classes
			WHERE id=? AND teacher_id=? AND active=1`, requestedID, teacherID).Scan(&valid)
		if valid == 1 {
			return requestedID
		}
	}
	var classID int64
	var matches int
	_ = a.db.QueryRow(`SELECT MIN(c.id),COUNT(DISTINCT c.id)
		FROM classes c JOIN class_students s ON s.class_id=c.id
		WHERE c.teacher_id=? AND c.active=1 AND s.student_id=?`, teacherID, studentID).
		Scan(&classID, &matches)
	if matches == 1 {
		return classID
	}
	return nil
}

func (a *app) writeResults(w http.ResponseWriter, query string, args ...any) {
	items, err := queryResults(a.db, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取成绩失败")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *app) getAllResults(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	a.writeResults(w, resultSelect+" WHERE c.teacher_id=?"+resultOrder, u.ID)
}

func (a *app) getResults(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	a.writeResults(w, resultSelect+" WHERE c.teacher_id=? AND r.competition_id=?"+resultOrder, u.ID, pathInt(r, "competitionId"))
}

func (a *app) clearResults(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if _, err := a.db.Exec("DELETE FROM results WHERE competition_id IN(SELECT id FROM competitions WHERE teacher_id=?)", u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "清空成绩失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *app) deleteResult(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	res, err := a.db.Exec("DELETE FROM results WHERE id=? AND competition_id IN(SELECT id FROM competitions WHERE teacher_id=?)", pathInt(r, "id"), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "删除成绩失败")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "成绩不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *app) resultRowsForExport(r *http.Request) ([]resultRow, error) {
	u := currentUser(r)
	return queryResults(a.db, resultSelect+" WHERE c.teacher_id=?"+resultOrder, u.ID)
}

func (a *app) exportMarkdown(w http.ResponseWriter, r *http.Request) {
	items, err := a.resultRowsForExport(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "导出成绩失败")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=results.md")
	fmt.Fprintln(w, "# 打字竞赛成绩记录\n\n| 排名 | 学号 | 姓名 | 文章 | 正确字数 | 已输入字数 | 正确率 | 速度(正确字/分) | 用时(秒) |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for i, item := range items {
		fmt.Fprintf(w, "| %d | %s | %s | %s | %d | %d | %.1f%% | %.1f | %d |\n", i+1, item.StudentID, item.StudentName, item.ArticleTitle, item.CorrectCount, item.TypedCount, item.Accuracy*100, item.Speed, item.Duration)
	}
}

func (a *app) exportCSV(w http.ResponseWriter, r *http.Request) {
	items, err := a.resultRowsForExport(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "导出成绩失败")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=results.csv")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"排名", "学号", "姓名", "文章", "正确字数", "已输入字数", "正确率", "速度(正确字/分)", "用时(秒)"})
	for i, item := range items {
		_ = cw.Write([]string{
			strconv.Itoa(i + 1), item.StudentID, item.StudentName, item.ArticleTitle,
			strconv.FormatInt(item.CorrectCount, 10), strconv.FormatInt(item.TypedCount, 10),
			fmt.Sprintf("%.1f%%", item.Accuracy*100), fmt.Sprintf("%.1f", item.Speed),
			strconv.FormatInt(item.Duration, 10),
		})
	}
	cw.Flush()
}
