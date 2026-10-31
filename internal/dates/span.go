package dates

import "time"

// MaxSpanDays is the longest window (100 years) a derived daily series or a
// date-range read may cover. Series cost grows with the number of days, so an
// absurd stored or requested date must not be able to ask for millennia.
const MaxSpanDays = 36525

// ExceedsMaxSpan reports whether to lies more than MaxSpanDays after from.
// Both are expected to be calendar days; a reversed range never exceeds.
func ExceedsMaxSpan(from, to time.Time) bool {
	return to.After(from.AddDate(0, 0, MaxSpanDays))
}

// ClampSpanStart returns from, moved forward when needed so [from, to] is at
// most MaxSpanDays long. The stored data is never altered; only the window.
func ClampSpanStart(from, to time.Time) time.Time {
	if floor := to.AddDate(0, 0, -MaxSpanDays); from.Before(floor) {
		return floor
	}
	return from
}
