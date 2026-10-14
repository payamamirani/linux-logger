package autotag

import (
	"testing"
)

func TestMatches(t *testing.T) {
	r := Rule{Name: "Browsing", AppPattern: "firefox"}

	if !r.Matches("Firefox", "anything") {
		t.Error("case-insensitive app match failed")
	}
	if r.Matches("chrome", "gmail") {
		t.Error("must not match other apps")
	}

	r2 := Rule{Name: "Mail", AppPattern: "chrome", TitlePattern: "Gmail"}
	if !r2.Matches("chrome", "Inbox — Gmail") {
		t.Error("title match should be case-insensitive substring")
	}
	if !r2.Matches("Google-Chrome", "Mail - Gmail - x") {
		t.Error("expected match on both patterns")
	}
	if r2.Matches("chrome", "news") {
		t.Error("title pattern must filter")
	}

	r3 := Rule{Name: "All"} // wildcard
	if !r3.Matches("anything", "whatever") {
		t.Error("empty patterns must match everything")
	}
}

func TestApply(t *testing.T) {
	rules := []Rule{
		{ID: 1, Name: "Browsing", AppPattern: "firefox"},
		{ID: 2, Name: "Mail", AppPattern: "chrome", TitlePattern: "gmail"},
	}
	usage := []Usage{
		{Start: 100, End: 200, App: "firefox", Title: "news"},
		{Start: 300, End: 400, App: "chrome", Title: "Gmail"},
		{Start: 500, End: 600, App: "gedit", Title: "notes"},
	}

	segs := Apply(rules, usage)
	if len(segs) != 2 {
		t.Fatalf("segments = %d, want 2: %+v", len(segs), segs)
	}
	if segs[0].Name != "Browsing" || segs[0].RuleID != 1 {
		t.Errorf("seg0 = %+v", segs[0])
	}
	if segs[1].Name != "Mail" || segs[1].Start != 300 {
		t.Errorf("seg1 = %+v", segs[1])
	}
}

func TestApplyOverlappingRules(t *testing.T) {
	rules := []Rule{
		{ID: 1, Name: "Work"},
		{ID: 2, Name: "Browsing", AppPattern: "firefox"},
	}
	usage := []Usage{{Start: 0, End: 60, App: "firefox", Title: "x"}}

	segs := Apply(rules, usage)
	if len(segs) != 2 {
		t.Fatalf("overlapping rules must both yield segments, got %d", len(segs))
	}
}

func TestApplySkipsEmptySegments(t *testing.T) {
	rules := []Rule{{ID: 1, Name: "All"}}
	usage := []Usage{{Start: 10, End: 10, App: "x", Title: "y"}}
	if segs := Apply(rules, usage); len(segs) != 0 {
		t.Fatalf("zero-length usage must be skipped, got %+v", segs)
	}
}
