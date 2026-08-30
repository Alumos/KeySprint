package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testApp(t *testing.T) *app {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if err = migrateSchema(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &app{db: db, hub: newHub(), jwtSecret: "test-secret", publicDir: t.TempDir(), timers: map[int64]*time.Timer{}}
}

func request(t *testing.T, h http.Handler, method, path, body, token string) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("x-admin-token", token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestHealth(t *testing.T) {
	status, out := request(t, testApp(t).routes(), "GET", "/api/health", "", "")
	if status != http.StatusOK || out["status"] != "ok" || out["version"] != version || out["commit"] != commit {
		t.Fatalf("health: %d %v", status, out)
	}
}

func TestSetupLoginAndArticle(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	status, out := request(t, h, "POST", "/api/setup/init", `{"teacherUsername":"admin","teacherPassword":"secret1","teacherDisplayName":"管理员"}`, "")
	if status != 200 || out["success"] != true {
		t.Fatalf("setup: %d %v", status, out)
	}
	status, out = request(t, h, "POST", "/api/admin/login", `{"Username":"admin","Password":"secret1"}`, "")
	if status != 200 {
		t.Fatalf("login: %d %v", status, out)
	}
	token, _ := out["token"].(string)
	status, out = request(t, h, "POST", "/api/articles", `{"Title":"测试文章","Content":"你好，世界。"}`, token)
	if status != 200 || out["success"] != true {
		t.Fatalf("article: %d %v", status, out)
	}
	status, selected := request(t, h, "GET", "/api/articles/selected", "", token)
	if status != 200 {
		t.Fatalf("selected articles route: %d %v", status, selected)
	}
	status, out = request(t, h, "POST", "/api/competition/create", `{"articleId":1,"timeLimit":60,"customCode":"NOPINYIN","pinyinEnabled":false}`, token)
	if status != 200 || out["success"] != true {
		t.Fatalf("create competition: %d %v", status, out)
	}
	status, out = request(t, h, "GET", "/api/competition/by-code/NOPINYIN", "", "")
	if status != 200 || out["pinyin_enabled"] != false {
		t.Fatalf("pinyin setting: %d %v", status, out)
	}
}

func TestSyncClassesFromClassSystem(t *testing.T) {
	a := testApp(t)
	status, out := request(t, a.routes(), "POST", "/api/setup/init", `{"teacherUsername":"teacher","teacherPassword":"secret1"}`, "")
	if status != 200 || out["success"] != true {
		t.Fatalf("setup: %d %v", status, out)
	}
	status, out = request(t, a.routes(), "POST", "/api/admin/login", `{"Username":"teacher","Password":"secret1"}`, "")
	if status != 200 {
		t.Fatalf("login: %d", status)
	}
	token, _ := out["token"].(string)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("teacher_username") != "teacher" || r.Header.Get("X-Teacher-Username") != "teacher" {
			t.Errorf("teacher identity was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"classes":[{"name":"一班","students":[{"id":"001","name":"张三"},{"studentId":"002","studentName":"李四"}]}]}`))
	}))
	defer remote.Close()
	a.classSystemURL = remote.URL
	a.classSystemPath = "/api/integration/classes"
	a.classSystemTimeout = time.Second
	status, out = request(t, a.routes(), "POST", "/api/classes/sync", "", token)
	if status != 200 || out["student_count"] != float64(2) {
		t.Fatalf("sync: %d %v", status, out)
	}
	var classes, students int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM classes").Scan(&classes)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM class_students").Scan(&students)
	if classes != 1 || students != 2 {
		t.Fatalf("unexpected synced data: classes=%d students=%d", classes, students)
	}
}

func TestSyncRenameKeepsClassAndResultAssociation(t *testing.T) {
	a := testApp(t)
	if status, _ := request(t, a.routes(), "POST", "/api/setup/init", `{"teacherUsername":"teacher","teacherPassword":"secret1"}`, ""); status != 200 {
		t.Fatalf("setup status=%d", status)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/classes/sync", nil)
	if _, _, err := a.replaceSyncedClasses(r, 1, []remoteClass{{Name: "四 1 班", Students: []remoteStudent{{ID: "001", Name: "张三"}}}}); err != nil {
		t.Fatal(err)
	}
	var classID int64
	_ = a.db.QueryRow("SELECT id FROM classes WHERE teacher_id=1").Scan(&classID)
	if _, err := a.db.Exec("INSERT INTO articles(teacher_id,title,content) VALUES(1,'文章','内容')"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("INSERT INTO competitions(teacher_id,article_id,access_code) VALUES(1,1,'RENAME')"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("INSERT INTO results(competition_id,student_id,student_name,class_id,total_count) VALUES(1,'001','张三',?,1)", classID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.replaceSyncedClasses(r, 1, []remoteClass{{Name: "五 1 班", Students: []remoteStudent{{ID: "001", Name: "张三"}}}}); err != nil {
		t.Fatal(err)
	}
	var renamed string
	var resultClassID int64
	_ = a.db.QueryRow("SELECT name FROM classes WHERE id=?", classID).Scan(&renamed)
	_ = a.db.QueryRow("SELECT class_id FROM results WHERE id=1").Scan(&resultClassID)
	if renamed != "五 1 班" || resultClassID != classID {
		t.Fatalf("rename association lost: name=%q class_id=%d original=%d", renamed, resultClassID, classID)
	}
}
