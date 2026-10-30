PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS teachers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  display_name TEXT NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS articles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  teacher_id INTEGER REFERENCES teachers(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS teacher_articles (
  teacher_id INTEGER NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
  article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  PRIMARY KEY (teacher_id, article_id)
);

CREATE TABLE IF NOT EXISTS competitions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  teacher_id INTEGER NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
  article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE RESTRICT,
  access_code TEXT NOT NULL UNIQUE COLLATE NOCASE,
  request_id TEXT,
  status TEXT NOT NULL DEFAULT 'waiting' CHECK(status IN ('waiting','running','stopped')),
  time_limit INTEGER NOT NULL DEFAULT 300 CHECK(time_limit BETWEEN 10 AND 86400),
  pinyin_enabled INTEGER NOT NULL DEFAULT 1,
  text_options TEXT NOT NULL DEFAULT '',
  started_at TEXT,
  stopped_at TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  competition_id INTEGER NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
  student_id TEXT NOT NULL,
  student_name TEXT NOT NULL,
  correct_count INTEGER NOT NULL DEFAULT 0 CHECK(correct_count >= 0),
  typed_count INTEGER NOT NULL DEFAULT 0 CHECK(typed_count >= correct_count),
  duration INTEGER NOT NULL DEFAULT 0 CHECK(duration >= 0),
  class_id INTEGER REFERENCES classes(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS classes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  teacher_id INTEGER NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  remote_id TEXT,
  source TEXT NOT NULL DEFAULT 'manual' CHECK(source IN ('manual','classorbit')),
  active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS class_students (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  class_id INTEGER NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
  student_id TEXT NOT NULL,
  student_name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(class_id, student_id)
);

CREATE TABLE IF NOT EXISTS app_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_competitions_teacher ON competitions(teacher_id, created_at DESC);
-- Composite indexes match the list/filter queries used by the teacher console.
-- Keep the existing single-purpose indexes for compatibility with older files;
-- these indexes are created idempotently on every startup.
CREATE INDEX IF NOT EXISTS idx_competitions_teacher_status_created
  ON competitions(teacher_id, status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_competitions_article ON competitions(article_id);
CREATE INDEX IF NOT EXISTS idx_results_competition ON results(competition_id);
CREATE INDEX IF NOT EXISTS idx_results_student_lookup ON results(student_id, competition_id);
CREATE INDEX IF NOT EXISTS idx_classes_teacher ON classes(teacher_id);
CREATE INDEX IF NOT EXISTS idx_class_students_student_class ON class_students(student_id, class_id);
CREATE INDEX IF NOT EXISTS idx_articles_teacher_created ON articles(teacher_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_teacher_articles_article_teacher ON teacher_articles(article_id, teacher_id);
