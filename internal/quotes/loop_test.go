package quotes

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/shopspring/decimal"
)

func TestInitializeSchedulePersistenceAndLegacy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restart.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	config, err := InitializeSchedule(ctx, conn, "")
	if err != nil || config != settings.DefaultQuoteRefreshSettings() {
		t.Fatalf("default: %+v %v", config, err)
	}
	config = settings.QuoteRefreshSettings{Enabled: false, Time: "09:30", Timezone: "Asia/Tokyo"}
	if err := settings.SetQuoteRefresh(ctx, conn, config); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	conn, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	got, err := InitializeSchedule(ctx, conn, "malformed ignored environment")
	if err != nil || got != config {
		t.Fatalf("restart: %+v %v", got, err)
	}
	for _, legacy := range []string{"07:25", "07:25 UTC", "invalid"} {
		conn := newJobTestConn(t)
		got, err := InitializeSchedule(ctx, conn, legacy)
		if legacy == "invalid" {
			if err == nil {
				t.Fatal("invalid legacy accepted")
			}
			continue
		}
		zone := "Local"
		if legacy == "07:25 UTC" {
			zone = "UTC"
		}
		if err != nil || !got.Enabled || got.Time != "07:25" || got.Timezone != zone {
			t.Fatalf("legacy %s: %+v %v", legacy, got, err)
		}
	}
}

func TestScheduleLiveTransitionsAndRestartDedupe(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "quotes-restart.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { conn.Close() }()
	asset := mustCreateQuotedAsset(t, ctx, conn, "Bitcoin", "crypto", "crypto", "BTC")
	mustCreateManualPosition(t, ctx, conn, "wallet", asset, 2)
	fake := &fakeConnector{prices: map[string]decimal.Decimal{"BTC": decimal.NewFromInt(100)}}
	history := &fakeHistoryConnector{spot: fake.prices}
	// Use one connector that counts both stages.
	provider := &loopConnector{fakeConnector: fake, history: history}
	service := marketdata.New(provider)
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	config := settings.DefaultQuoteRefreshSettings()
	config.Enabled = false
	save := func() {
		t.Helper()
		if err := settings.SetQuoteRefresh(ctx, conn, config); err != nil {
			t.Fatal(err)
		}
	}
	save()
	state := scheduleState{}
	cycle := func(at time.Time, backfill bool) {
		t.Helper()
		if err := state.cycle(ctx, conn, service, at, backfill); err != nil {
			t.Fatal(err)
		}
	}
	cycle(now, true)
	if len(fake.asked) != 0 || len(history.calls) != 0 {
		t.Fatal("disabled made provider requests")
	}
	config.Enabled = true
	save()
	cycle(now, false)
	if len(fake.asked) != 1 || len(history.calls) != 1 {
		t.Fatalf("enable: spot=%d history=%d", len(fake.asked), len(history.calls))
	}
	config.Time = "20:00"
	config.Timezone = "UTC"
	save()
	cycle(now, false)
	if state.nextDaily.Hour() != 20 || len(fake.asked) != 1 {
		t.Fatal("reschedule forced work or wrong slot")
	}
	// A fresh service must dedupe using stored spot quotes and coverage.
	conn.Close()
	conn, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service = marketdata.New(provider)
	restart := scheduleState{}
	if err := restart.cycle(ctx, conn, service, now, false); err != nil {
		t.Fatal(err)
	}
	if len(fake.asked) != 1 || len(history.calls) != 1 {
		t.Fatal("restart duplicated provider work")
	}
	cycle(time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC), false)
	if len(fake.asked) != 2 {
		t.Fatalf("daily did not refresh: %d", len(fake.asked))
	}
	config.Enabled = false
	save()
	cycle(now.AddDate(0, 0, 1), true)
	if len(fake.asked) != 2 {
		t.Fatal("disable failed")
	}
	config.Enabled = true
	save()
	service = marketdata.New(provider)
	cycle(now.AddDate(0, 0, 1), false)
	if len(fake.asked) != 3 || len(history.calls) != 2 {
		t.Fatal("re-enable missed catch-up")
	}
}

type loopConnector struct {
	*fakeConnector
	history   *fakeHistoryConnector
	afterSpot func()
}

func (f *loopConnector) Quote(ctx context.Context, instrument marketdata.Instrument) (marketdata.Quote, error) {
	quote, err := f.fakeConnector.Quote(ctx, instrument)
	if f.afterSpot != nil {
		f.afterSpot()
	}
	return quote, err
}
func (f *loopConnector) History(ctx context.Context, instrument marketdata.Instrument, from, to time.Time) (marketdata.History, error) {
	return f.history.History(ctx, instrument, from, to)
}

func TestScheduleDisableBetweenStagesAndFailClosed(t *testing.T) {
	ctx := context.Background()
	conn := newJobTestConn(t)
	asset := mustCreateQuotedAsset(t, ctx, conn, "Bitcoin", "crypto", "crypto", "BTC")
	mustCreateManualPosition(t, ctx, conn, "wallet", asset, 1)
	config := settings.DefaultQuoteRefreshSettings()
	if err := settings.SetQuoteRefresh(ctx, conn, config); err != nil {
		t.Fatal(err)
	}
	history := &fakeHistoryConnector{}
	provider := &loopConnector{fakeConnector: &fakeConnector{prices: map[string]decimal.Decimal{"BTC": decimal.NewFromInt(100)}}, history: history}
	provider.afterSpot = func() {
		config.Enabled = false
		if err := settings.SetQuoteRefresh(ctx, conn, config); err != nil {
			t.Fatal(err)
		}
	}
	state := scheduleState{}
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	if err := state.cycle(ctx, conn, marketdata.New(provider), now, false); err != nil {
		t.Fatal(err)
	}
	if len(history.calls) != 0 {
		t.Fatal("history continued after disable")
	}
	if err := settings.Set(ctx, conn, settings.KeyQuoteRefresh, "broken", false, nil); err != nil {
		t.Fatal(err)
	}
	if err := state.cycle(ctx, conn, marketdata.New(provider), now, true); err == nil {
		t.Fatal("invalid config did not fail closed")
	}
	if len(provider.asked) != 1 {
		t.Fatal("invalid config requested prices")
	}
}

func TestScheduleNoEligibleHoldingsAndWakeCancellation(t *testing.T) {
	conn := newJobTestConn(t)
	provider := &loopConnector{fakeConnector: &fakeConnector{}, history: &fakeHistoryConnector{}}
	state := scheduleState{}
	now := time.Now()
	if err := state.cycle(context.Background(), conn, marketdata.New(provider), now, false); err != nil {
		t.Fatal(err)
	}
	mustCreateQuotedAsset(t, context.Background(), conn, "Bitcoin", "crypto", "crypto", "BTC")
	if err := state.cycle(context.Background(), conn, marketdata.New(provider), now, true); err != nil {
		t.Fatal(err)
	}
	if len(provider.asked) != 0 || len(provider.history.calls) != 0 {
		t.Fatal("unheld asset requested prices")
	}
	for {
		select {
		case <-configurationChanges:
			continue
		default:
		}
		break
	}
	ConfigurationChanged()
	if backfill, running := waitForWork(context.Background(), time.Hour); backfill || !running {
		t.Fatal("configuration wake treated as backfill")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, running := waitForWork(ctx, time.Hour); running {
		t.Fatal("cancelled wait continued")
	}
}
