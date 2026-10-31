package dates_test

import (
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/dates"
)

func day(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

func TestExceedsMaxSpanIsExactAtTheBoundary(t *testing.T) {
	from := day(2000, time.January, 1)
	limit := from.AddDate(0, 0, dates.MaxSpanDays)
	cases := []struct {
		name string
		to   time.Time
		want bool
	}{
		{"same day", from, false},
		{"reversed", from.AddDate(0, 0, -5), false},
		{"exactly the cap", limit, false},
		{"one day over", limit.AddDate(0, 0, 1), true},
		{"millennia", day(9999, time.December, 31), true},
	}
	for _, c := range cases {
		if got := dates.ExceedsMaxSpan(from, c.to); got != c.want {
			t.Errorf("%s: ExceedsMaxSpan = %v, want %v", c.name, got, c.want)
		}
	}
	if dates.ExceedsMaxSpan(day(1, time.January, 1), day(9999, time.December, 31)) != true {
		t.Error("year 1 to year 9999 must exceed the cap")
	}
}

func TestClampSpanStartKeepsAtMostTheCap(t *testing.T) {
	to := day(2026, time.October, 31)
	floor := to.AddDate(0, 0, -dates.MaxSpanDays)

	if got := dates.ClampSpanStart(day(2020, time.January, 1), to); !got.Equal(day(2020, time.January, 1)) {
		t.Errorf("recent start moved to %v", got)
	}
	if got := dates.ClampSpanStart(floor, to); !got.Equal(floor) {
		t.Errorf("start at the floor moved to %v", got)
	}
	for _, from := range []time.Time{floor.AddDate(0, 0, -1), day(202, time.January, 1), day(1, time.January, 1)} {
		got := dates.ClampSpanStart(from, to)
		if !got.Equal(floor) {
			t.Errorf("clamp(%v) = %v, want %v", from, got, floor)
		}
		if dates.ExceedsMaxSpan(got, to) {
			t.Errorf("clamped start %v still exceeds the cap", got)
		}
	}
}
