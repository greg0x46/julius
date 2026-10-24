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
