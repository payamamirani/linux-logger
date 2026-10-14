package storage

import (
	"context"
	"fmt"
	"time"
)

// autotags.go — autotag rule persistence (M4). Rules are stored once and
// evaluated live over the recorded app_usage data.

// AutotagRule assigns a tag to app/title usage matching its patterns.
// Empty patterns act as wildcards; non-empty ones are case-insensitive
// substring matches.
type AutotagRule struct {
	ID           int64
	Name         string
	AppPattern   string
	TitlePattern string
}

// AddAutotag stores a rule.
func (s *Store) AddAutotag(ctx context.Context, name, appPattern, titlePattern string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO autotags (name, app_pattern, title_pattern, created_at)
		 VALUES (?, ?, ?, ?)`,
		name, appPattern, titlePattern, time.Now().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert autotag: %w", err)
	}
	return nil
}

// DeleteAutotag removes a rule by id.
func (s *Store) DeleteAutotag(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM autotags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete autotag %d: %w", id, err)
	}
	return nil
}

// Autotags returns all rules, oldest first (rule order matters when
// multiple rules match the same time).
func (s *Store) Autotags(ctx context.Context) ([]AutotagRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, app_pattern, title_pattern FROM autotags ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query autotags: %w", err)
	}
	defer rows.Close()

	var out []AutotagRule
	for rows.Next() {
		var r AutotagRule
		if err := rows.Scan(&r.ID, &r.Name, &r.AppPattern, &r.TitlePattern); err != nil {
			return nil, fmt.Errorf("scan autotag: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
