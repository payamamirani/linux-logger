package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// tags.go — manual tag persistence (M3) and screenshot deletion (M5).

// TagRecord is one row of the tags table.
type TagRecord struct {
	ID       int64
	Start    string // RFC3339 local
	End      string
	Day      string
	Name     string // comma-separated hierarchy
	Note     string
	Billable bool
}

// AddTag records one manually assigned tag.
func (s *Store) AddTag(start, end time.Time, name, note string, billable bool) error {
	_, err := s.db.Exec(
		`INSERT INTO tags (start, end, day, name, note, billable)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		start.Format(time.RFC3339), end.Format(time.RFC3339),
		start.Format("2006-01-02"), name, note, boolToInt(billable),
	)
	if err != nil {
		return fmt.Errorf("insert tag: %w", err)
	}
	return nil
}

// DeleteTag removes a tag by id.
func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete tag %d: %w", id, err)
	}
	return nil
}

// TagsForDay returns all tags of one day ordered by start time.
func (s *Store) TagsForDay(ctx context.Context, day string) ([]TagRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, start, end, day, name, note, billable
		FROM tags WHERE day = ? ORDER BY start`, day)
	if err != nil {
		return nil, fmt.Errorf("query tags %s: %w", day, err)
	}
	defer rows.Close()

	var out []TagRecord
	for rows.Next() {
		var r TagRecord
		var billable int
		if err := rows.Scan(&r.ID, &r.Start, &r.End, &r.Day, &r.Name, &r.Note, &billable); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		r.Billable = billable != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// TagRow is a raw tag row in a date range (for the timesheet).
type TagRow struct {
	Day      string
	Name     string
	Note     string
	Billable bool
	Start    string
	End      string
}

// TagRowsRange returns all tag rows whose day is within [from, to]
// (inclusive, YYYY-MM-DD strings compare lexicographically).
func (s *Store) TagRowsRange(ctx context.Context, from, to string) ([]TagRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, name, note, billable, start, end
		FROM tags WHERE day >= ? AND day <= ? ORDER BY day, start`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query tags range: %w", err)
	}
	defer rows.Close()

	var out []TagRow
	for rows.Next() {
		var r TagRow
		var billable int
		if err := rows.Scan(&r.Day, &r.Name, &r.Note, &billable, &r.Start, &r.End); err != nil {
			return nil, fmt.Errorf("scan tag row: %w", err)
		}
		r.Billable = billable != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteScreenshot removes a screenshot file and its index row.
func (s *Store) DeleteScreenshot(ctx context.Context, id int64) error {
	var path string
	err := s.db.QueryRowContext(ctx,
		`SELECT path FROM screenshots WHERE id = ?`, id).Scan(&path)
	if err != nil {
		return fmt.Errorf("find screenshot %d: %w", id, err)
	}

	if err := os.Remove(filepath.Join(s.dataDir, path)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM screenshots WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete row %d: %w", id, err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
