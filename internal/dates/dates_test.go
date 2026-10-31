package dates_test

import (
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/dates"
)

// TestDaySameWallClockDateInDifferentZonesCompareEqual is the whole reason
// this package exists: two engines can arrive at the same calendar date by
// different routes — one parsing a UTC date column, one computing in
// time.Local — and the results must compare equal. Leaving each caller's own
// zone on the truncated value would produce two different instants for what
// both engines consider the same date.
func TestDaySameWallClockDateInDifferentZonesCompareEqual(t *testing.T) {
	utc := time.Date(2026, 5, 10, 3, 0, 0, 0, time.UTC)
	east := time.Date(2026, 5, 10, 23, 30, 0, 0, time.FixedZone("UTC+9", 9*3600))

	if !dates.Day(utc).Equal(dates.Day(east)) {
		t.Errorf("Day(%v) = %v, Day(%v) = %v — the same calendar day must compare equal",
			utc, dates.Day(utc), east, dates.Day(east))
	}
	if got := dates.Day(utc); got != time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC) {
		t.Errorf("Day = %v, want midnight UTC on 2026-05-10", got)
	}
}

// TestDayKeepsTheWallClockDate pins that Day reads the date off the time as
// the caller sees it rather than converting to UTC first — 2026-05-10 late
// evening in a +09:00 zone is the 10th, not the 9th.
func TestDayKeepsTheWallClockDate(t *testing.T) {
	east := time.Date(2026, 5, 10, 23, 30, 0, 0, time.FixedZone("UTC+9", 9*3600))
	if got := dates.Day(east); got.Day() != 10 || got.Month() != time.May {
		t.Errorf("Day = %v, want 2026-05-10", got)
	}
}

// TestDayOfOneInstantDiffersAcrossZones pins the corollary of the rule
// above, which is the sharp edge a caller has to know about: Day is not an
// instant-to-day mapping. One moment in time is a different calendar date
// depending on the zone the time.Time carries, and Day faithfully reports
// each. A caller that means "the day this instant fell on in zone Z" has to
// convert to Z before calling.
func TestDayOfOneInstantDiffersAcrossZones(t *testing.T) {
	instant := time.Date(2026, 5, 11, 1, 0, 0, 0, time.UTC)
	west := instant.In(time.FixedZone("UTC-5", -5*3600)) // 2026-05-10 20:00

	if dates.Day(instant).Equal(dates.Day(west)) {
		t.Errorf("Day(%v) = %v and Day(%v) = %v — the same instant is a different wall-clock date in each zone, and Day must report each faithfully",
			instant, dates.Day(instant), west, dates.Day(west))
	}
	if got := dates.Day(west); got != time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC) {
		t.Errorf("Day(west) = %v, want midnight UTC on 2026-05-10", got)
	}
}

// TestDayIsIdempotent pins that re-truncating an already-truncated day is a
// no-op — callers layer Day over each other freely (BuildSeries truncates its
// window, then ListPlanInstallments truncates it again).
func TestDayIsIdempotent(t *testing.T) {
	once := dates.Day(time.Date(2026, 5, 10, 14, 22, 9, 500, time.UTC))
	if twice := dates.Day(once); !twice.Equal(once) {
		t.Errorf("Day(Day(t)) = %v, want %v", twice, once)
	}
}

// TestDaysInMonthIsZoneIndependent pins the property that let DaysInMonth
// drop the *time.Location parameter the three copies it replaced disagreed
// about (one keyed on time.Local, two on UTC): how many days a month has is
// calendar arithmetic, not a point in time, so no zone can change it.
func TestDaysInMonthIsZoneIndependent(t *testing.T) {
	// Zones on both sides of UTC, plus one that historically shifted its
	// clocks at midnight — the case that makes the day-zero normalization
	// ambiguous if it were done in a local zone rather than UTC.
	for _, name := range []string{"UTC", "America/Sao_Paulo", "Asia/Tokyo", "America/Los_Angeles"} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Skipf("zone %s unavailable: %v", name, err)
		}
		for month := time.January; month <= time.December; month++ {
			want := dates.DaysInMonth(2026, month)
			got := time.Date(2026, month+1, 0, 0, 0, 0, 0, loc).Day()
			if got != want {
				t.Errorf("%s: %v 2026 has %d days, DaysInMonth says %d", name, month, got, want)
			}
		}
	}
}

// TestDaysInMonthLeapYear pins February, the only month whose answer moves.
func TestDaysInMonthLeapYear(t *testing.T) {
	for _, tc := range []struct {
		year int
		want int
	}{
		{2026, 28},
		{2024, 29},
		{2000, 29}, // divisible by 400 — a leap year
		{1900, 28}, // divisible by 100 but not 400 — not one
	} {
		if got := dates.DaysInMonth(tc.year, time.February); got != tc.want {
			t.Errorf("DaysInMonth(%d, February) = %d, want %d", tc.year, got, tc.want)
		}
	}
}

// TestDaysInMonthClampsAMonthlyCadence exercises what every caller actually
// uses it for: a commitment or card closing on the 31st has to land on the
// last day of a shorter month, never spill into the next one.
func TestDaysInMonthClampsAMonthlyCadence(t *testing.T) {
	for _, tc := range []struct {
		month time.Month
		want  int
	}{
		{time.November, 30},
		{time.February, 28},
		{time.December, 31},
	} {
		day := 31
		if max := dates.DaysInMonth(2026, tc.month); day > max {
			day = max
		}
		if day != tc.want {
			t.Errorf("day 31 clamped to %v 2026 = %d, want %d", tc.month, day, tc.want)
		}
	}
}

func ymd(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// TestAddMonthsClampsToTheLastDayOfAShorterMonth pins the rule that replaced
// time.AddDate's overflow (Jan 31 + 1 month → Mar 3), for days 29-31 across a
// leap and a non-leap February.
func TestAddMonthsClampsToTheLastDayOfAShorterMonth(t *testing.T) {
	for _, tc := range []struct {
		name string
		day  time.Time
		n    int
		want time.Time
	}{
		{"31 into non-leap February", ymd(2026, 1, 31), 1, ymd(2026, 2, 28)},
		{"30 into non-leap February", ymd(2026, 1, 30), 1, ymd(2026, 2, 28)},
		{"29 into non-leap February", ymd(2026, 1, 29), 1, ymd(2026, 2, 28)},
		{"31 into leap February", ymd(2028, 1, 31), 1, ymd(2028, 2, 29)},
		{"30 into leap February", ymd(2028, 1, 30), 1, ymd(2028, 2, 29)},
		{"29 into leap February", ymd(2028, 1, 29), 1, ymd(2028, 2, 29)},
		{"31 into a 30-day month", ymd(2026, 3, 31), 1, ymd(2026, 4, 30)},
		{"31 over a short month lands on 31 again", ymd(2026, 1, 31), 2, ymd(2026, 3, 31)},
		{"leap day to the next non-leap year", ymd(2028, 2, 29), 12, ymd(2029, 2, 28)},
		{"leap day to the next leap year", ymd(2028, 2, 29), 48, ymd(2032, 2, 29)},
		{"zero is the same day", ymd(2026, 1, 31), 0, ymd(2026, 1, 31)},
		{"a short month is not clamped further", ymd(2026, 1, 15), 1, ymd(2026, 2, 15)},
		{"December rolls into January", ymd(2026, 12, 31), 1, ymd(2027, 1, 31)},
		{"November rolls over December into January", ymd(2026, 11, 30), 2, ymd(2027, 1, 30)},
		{"negative into non-leap February", ymd(2026, 3, 31), -1, ymd(2026, 2, 28)},
		{"negative into leap February", ymd(2028, 3, 30), -1, ymd(2028, 2, 29)},
		{"negative across a year", ymd(2026, 1, 31), -2, ymd(2025, 11, 30)},
		{"large n", ymd(2026, 1, 31), 1200, ymd(2126, 1, 31)},
		{"large n into February", ymd(2026, 1, 31), 1201, ymd(2126, 2, 28)},
		{"large negative n", ymd(2026, 5, 31), -1200, ymd(1926, 5, 31)},
	} {
		if got := dates.AddMonths(tc.day, tc.n); !got.Equal(tc.want) {
			t.Errorf("%s: AddMonths(%s, %d) = %s, want %s", tc.name,
				tc.day.Format("2006-01-02"), tc.n, got.Format("2006-01-02"), tc.want.Format("2006-01-02"))
		}
	}
}

// TestAddMonthsDoesNotDriftWhenSteppedFromTheAnchor pins that every result
// comes from the original day: a series anchored on the 31st comes back to
// the 31st after each short month.
func TestAddMonthsDoesNotDriftWhenSteppedFromTheAnchor(t *testing.T) {
	anchor := ymd(2026, 1, 31)
	want := []string{
		"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31", "2026-06-30",
		"2026-07-31", "2026-08-31", "2026-09-30", "2026-10-31", "2026-11-30", "2026-12-31",
		"2027-01-31", "2027-02-28",
	}
	for n, w := range want {
		if got := dates.AddMonths(anchor, n).Format("2006-01-02"); got != w {
			t.Errorf("AddMonths(anchor, %d) = %s, want %s", n, got, w)
		}
	}
}

// TestAddMonthsReadsTheWallClockDayAndStampsMidnightUTC mirrors Day: the date
// is the one the caller's zone sees, and the result drops the time of day.
func TestAddMonthsReadsTheWallClockDayAndStampsMidnightUTC(t *testing.T) {
	east := time.Date(2026, 1, 31, 23, 30, 0, 0, time.FixedZone("UTC+9", 9*3600))
	if got, want := dates.AddMonths(east, 1), ymd(2026, 2, 28); got != want {
		t.Errorf("AddMonths = %v, want %v", got, want)
	}
}

// TestAddMonthsOnDayKeepsTheAnchorOfAClampedDate pins the re-anchoring a
// caller needs when it only holds a clamped date: Feb 28 anchored on the 31st
// steps to Mar 31, where AddMonths would stay on the 28th.
func TestAddMonthsOnDayKeepsTheAnchorOfAClampedDate(t *testing.T) {
	clamped := ymd(2026, 2, 28)
	for _, tc := range []struct {
		n    int
		want time.Time
	}{
		{0, ymd(2026, 2, 28)},
		{1, ymd(2026, 3, 31)},
		{2, ymd(2026, 4, 30)},
		{3, ymd(2026, 5, 31)},
		{12, ymd(2027, 2, 28)},
		{-1, ymd(2026, 1, 31)},
	} {
		if got := dates.AddMonthsOnDay(clamped, 31, tc.n); !got.Equal(tc.want) {
			t.Errorf("AddMonthsOnDay(Feb 28, 31, %d) = %s, want %s", tc.n, got.Format("2006-01-02"), tc.want.Format("2006-01-02"))
		}
	}
	if got := dates.AddMonths(clamped, 1); !got.Equal(ymd(2026, 3, 28)) {
		t.Errorf("AddMonths(Feb 28, 1) = %s, want 2026-03-28 (own day)", got.Format("2006-01-02"))
	}
}
