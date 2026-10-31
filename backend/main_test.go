package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
	a := &app{db: db, hub: newHub(), jwtSecret: "test-secret", publicDir: t.TempDir(), classSource: "manual", classSystemETags: map[string]string{}, timers: map[int64]*time.Timer{}, aiModel: defaultAIModel, aiReasoningEffort: defaultAIReasoning}
	a.loadSiteConfig()
	a.loadAIConfig()
	return a
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
	var remoteRequests atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteRequests.Add(1)
		if r.URL.Query().Get("teacher_username") != "teacher" || r.Header.Get("X-Teacher-Username") != "teacher" {
			t.Errorf("teacher identity was not forwarded")
		}
		if r.Header.Get("Authorization") != "Bearer test-shared-token" {
			t.Errorf("shared token was not forwarded")
		}
		if r.Header.Get("If-None-Match") == `"roster-1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"roster-1"`)
		_, _ = w.Write([]byte(`{"classes":[{"id":"class-1","name":"一班","students":[{"id":"001","name":"张三"},{"studentId":"002","studentName":"李四"}]}]}`))
	}))
	defer remote.Close()
	a.classSource = "classorbit"
	a.classSystemURL = remote.URL
	a.classSystemPath = "/api/integration/classes"
	a.classSystemToken = "test-shared-token"
	a.classSystemTimeout = time.Second
	status, out = request(t, a.routes(), "POST", "/api/integration/class-system/test", "", token)
	if status != http.StatusOK || out["class_count"] != float64(1) || out["student_count"] != float64(2) {
		t.Fatalf("connection test: %d %v", status, out)
	}
	status, out = request(t, a.routes(), "POST", "/api/classes/sync", "", token)
	if status != 200 || out["student_count"] != float64(2) {
		t.Fatalf("sync: %d %v", status, out)
	}
	var classes, students, remoteClasses int
	var remoteID string
	_ = a.db.QueryRow("SELECT COUNT(*) FROM classes").Scan(&classes)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM class_students").Scan(&students)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM classes WHERE source='classorbit'").Scan(&remoteClasses)
	_ = a.db.QueryRow("SELECT remote_id FROM classes WHERE source='classorbit'").Scan(&remoteID)
	if classes != 1 || students != 2 || remoteClasses != 1 || remoteID != "class-1" {
		t.Fatalf("unexpected synced data: classes=%d students=%d remote=%d remote_id=%q", classes, students, remoteClasses, remoteID)
	}
	status, out = request(t, a.routes(), "POST", "/api/classes/sync", "", token)
	if status != http.StatusOK || out["unchanged"] != true {
		t.Fatalf("conditional sync: %d %v", status, out)
	}
	if remoteRequests.Load() != 3 {
		t.Fatalf("remote requests=%d, want 3", remoteRequests.Load())
	}
}

func TestClassOrbitErrorDetailsAreForwarded(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	if status, _ := request(t, h, "POST", "/api/setup/init", `{"teacherUsername":"teacher","teacherPassword":"secret1"}`, ""); status != http.StatusOK {
		t.Fatalf("setup status=%d", status)
	}
	_, login := request(t, h, "POST", "/api/admin/login", `{"Username":"teacher","Password":"secret1"}`, "")
	token, _ := login["token"].(string)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"教师账号不存在"}`))
	}))
	defer remote.Close()
	a.classSource = "classorbit"
	a.classSystemURL = remote.URL
	a.classSystemPath = "/api/integration/classes"
	a.classSystemToken = "test-token"
	a.classSystemTimeout = time.Second
	status, out := request(t, h, "POST", "/api/integration/class-system/test", "", token)
	message, _ := out["error"].(string)
	if status != http.StatusBadGateway || !strings.Contains(message, "教师账号不存在") {
		t.Fatalf("remote error: %d %v", status, out)
	}
}

func TestIntegrationConfigPersistsClassSource(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	if status, _ := request(t, h, "POST", "/api/setup/init", `{"teacherUsername":"admin","teacherPassword":"secret1"}`, ""); status != http.StatusOK {
		t.Fatalf("setup status=%d", status)
	}
	status, login := request(t, h, "POST", "/api/admin/login", `{"Username":"admin","Password":"secret1"}`, "")
	if status != http.StatusOK {
		t.Fatalf("login status=%d", status)
	}
	token, _ := login["token"].(string)
	status, out := request(t, h, "PUT", "/api/integration/class-system", `{"source":"classorbit","url":"http://host.docker.internal:8080/","path":"/api/integration/classes","token":"shared-secret","timeout":"8s"}`, token)
	if status != http.StatusOK || out["success"] != true {
		t.Fatalf("save integration config: %d %v", status, out)
	}
	if a.classSource != "classorbit" || a.classSystemURL != "http://host.docker.internal:8080" || a.classSystemToken != "shared-secret" {
		t.Fatalf("unexpected runtime config: source=%q url=%q token=%q", a.classSource, a.classSystemURL, a.classSystemToken)
	}
	var stored string
	if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key='class_source'").Scan(&stored); err != nil || stored != "classorbit" {
		t.Fatalf("source was not persisted: value=%q err=%v", stored, err)
	}
}

func TestClassOrbitConfigRequiresURLAndToken(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	if status, _ := request(t, h, "POST", "/api/setup/init", `{"teacherUsername":"admin","teacherPassword":"secret1"}`, ""); status != http.StatusOK {
		t.Fatalf("setup status=%d", status)
	}
	_, login := request(t, h, "POST", "/api/admin/login", `{"Username":"admin","Password":"secret1"}`, "")
	token, _ := login["token"].(string)
	status, _ := request(t, h, "PUT", "/api/integration/class-system", `{"source":"classorbit","url":"http://host.docker.internal:8080","token":""}`, token)
	if status != http.StatusBadRequest {
		t.Fatalf("empty ClassOrbit token status=%d", status)
	}
}

func TestLegacyIntegrationConfigEnablesClassOrbitSource(t *testing.T) {
	a := testApp(t)
	if _, err := a.db.Exec("INSERT INTO app_meta(key,value) VALUES('class_system_url','http://classorbit:8080'),('class_system_token','legacy-secret')"); err != nil {
		t.Fatal(err)
	}
	if err := a.loadIntegrationConfig(); err != nil {
		t.Fatal(err)
	}
	if a.classSource != "classorbit" {
		t.Fatalf("legacy config source=%q", a.classSource)
	}
}

func TestClassSyncPreservesManualClasses(t *testing.T) {
	a := testApp(t)
	if status, _ := request(t, a.routes(), "POST", "/api/setup/init", `{"teacherUsername":"teacher","teacherPassword":"secret1"}`, ""); status != 200 {
		t.Fatalf("setup status=%d", status)
	}
	if _, err := a.db.Exec("INSERT INTO classes(teacher_id,name,source) VALUES(1,'手工班','manual')"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/classes/sync", nil)
	if _, _, err := a.replaceSyncedClasses(r, 1, []remoteClass{{ID: "remote-1", Name: "接口班"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.replaceSyncedClasses(r, 1, nil); err != nil {
		t.Fatal(err)
	}
	var manualActive, remoteActive int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM classes WHERE source='manual' AND active=1").Scan(&manualActive)
	_ = a.db.QueryRow("SELECT COUNT(*) FROM classes WHERE source='classorbit' AND active=1").Scan(&remoteActive)
	if manualActive != 1 || remoteActive != 0 {
		t.Fatalf("unexpected source state: manual=%d remote=%d", manualActive, remoteActive)
	}
}

func TestSyncRenameKeepsClassAndResultAssociation(t *testing.T) {
	a := testApp(t)
	if status, _ := request(t, a.routes(), "POST", "/api/setup/init", `{"teacherUsername":"teacher","teacherPassword":"secret1"}`, ""); status != 200 {
		t.Fatalf("setup status=%d", status)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/classes/sync", nil)
	if _, _, err := a.replaceSyncedClasses(r, 1, []remoteClass{{ID: "stable-1", Name: "四 1 班", Students: []remoteStudent{{ID: "001", Name: "张三"}}}}); err != nil {
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
	if _, err := a.db.Exec("INSERT INTO results(competition_id,student_id,student_name,class_id,typed_count) VALUES(1,'001','张三',?,1)", classID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.replaceSyncedClasses(r, 1, []remoteClass{{ID: "stable-1", Name: "五 1 班", Students: []remoteStudent{{ID: "999", Name: "新同学"}}}}); err != nil {
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

func TestCompetitionCreationIsIdempotent(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	if status, _ := request(t, h, "POST", "/api/setup/init", `{"teacherUsername":"teacher","teacherPassword":"secret1"}`, ""); status != http.StatusOK {
		t.Fatalf("setup status=%d", status)
	}
	_, login := request(t, h, "POST", "/api/admin/login", `{"Username":"teacher","Password":"secret1"}`, "")
	token, _ := login["token"].(string)
	if status, _ := request(t, h, "POST", "/api/articles", `{"title":"金色花","content":"一二三四五六七八九十"}`, token); status != http.StatusOK {
		t.Fatalf("article status=%d", status)
	}
	body := `{"articleId":1,"timeLimit":300,"requestId":"request-123"}`
	status, first := request(t, h, "POST", "/api/competition/create", body, token)
	if status != http.StatusOK || first["created"] != true {
		t.Fatalf("first create: %d %v", status, first)
	}
	status, second := request(t, h, "POST", "/api/competition/create", body, token)
	if status != http.StatusOK || second["created"] != false || first["id"] != second["id"] || first["accessCode"] != second["accessCode"] {
		t.Fatalf("idempotent create: first=%v second=%v", first, second)
	}
	var count int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM competitions").Scan(&count)
	if count != 1 {
		t.Fatalf("competition count=%d, want 1", count)
	}
}

func TestResultMetricsAreDerivedFromCountsAndDuration(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	if status, _ := request(t, h, "POST", "/api/setup/init", `{"teacherUsername":"teacher","teacherPassword":"secret1"}`, ""); status != http.StatusOK {
		t.Fatalf("setup status=%d", status)
	}
	_, login := request(t, h, "POST", "/api/admin/login", `{"Username":"teacher","Password":"secret1"}`, "")
	token, _ := login["token"].(string)
	request(t, h, "POST", "/api/articles", `{"title":"测试","content":"一二三四五六七八九十"}`, token)
	_, competition := request(t, h, "POST", "/api/competition/create", `{"articleId":1,"timeLimit":300,"customCode":"METRICS","requestId":"metrics-123"}`, token)
	competitionID := int64(competition["id"].(float64))
	status, _ := request(t, h, "POST", "/api/competition/start", fmt.Sprintf(`{"id":%d}`, competitionID), token)
	if status != http.StatusOK {
		t.Fatalf("start status=%d", status)
	}

	// Deliberately include bogus legacy derived values. The server must ignore
	// them and calculate 6/8 = 75%% and 6*60/30 = 12 correct chars/minute.
	payload := fmt.Sprintf(`{"competitionId":%d,"studentId":"001","studentName":"张三","correctCount":6,"typedCount":8,"duration":30,"accuracy":0.01,"speed":9999,"accessCode":"METRICS"}`, competitionID)
	status, result := request(t, h, "POST", "/api/results", payload, "")
	if status != http.StatusOK || result["success"] != true {
		t.Fatalf("submit: %d %v", status, result)
	}
	items, err := queryResults(a.db, resultSelect+" WHERE r.competition_id=?"+resultOrder, competitionID)
	if err != nil || len(items) != 1 {
		t.Fatalf("query results: %v %#v", err, items)
	}
	if math.Abs(items[0].Accuracy-0.75) > 0.0001 || math.Abs(items[0].Speed-12) > 0.0001 || items[0].TypedCount != 8 {
		t.Fatalf("metrics: %#v", items[0])
	}
}

func TestLegacyResultsAreNormalizedWithoutDataLoss(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE teachers(id INTEGER PRIMARY KEY,username TEXT,password_hash TEXT,display_name TEXT,is_admin INTEGER,created_at TEXT);
		CREATE TABLE articles(id INTEGER PRIMARY KEY,teacher_id INTEGER,title TEXT,content TEXT,created_at TEXT);
		CREATE TABLE competitions(id INTEGER PRIMARY KEY,teacher_id INTEGER,article_id INTEGER,access_code TEXT UNIQUE,status TEXT DEFAULT 'waiting',time_limit INTEGER DEFAULT 300,started_at TEXT,stopped_at TEXT,created_at TEXT);
		CREATE TABLE classes(id INTEGER PRIMARY KEY,teacher_id INTEGER,name TEXT,created_at TEXT);
		CREATE TABLE class_students(id INTEGER PRIMARY KEY,class_id INTEGER,student_id TEXT,student_name TEXT,created_at TEXT);
		CREATE TABLE results(id INTEGER PRIMARY KEY,competition_id INTEGER,student_id TEXT,student_name TEXT,correct_count INTEGER,total_count INTEGER,accuracy REAL,speed REAL,duration INTEGER,created_at TEXT);
		INSERT INTO teachers VALUES(1,'teacher','hash','教师',1,'2026-01-01');
		INSERT INTO articles VALUES(1,1,'金色花','一二三四五六七八九十','2026-01-01');
		INSERT INTO competitions VALUES(1,1,1,'LEGACY','stopped',300,'2026-01-01','2026-01-01','2026-01-01');
		INSERT INTO results VALUES(1,1,'001','张三',6,8,0.01,9999,30,'2026-01-01');
	`)
	if err != nil {
		t.Fatal(err)
	}
	// Startup applies the current idempotent schema before explicit migrations.
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if err = migrateSchema(db); err != nil {
		t.Fatal(err)
	}

	typedExists, err := hasColumn(db, "results", "typed_count")
	if err != nil || !typedExists {
		t.Fatalf("typed_count migration: exists=%v err=%v", typedExists, err)
	}
	accuracyExists, _ := hasColumn(db, "results", "accuracy")
	speedExists, _ := hasColumn(db, "results", "speed")
	if accuracyExists || speedExists {
		t.Fatalf("derived columns still exist: accuracy=%v speed=%v", accuracyExists, speedExists)
	}
	var correct, typed, duration int64
	if err = db.QueryRow("SELECT correct_count,typed_count,duration FROM results WHERE id=1").Scan(&correct, &typed, &duration); err != nil {
		t.Fatal(err)
	}
	if correct != 6 || typed != 8 || duration != 30 {
		t.Fatalf("migrated result: correct=%d typed=%d duration=%d", correct, typed, duration)
	}
}

// startCompetition creates an article and a started competition, returning its ID.
func startCompetition(t *testing.T, a *app, h http.Handler, token, code string) int64 {
	t.Helper()
	if status, out := request(t, h, "POST", "/api/articles", `{"title":"范文","content":"一二三四五六七八九十"}`, token); status != http.StatusOK {
		t.Fatalf("article: %d %v", status, out)
	}
	status, created := request(t, h, "POST", "/api/competition/create",
		`{"articleId":1,"timeLimit":120,"customCode":"`+code+`"}`, token)
	if status != http.StatusOK {
		t.Fatalf("create competition: %d %v", status, created)
	}
	id := int64(created["id"].(float64))
	if status, out := request(t, h, "POST", "/api/competition/start", fmt.Sprintf(`{"id":%d}`, id), token); status != http.StatusOK {
		t.Fatalf("start competition: %d %v", status, out)
	}
	return id
}

func loginTeacher(t *testing.T, h http.Handler) string {
	t.Helper()
	if status, _ := request(t, h, "POST", "/api/setup/init", `{"teacherUsername":"teacher","teacherPassword":"secret1"}`, ""); status != http.StatusOK {
		t.Fatalf("setup status=%d", status)
	}
	_, login := request(t, h, "POST", "/api/admin/login", `{"Username":"teacher","Password":"secret1"}`, "")
	token, _ := login["token"].(string)
	if token == "" {
		t.Fatalf("login failed: %v", login)
	}
	return token
}

func TestTextOptionsRoundTrip(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	request(t, h, "POST", "/api/articles", `{"title":"选项","content":"一二三，四。五"}`, token)

	status, created := request(t, h, "POST", "/api/competition/create",
		`{"articleId":1,"timeLimit":120,"customCode":"OPTS","textOptions":{"pinyin":false,"stripSymbols":true,"unifySymbols":true,"stripBreaks":false}}`, token)
	if status != http.StatusOK || created["success"] != true {
		t.Fatalf("create: %d %v", status, created)
	}
	status, comp := request(t, h, "GET", "/api/competition/by-code/OPTS", "", "")
	if status != http.StatusOK {
		t.Fatalf("by code: %d %v", status, comp)
	}
	if comp["pinyin_enabled"] != false {
		t.Fatalf("legacy pinyin projection = %v", comp["pinyin_enabled"])
	}
	options, ok := comp["text_options"].(map[string]any)
	if !ok || options["pinyin"] != false || options["stripSymbols"] != true || options["unifySymbols"] != true || options["stripBreaks"] != false {
		t.Fatalf("text options: %#v", comp["text_options"])
	}

	// A page from the previous release only knows pinyinEnabled.
	status, legacy := request(t, h, "POST", "/api/competition/create", `{"articleId":1,"timeLimit":60,"customCode":"LEGACY","pinyinEnabled":false}`, token)
	if status != http.StatusOK || legacy["success"] != true {
		t.Fatalf("legacy create: %d %v", status, legacy)
	}
	status, comp = request(t, h, "GET", "/api/competition/by-code/LEGACY", "", "")
	if status != http.StatusOK {
		t.Fatalf("legacy by code: %d %v", status, comp)
	}
	options, ok = comp["text_options"].(map[string]any)
	if !ok || options["pinyin"] != false || options["stripSymbols"] != false {
		t.Fatalf("legacy options: %#v", comp["text_options"])
	}
}

func TestResultSubmissionRequiresAccessCode(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	id := startCompetition(t, a, h, token, "REQ1")
	body := func(code string) string {
		payload := fmt.Sprintf(`{"competitionId":%d,"studentId":"001","studentName":"张三","correctCount":5,"typedCount":6,"duration":30`, id)
		if code != "" {
			payload += `,"accessCode":"` + code + `"`
		}
		return payload + "}"
	}
	if status, out := request(t, h, "POST", "/api/results", body(""), ""); status != http.StatusForbidden {
		t.Fatalf("missing code: %d %v", status, out)
	}
	if status, out := request(t, h, "POST", "/api/results", body("WRONG"), ""); status != http.StatusForbidden {
		t.Fatalf("wrong code: %d %v", status, out)
	}
	if status, out := request(t, h, "POST", "/api/results", body("REQ1"), ""); status != http.StatusOK || out["success"] != true {
		t.Fatalf("valid code: %d %v", status, out)
	}
}

func TestResultSubmissionClosesAfterGrace(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	id := startCompetition(t, a, h, token, "GRACE")
	payload := fmt.Sprintf(`{"competitionId":%d,"studentId":"001","studentName":"张三","correctCount":5,"typedCount":6,"duration":30,"accessCode":"GRACE"}`, id)

	if _, err := a.db.Exec("UPDATE competitions SET status='stopped', stopped_at='2026-01-01 00:00:00' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if status, out := request(t, h, "POST", "/api/results", payload, ""); status != http.StatusConflict {
		t.Fatalf("late submission: %d %v", status, out)
	}

	// The grace window keeps the final keystrokes of a race accepted.
	if _, err := a.db.Exec("UPDATE competitions SET status='stopped', stopped_at=datetime('now') WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if status, out := request(t, h, "POST", "/api/results", payload, ""); status != http.StatusOK {
		t.Fatalf("grace submission: %d %v", status, out)
	}
}

func TestPublicArticleRequiresCompetitionCode(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	request(t, h, "POST", "/api/articles", `{"title":"范文","content":"一二三"}`, token)

	if status, _ := request(t, h, "GET", "/api/articles/1", "", ""); status != http.StatusNotFound {
		t.Fatalf("article without code: %d", status)
	}
	if status, _ := request(t, h, "POST", "/api/competition/create", `{"articleId":1,"timeLimit":120,"customCode":"READ"}`, token); status != http.StatusOK {
		t.Fatalf("create competition: %d", status)
	}
	status, article := request(t, h, "GET", "/api/articles/1?code=READ", "", "")
	if status != http.StatusOK || article["content"] != "一二三" {
		t.Fatalf("article with code: %d %v", status, article)
	}
	if status, _ := request(t, h, "GET", "/api/articles/1?code=NOPE", "", ""); status != http.StatusNotFound {
		t.Fatalf("article with wrong code: %d", status)
	}
}

func TestPublicRosterRequiresCompetitionCode(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	if _, err := a.db.Exec("INSERT INTO classes(teacher_id,name) VALUES(1,'一班')"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.Exec("INSERT INTO class_students(class_id,student_id,student_name) VALUES(1,'001','张三')"); err != nil {
		t.Fatal(err)
	}

	if status, _ := request(t, h, "GET", "/api/classes/by-teacher/1", "", ""); status != http.StatusNotFound {
		t.Fatalf("classes without code: %d", status)
	}
	if status, _ := request(t, h, "GET", "/api/classes/1/students/public", "", ""); status != http.StatusNotFound {
		t.Fatalf("students without code: %d", status)
	}

	request(t, h, "POST", "/api/articles", `{"title":"范文","content":"一二三"}`, token)
	if status, _ := request(t, h, "POST", "/api/competition/create", `{"articleId":1,"timeLimit":120,"customCode":"ROSTER"}`, token); status != http.StatusOK {
		t.Fatalf("create competition: %d", status)
	}
	if status, _ := request(t, h, "GET", "/api/classes/by-teacher/1?code=ROSTER", "", ""); status != http.StatusOK {
		t.Fatalf("classes with code: %d", status)
	}
	if status, _ := request(t, h, "GET", "/api/classes/1/students/public?code=ROSTER", "", ""); status != http.StatusOK {
		t.Fatalf("students with code: %d", status)
	}
}

func TestWebsocketOriginPolicy(t *testing.T) {
	a := &app{allowedOrigins: []string{"cdn.example.com"}}
	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"non-browser client", "", true},
		{"same host", "https://origin.local", true},
		{"other host", "https://evil.example", false},
		{"explicitly allowed cdn", "https://cdn.example.com", true},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Host = "origin.local"
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if got := a.originAllowed(r); got != tc.want {
			t.Fatalf("%s: originAllowed=%v", tc.name, got)
		}
	}
}

func TestWebsocketRejectsMissingCredentials(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	if status, out := request(t, h, "GET", "/ws", "", ""); status != http.StatusUnauthorized {
		t.Fatalf("missing credentials: %d %v", status, out)
	}
	r := httptest.NewRequest("GET", "/ws", nil)
	r.Host = "origin.local"
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross origin: %d", w.Code)
	}
}

func TestSiteConfigDefaultsAndUpdate(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	status, site := request(t, h, "GET", "/api/site", "", "")
	if status != http.StatusOK || site["title"] != "KeySprint" || site["icp"] != "" {
		t.Fatalf("default site: %d %v", status, site)
	}
	token := loginTeacher(t, h)
	status, out := request(t, h, "PUT", "/api/site", `{"title":"鱼跃打字","tagline":"课堂打字竞赛","icp":"苏ICP备12345678号-1"}`, token)
	if status != http.StatusOK || out["success"] != true {
		t.Fatalf("update site: %d %v", status, out)
	}
	status, site = request(t, h, "GET", "/api/site", "", "")
	if status != http.StatusOK || site["title"] != "鱼跃打字" || site["tagline"] != "课堂打字竞赛" || site["icp"] != "苏ICP备12345678号-1" {
		t.Fatalf("updated site: %d %v", status, site)
	}
	var stored string
	if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key='icp_number'").Scan(&stored); err != nil || stored != "苏ICP备12345678号-1" {
		t.Fatalf("stored icp: %q %v", stored, err)
	}
	if status, _ := request(t, h, "PUT", "/api/site", `{"title":"   "}`, token); status != http.StatusBadRequest {
		t.Fatalf("empty title: %d", status)
	}
	longTitle := strings.Repeat("长", maxSiteTitleRunes+1)
	if status, _ := request(t, h, "PUT", "/api/site", fmt.Sprintf(`{"title":%q}`, longTitle), token); status != http.StatusBadRequest {
		t.Fatalf("long title: %d", status)
	}
	// Startup reload keeps the saved appearance after a restart.
	a.loadSiteConfig()
	if a.currentSite().Title != "鱼跃打字" || a.currentSite().ICP != "苏ICP备12345678号-1" {
		t.Fatalf("reload: %#v", a.currentSite())
	}
}

func TestSiteUpdateRequiresAdmin(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	adminToken := loginTeacher(t, h)
	if status, out := request(t, h, "POST", "/api/teachers", `{"Username":"teacher2","Password":"secret1","DisplayName":"普通教师"}`, adminToken); status != http.StatusOK {
		t.Fatalf("create teacher: %d %v", status, out)
	}
	_, login := request(t, h, "POST", "/api/admin/login", `{"Username":"teacher2","Password":"secret1"}`, "")
	token, _ := login["token"].(string)
	if status, _ := request(t, h, "PUT", "/api/site", `{"title":"越权修改"}`, token); status != http.StatusForbidden {
		t.Fatalf("non-admin update: %d", status)
	}
	if status, _ := request(t, h, "GET", "/api/site", "", ""); status != http.StatusOK {
		t.Fatalf("public site read: %d", status)
	}
}
func TestAIConfigDefaultsSaveAndMask(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)

	status, cfg := request(t, h, "GET", "/api/ai/config", "", token)
	if status != http.StatusOK || cfg["model"] != defaultAIModel || cfg["reasoningEffort"] != defaultAIReasoning || cfg["hasKey"] != false {
		t.Fatalf("default ai config: %d %v", status, cfg)
	}
	status, _ = request(t, h, "PUT", "/api/ai/config", `{"baseUrl":"https://ai.example.com/v1/","apiKey":"sl_secret_abcd1234","model":"gpt-6-luna","reasoningEffort":"high"}`, token)
	if status != http.StatusOK {
		t.Fatalf("save ai config: %d", status)
	}
	status, cfg = request(t, h, "GET", "/api/ai/config", "", token)
	if status != http.StatusOK || cfg["baseUrl"] != "https://ai.example.com/v1" || cfg["keyHint"] != "****1234" || cfg["hasKey"] != true || cfg["reasoningEffort"] != "high" {
		t.Fatalf("masked ai config: %d %v", status, cfg)
	}
	a.loadAIConfig()
	if got := a.aiConfigSnapshot(); got.APIKey != "sl_secret_abcd1234" || got.Model != "gpt-6-luna" || got.BaseURL != "https://ai.example.com/v1" {
		t.Fatalf("persisted ai config: %#v", got)
	}
	// Empty apiKey keeps the stored key; only model and effort change.
	request(t, h, "PUT", "/api/ai/config", `{"baseUrl":"https://ai.example.com/v1","apiKey":"","model":"other-model","reasoningEffort":"low"}`, token)
	if got := a.aiConfigSnapshot(); got.APIKey != "sl_secret_abcd1234" || got.Model != "other-model" || got.ReasoningEffort != "low" {
		t.Fatalf("keep api key: %#v", got)
	}
	if status, _ := request(t, h, "PUT", "/api/ai/config", `{"baseUrl":"ftp://nope","model":"m","reasoningEffort":"low"}`, token); status != http.StatusBadRequest {
		t.Fatalf("bad base url: %d", status)
	}
	if status, _ := request(t, h, "PUT", "/api/ai/config", `{"baseUrl":"https://x.example","model":"m","reasoningEffort":"extreme"}`, token); status != http.StatusBadRequest {
		t.Fatalf("bad reasoning effort: %d", status)
	}
}

func TestAIConfigRequiresAdmin(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	adminToken := loginTeacher(t, h)
	if status, out := request(t, h, "POST", "/api/teachers", `{"Username":"teacher2","Password":"secret1","DisplayName":"普通教师"}`, adminToken); status != http.StatusOK {
		t.Fatalf("create teacher: %d %v", status, out)
	}
	_, login := request(t, h, "POST", "/api/admin/login", `{"Username":"teacher2","Password":"secret1"}`, "")
	token, _ := login["token"].(string)
	if status, _ := request(t, h, "GET", "/api/ai/config", "", token); status != http.StatusForbidden {
		t.Fatalf("non-admin ai config: %d", status)
	}
	if status, _ := request(t, h, "POST", "/api/ai/models", "", token); status != http.StatusForbidden {
		t.Fatalf("non-admin ai models: %d", status)
	}
}

func TestAIProbeModels(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"zeta-model"},{"id":"alpha-model"}]}`)
	}))
	defer upstream.Close()
	if status, _ := request(t, h, "PUT", "/api/ai/config", `{"baseUrl":"`+upstream.URL+`/v1","apiKey":"test-key","model":"alpha-model","reasoningEffort":"medium"}`, token); status != http.StatusOK {
		t.Fatalf("configure ai: %d", status)
	}
	status, out := request(t, h, "POST", "/api/ai/models", "", token)
	models, _ := out["models"].([]any)
	if status != http.StatusOK || len(models) != 2 || models[0] != "alpha-model" || models[1] != "zeta-model" {
		t.Fatalf("probe models: %d %v", status, out)
	}
}

func TestAIGenerateArticle(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	var sawReasoning bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		sawReasoning = strings.Contains(string(body), `"reasoning_effort":"medium"`)
		w.Header().Set("Content-Type", "application/json")
		payload := `{"title":"秋天的校园","content":"` + strings.Repeat("秋风", 150) + `"}`
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, strconv.Quote(payload))
	}))
	defer upstream.Close()
	if status, _ := request(t, h, "PUT", "/api/ai/config", `{"baseUrl":"`+upstream.URL+`/v1","apiKey":"test-key","model":"gpt-6-luna","reasoningEffort":"medium"}`, token); status != http.StatusOK {
		t.Fatalf("configure ai: %d", status)
	}
	status, out := request(t, h, "POST", "/api/ai/generate", `{"topic":"秋天的校园","style":"记叙文","minChars":150,"maxChars":300}`, token)
	if status != http.StatusOK || out["success"] != true || out["title"] != "秋天的校园" {
		t.Fatalf("generate: %d %v", status, out)
	}
	if !sawReasoning {
		t.Fatal("reasoning_effort was not sent upstream")
	}
	chars, _ := out["chars"].(float64)
	if chars < 150 || chars > 300 {
		t.Fatalf("generated chars: %v", out["chars"])
	}
	var title, source string
	if err := a.db.QueryRow("SELECT title, source FROM articles ORDER BY id DESC LIMIT 1").Scan(&title, &source); err != nil || title != "秋天的校园" || source != "ai" {
		t.Fatalf("generated article row: %q %q %v", title, source, err)
	}
}

func TestAIGenerateRetriesWithoutReasoning(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	attempts := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "reasoning_effort") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"unsupported parameter: reasoning_effort"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		payload := `{"title":"雨后的操场","content":"` + strings.Repeat("雨", 120) + `"}`
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, strconv.Quote(payload))
	}))
	defer upstream.Close()
	if status, _ := request(t, h, "PUT", "/api/ai/config", `{"baseUrl":"`+upstream.URL+`/v1","apiKey":"test-key","model":"gpt-6-luna","reasoningEffort":"high"}`, token); status != http.StatusOK {
		t.Fatalf("configure ai: %d", status)
	}
	status, out := request(t, h, "POST", "/api/ai/generate", `{"topic":"雨后的操场","minChars":120,"maxChars":240}`, token)
	if status != http.StatusOK || out["success"] != true {
		t.Fatalf("generate with fallback: %d %v", status, out)
	}
	if attempts != 2 {
		t.Fatalf("upstream attempts = %d, want 2", attempts)
	}
}

func TestAIGenerateRequiresConfiguration(t *testing.T) {
	a := testApp(t)
	h := a.routes()
	token := loginTeacher(t, h)
	if status, _ := request(t, h, "POST", "/api/ai/generate", `{"topic":"春天","minChars":200,"maxChars":400}`, token); status != http.StatusBadRequest {
		t.Fatalf("unconfigured generate: %d", status)
	}
	if status, _ := request(t, h, "POST", "/api/ai/generate", `{"topic":""}`, token); status != http.StatusBadRequest {
		t.Fatalf("missing topic: %d", status)
	}
}

func TestParseGeneratedArticle(t *testing.T) {
	in := aiGenerateInput{MinChars: 100, MaxChars: 300}
	cases := []struct {
		name    string
		raw     string
		title   string
		chars   int
		wantErr bool
	}{
		{name: "json", raw: `{"title":"雨后","content":"` + strings.Repeat("雨", 120) + `"}`, title: "雨后", chars: 120},
		{name: "fenced json", raw: "```json\n{\"title\":\"雨后\",\"content\":\"" + strings.Repeat("雨", 120) + "\"}\n```", title: "雨后", chars: 120},
		{name: "plain text", raw: "标题：雨后\n\n" + strings.Repeat("雨", 120), title: "雨后", chars: 120},
		{name: "too short", raw: `{"title":"短","content":"` + strings.Repeat("雨", 10) + `"}`, wantErr: true},
		{name: "empty", raw: " ", wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseGeneratedArticle(tc.raw, in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.Title != tc.title || got.Chars != tc.chars {
			t.Fatalf("%s: got title=%q chars=%d", tc.name, got.Title, got.Chars)
		}
	}
}
