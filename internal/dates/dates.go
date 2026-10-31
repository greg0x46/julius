// Package dates holds the app's calendar-day convention. Every domain
// engine that reasons in days — timeline entries, scenario installments,
// credit-card due dates — needs the same truncation, and each had grown its
// own private copy of it; one shared definition keeps a day meaning the
// same thing across engines that otherwise must not depend on each other.
package dates

import "time"

// Day is the calendar date t falls on *as the caller's own zone sees it*,
// stamped midnight UTC. It does not convert t to UTC first: a timestamp at
// 23:30 in a +09:00 zone is the 10th, not the 9th.
//
// The midnight-UTC stamp is a normalization of the *result*, not of the
// input — it exists so two engines that arrived at the same date by
// different routes (one parsing a UTC date column, one computing in
// time.Local) produce the identical time.Time and can compare it with ==
// and Equal. Dates stored by this app are UTC-formatted (see internal/db),
// so for them the stamp is a no-op and Day is idempotent.
//
// The corollary is deliberate and worth knowing before passing a
// non-UTC time: two time.Time values for the same *instant* in different
// zones can yield different days, because they are different wall-clock
// dates. Callers that mean "the day this instant fell on in zone Z" must
// convert to Z themselves before calling — as the manual transaction handler
// does when it anchors on time.Local noon.
func Day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// DaysInMonth is how many days the given calendar month has — 28 or 29 for
// February, depending on the year.
//
// It takes no location on purpose. The answer is calendar arithmetic, not a
// point in time: March has 31 days in every zone, so the three copies this
// replaced (one keyed on time.Local, two on UTC) were always computing the
// same number by different routes. Doing it in UTC keeps the day-zero
// normalization below away from DST, where midnight in a shifting zone is
// either skipped or ambiguous.
//
// Callers use it to clamp a monthly cadence onto a shorter month: a card
// closing on the 31st falls on the 30th in November, not on December 1st.
func DaysInMonth(year int, month time.Month) int {
	// Day zero of the next month is the last day of this one.
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// AddMonths is day moved by n months (negative goes back) on the same day of
// the month, clamped to the last day of a shorter month: Jan 31 + 1 is Feb
// 28 (29 in a leap year), Jan 31 + 2 is Mar 31. Time.AddDate would normalize
// the overflow instead (Jan 31 + 1 → Mar 3).
//
// Every result is computed from day alone, never from a previous result, so
// stepping n = 0, 1, 2… keeps the original day instead of drifting to the 28th
// after the first short month. Like Day, the result is midnight UTC.
func AddMonths(day time.Time, n int) time.Time {
	return AddMonthsOnDay(day, day.Day(), n)
}

// AddMonthsOnDay is AddMonths landing on anchorDay rather than day's own day
// of the month, still clamped to the target month's length. A caller holding
// an already clamped date (Feb 28) uses it to keep stepping from the day the
// series was anchored on (31): n = 0 → Feb 28, n = 1 → Mar 31.
func AddMonthsOnDay(day time.Time, anchorDay, n int) time.Time {
	// time.Date normalizes the month, which also covers a negative n and a
	// December rollover; the day is fixed once the target month is known.
	first := time.Date(day.Year(), day.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	year, month := first.Year(), first.Month()
	return time.Date(year, month, min(anchorDay, DaysInMonth(year, month)), 0, 0, 0, 0, time.UTC)
}
