package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const resultSelect = `SELECT r.id,r.competition_id,r.student_id,r.student_name,r.class_id,COALESCE(cl.name,''),r.correct_count,r.total_count,r.accuracy,r.speed,r.duration,r.created_at,c.access_code,a.title FROM results r JOIN competitions c ON c.id=r.competition_id JOIN articles a ON a.id=c.article_id LEFT JOIN classes cl ON cl.id=r.class_id`

func nullableInt(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func scanResults(rows *sql.Rows) []map[string]any {
	out := []map[string]any{}
	for rows.Next() {
		var id, comp, correct, total, duration int64
		var classID sql.NullInt64
		var sid, name, className, created, code, title string
		var accuracy, speed float64
		_ = rows.Scan(&id, &comp, &sid, &name, &classID, &className, &correct, &total, &accuracy, &speed, &duration, &created, &code, &title)
		out = append(out, map[string]any{"id": id, "competition_id": comp, "student_id": sid, "student_name": name, "class_id": nullableInt(classID), "class_name": className, "correct_count": correct, "total_count": total, "accuracy": accuracy, "speed": speed, "duration": duration, "created_at": created, "access_code": code, "article_title": title})
	}
	return out
}

func (a *app) submitResult(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CompetitionID int64   `json:"competitionId"`
		ClassID       int64   `json:"classId"`
		StudentID     string  `json:"studentId"`
		StudentName   string  `json:"studentName"`
		Correct       int64   `json:"correctCount"`
		Total         int64   `json:"totalCount"`
		Accuracy      float64 `json:"accuracy"`
		Speed         float64 `json:"speed"`
		Duration      int64   `json:"duration"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.StudentID = strings.TrimSpace(in.StudentID)
	in.StudentName = strings.TrimSpace(in.StudentName)
	if in.CompetitionID < 1 || in.StudentID == "" {
		writeError(w, 400, "参数不完整")
		return
	}
	if in.Total <= 0 {
		writeJSON(w, 200, map[string]any{"success": true, "skipped": true})
		return
	}
	var limit int64
	var started sql.NullString
	var teacherID int64
	if a.db.QueryRow("SELECT teacher_id,time_limit,started_at FROM competitions WHERE id=?", in.CompetitionID).Scan(&teacherID, &limit, &started) != nil {
		writeError(w, 404, "竞赛不存在")
		return
	}
	if in.Correct < 0 {
		in.Correct = 0
	}
	if in.Correct > in.Total {
		in.Correct = in.Total
	}
	if in.Accuracy < 0 {
		in.Accuracy = 0
	}
	if in.Accuracy > 1 {
		in.Accuracy = 1
	}
	if in.Speed < 0 {
		in.Speed = 0
	}
	if in.Duration < 0 {
		in.Duration = 0
	}
	if in.Duration > limit {
		in.Duration = limit
	}
	if in.StudentName == "" {
		in.StudentName = in.StudentID
	}
	var classValue any
	if in.ClassID > 0 {
		var valid int
		_ = a.db.QueryRow("SELECT COUNT(*) FROM classes WHERE id=? AND teacher_id=? AND active=1", in.ClassID, teacherID).Scan(&valid)
		if valid == 0 {
			in.ClassID = 0
		}
	}
	if in.ClassID == 0 {
		var candidate int64
		var matches int
		_ = a.db.QueryRow(`SELECT MIN(c.id),COUNT(DISTINCT c.id) FROM classes c JOIN class_students s ON s.class_id=c.id WHERE c.teacher_id=? AND c.active=1 AND s.student_id=?`, teacherID, in.StudentID).Scan(&candidate, &matches)
		if matches == 1 {
			in.ClassID = candidate
		}
	}
	if in.ClassID > 0 {
		classValue = in.ClassID
	}
	_, err := a.db.Exec(`INSERT INTO results(competition_id,student_id,student_name,class_id,correct_count,total_count,accuracy,speed,duration) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(competition_id,student_id) DO UPDATE SET student_name=excluded.student_name,class_id=COALESCE(excluded.class_id,results.class_id),correct_count=excluded.correct_count,total_count=excluded.total_count,accuracy=excluded.accuracy,speed=excluded.speed,duration=excluded.duration,created_at=datetime('now') WHERE excluded.correct_count>results.correct_count OR (excluded.correct_count=results.correct_count AND excluded.speed>results.speed)`, in.CompetitionID, in.StudentID, in.StudentName, classValue, in.Correct, in.Total, in.Accuracy, in.Speed, in.Duration)
	if err != nil {
		writeError(w, 500, "保存成绩失败")
		return
	}
	a.hub.broadcast(in.CompetitionID, "result:update", map[string]any{"competitionId": in.CompetitionID, "studentId": in.StudentID, "studentName": in.StudentName})
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) getAllResults(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query(resultSelect+" WHERE c.teacher_id=? ORDER BY r.speed DESC,r.accuracy DESC,r.correct_count DESC", u.ID)
	if err != nil {
		writeError(w, 500, "读取成绩失败")
		return
	}
	defer rows.Close()
	writeJSON(w, 200, scanResults(rows))
}
func (a *app) getResults(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query(resultSelect+" WHERE c.teacher_id=? AND r.competition_id=? ORDER BY r.speed DESC,r.accuracy DESC", u.ID, pathInt(r, "competitionId"))
	if err != nil {
		writeError(w, 500, "读取成绩失败")
		return
	}
	defer rows.Close()
	writeJSON(w, 200, scanResults(rows))
}
func (a *app) clearResults(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	_, _ = a.db.Exec("DELETE FROM results WHERE competition_id IN(SELECT id FROM competitions WHERE teacher_id=?)", u.ID)
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *app) deleteResult(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	res, _ := a.db.Exec("DELETE FROM results WHERE id=? AND competition_id IN(SELECT id FROM competitions WHERE teacher_id=?)", pathInt(r, "id"), u.ID)
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "成绩不存在")
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) resultRowsForExport(r *http.Request) []map[string]any {
	u := currentUser(r)
	rows, err := a.db.Query(resultSelect+" WHERE c.teacher_id=? ORDER BY r.speed DESC,r.accuracy DESC", u.ID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanResults(rows)
}
func (a *app) exportMarkdown(w http.ResponseWriter, r *http.Request) {
	items := a.resultRowsForExport(r)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=results.md")
	fmt.Fprintln(w, "# 打字竞赛成绩记录\n\n| 排名 | 学号 | 姓名 | 文章 | 正确字数 | 总字数 | 正确率 | 速度(字/分) | 用时(秒) |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for i, x := range items {
		fmt.Fprintf(w, "| %d | %v | %v | %v | %v | %v | %.1f%% | %.1f | %v |\n", i+1, x["student_id"], x["student_name"], x["article_title"], x["correct_count"], x["total_count"], x["accuracy"].(float64)*100, x["speed"].(float64), x["duration"])
	}
}
func (a *app) exportCSV(w http.ResponseWriter, r *http.Request) {
	items := a.resultRowsForExport(r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=results.csv")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"排名", "学号", "姓名", "文章", "正确字数", "总字数", "正确率", "速度(字/分)", "用时(秒)"})
	for i, x := range items {
		_ = cw.Write([]string{strconv.Itoa(i + 1), fmt.Sprint(x["student_id"]), fmt.Sprint(x["student_name"]), fmt.Sprint(x["article_title"]), fmt.Sprint(x["correct_count"]), fmt.Sprint(x["total_count"]), fmt.Sprintf("%.1f%%", x["accuracy"].(float64)*100), fmt.Sprintf("%.1f", x["speed"].(float64)), fmt.Sprint(x["duration"])})
	}
	cw.Flush()
}
