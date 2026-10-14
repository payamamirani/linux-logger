// Package autotag computes tag segments from rules applied to recorded
// application usage — the ManicTime autotagging model: rules are data,
// autotags are always recalculated, never stored.
package autotag

import (
	"sort"
	"strings"
)

// Rule is one autotag rule (patterns are case-insensitive substrings;
// empty patterns match anything).
type Rule struct {
	ID           int64
	Name         string
	AppPattern   string
	TitlePattern string
}

// Usage is one recorded application-usage segment.
type Usage struct {
	Start, End float64 // seconds of day
	App        string
	Title      string
}

// Segment is one computed autotag block.
type Segment struct {
	Start, End float64
	Name       string // the rule's tag name
	RuleID     int64
	App        string // matched app (for details display)
}

// Matches reports whether an app/title pair triggers the rule.
func (r Rule) Matches(app, title string) bool {
	if r.AppPattern != "" && !containsFold(app, r.AppPattern) {
		return false
	}
	if r.TitlePattern != "" && !containsFold(title, r.TitlePattern) {
		return false
	}
	return true
}

// Apply evaluates all rules over the usage segments. Each matching (rule,
// usage) pair yields one output segment; overlapping outputs from different
// rules are kept (ManicTime shows overlapping autotags on separate lines —
// we simply stack them, later rules after earlier ones).
func Apply(rules []Rule, usage []Usage) []Segment {
	var out []Segment
	for _, u := range usage {
		if u.End <= u.Start {
			continue
		}
		for _, r := range rules {
			if r.Matches(u.App, u.Title) {
				out = append(out, Segment{
					Start: u.Start, End: u.End,
					Name: r.Name, RuleID: r.ID, App: u.App,
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].End < out[j].End
	})
	return out
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
