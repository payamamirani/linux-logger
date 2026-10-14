package storage

import (
	"context"
	"fmt"
	"time"
)

// activity.go — persistence for the M1 data engine: focused-window usage
// segments and active/away segments.

// SaveAppUsage records one focused-window usage segment.
func (s *Store) SaveAppUsage(start, end time.Time, app, title string, workspace int) error {
	_, err := s.db.Exec(
		`INSERT INTO app_usage (start, end, day, app, title, workspace)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		start.Format(time.RFC3339), end.Format(time.RFC3339),
		start.Format("2006-01-02"), app, title, workspace,
	)
	if err != nil {
		return fmt.Errorf("insert app_usage: %w", err)
	}
	return nil
}

// SaveComputerUsage records one active/away segment.
func (s *Store) SaveComputerUsage(start, end time.Time, state string) error {
	_, err := s.db.Exec(
		`INSERT INTO computer_usage (start, end, day, state)
		 VALUES (?, ?, ?, ?)`,
		start.Format(time.RFC3339), end.Format(time.RFC3339),
		start.Format("2006-01-02"), state,
	)
	if err != nil {
		return fmt.Errorf("insert computer_usage: %w", err)
	}
	return nil
}

// AppUsageRecord is one row of app_usage.
type AppUsageRecord struct {
	ID        int64
	Start     string
	End       string
	Day       string
	App       string
	Title     string
	Workspace int
}

// AppUsageForDay returns all focus segments of one day ordered by time.
func (s *Store) AppUsageForDay(ctx context.Context, day string) ([]AppUsageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, start, end, day, app, title, workspace
		FROM app_usage WHERE day = ? ORDER BY start`, day)
	if err != nil {
		return nil, fmt.Errorf("query app_usage %s: %w", day, err)
	}
	defer rows.Close()

	var out []AppUsageRecord
	for rows.Next() {
		var r AppUsageRecord
		if err := rows.Scan(&r.ID, &r.Start, &r.End, &r.Day, &r.App, &r.Title, &r.Workspace); err != nil {
			return nil, fmt.Errorf("scan app_usage: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ComputerUsageRecord is one row of computer_usage.
type ComputerUsageRecord struct {
	ID    int64
	Start string
	End   string
	Day   string
	State string
}

// ComputerUsageForDay returns the active/away segments of one day.
func (s *Store) ComputerUsageForDay(ctx context.Context, day string) ([]ComputerUsageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, start, end, day, state
		FROM computer_usage WHERE day = ? ORDER BY start`, day)
	if err != nil {
		return nil, fmt.Errorf("query computer_usage %s: %w", day, err)
	}
	defer rows.Close()

	var out []ComputerUsageRecord
	for rows.Next() {
		var r ComputerUsageRecord
		if err := rows.Scan(&r.ID, &r.Start, &r.End, &r.Day, &r.State); err != nil {
			return nil, fmt.Errorf("scan computer_usage: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
