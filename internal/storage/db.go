package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/payamam/payam-logger/internal/capture"
	_ "modernc.org/sqlite" // register the "sqlite" driver (pure Go, no cgo)
)

// schemaVersion is the current DB schema version, stored in
// PRAGMA user_version. Migrations bump this and add steps below.
const schemaVersion = 1

// migrations maps a schema version to the statements that upgrade a DB from
// that version to the next one.
var migrations = map[int][]string{
	0: {
		// v1: initial schema. `day` is the local YYYY-MM-DD taken from
		// taken_at — denormalized on purpose so the frontend can group and
		// filter days with a plain indexed equality.
		`CREATE TABLE screenshots (
			id           INTEGER PRIMARY KEY,
			taken_at     TEXT NOT NULL,   -- RFC3339, local offset
			day          TEXT NOT NULL,   -- YYYY-MM-DD (local)
			path         TEXT NOT NULL,   -- relative to DataDir
			size_bytes   INTEGER NOT NULL,
			width        INTEGER,
			height       INTEGER,
			app          TEXT,            -- reserved: active application
			window_title TEXT             -- reserved: active window title
		)`,
		`CREATE INDEX idx_screenshots_taken_at ON screenshots(taken_at)`,
		`CREATE INDEX idx_screenshots_day ON screenshots(day)`,
	},
}

// DaySummary aggregates one recorded day, for calendar/timeline navigation.
type DaySummary struct {
	Day       string // YYYY-MM-DD
	Count     int    // number of screenshots
	FirstAt   string // first taken_at of the day
	LastAt    string // last taken_at of the day
	TotalSize int64  // sum of file sizes in bytes
}

// Record describes one stored screenshot (one index row).
type Record struct {
	ID          int64
	TakenAt     string // RFC3339
	Day         string // YYYY-MM-DD
	Path        string // relative to DataDir
	SizeBytes   int64
	Width       int
	Height      int
	App         string // empty until active-window collection lands
	WindowTitle string // empty until active-window collection lands
}

// openDB opens (and creates if needed) the SQLite index at s.dbPath and
// brings the schema up to date.
func (s *Store) openDB() error {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", s.dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	// The daemon is the only writer; a single connection sidesteps
	// SQLITE_BUSY entirely.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return err
	}
	s.db = db
	return nil
}

// migrate applies pending schema migrations step by step.
func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for version < schemaVersion {
		stmts, ok := migrations[version]
		if !ok {
			return fmt.Errorf("no migration from schema version %d", version)
		}
		for _, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("migrate v%d: %w", version+1, err)
			}
		}
		version++
		if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
			return fmt.Errorf("set schema version %d: %w", version, err)
		}
	}
	return nil
}

// Close closes the index database.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// indexScreenshot records a saved screenshot in the index.
func (s *Store) indexScreenshot(shot *capture.Screenshot, relPath string) error {
	_, err := s.db.Exec(
		`INSERT INTO screenshots (taken_at, day, path, size_bytes, width, height)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		shot.TakenAt.Format(time.RFC3339),
		shot.TakenAt.Format("2006-01-02"),
		relPath,
		len(shot.Data),
		shot.Width,
		shot.Height,
	)
	return err
}

// Days returns one summary row per recorded day, newest first.
func (s *Store) Days(ctx context.Context) ([]DaySummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, COUNT(*), MIN(taken_at), MAX(taken_at), SUM(size_bytes)
		FROM screenshots
		GROUP BY day
		ORDER BY day DESC`)
	if err != nil {
		return nil, fmt.Errorf("query days: %w", err)
	}
	defer rows.Close()

	var out []DaySummary
	for rows.Next() {
		var d DaySummary
		if err := rows.Scan(&d.Day, &d.Count, &d.FirstAt, &d.LastAt, &d.TotalSize); err != nil {
			return nil, fmt.Errorf("scan day summary: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ScreenshotsForDay returns all records of one day ordered by time.
func (s *Store) ScreenshotsForDay(ctx context.Context, day string) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, taken_at, day, path, size_bytes, width, height, app, window_title
		FROM screenshots
		WHERE day = ?
		ORDER BY taken_at`, day)
	if err != nil {
		return nil, fmt.Errorf("query day %s: %w", day, err)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var r Record
		var app, title sql.NullString
		if err := rows.Scan(&r.ID, &r.TakenAt, &r.Day, &r.Path, &r.SizeBytes,
			&r.Width, &r.Height, &app, &title); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		r.App = app.String
		r.WindowTitle = title.String
		out = append(out, r)
	}
	return out, rows.Err()
}
