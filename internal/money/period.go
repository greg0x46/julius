package money

import (
	"fmt"
	"time"

	"github.com/greg0x46/julius/internal/dates"
)

// Date is a timezone-less calendar date, used for period boundaries where a
// full time.Time (with its timezone baggage) would invite bugs comparing
// dates computed in different locations.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func dateFromTime(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

func (d Date) asTime() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

func (d Date) addDays(days int) Date {
	return dateFromTime(d.asTime().AddDate(0, 0, days))
}

func (d Date) firstOfMonth() Date {
	return Date{Year: d.Year, Month: d.Month, Day: 1}
}

// weekday reports the ISO weekday with Monday = 0, matching Python's
// date.weekday(), which Go's time.Weekday (Sunday = 0) does not.
func (d Date) weekday() int {
	return (int(d.asTime().Weekday()) + 6) % 7
}

// PeriodFor mirrors rules.period_for: it buckets occurredAt (nil meaning the
// transaction has no date) into the Period that GroupBy selects, computed in
// the caller's timezone since the same instant can fall in different local
// days/weeks/months depending on it.
func PeriodFor(occurredAt *time.Time, groupBy GroupBy, timezone string) (Period, error) {
	if groupBy == GroupNone {
		return Period{Key: "all", Kind: PeriodNone}, nil
	}
	if occurredAt == nil {
		return Period{Key: "undated", Kind: PeriodUndated}, nil
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Period{}, fmt.Errorf("load timezone %q: %w", timezone, err)
	}
	localDate := dateFromTime(occurredAt.In(loc))

	switch groupBy {
	case GroupDay:
		return Period{
			Key:       "day:" + localDate.String(),
			Kind:      PeriodDay,
			StartDate: &localDate,
			EndDate:   &localDate,
		}, nil
	case GroupWeek:
		start := localDate.addDays(-localDate.weekday())
		end := start.addDays(6)
		return Period{
			Key:       "week:" + start.String(),
			Kind:      PeriodWeek,
			StartDate: &start,
			EndDate:   &end,
		}, nil
	case GroupMonth:
		start := localDate.firstOfMonth()
		nextMonth := start.addDays(dates.DaysInMonth(start.Year, start.Month))
		end := nextMonth.addDays(-1)
		return Period{
			Key:       fmt.Sprintf("month:%04d-%02d", start.Year, int(start.Month)),
			Kind:      PeriodMonth,
			StartDate: &start,
			EndDate:   &end,
		}, nil
	case GroupYear:
		start := Date{Year: localDate.Year, Month: time.January, Day: 1}
		end := Date{Year: localDate.Year, Month: time.December, Day: 31}
		return Period{
			Key:       fmt.Sprintf("year:%04d", localDate.Year),
			Kind:      PeriodYear,
			StartDate: &start,
			EndDate:   &end,
		}, nil
	default:
		return Period{}, fmt.Errorf("unknown group_by %q", groupBy)
	}
}
