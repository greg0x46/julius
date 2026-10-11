package exchange

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

type fakeProvider struct {
	rates map[string]ProviderRate
	err   error
	calls int
}

func (p *fakeProvider) Name() string { return "fake" }

func (p *fakeProvider) Rate(_ context.Context, base, quote string, day time.Time) (ProviderRate, error) {
	p.calls++
	if p.err != nil {
		return ProviderRate{}, p.err
	}
	rate, ok := p.rates[base+"/"+quote+"/"+formatDate(day)]
	if !ok {
		return ProviderRate{}, errors.New("not found")
	}
	return rate, nil
}

func (p *fakeProvider) set(base, quote string, day time.Time, value string, effective time.Time) {
	if p.rates == nil {
		p.rates = map[string]ProviderRate{}
	}
	p.rates[base+"/"+quote+"/"+formatDate(day)] = ProviderRate{Value: decimal.RequireFromString(value), EffectiveOn: effective}
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// 2026-10-07 is a Wednesday; the 9th a Friday.
var (
	wed = day(2026, 10, 7)
	fri = day(2026, 10, 9)
	sat = day(2026, 10, 10)
	sun = day(2026, 10, 11)
)

func newService(conn *sql.DB, provider Provider, now time.Time) (*Service, *clock) {
	c := &clock{t: now}
	s := New(conn, provider)
	s.now = c.now
	return s, c
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestLatestPersistsRateWithProvenance(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("USD", "BRL", sun, "5.005", fri)
		p.set("EUR", "BRL", sun, "5.6086", fri)
		s, _ := newService(conn, p, sun.Add(13*time.Hour))

		usd, err := s.Latest(context.Background(), "usd", "BRL")
		if err != nil {
			t.Fatal(err)
		}
		if !usd.Value.Equal(dec("5.005")) || usd.Source != "fake" || !usd.EffectiveOn.Equal(fri) || !usd.AsOf.Equal(sun) || usd.Stale {
			t.Fatalf("USD/BRL = %+v", usd)
		}
		if got := usd.Convert(dec("100")); !got.Equal(dec("500.5")) {
			t.Fatalf("100 USD = %s BRL", got)
		}
		eur, err := s.Latest(context.Background(), "EUR", "BRL")
		if err != nil {
			t.Fatal(err)
		}
		if got := eur.Convert(dec("10.00")); !got.Equal(dec("56.086")) {
			t.Fatalf("10 EUR = %s BRL", got)
		}

		var rate, source, effective string
		if err := conn.QueryRow(`SELECT rate, source, effective_on FROM exchange_rates
			WHERE base_currency = 'USD' AND quote_currency = 'BRL' AND as_of = '2026-10-11'`).
			Scan(&rate, &source, &effective); err != nil {
			t.Fatal(err)
		}
		if rate != "5.005" || source != "fake" || effective != "2026-10-09" {
			t.Fatalf("stored %s %s %s", rate, source, effective)
		}
	})
}

func TestHistoricalUsesThatDaysRate(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("USD", "BRL", sun, "5.005", fri)
		p.set("USD", "BRL", wed, "5.40", wed)
		s, _ := newService(conn, p, sun.Add(13*time.Hour))

		if _, err := s.Latest(context.Background(), "USD", "BRL"); err != nil {
			t.Fatal(err)
		}
		past, err := s.On(context.Background(), "USD", "BRL", wed.Add(15*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if !past.Value.Equal(dec("5.40")) || !past.EffectiveOn.Equal(wed) || past.Stale {
			t.Fatalf("historical = %+v", past)
		}
	})
}

func TestWeekendFallsBackToPreviousBusinessDay(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("USD", "BRL", sat, "5.005", fri)
		s, _ := newService(conn, p, sun.Add(13*time.Hour))

		rate, err := s.On(context.Background(), "USD", "BRL", sat)
		if err != nil {
			t.Fatal(err)
		}
		if !rate.AsOf.Equal(sat) || !rate.EffectiveOn.Equal(fri) || rate.Stale {
			t.Fatalf("saturday = %+v", rate)
		}
	})
}

func TestProviderRateDatedAfterDayIsRejected(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		// A provider answering a pre-coverage day with its first rate.
		p.set("USD", "BRL", wed, "5.40", fri)
		s, _ := newService(conn, p, sun)

		if _, err := s.On(context.Background(), "USD", "BRL", wed); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}

func TestFutureDayIsUnavailable(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		s, _ := newService(conn, p, sun)
		if _, err := s.On(context.Background(), "USD", "BRL", sun.AddDate(0, 0, 1)); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if p.calls != 0 {
			t.Fatalf("provider calls = %d", p.calls)
		}
	})
}

func TestCacheReuse(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("USD", "BRL", sun, "5.005", fri)
		p.set("USD", "BRL", wed, "5.40", wed)
		s, c := newService(conn, p, sun.Add(13*time.Hour))
		ctx := context.Background()

		for range 3 {
			if _, err := s.Latest(ctx, "USD", "BRL"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.On(ctx, "USD", "BRL", wed); err != nil {
				t.Fatal(err)
			}
		}
		if p.calls != 2 {
			t.Fatalf("provider calls = %d, want 2", p.calls)
		}

		// Today's row may still change: refreshed past the TTL. Wednesday's
		// was fetched after Wednesday ended, so it is final.
		c.t = c.t.Add(s.TTL)
		if _, err := s.Latest(ctx, "USD", "BRL"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.On(ctx, "USD", "BRL", wed); err != nil {
			t.Fatal(err)
		}
		if p.calls != 3 {
			t.Fatalf("provider calls = %d, want 3", p.calls)
		}

		// A fresh service over the same database reuses the persisted rows.
		other, _ := newService(conn, p, c.t)
		if _, err := other.On(ctx, "USD", "BRL", wed); err != nil {
			t.Fatal(err)
		}
		if p.calls != 3 {
			t.Fatalf("provider calls = %d, want 3", p.calls)
		}
	})
}

func TestSameCurrencyIsIdentityWithoutProvider(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		s, _ := newService(conn, p, sun)
		rate, err := s.On(context.Background(), "BRL", "brl", wed)
		if err != nil {
			t.Fatal(err)
		}
		if got := rate.Convert(dec("123.4567")); !got.Equal(dec("123.4567")) || rate.Source != SourceIdentity {
			t.Fatalf("identity = %+v, converted %s", rate, got)
		}
		if p.calls != 0 {
			t.Fatalf("provider calls = %d", p.calls)
		}
	})
}

func TestInverseRateReusesStoredPair(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("USD", "BRL", wed, "5.005", wed)
		s, _ := newService(conn, p, sun)
		ctx := context.Background()

		if _, err := s.On(ctx, "USD", "BRL", wed); err != nil {
			t.Fatal(err)
		}
		inverse, err := s.On(ctx, "BRL", "USD", wed)
		if err != nil {
			t.Fatal(err)
		}
		if p.calls != 1 {
			t.Fatalf("provider calls = %d, want 1", p.calls)
		}
		if inverse.Base != "BRL" || inverse.Quote != "USD" || !inverse.Inverted || !inverse.Value.Equal(dec("5.005")) {
			t.Fatalf("inverse = %+v", inverse)
		}
		if got := inverse.Convert(dec("500.5")); !got.Equal(dec("100")) {
			t.Fatalf("500.5 BRL = %s USD", got)
		}
		if got := inverse.Convert(dec("1")); !got.Equal(dec("0.199800199800")) {
			t.Fatalf("1 BRL = %s USD", got)
		}
	})
}

func TestPrecisionSurvivesPersistence(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("USD", "BRL", wed, "5.12345678901234567890", wed)
		s, _ := newService(conn, p, sun)
		ctx := context.Background()

		if _, err := s.On(ctx, "USD", "BRL", wed); err != nil {
			t.Fatal(err)
		}
		other, _ := newService(conn, p, sun)
		rate, err := other.On(ctx, "USD", "BRL", wed)
		if err != nil {
			t.Fatal(err)
		}
		if p.calls != 1 {
			t.Fatalf("provider calls = %d, want 1", p.calls)
		}
		if got := rate.Convert(dec("0.10")); !got.Equal(dec("0.512345678901234567890")) {
			t.Fatalf("0.10 USD = %s BRL", got)
		}
	})
}

func TestProviderErrorWithoutStoredRateIsUnavailable(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{err: errors.New("boom")}
		s, c := newService(conn, p, sun)
		ctx := context.Background()

		if _, err := s.Latest(ctx, "USD", "BRL"); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		// Within the cooldown the provider is not asked again.
		if _, err := s.Latest(ctx, "USD", "BRL"); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if p.calls != 1 {
			t.Fatalf("provider calls = %d, want 1", p.calls)
		}
		c.t = c.t.Add(s.Cooldown)
		s.Latest(ctx, "USD", "BRL")
		if p.calls != 2 {
			t.Fatalf("provider calls = %d, want 2", p.calls)
		}
	})
}

func TestProviderErrorFallsBackToStaleStoredRate(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("BRL", "USD", wed, "0.2", wed)
		s, c := newService(conn, p, wed.Add(20*time.Hour))
		ctx := context.Background()
		if _, err := s.Latest(ctx, "BRL", "USD"); err != nil {
			t.Fatal(err)
		}

		c.t = sun.Add(13 * time.Hour)
		p.err = errors.New("boom")
		rate, err := s.Latest(ctx, "USD", "BRL")
		if err != nil {
			t.Fatal(err)
		}
		if !rate.Stale || !rate.AsOf.Equal(sun) || !rate.EffectiveOn.Equal(wed) || !rate.Inverted {
			t.Fatalf("fallback = %+v", rate)
		}
		if got := rate.Convert(dec("1")); !got.Equal(dec("5")) {
			t.Fatalf("1 USD = %s BRL", got)
		}

		// Never a rate from after the asked day.
		if _, err := s.On(ctx, "USD", "BRL", wed.AddDate(0, 0, -1)); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}

func TestOldEffectiveRateIsStale(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("USD", "BRL", sun, "5.40", sun.AddDate(0, 0, -8))
		s, _ := newService(conn, p, sun)
		rate, err := s.Latest(context.Background(), "USD", "BRL")
		if err != nil {
			t.Fatal(err)
		}
		if !rate.Stale {
			t.Fatalf("rate = %+v, want stale", rate)
		}
	})
}

func TestInvalidCurrency(t *testing.T) {
	s, _ := newService(nil, &fakeProvider{}, sun)
	for _, pair := range [][2]string{{"US", "BRL"}, {"USD", "BR1"}, {"", "BRL"}} {
		if _, err := s.Latest(context.Background(), pair[0], pair[1]); !errors.Is(err, ErrInvalidCurrency) {
			t.Fatalf("%v: err = %v", pair, err)
		}
	}
}

func TestHolidayAndTransactionTimezone(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		holiday := day(2026, 4, 6) // Easter Monday.
		previous := day(2026, 4, 2)
		p := &fakeProvider{}
		p.set("EUR", "BRL", holiday, "6.25", previous)
		s, _ := newService(conn, p, sun)
		transaction := time.Date(2026, 4, 6, 23, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
		rate, err := s.On(context.Background(), "EUR", "BRL", transaction)
		if err != nil {
			t.Fatal(err)
		}
		if !rate.AsOf.Equal(holiday) || !rate.EffectiveOn.Equal(previous) || rate.Stale {
			t.Fatalf("holiday rate = %+v", rate)
		}
		if _, err := s.On(context.Background(), "EUR", "BRL", transaction); err != nil {
			t.Fatal(err)
		}
		if p.calls != 1 {
			t.Fatalf("provider calls = %d", p.calls)
		}
	})
}

func TestExpiredCacheFailureAndRecovery(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{}
		p.set("USD", "BRL", sun, "5", fri)
		s, c := newService(conn, p, sun.Add(12*time.Hour))
		ctx := context.Background()
		if _, err := s.Latest(ctx, "USD", "BRL"); err != nil {
			t.Fatal(err)
		}
		c.t = c.t.Add(s.TTL)
		p.err = errors.New("offline")
		for range 2 {
			rate, err := s.Latest(ctx, "USD", "BRL")
			if err != nil || !rate.Stale || !rate.Value.Equal(dec("5")) {
				t.Fatalf("stale cache = %+v, %v", rate, err)
			}
		}
		if p.calls != 2 {
			t.Fatalf("provider calls = %d", p.calls)
		}
		c.t = c.t.Add(s.Cooldown)
		p.err = nil
		p.set("USD", "BRL", sun, "5.1", fri)
		rate, err := s.Latest(ctx, "USD", "BRL")
		if err != nil || rate.Stale || !rate.Value.Equal(dec("5.1")) {
			t.Fatalf("recovered rate = %+v, %v", rate, err)
		}
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM exchange_rates`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("upsert count = %d, %v", count, err)
		}
	})
}

func TestNonPositiveProviderRatesAreUnavailable(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		for _, value := range []string{"0", "-1"} {
			p := &fakeProvider{}
			p.set("USD", "BRL", wed, value, wed)
			s, _ := newService(conn, p, sun)
			if _, err := s.On(context.Background(), "USD", "BRL", wed); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("rate %s: %v", value, err)
			}
		}
	})
}

func TestFreshInverseCacheWinsOverExpiredDirectCache(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		p := &fakeProvider{err: errors.New("offline")}
		s, c := newService(conn, p, sun.Add(13*time.Hour))
		ctx := context.Background()
		for _, rate := range []Rate{
			{Base: "USD", Quote: "BRL", Value: dec("5"), AsOf: sun, EffectiveOn: fri, Source: "old", FetchedAt: c.t.Add(-2 * s.TTL)},
			{Base: "BRL", Quote: "USD", Value: dec("0.25"), AsOf: sun, EffectiveOn: fri, Source: "new", FetchedAt: c.t},
		} {
			if err := s.save(ctx, rate); err != nil {
				t.Fatal(err)
			}
		}
		rate, err := s.Latest(ctx, "USD", "BRL")
		if err != nil || rate.Stale || !rate.Inverted || rate.Source != "new" || !rate.Convert(dec("10")).Equal(dec("40")) {
			t.Fatalf("cached rate = %+v, %v", rate, err)
		}
		if p.calls != 0 {
			t.Fatalf("provider calls = %d", p.calls)
		}
		fallback, err := s.latestStored(ctx, "USD", "BRL", sun)
		if err != nil || fallback == nil || fallback.Source != "new" {
			t.Fatalf("latest stored = %+v, %v", fallback, err)
		}
		if err := s.save(ctx, Rate{Base: "BRL", Quote: "USD", Value: dec("0.25"), AsOf: sun,
			EffectiveOn: wed, Source: "older-effective", FetchedAt: c.t.Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
		cached, err := s.cached(ctx, "USD", "BRL", sun)
		if err != nil || cached == nil || cached.Source != "old" {
			t.Fatalf("newer effective date should win: %+v, %v", cached, err)
		}
	})
}
