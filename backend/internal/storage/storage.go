// Package storage persists screenshots on disk and their metadata in a
// SQLite index.
//
// On-disk layout (deliberately boring and greppable — the frontend can browse
// it directly even without the DB):
//
//	<DataDir>/screenshots/2026/10/14/20261014-194103.jpg
//	<DataDir>/index.db
//
// Screenshots are private data: files are written 0600 and directories 0700.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/payamam/payam-logger/backend/internal/capture"
)

// Store owns the on-disk layout and the index database.
type Store struct {
	dataDir string
	dbPath  string
	db      *sql.DB
}

// New creates the directory layout and opens the index database.
func New(dataDir, dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "screenshots"), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &Store{dataDir: dataDir, dbPath: dbPath}
	if err := s.openDB(); err != nil {
		return nil, err
	}
	return s, nil
}

// DataDir returns the root data directory.
func (s *Store) DataDir() string { return s.dataDir }

// SaveScreenshot writes shot into the dated layout and records it in the
// index. It returns the path relative to DataDir.
func (s *Store) SaveScreenshot(shot *capture.Screenshot) (string, error) {
	t := shot.TakenAt
	rel := filepath.Join(
		"screenshots",
		fmt.Sprintf("%04d/%02d/%02d", t.Year(), int(t.Month()), t.Day()),
		fmt.Sprintf("%02d%02d%02d.jpg", t.Hour(), t.Minute(), t.Second()),
	)
	abs := filepath.Join(s.dataDir, rel)

	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return "", fmt.Errorf("create screenshot dir: %w", err)
	}
	if err := os.WriteFile(abs, shot.Data, 0o600); err != nil {
		return "", fmt.Errorf("write screenshot: %w", err)
	}

	if err := s.indexScreenshot(shot, rel); err != nil {
		return rel, fmt.Errorf("index screenshot: %w", err)
	}
	return rel, nil
}
