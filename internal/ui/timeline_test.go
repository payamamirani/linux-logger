package ui

import "testing"

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		sec  float64
		want string
	}{
		{0, "0s"},
		{45, "45s"},
		{59, "59s"},
		{60, "1m 0s"},
		{13 * 60, "13m 0s"},
		{13*60 + 45, "13m 45s"},
		{3600, "1h"},
		{(2*3600 + 13*60), "2h 13m"}, // the spec example
		{86399, "23h 59m"},
	}
	for _, c := range cases {
		if got := formatDuration(c.sec); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.sec, got, c.want)
		}
	}
}

func TestFormatClock(t *testing.T) {
	if got := formatClock(8 * 3600); got != "08:00" {
		t.Errorf("formatClock(8h) = %q, want 08:00", got)
	}
	if got := formatClock(10*3600 + 13*60 + 59); got != "10:13" {
		t.Errorf("formatClock(10:13:59) = %q, want 10:13", got)
	}
}
