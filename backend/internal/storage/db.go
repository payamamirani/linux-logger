package storage

// The SQLite index is the single source of truth for the frontend timeline.
//
// Planned schema (v1):
//
//	CREATE TABLE screenshots (
//	  id           INTEGER PRIMARY KEY,
//	  taken_at     TEXT NOT NULL,   -- UTC, RFC3339
//	  path         TEXT NOT NULL,   -- relative to DataDir
//	  size_bytes   INTEGER NOT NULL,
//	  width        INTEGER,
//	  height       INTEGER,
//	  app          TEXT,            -- future: active application
//	  window_title TEXT             -- future: active window title
//	);
//	CREATE INDEX idx_screenshots_taken_at ON screenshots(taken_at);
//
// TODO(backend): add modernc.org/sqlite (pure Go, no cgo) and:
//   - open/create the DB in openDB
//   - run migrations (schema versioning)
//   - insert a row per screenshot in indexScreenshot
//   - query helpers for the frontend (by day / by time range)
//
// Until then both methods are no-ops so the daemon can run end-to-end with
// filesystem storage only.

// openDB opens (and creates if needed) the SQLite index.
func (s *Store) openDB() error {
	return nil // index not implemented yet
}

// indexScreenshot records a saved screenshot in the index.
func (s *Store) indexScreenshot(shot any, relPath string) error {
	_ = shot
	_ = relPath
	return nil // index not implemented yet
}
