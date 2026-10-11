// Package exchange resolves persisted decimal rates without changing original values.
// Historical lookups use the last business day on or before the requested day.
// Rows fetched after that day ended (UTC) are final; others refresh after TTL.
// Provider failure returns a stored rate marked stale, or ErrUnavailable.
// Rates more than MaxLag before the requested day are also marked stale.
package exchange

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
)

// ErrUnavailable means no rate could be resolved for the pair and day.
var ErrUnavailable = errors.New("exchange rate unavailable")

// ErrInvalidCurrency means a currency is not a three-letter ISO 4217 code.
var ErrInvalidCurrency = errors.New("invalid currency code")

// SourceIdentity is the source of a same-currency rate.
const SourceIdentity = "identity"

// DivisionPrecision is how many decimal places an inverse conversion keeps.
// A direct conversion is an exact multiplication.
const DivisionPrecision = 12

// Rate converts Base into Quote for the day AsOf.
type Rate struct {
	Base  string
	Quote string
	// Value is the persisted figure. Normally 1 Base = Value Quote; when
	// Inverted, the row was stored the other way round (1 Quote = Value
	// Base), and Convert divides instead of rounding Value into a reciprocal.
	Value    decimal.Decimal
	Inverted bool
	// AsOf is the day asked for; EffectiveOn is the day the rate refers to.
	AsOf        time.Time
	EffectiveOn time.Time
	Source      string
	FetchedAt   time.Time
	// Stale is set when the rate could not be refreshed or is older than
	// MaxLag. Consumers should disclose it rather than hide it.
	Stale bool
}

// Convert turns amount in Base into Quote.
func (r Rate) Convert(amount decimal.Decimal) decimal.Decimal {
	if r.Inverted {
		return amount.DivRound(r.Value, DivisionPrecision)
	}
	return amount.Mul(r.Value)
}

// ProviderRate is a provider's answer: 1 base = Value quote on EffectiveOn.
type ProviderRate struct {
	Value       decimal.Decimal
	EffectiveOn time.Time
}

// Provider fetches rates from an external source.
type Provider interface {
	Name() string
	// Rate is the rate of the last business day on or before day.
	Rate(ctx context.Context, base, quote string, day time.Time) (ProviderRate, error)
}

type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Service resolves rates through the cache in exchange_rates and the provider.
type Service struct {
	conn     Querier
	provider Provider
	now      func() time.Time

	// TTL is how long a row that may still change is reused.
	TTL time.Duration
	// MaxLag is how far before the asked day a rate may be and still count
	// as current; it covers weekends and long holidays.
	MaxLag time.Duration
	// Cooldown is how long the provider is left alone after a failure.
	Cooldown time.Duration

	// mu serializes provider calls, so concurrent reads of the same missing
	// rate cost one request.
	mu       sync.Mutex
	failedAt time.Time
}

// New builds a Service. A nil provider uses Frankfurter.
func New(conn Querier, provider Provider) *Service {
	if provider == nil {
		provider = NewFrankfurter()
	}
	return &Service{
		conn:     conn,
		provider: provider,
		now:      time.Now,
		TTL:      time.Hour,
		MaxLag:   7 * 24 * time.Hour,
		Cooldown: time.Minute,
	}
}

// Latest is the current rate from base to quote.
func (s *Service) Latest(ctx context.Context, base, quote string) (Rate, error) {
	return s.resolve(ctx, base, quote, s.today())
}

// On is the rate from base to quote for the calendar day of t (as t's zone
// sees it). A future day has no rate yet.
func (s *Service) On(ctx context.Context, base, quote string, t time.Time) (Rate, error) {
	day := dates.Day(t)
	if day.After(s.today()) {
		return Rate{}, fmt.Errorf("%w: %s is in the future", ErrUnavailable, formatDate(day))
	}
	return s.resolve(ctx, base, quote, day)
}

func (s *Service) today() time.Time { return dates.Day(s.now().UTC()) }

func (s *Service) resolve(ctx context.Context, base, quote string, day time.Time) (Rate, error) {
	base, quote = strings.ToUpper(base), strings.ToUpper(quote)
	if !validCurrency(base) || !validCurrency(quote) {
		return Rate{}, fmt.Errorf("%w: %q/%q", ErrInvalidCurrency, base, quote)
	}
	if base == quote {
		return Rate{Base: base, Quote: quote, Value: decimal.NewFromInt(1), AsOf: day, EffectiveOn: day,
			Source: SourceIdentity, FetchedAt: s.now()}, nil
	}

	if cached, err := s.cached(ctx, base, quote, day); err != nil || cached != nil && s.fresh(*cached) {
		return s.finish(cached, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Another caller may have refreshed it while this one waited.
	cached, err := s.cached(ctx, base, quote, day)
	if err != nil || cached != nil && s.fresh(*cached) {
		return s.finish(cached, err)
	}

	var fetchErr error
	if !s.failedAt.IsZero() && s.now().Sub(s.failedAt) < s.Cooldown {
		fetchErr = errors.New("provider cooling down after a failure")
	} else {
		var rate Rate
		rate, fetchErr = s.fetch(ctx, base, quote, day)
		if fetchErr == nil {
			s.failedAt = time.Time{}
			return s.finish(&rate, nil)
		}
		if ctx.Err() == nil {
			s.failedAt = s.now()
		}
	}

	fallback, err := s.latestStored(ctx, base, quote, day)
	if err != nil {
		return Rate{}, err
	}
	if fallback == nil {
		return Rate{}, fmt.Errorf("%w: %s/%s on %s: %w", ErrUnavailable, base, quote, formatDate(day), fetchErr)
	}
	fallback.AsOf = day
	fallback.Stale = true
	return *fallback, nil
}

func (s *Service) fetch(ctx context.Context, base, quote string, day time.Time) (Rate, error) {
	got, err := s.provider.Rate(ctx, base, quote, day)
	if err != nil {
		return Rate{}, err
	}
	effective := dates.Day(got.EffectiveOn)
	if !got.Value.IsPositive() {
		return Rate{}, fmt.Errorf("%s: non-positive rate %s", s.provider.Name(), got.Value)
	}
	if effective.After(day) {
		return Rate{}, fmt.Errorf("%s: rate for %s is dated after %s", s.provider.Name(), formatDate(effective), formatDate(day))
	}
	rate := Rate{Base: base, Quote: quote, Value: got.Value, AsOf: day, EffectiveOn: effective,
		Source: s.provider.Name(), FetchedAt: s.now()}
	if err := s.save(ctx, rate); err != nil {
		return Rate{}, err
	}
	return rate, nil
}

func (s *Service) finish(rate *Rate, err error) (Rate, error) {
	if err != nil {
		return Rate{}, err
	}
	if rate.AsOf.Sub(rate.EffectiveOn) > s.MaxLag {
		rate.Stale = true
	}
	return *rate, nil
}

// fresh reports whether a cached row can be used without asking the provider.
func (s *Service) fresh(rate Rate) bool {
	if !rate.FetchedAt.Before(rate.AsOf.AddDate(0, 0, 1)) {
		return true
	}
	return s.now().Sub(rate.FetchedAt) < s.TTL
}

func validCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}
