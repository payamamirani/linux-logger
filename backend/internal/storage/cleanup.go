package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Cleanup deletes every screenshot older than before — both the JPEG files
// (with their directories) and the index rows — and returns how many
// screenshots were removed.
func (s *Store) Cleanup(ctx context.Context, before time.Time) (int, error) {
	cutoffDay := before.Format("2006-01-02")

	rows, err := s.db.QueryContext(ctx,
		`SELECT path FROM screenshots WHERE day < ?`, cutoffDay)
	if err != nil {
		return 0, fmt.Errorf("select expired screenshots: %w", err)
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan expired path: %w", err)
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	if len(paths) == 0 {
		return 0, nil
	}

	for _, p := range paths {
		// A missing file is fine — the row is removed regardless.
		if err := os.Remove(filepath.Join(s.dataDir, p)); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("remove %s: %w", p, err)
		}
	}

	res, err := s.db.ExecContext(ctx,
		`DELETE FROM screenshots WHERE day < ?`, cutoffDay)
	if err != nil {
		return 0, fmt.Errorf("delete expired rows: %w", err)
	}
	deleted, _ := res.RowsAffected()

	s.removeEmptyDayDirs()

	return int(deleted), nil
}

// removeEmptyDayDirs prunes empty YYYY/MM/DD directories left behind after
// file deletions.
func (s *Store) removeEmptyDayDirs() {
	root := filepath.Join(s.dataDir, "screenshots")

	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil //nolint:nilerr // walking best-effort
	})

	// Deepest first, so parents see their children gone.
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
}
