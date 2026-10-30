package main

import (
	"bytes"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

func (a *app) getClasses(w http.ResponseWriter, r *http.Request) {
	a.classesForTeacher(w, currentUser(r).ID)
}
func (a *app) publicClasses(w http.ResponseWriter, r *http.Request) {
	teacher := pathInt(r, "teacherId")
	if !a.competitionCodeForTeacher(cleanCode(r.URL.Query().Get("code")), teacher) {
		writeError(w, 404, "班级不存在")
		return
	}
	a.classesForTeacher(w, teacher)
}

// competitionCodeForTeacher gates the public roster endpoints on a live
// competition code so class names cannot be enumerated by teacher ID.
func (a *app) competitionCodeForTeacher(code string, teacherID int64) bool {
	if code == "" || teacherID < 1 {
		return false
	}
	var count int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM competitions WHERE access_code=? AND teacher_id=? AND status IN ('waiting','running')`, code, teacherID).Scan(&count)
	return count > 0
}

func (a *app) publicClassAllowed(code string, classID int64) bool {
	if code == "" || classID < 1 {
		return false
	}
	var count int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM competitions c JOIN classes cl ON cl.teacher_id=c.teacher_id
		WHERE c.access_code=? AND c.status IN ('waiting','running') AND cl.id=? AND cl.active=1`, code, classID).Scan(&count)
	return count > 0
}
func (a *app) classesForTeacher(w http.ResponseWriter, teacher int64) {
	rows, err := a.db.Query(`SELECT c.id,c.name,c.source,c.created_at,COUNT(s.id) FROM classes c LEFT JOIN class_students s ON s.class_id=c.id WHERE c.teacher_id=? AND c.active=1 GROUP BY c.id ORDER BY c.created_at DESC`, teacher)
	if err != nil {
		writeError(w, 500, "读取班级失败")
		return
	}
	defer rows.Close()
	out := make([]classAPIItem, 0)
	for rows.Next() {
		var id, count int64
		var name, source, created string
		_ = rows.Scan(&id, &name, &source, &created, &count)
		out = append(out, classAPIItem{ID: id, Name: name, Source: source, CreatedAt: created, StudentCount: count})
	}
	writeJSON(w, 200, out)
}

func (a *app) ownClass(r *http.Request, id int64) bool {
	var n int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM classes WHERE id=? AND teacher_id=? AND active=1", id, currentUser(r).ID).Scan(&n)
	return n > 0
}
func (a *app) getClassStudents(w http.ResponseWriter, r *http.Request) {
	id := pathInt(r, "id")
	if !a.ownClass(r, id) {
		writeError(w, 404, "班级不存在")
		return
	}
	a.studentsForClass(w, id, false)
}
func (a *app) publicStudents(w http.ResponseWriter, r *http.Request) {
	id := pathInt(r, "id")
	if !a.publicClassAllowed(cleanCode(r.URL.Query().Get("code")), id) {
		writeError(w, 404, "班级不存在")
		return
	}
	a.studentsForClass(w, id, true)
}
func (a *app) studentsForClass(w http.ResponseWriter, id int64, public bool) {
	rows, err := a.db.Query("SELECT s.student_id,s.student_name FROM class_students s JOIN classes c ON c.id=s.class_id WHERE s.class_id=? AND c.active=1 ORDER BY s.student_id", id)
	if err != nil {
		writeError(w, 500, "读取学生失败")
		return
	}
	defer rows.Close()
	if public {
		out := make([]map[string]string, 0)
		for rows.Next() {
			var sid, name string
			_ = rows.Scan(&sid, &name)
			out = append(out, map[string]string{"studentId": sid, "studentName": name})
		}
		writeJSON(w, 200, out)
		return
	}
	out := make([]studentAPIItem, 0)
	for rows.Next() {
		var sid, name string
		_ = rows.Scan(&sid, &name)
		out = append(out, studentAPIItem{StudentID: sid, StudentName: name})
	}
	writeJSON(w, 200, out)
}

func (a *app) importClass(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, 400, "文件不能超过10MB")
		return
	}
	name := strings.TrimSpace(r.FormValue("className"))
	if name == "" {
		writeError(w, 400, "请输入班级名称")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "请上传 Excel 文件")
		return
	}
	defer file.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(file)
	rows, err := readExcelRows(buf.Bytes(), header.Filename)
	if err != nil || len(rows) < 2 {
		writeError(w, 400, "无法读取 Excel，或文件中没有学生数据")
		return
	}
	idCol, nameCol := headerColumns(rows[0])
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "导入失败")
		return
	}
	insertStudent, err := tx.PrepareContext(r.Context(), `INSERT INTO class_students(class_id,student_id,student_name) VALUES(?,?,?) ON CONFLICT(class_id,student_id) DO UPDATE SET student_name=excluded.student_name`)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, 500, "导入失败")
		return
	}
	defer insertStudent.Close()
	res, err := tx.Exec("INSERT INTO classes(teacher_id,name) VALUES(?,?)", u.ID, name)
	if err != nil {
		tx.Rollback()
		writeError(w, 500, "导入失败")
		return
	}
	classID, _ := res.LastInsertId()
	count := 0
	seenStudents := make(map[string]struct{}, len(rows)-1)
	for _, row := range rows[1:] {
		if idCol >= len(row) {
			continue
		}
		sid := strings.TrimSpace(row[idCol])
		if sid == "" {
			continue
		}
		if _, exists := seenStudents[sid]; exists {
			continue
		}
		seenStudents[sid] = struct{}{}
		sname := sid
		if nameCol < len(row) && strings.TrimSpace(row[nameCol]) != "" {
			sname = strings.TrimSpace(row[nameCol])
		}
		if _, err = insertStudent.ExecContext(r.Context(), classID, sid, sname); err != nil {
			tx.Rollback()
			writeError(w, 500, "导入失败")
			return
		}
		count++
	}
	if count == 0 {
		tx.Rollback()
		writeError(w, 400, "未解析到有效学生数据")
		return
	}
	if tx.Commit() != nil {
		writeError(w, 500, "导入失败")
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "classId": classID, "count": count})
}

func readExcelRows(data []byte, name string) ([][]string, error) {
	if strings.EqualFold(filepath.Ext(name), ".xls") {
		book, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
		if err != nil {
			return nil, err
		}
		sheet := book.GetSheet(0)
		if sheet == nil {
			return nil, sql.ErrNoRows
		}
		rows := make([][]string, 0, int(sheet.MaxRow)+1)
		for i := 0; i <= int(sheet.MaxRow); i++ {
			row := safeXLSRow(sheet, i)
			if row == nil {
				rows = append(rows, nil)
				continue
			}
			cols := make([]string, row.LastCol())
			for j := range cols {
				cols[j] = row.Col(j)
			}
			rows = append(rows, cols)
		}
		return rows, nil
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) == 0 {
		return nil, sql.ErrNoRows
	}
	return book.GetRows(sheets[0])
}

func safeXLSRow(sheet *xls.WorkSheet, index int) (row *xls.Row) {
	defer func() { _ = recover() }()
	return sheet.Row(index)
}

func headerColumns(row []string) (int, int) {
	idCol, nameCol := -1, -1
	for i, v := range row {
		s := strings.TrimSpace(v)
		if idCol < 0 && (s == "学号" || s == "学籍号" || strings.EqualFold(s, "id") || strings.Contains(s, "编号")) {
			idCol = i
		}
		if nameCol < 0 && (s == "姓名" || s == "学生姓名" || strings.Contains(s, "名字")) {
			nameCol = i
		}
	}
	if idCol < 0 {
		idCol = 0
	}
	if nameCol < 0 {
		if len(row) > 1 {
			nameCol = 1
		} else {
			nameCol = 0
		}
	}
	return idCol, nameCol
}

func (a *app) deleteClass(w http.ResponseWriter, r *http.Request) {
	id := pathInt(r, "id")
	if !a.ownClass(r, id) {
		writeError(w, 404, "班级不存在")
		return
	}
	if _, err := a.db.Exec("DELETE FROM classes WHERE id=?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除班级失败")
		return
	}
	a.clearClassSystemETags()
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *app) studentAnalytics(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	items, err := queryResults(a.db, resultSelect+" WHERE c.teacher_id=? AND r.student_id=? ORDER BY c.created_at", u.ID, r.PathValue("studentId"))
	if err != nil {
		writeError(w, 500, "读取分析失败")
		return
	}
	writeJSON(w, 200, items)
}
func (a *app) classStudents(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query(`SELECT DISTINCT s.student_id,s.student_name FROM class_students s JOIN classes c ON c.id=s.class_id WHERE c.teacher_id=? AND c.active=1 AND c.name=? ORDER BY s.student_id`, u.ID, r.PathValue("className"))
	if err != nil {
		writeError(w, 500, "读取失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var sid, name string
		_ = rows.Scan(&sid, &name)
		out = append(out, map[string]any{"student_id": sid, "student_name": name})
	}
	writeJSON(w, 200, out)
}
func (a *app) classAnalytics(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.Query(`SELECT c.id,c.access_code,c.created_at,a.title,
		COUNT(DISTINCT r.student_id),
		ROUND(AVG(CASE WHEN r.duration>0 THEN r.correct_count*60.0/r.duration ELSE 0 END),1),
		ROUND(AVG(CASE WHEN r.typed_count>0 THEN r.correct_count*100.0/r.typed_count ELSE 0 END),1),
		ROUND(AVG(r.correct_count),1),ROUND(AVG(r.duration),1)
		FROM results r
		JOIN competitions c ON c.id=r.competition_id
		JOIN articles a ON a.id=c.article_id
		JOIN classes cl ON cl.id=r.class_id
		WHERE c.teacher_id=? AND cl.teacher_id=? AND cl.active=1 AND cl.name=?
		GROUP BY c.id ORDER BY c.created_at`, u.ID, u.ID, r.PathValue("className"))
	if err != nil {
		writeError(w, 500, "读取分析失败")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, count int64
		var code, created, title string
		var speed, accuracy, correct, duration sql.NullFloat64
		_ = rows.Scan(&id, &code, &created, &title, &count, &speed, &accuracy, &correct, &duration)
		out = append(out, map[string]any{"competition_id": id, "access_code": code, "comp_created_at": created, "article_title": title, "student_count": count, "avg_speed": speed.Float64, "avg_accuracy": accuracy.Float64, "avg_correct": correct.Float64, "avg_duration": duration.Float64})
	}
	writeJSON(w, 200, out)
}
