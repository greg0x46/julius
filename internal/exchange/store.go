package exchange

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

const dateLayout = "2006-01-02"

func formatDate(t time.Time) string { return t.Format(dateLayout) }

const rateSelect = `SELECT base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at FROM exchange_rates`

func (s *Service) save(ctx context.Context, rate Rate) error {
	_, err := s.conn.ExecContext(ctx, `
		INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (base_currency, quote_currency, as_of) DO UPDATE SET
			effective_on = excluded.effective_on, rate = excluded.rate,
			source = excluded.source, fetched_at = excluded.fetched_at`,
		rate.Base, rate.Quote, formatDate(rate.AsOf), formatDate(rate.EffectiveOn),
		money.CanonicalDecimal(rate.Value), rate.Source, db.FormatTime(rate.FetchedAt))
	return err
}

// cached is the row stored for the pair and day, in either orientation;
// nil when there is none.
func (s *Service) cached(ctx context.Context, base, quote string, day time.Time) (*Rate, error) {
	var best *Rate
	for _, p := range orientations(base, quote) {
		rate, err := s.queryRate(ctx, p.inverted,
			rateSelect+` WHERE base_currency = ? AND quote_currency = ? AND as_of = ?`,
			p.base, p.quote, formatDate(day))
		if err != nil {
			return nil, err
		}
		if rate != nil && (best == nil || rate.EffectiveOn.After(best.EffectiveOn) ||
			rate.EffectiveOn.Equal(best.EffectiveOn) && rate.FetchedAt.After(best.FetchedAt)) {
			best = rate
		}
	}
	return best, nil
}

// latestStored is the most recent rate in effect on or before day, in either
// orientation; nil when there is none.
func (s *Service) latestStored(ctx context.Context, base, quote string, day time.Time) (*Rate, error) {
	var best *Rate
	for _, p := range orientations(base, quote) {
		rate, err := s.queryRate(ctx, p.inverted,
			rateSelect+` WHERE base_currency = ? AND quote_currency = ? AND effective_on <= ?
				ORDER BY effective_on DESC, fetched_at DESC LIMIT 1`,
			p.base, p.quote, formatDate(day))
		if err != nil {
			return nil, err
		}
		if rate != nil && (best == nil || rate.EffectiveOn.After(best.EffectiveOn) ||
			rate.EffectiveOn.Equal(best.EffectiveOn) && rate.FetchedAt.After(best.FetchedAt)) {
			best = rate
		}
	}
	return best, nil
}

type orientation struct {
	base, quote string
	inverted    bool
}

func orientations(base, quote string) []orientation {
	return []orientation{{base, quote, false}, {quote, base, true}}
}

func (s *Service) queryRate(ctx context.Context, inverted bool, query string, args ...any) (*Rate, error) {
	var (
		rate                              Rate
		asOf, effectiveOn, value, fetched string
	)
	err := s.conn.QueryRowContext(ctx, query, args...).
		Scan(&rate.Base, &rate.Quote, dbtypes.Date(&asOf), dbtypes.Date(&effectiveOn), dbtypes.Decimal(&value), &rate.Source, dbtypes.Time(&fetched))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if rate.AsOf, err = dbtypes.ParseDate(asOf); err != nil {
		return nil, err
	}
	if rate.EffectiveOn, err = dbtypes.ParseDate(effectiveOn); err != nil {
		return nil, err
	}
	if rate.Value, err = decimal.NewFromString(value); err != nil {
		return nil, err
	}
	if rate.FetchedAt, err = dbtypes.ParseTime(fetched); err != nil {
		return nil, err
	}
	if inverted {
		rate.Base, rate.Quote, rate.Inverted = rate.Quote, rate.Base, true
	}
	return &rate, nil
}
