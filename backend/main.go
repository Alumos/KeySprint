package main

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

var version = "dev"
var commit = "unknown"

type app struct {
	db                 *sql.DB
	hub                *hub
	jwtSecret          string
	publicDir          string
	classSystemURL     string
	classSystemPath    string
	classSystemToken   string
	classSystemTimeout time.Duration
	configMu           sync.RWMutex
	timerMu            sync.Mutex
	timers             map[int64]*time.Timer
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Printf("KeySprint %s (%s)\n", version, commit)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		res, err := (&http.Client{Timeout: 3 * time.Second}).Get(getenv("HEALTHCHECK_URL", "http://127.0.0.1:8080/api/health"))
		if err != nil {
			os.Exit(1)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}

	dbPath := getenv("DB_PATH", "./data/keysprint.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-20000)&_pragma=temp_store(MEMORY)")
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(schema); err != nil {
		log.Fatal(err)
	}
	if err = migrateSchema(db); err != nil {
		log.Fatal(err)
	}
	timeout, err := time.ParseDuration(getenv("CLASS_SYSTEM_TIMEOUT", "8s"))
	if err != nil || timeout <= 0 {
		timeout = 8 * time.Second
	}
	a := &app{
		db:                 db,
		hub:                newHub(),
		jwtSecret:          getenv("JWT_SECRET", "change-me-in-production"),
		publicDir:          getenv("PUBLIC_DIR", "../frontend/dist"),
		classSystemURL:     strings.TrimRight(os.Getenv("CLASS_SYSTEM_URL"), "/"),
		classSystemPath:    getenv("CLASS_SYSTEM_PATH", "/api/integration/classes"),
		classSystemToken:   os.Getenv("CLASS_SYSTEM_TOKEN"),
		classSystemTimeout: timeout,
		timers:             map[int64]*time.Timer{},
	}
	_ = a.loadIntegrationConfig()
	a.restoreTimers()
	srv := &http.Server{Addr: getenv("ADDR", ":8080"), Handler: a.routes(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 75 * time.Second}
	log.Printf("KeySprint %s (%s) listening on %s", version, commit, srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func getenv(k, v string) string {
	if s := os.Getenv(k); s != "" {
		return s
	}
	return v
}

func migrateSchema(db *sql.DB) error {
	// Earlier database schemas may predate per-competition pinyin settings.
	// Ignore the duplicate-column error for databases created from the current schema.
	_, _ = db.Exec(`ALTER TABLE competitions ADD COLUMN pinyin_enabled INTEGER NOT NULL DEFAULT 1`)
	_, _ = db.Exec(`ALTER TABLE classes ADD COLUMN remote_id TEXT`)
	_, _ = db.Exec(`ALTER TABLE classes ADD COLUMN active INTEGER NOT NULL DEFAULT 1`)
	_, _ = db.Exec(`ALTER TABLE results ADD COLUMN class_id INTEGER REFERENCES classes(id) ON DELETE SET NULL`)
	// Backfill legacy results only when student ID + name identifies one class.
	// Ambiguous historical records stay unassigned instead of being attributed
	// to the wrong class.
	_, _ = db.Exec(`UPDATE results SET class_id=(
		SELECT MIN(s.class_id) FROM class_students s
		JOIN classes cl ON cl.id=s.class_id AND cl.active=1
		JOIN competitions cp ON cp.id=results.competition_id
		WHERE cl.teacher_id=cp.teacher_id AND s.student_id=results.student_id AND s.student_name=results.student_name
	) WHERE class_id IS NULL AND (
		SELECT COUNT(DISTINCT s.class_id) FROM class_students s
		JOIN classes cl ON cl.id=s.class_id AND cl.active=1
		JOIN competitions cp ON cp.id=results.competition_id
		WHERE cl.teacher_id=cp.teacher_id AND s.student_id=results.student_id AND s.student_name=results.student_name
	)=1`)
	// Earlier schemas allowed duplicate attempts. Keep each student's
	// best attempt before adding the conflict target used by the new upsert.
	if _, err := db.Exec(`DELETE FROM results WHERE id IN (
		SELECT id FROM (SELECT id, ROW_NUMBER() OVER (
			PARTITION BY competition_id, student_id
			ORDER BY correct_count DESC, speed DESC, id DESC
		) AS rn FROM results) WHERE rn > 1
	)`); err != nil {
		return err
	}
	// Recreate this named index in case an earlier build created a non-unique
	// lookup index with the same name.
	_, _ = db.Exec(`DROP INDEX IF EXISTS idx_results_student_competition`)
	_, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_results_student_competition ON results(competition_id,student_id)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_results_class ON results(class_id)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_results_class_competition ON results(class_id,competition_id)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_classes_teacher_active_name ON classes(teacher_id,active,name)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_classes_teacher_remote ON classes(teacher_id,remote_id) WHERE remote_id IS NOT NULL`)
	return err
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := a.db.PingContext(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "数据库不可用")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": version, "commit": commit})
	})
	mux.HandleFunc("GET /api/setup/status", a.setupStatus)
	mux.HandleFunc("POST /api/setup/init", a.setupInit)
	mux.HandleFunc("POST /api/admin/login", a.login)
	// This literal route must be registered at the top level. Otherwise the
	// public /api/articles/{id} route treats "selected" as an article ID.
	mux.Handle("GET /api/articles/selected", a.requireAuth(http.HandlerFunc(a.getSelectedArticles)))
	mux.HandleFunc("GET /api/articles/{id}", a.getPublicArticle)
	mux.HandleFunc("GET /api/competition/by-code/{code}", a.competitionByCode)
	mux.HandleFunc("GET /api/classes/by-teacher/{teacherId}", a.publicClasses)
	mux.HandleFunc("GET /api/classes/{id}/students/public", a.publicStudents)
	mux.HandleFunc("POST /api/results", a.submitResult)
	mux.HandleFunc("GET /ws", a.hub.serve)

	secured := http.NewServeMux()
	secured.HandleFunc("GET /api/session", a.session)
	secured.HandleFunc("GET /api/teachers", a.getTeachers)
	secured.HandleFunc("POST /api/teachers", a.createTeacher)
	secured.HandleFunc("DELETE /api/teachers/{id}", a.deleteTeacher)
	secured.HandleFunc("PUT /api/teachers/{id}/reset-password", a.resetPassword)
	secured.HandleFunc("PUT /api/teacher/password", a.changePassword)
	secured.HandleFunc("GET /api/articles", a.getArticles)
	secured.HandleFunc("POST /api/articles", a.createArticle)
	secured.HandleFunc("PUT /api/articles/{id}", a.updateArticle)
	secured.HandleFunc("DELETE /api/articles/{id}", a.deleteArticle)
	secured.HandleFunc("GET /api/library", a.getLibrary)
	secured.HandleFunc("POST /api/library/pick", a.pickArticle)
	secured.HandleFunc("GET /api/competitions", a.getCompetitions)
	secured.HandleFunc("GET /api/competition/active", a.getActiveCompetitions)
	secured.HandleFunc("POST /api/competition/create", a.createCompetition)
	secured.HandleFunc("POST /api/competition/start", a.startCompetition)
	secured.HandleFunc("POST /api/competition/stop", a.stopCompetition)
	secured.HandleFunc("DELETE /api/competitions/{id}", a.deleteCompetition)
	secured.HandleFunc("GET /api/competitions/{id}/participants", a.getParticipants)
	secured.HandleFunc("GET /api/results", a.getAllResults)
	secured.HandleFunc("GET /api/results/{competitionId}", a.getResults)
	secured.HandleFunc("DELETE /api/results", a.clearResults)
	secured.HandleFunc("DELETE /api/results/{id}", a.deleteResult)
	secured.HandleFunc("GET /api/results/export/markdown", a.exportMarkdown)
	secured.HandleFunc("GET /api/results/export/csv", a.exportCSV)
	secured.HandleFunc("GET /api/classes", a.getClasses)
	secured.HandleFunc("POST /api/classes/import", a.importClass)
	secured.HandleFunc("GET /api/classes/integration/status", a.classSystemStatus)
	secured.HandleFunc("POST /api/classes/sync", a.syncClasses)
	secured.HandleFunc("GET /api/integration/class-system", a.getIntegrationConfig)
	secured.HandleFunc("PUT /api/integration/class-system", a.updateIntegrationConfig)
	secured.HandleFunc("GET /api/classes/{id}/students", a.getClassStudents)
	secured.HandleFunc("DELETE /api/classes/{id}", a.deleteClass)
	secured.HandleFunc("GET /api/analytics/student/{studentId}", a.studentAnalytics)
	secured.HandleFunc("GET /api/analytics/class/{className}", a.classAnalytics)
	secured.HandleFunc("GET /api/analytics/class/{className}/students", a.classStudents)
	mux.Handle("/api/", a.requireAuth(secured))

	static := http.FileServer(http.Dir(a.publicDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, 404, "接口不存在")
			return
		}
		path := filepath.Join(a.publicDir, filepath.Clean(r.URL.Path))
		if r.URL.Path != "/" {
			if st, err := os.Stat(path); err == nil && !st.IsDir() {
				static.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(a.publicDir, "index.html"))
	})
	return noCacheAPI(mux)
}

func noCacheAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) restoreTimers() {
	rows, err := a.db.Query("SELECT id,time_limit,started_at FROM competitions WHERE status='running'")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, limit int64
		var started string
		if rows.Scan(&id, &limit, &started) == nil {
			t, _ := time.Parse("2006-01-02 15:04:05", started)
			remain := time.Duration(limit+10)*time.Second - time.Since(t.UTC())
			if remain <= 0 {
				_, _ = a.db.Exec("UPDATE competitions SET status='stopped',stopped_at=datetime('now') WHERE id=?", id)
			} else {
				a.scheduleStop(id, remain)
			}
		}
	}
}

func (a *app) scheduleStop(id int64, after time.Duration) {
	a.timerMu.Lock()
	defer a.timerMu.Unlock()
	if old := a.timers[id]; old != nil {
		old.Stop()
	}
	a.timers[id] = time.AfterFunc(after, func() {
		_, _ = a.db.Exec("UPDATE competitions SET status='stopped',stopped_at=datetime('now') WHERE id=? AND status='running'", id)
		a.hub.broadcast(id, "competition:stop", map[string]any{"id": id, "reason": "timeout"})
		a.timerMu.Lock()
		delete(a.timers, id)
		a.timerMu.Unlock()
	})
}

func (a *app) cancelStop(id int64) {
	a.timerMu.Lock()
	defer a.timerMu.Unlock()
	if t := a.timers[id]; t != nil {
		t.Stop()
		delete(a.timers, id)
	}
}
