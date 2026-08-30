package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The remote service only needs the teacher identity. Passwords and the local
// JWT are never forwarded to another system.
type remoteClass struct {
	ID        string          `json:"id"`
	ClassID   string          `json:"classId"`
	LegacyID  string          `json:"class_id"`
	Name      string          `json:"name"`
	ClassName string          `json:"className"`
	Students  []remoteStudent `json:"students"`
}

type remoteStudent struct {
	ID          string `json:"id"`
	StudentID   string `json:"studentId"`
	LegacyID    string `json:"student_id"`
	Name        string `json:"name"`
	StudentName string `json:"studentName"`
	LegacyName  string `json:"student_name"`
}

type remoteClassesResponse struct {
	Classes []remoteClass `json:"classes"`
}

type classAPIItem struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	CreatedAt    string `json:"created_at"`
	StudentCount int64  `json:"student_count"`
}

type studentAPIItem struct {
	StudentID   string `json:"student_id"`
	StudentName string `json:"student_name"`
}

func (a *app) classSystemStatus(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": a.classSystemURL != "",
		"path":       a.classSystemPath,
	})
}

func (a *app) syncClasses(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	configured := a.classSystemURL != ""
	a.configMu.RUnlock()
	if !configured {
		writeError(w, http.StatusServiceUnavailable, "尚未配置班级积分系统接口")
		return
	}
	u := currentUser(r)
	var username string
	if err := a.db.QueryRowContext(r.Context(), "SELECT username FROM teachers WHERE id=?", u.ID).Scan(&username); err != nil {
		writeError(w, http.StatusUnauthorized, "教师账号不存在")
		return
	}
	classes, err := a.fetchRemoteClasses(r, username)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	classCount, studentCount, err := a.replaceSyncedClasses(r, u.ID, classes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "同步班级名单失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "class_count": classCount, "student_count": studentCount})
}

func (a *app) fetchRemoteClasses(r *http.Request, username string) ([]remoteClass, error) {
	a.configMu.RLock()
	baseURL, path, sharedToken, timeout := a.classSystemURL, a.classSystemPath, a.classSystemToken, a.classSystemTimeout
	a.configMu.RUnlock()
	endpoint := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("班级积分系统地址无效")
	}
	q := u.Query()
	q.Set("teacher_username", username)
	u.RawQuery = q.Encode()

	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("无法创建班级积分系统请求")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Teacher-Username", username)
	if sharedToken != "" {
		req.Header.Set("Authorization", "Bearer "+sharedToken)
	}
	res, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("班级积分系统连接失败")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("班级积分系统返回错误 (%d)", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("读取班级积分系统响应失败")
	}
	var envelope remoteClassesResponse
	if len(strings.TrimSpace(string(body))) > 0 && strings.HasPrefix(strings.TrimSpace(string(body)), "[") {
		if err := json.Unmarshal(body, &envelope.Classes); err != nil {
			return nil, fmt.Errorf("班级积分系统响应格式错误")
		}
	} else if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("班级积分系统响应格式错误")
	}
	return envelope.Classes, nil
}

func (a *app) replaceSyncedClasses(r *http.Request, teacherID int64, classes []remoteClass) (int, int, error) {
	ctx := r.Context()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	// A sync can contain hundreds of students. Reusing statements keeps the
	// SQLite parser/VM setup out of the per-student loop while retaining the
	// single transaction that makes the replacement atomic.
	findRemoteClass, err := tx.PrepareContext(ctx, "SELECT id FROM classes WHERE teacher_id=? AND remote_id=?")
	if err != nil {
		return 0, 0, err
	}
	defer findRemoteClass.Close()
	findNamedClass, err := tx.PrepareContext(ctx, "SELECT id FROM classes WHERE teacher_id=? AND name=?")
	if err != nil {
		return 0, 0, err
	}
	defer findNamedClass.Close()
	insertClass, err := tx.PrepareContext(ctx, "INSERT INTO classes(teacher_id,name,remote_id,active) VALUES(?,?,?,1)")
	if err != nil {
		return 0, 0, err
	}
	defer insertClass.Close()
	updateClass, err := tx.PrepareContext(ctx, "UPDATE classes SET name=?,remote_id=?,active=1 WHERE id=? AND teacher_id=?")
	if err != nil {
		return 0, 0, err
	}
	defer updateClass.Close()
	deleteStudents, err := tx.PrepareContext(ctx, "DELETE FROM class_students WHERE class_id=?")
	if err != nil {
		return 0, 0, err
	}
	defer deleteStudents.Close()
	insertStudent, err := tx.PrepareContext(ctx, "INSERT INTO class_students(class_id,student_id,student_name) VALUES(?,?,?)")
	if err != nil {
		return 0, 0, err
	}
	defer insertStudent.Close()
	classCount, studentCount := 0, 0
	// A sync is authoritative: classes absent from the remote response become
	// inactive, so renamed/deleted classes cannot remain selectable locally.
	if _, err = tx.Exec("UPDATE classes SET active=0 WHERE teacher_id=?", teacherID); err != nil {
		return 0, 0, err
	}
	seenClasses := make(map[string]struct{}, len(classes))
	usedLocalClasses := make(map[int64]struct{}, len(classes))
	for _, c := range classes {
		name := strings.TrimSpace(firstNonEmpty(c.Name, c.ClassName))
		if name == "" {
			continue
		}
		remoteID := strings.TrimSpace(firstNonEmpty(c.ID, c.ClassID, c.LegacyID))
		key := firstNonEmpty(remoteID, "name:"+name)
		if _, ok := seenClasses[key]; ok {
			continue
		}
		seenClasses[key] = struct{}{}
		studentIDs := remoteStudentIDs(c.Students)
		var classID int64
		if remoteID != "" {
			_ = findRemoteClass.QueryRowContext(ctx, teacherID, remoteID).Scan(&classID)
		}
		if classID == 0 {
			_ = findNamedClass.QueryRowContext(ctx, teacherID, name).Scan(&classID)
		}
		if _, used := usedLocalClasses[classID]; used {
			classID = 0
		}
		if classID == 0 && len(studentIDs) > 0 {
			placeholders := strings.TrimRight(strings.Repeat("?,", len(studentIDs)), ",")
			args := []any{teacherID}
			for _, id := range studentIDs {
				args = append(args, id)
			}
			var overlap int64
			query := "SELECT c.id,COUNT(DISTINCT s.student_id) FROM classes c JOIN class_students s ON s.class_id=c.id WHERE c.teacher_id=? AND s.student_id IN (" + placeholders + ") GROUP BY c.id ORDER BY COUNT(DISTINCT s.student_id) DESC LIMIT 1"
			var candidate int64
			if tx.QueryRow(query, args...).Scan(&candidate, &overlap) == nil && overlap >= int64((len(studentIDs)+1)/2) {
				if _, used := usedLocalClasses[candidate]; !used {
					classID = candidate
				}
			}
		}
		if classID == 0 {
			result, insertErr := insertClass.ExecContext(ctx, teacherID, name, nullableString(remoteID))
			if insertErr != nil {
				return 0, 0, insertErr
			}
			classID, err = result.LastInsertId()
		} else {
			if _, err = updateClass.ExecContext(ctx, name, nullableString(remoteID), classID, teacherID); err != nil {
				return 0, 0, err
			}
		}
		usedLocalClasses[classID] = struct{}{}
		if _, err = deleteStudents.ExecContext(ctx, classID); err != nil {
			return 0, 0, err
		}
		seenStudents := map[string]struct{}{}
		for _, s := range c.Students {
			id := strings.TrimSpace(firstNonEmpty(s.ID, s.StudentID, s.LegacyID))
			name := strings.TrimSpace(firstNonEmpty(s.Name, s.StudentName, s.LegacyName))
			if id == "" {
				continue
			}
			if name == "" {
				name = id
			}
			if _, ok := seenStudents[id]; ok {
				continue
			}
			seenStudents[id] = struct{}{}
			if _, err = insertStudent.ExecContext(ctx, classID, id, name); err != nil {
				return 0, 0, err
			}
			studentCount++
		}
		classCount++
	}
	if _, err = tx.Exec("DELETE FROM class_students WHERE class_id IN (SELECT id FROM classes WHERE teacher_id=? AND active=0)", teacherID); err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return classCount, studentCount, nil
}

func remoteStudentIDs(students []remoteStudent) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(students))
	for _, student := range students {
		id := strings.TrimSpace(firstNonEmpty(student.ID, student.StudentID, student.LegacyID))
		if id != "" {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
