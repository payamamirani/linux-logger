// Package web serves the embedded web UI and its JSON API on localhost.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/payamam/payam-logger/internal/autotag"
	"github.com/payamam/payam-logger/internal/config"
	"github.com/payamam/payam-logger/internal/storage"
)

// Server is the embedded UI/API HTTP server.
type Server struct {
	store *storage.Store
	cfg   *config.Config
	log   *slog.Logger
}

var dayRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// New creates the server (does not start listening).
func New(store *storage.Store, cfg *config.Config, log *slog.Logger) *Server {
	return &Server{store: store, cfg: cfg, log: log}
}

// Run blocks serving HTTP on 127.0.0.1:<port> until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.cfg.Server.Port))
	srv := &http.Server{Addr: addr, Handler: s.Handler()}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("web listen %s: %w", addr, err)
	}
	s.log.Info("web UI listening", "addr", "http://"+addr)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Handler builds the full HTTP handler: /api/* + static UI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/days", s.handleDays)
	mux.HandleFunc("GET /api/day/{day}", s.handleDay)
	mux.HandleFunc("GET /api/screenshot/{id}", s.handleScreenshot)
	mux.HandleFunc("DELETE /api/screenshot/{id}", s.handleDeleteScreenshot)
	mux.HandleFunc("POST /api/tags", s.handleAddTag)
	mux.HandleFunc("DELETE /api/tags/{id}", s.handleDeleteTag)
	mux.HandleFunc("GET /api/autotags", s.handleAutotags)
	mux.HandleFunc("POST /api/autotags", s.handleAddAutotag)
	mux.HandleFunc("DELETE /api/autotags/{id}", s.handleDeleteAutotag)
	mux.HandleFunc("GET /api/timesheet", s.handleTimesheet)
	mux.HandleFunc("PUT /api/settings", s.handleSettings)

	mux.Handle("/", http.FileServer(http.FS(uiFS())))

	return mux
}

// --- helpers -----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// secOfDay converts an RFC3339 timestamp to seconds of its local day.
func secOfDay(s string) float64 {
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return -1
	}
	return float64(ts.Hour()*3600 + ts.Minute()*60 + ts.Second())
}

func hhmm(sec float64) string {
	return fmt.Sprintf("%02d:%02d", int(sec)/3600, int(sec)%3600/60)
}

// --- status -------------------------------------------------------------------

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"version":       "web-1",
		"intervalSec":   s.cfg.Capture.IntervalSec,
		"retentionDays": s.cfg.Storage.RetentionDays,
	})
}

// --- days ---------------------------------------------------------------------

func (s *Server) handleDays(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	set := map[string]struct{}{}
	for _, q := range []string{
		`SELECT DISTINCT day FROM screenshots`,
		`SELECT DISTINCT day FROM app_usage`,
		`SELECT DISTINCT day FROM computer_usage`,
		`SELECT DISTINCT day FROM tags`,
	} {
		rows, err := s.store.DB().QueryContext(ctx, q)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		for rows.Next() {
			var d string
			if rows.Scan(&d) == nil {
				set[d] = struct{}{}
			}
		}
		rows.Close()
	}
	days := make([]string, 0, len(set))
	for d := range set {
		days = append(days, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	writeJSON(w, 200, days)
}

// --- day payload ----------------------------------------------------------------

type segJSON struct {
	Start float64 `json:"start"` // seconds of day
	End   float64 `json:"end"`
	App   string  `json:"app,omitempty"`
	Title string  `json:"title,omitempty"`
	State string  `json:"state,omitempty"`
	Name  string  `json:"name,omitempty"`
	ID    int64   `json:"id,omitempty"`
}

type shotJSON struct {
	ID        int64   `json:"id"`
	TakenAt   string  `json:"takenAt"`
	Time      string  `json:"time"` // HH:MM:SS
	T         float64 `json:"t"`    // seconds of day
	SizeBytes int64   `json:"sizeBytes"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
}

type tagJSON struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Note     string  `json:"note"`
	Billable bool    `json:"billable"`
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	StartStr string  `json:"startStr"`
	EndStr   string  `json:"endStr"`
}

func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	day := r.PathValue("day")
	if !dayRe.MatchString(day) {
		writeErr(w, 400, fmt.Errorf("day must be YYYY-MM-DD"))
		return
	}
	ctx := r.Context()

	shots, err := s.store.ScreenshotsForDay(ctx, day)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	shotsJ := make([]shotJSON, 0, len(shots))
	for _, sh := range shots {
		shotsJ = append(shotsJ, shotJSON{
			ID: sh.ID, TakenAt: sh.TakenAt, Time: sh.TakenAt[11:19],
			T: secOfDay(sh.TakenAt), SizeBytes: sh.SizeBytes,
			Width: sh.Width, Height: sh.Height,
		})
	}

	apps, err := s.store.AppUsageForDay(ctx, day)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	appsJ := []segJSON{}
	usageForAutotag := make([]autotag.Usage, 0, len(apps))
	for _, a := range apps {
		lo, hi := secOfDay(a.Start), secOfDay(a.End)
		if hi <= lo {
			continue
		}
		appsJ = append(appsJ, segJSON{Start: lo, End: hi, App: a.App, Title: a.Title})
		usageForAutotag = append(usageForAutotag, autotag.Usage{Start: lo, End: hi, App: a.App, Title: a.Title})
	}

	cu, err := s.store.ComputerUsageForDay(ctx, day)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	usageJ := []segJSON{}
	for _, u := range cu {
		lo, hi := secOfDay(u.Start), secOfDay(u.End)
		if hi <= lo {
			continue
		}
		usageJ = append(usageJ, segJSON{Start: lo, End: hi, State: u.State})
	}

	tags, err := s.store.TagsForDay(ctx, day)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	tagsJ := []tagJSON{}
	for _, t := range tags {
		lo, hi := secOfDay(t.Start), secOfDay(t.End)
		tagsJ = append(tagsJ, tagJSON{
			ID: t.ID, Name: t.Name, Note: t.Note, Billable: t.Billable,
			Start: lo, End: hi, StartStr: hhmm(lo), EndStr: hhmm(hi),
		})
	}

	rules, err := s.store.Autotags(ctx)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	atRules := make([]autotag.Rule, len(rules))
	rulesJ := make([]map[string]any, len(rules))
	for i, rr := range rules {
		atRules[i] = autotag.Rule{ID: rr.ID, Name: rr.Name, AppPattern: rr.AppPattern, TitlePattern: rr.TitlePattern}
		rulesJ[i] = map[string]any{"id": rr.ID, "name": rr.Name, "appPattern": rr.AppPattern, "titlePattern": rr.TitlePattern}
	}
	autoJ := []segJSON{}
	for _, seg := range autotag.Apply(atRules, usageForAutotag) {
		autoJ = append(autoJ, segJSON{Start: seg.Start, End: seg.End, Name: seg.Name, App: seg.App, ID: seg.RuleID})
	}

	writeJSON(w, 200, map[string]any{
		"day":             day,
		"intervalSec":     s.cfg.Capture.IntervalSec,
		"screenshots":     shotsJ,
		"appUsage":        appsJ,
		"computerUsage":   usageJ,
		"tags":            tagsJ,
		"autotagSegments": autoJ,
		"autotagRules":    rulesJ,
	})
}

// --- screenshots -----------------------------------------------------------------

func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	var path string
	err := s.store.DB().QueryRowContext(r.Context(),
		`SELECT path FROM screenshots WHERE id = ?`, id).Scan(&path)
	if err != nil {
		writeErr(w, 404, fmt.Errorf("screenshot %d not found", id))
		return
	}
	data, err := s.store.ReadScreenshotAbs(path)
	if err != nil {
		writeErr(w, 500, err)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if r.URL.Query().Get("download") != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="payam-logger-`+strconv.FormatInt(id, 10)+`.jpg"`)
	}
	_, _ = w.Write(data)
}

func (s *Server) handleDeleteScreenshot(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.store.DeleteScreenshot(r.Context(), id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- tags -------------------------------------------------------------------------

type addTagReq struct {
	Day      string `json:"day"`
	StartSec int    `json:"startSec"` // seconds of day
	EndSec   int    `json:"endSec"`
	Name     string `json:"name"`
	Note     string `json:"note"`
	Billable bool   `json:"billable"`
}

func (s *Server) handleAddTag(w http.ResponseWriter, r *http.Request) {
	var req addTagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if !dayRe.MatchString(req.Day) || req.Name == "" || req.EndSec <= req.StartSec {
		writeErr(w, 400, fmt.Errorf("invalid tag payload"))
		return
	}
	day, _ := time.Parse("2006-01-02", req.Day)
	start := time.Date(day.Year(), day.Month(), day.Day(),
		req.StartSec/3600, (req.StartSec%3600)/60, 0, 0, time.Local)
	end := start.Add(time.Duration((req.EndSec - req.StartSec) * int(time.Second)))

	if err := s.store.AddTag(start, end, req.Name, req.Note, req.Billable); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.store.DeleteTag(r.Context(), id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- autotags -----------------------------------------------------------------------

type addAutotagReq struct {
	Name         string `json:"name"`
	AppPattern   string `json:"appPattern"`
	TitlePattern string `json:"titlePattern"`
}

func (s *Server) handleAutotags(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.Autotags(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	out := make([]map[string]any, len(rules))
	for i, rr := range rules {
		out[i] = map[string]any{"id": rr.ID, "name": rr.Name, "appPattern": rr.AppPattern, "titlePattern": rr.TitlePattern}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleAddAutotag(w http.ResponseWriter, r *http.Request) {
	var req addAutotagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, 400, fmt.Errorf("name is required"))
		return
	}
	if err := s.store.AddAutotag(r.Context(),
		strings.TrimSpace(req.Name),
		strings.TrimSpace(req.AppPattern),
		strings.TrimSpace(req.TitlePattern)); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteAutotag(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.store.DeleteAutotag(r.Context(), id); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- timesheet ------------------------------------------------------------------------

func (s *Server) handleTimesheet(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	level, _ := strconv.Atoi(r.URL.Query().Get("level"))
	if !dayRe.MatchString(from) || !dayRe.MatchString(to) {
		writeErr(w, 400, fmt.Errorf("from/to must be YYYY-MM-DD"))
		return
	}

	rows, err := s.store.TagRowsRange(r.Context(), from, to)
	if err != nil {
		writeErr(w, 500, err)
		return
	}

	type res struct {
		Group string  `json:"group"`
		Total float64 `json:"totalSeconds"`
		Days  int     `json:"days"`
	}
	agg := map[string]*res{}
	for _, row := range rows {
		key := row.Name
		if level > 0 {
			parts := strings.Split(row.Name, ",")
			if level > len(parts) {
				continue
			}
			key = strings.TrimSpace(parts[level-1])
		}
		rr := agg[key]
		if rr == nil {
			rr = &res{Group: key}
			agg[key] = rr
		}
		lo, _ := time.Parse(time.RFC3339, row.Start)
		hi, _ := time.Parse(time.RFC3339, row.End)
		d := hi.Sub(lo).Seconds()
		rr.Total += d
		rr.Days++
	}
	out := []*res{}
	for _, rr := range agg {
		out = append(out, rr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	writeJSON(w, 200, out)
}

// --- settings ---------------------------------------------------------------------------

type settingsReq struct {
	IntervalSec   int `json:"intervalSec"`
	RetentionDays int `json:"retentionDays"`
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}

	cfg, err := config.Load("")
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	cfg.Capture.IntervalSec = req.IntervalSec
	cfg.Storage.RetentionDays = req.RetentionDays
	if err := cfg.ValidatePublic(); err != nil {
		writeErr(w, 400, err)
		return
	}
	path, err := config.DefaultPath()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if err := cfg.Save(path); err != nil {
		writeErr(w, 500, err)
		return
	}

	// Respond first, then restart the service so new settings load.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	time.AfterFunc(500*time.Millisecond, func() {
		out, err := exec.Command("systemctl", "--user", "restart", "payam-logger").CombinedOutput()
		if err != nil {
			s.log.Warn("settings restart failed", "err", err, "out", string(out))
		}
	})
}
