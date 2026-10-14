// Package scheduler provides a simple fixed-interval job runner.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler triggers a job every interval until its context is cancelled.
type Scheduler struct {
	interval time.Duration
	log      *slog.Logger
}

// New creates a Scheduler firing every interval.
func New(interval time.Duration, log *slog.Logger) *Scheduler {
	return &Scheduler{interval: interval, log: log}
}

// Run blocks until ctx is cancelled. It fires fn once immediately on start,
// then on every tick. Errors from fn are logged and never abort the loop —
// a failed capture must not stop the service.
func (s *Scheduler) Run(ctx context.Context, fn func(context.Context) error) error {
	s.log.Info("scheduler started", "interval", s.interval.String())

	// Fire immediately so the first screenshot lands on startup, not
	// after a full interval.
	if err := fn(ctx); err != nil {
		s.log.Error("job failed", "err", err)
	}

	t := time.NewTicker(s.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler stopped")
			return ctx.Err()
		case <-t.C:
			if err := fn(ctx); err != nil {
				s.log.Error("job failed", "err", err)
			}
		}
	}
}
