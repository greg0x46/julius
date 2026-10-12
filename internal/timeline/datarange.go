package timeline

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/db"
)

const dateOnlyLayout = "2006-01-02"

// DataRange is the widest window worth plotting: everything on record on one
// side, everything already scheduled on the other.
type DataRange struct {
	From, To time.Time
}

// DataRange answers "how far does the data actually reach", so a caller can
// ask for the whole history without inventing a start date.
//
// From is the day of the oldest transaction of any origin — unlike
// networth's earliestCashTransactionDay, lançamentos manuais count here,
// since the question is what the balance curve can show, not how far back
// the sync provider's coverage reaches.
//
// To cannot mean "the last projected event": recurring schedules generate
// occurrences for whatever range they are asked about, so their future is
// unbounded by construction. What does end is a planned installment — a
// debt's or a plan's — so To is the last of those, and the current month's
// end when there is none, which is also the floor: a plan that finished last
// year must not shrink the window to the past.
func LoadDataRange(ctx context.Context, q Querier, today time.Time, timezone string) (DataRange, error) {
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return DataRange{}, fmt.Errorf("data range timezone: %w", err)
	}
	day := dates.Day(today.In(loc))
	monthEnd := time.Date(day.Year(), day.Month(), dates.DaysInMonth(day.Year(), day.Month()), 0, 0, 0, 0, time.UTC)

	from := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	var earliest sql.NullString
	if err := q.QueryRowContext(ctx, `
		SELECT MIN(occurred_at) FROM financial_transactions WHERE occurred_at IS NOT NULL`).Scan(&earliest); err != nil {
		return DataRange{}, fmt.Errorf("earliest transaction: %w", err)
	}
	if earliest.Valid {
		parsed, err := db.ParseTime(earliest.String)
		if err != nil {
			return DataRange{}, fmt.Errorf("parse earliest transaction %q: %w", earliest.String, err)
		}
		from = dates.Day(parsed.In(loc))
	}

	to := monthEnd
	var latest sql.NullString
	if err := q.QueryRowContext(ctx, `
		SELECT MAX(projected_at) FROM scenario_transactions`).Scan(&latest); err != nil {
		return DataRange{}, fmt.Errorf("latest planned installment: %w", err)
	}
	if latest.Valid {
		// projected_at is a date-only column (see scenarios.dateLayout), not
		// the timestamp format db.ParseTime reads.
		parsed, err := time.Parse(dateOnlyLayout, latest.String)
		if err != nil {
			return DataRange{}, fmt.Errorf("parse latest planned installment %q: %w", latest.String, err)
		}
		if planned := dates.Day(parsed); planned.After(to) {
			to = planned
		}
	}

	if from.After(to) {
		from = to
	}
	return DataRange{From: from, To: to}, nil
}
