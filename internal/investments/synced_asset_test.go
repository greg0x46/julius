package investments_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/investments"
)

func (f *ledgerFixture) assetCount() int {
	f.t.Helper()
	var n int
	if err := f.conn.QueryRow(`SELECT COUNT(*) FROM investment_assets`).Scan(&n); err != nil {
		f.t.Fatalf("count assets: %v", err)
	}
	return n
}

func TestFindSyncedAssetFindsWhatResolveSyncedAssetCreatedWithoutCreatingAnything(t *testing.T) {
	f := newLedgerFixture(t)
	padded := equity("10")
	padded.Name, padded.Code = strRef("  Bova ETF "), strRef(" bova-11 ")
	for name, holding := range map[string]investments.SyncedHolding{
		"coded equity":          equity("10"),
		"padded name and code":  padded,
		"fixed income by terms": cdb("2026-09-13T03:00:00Z"),
		"fixed income by code":  lca(),
	} {
		t.Run(name, func(t *testing.T) {
			before := f.assetCount()
			if _, found, err := investments.FindSyncedAsset(f.ctx, f.conn, holding); err != nil || found {
				t.Fatalf("FindSyncedAsset before resolve = found %v, err %v; want not found", found, err)
			}
			if f.assetCount() != before {
				t.Fatal("FindSyncedAsset created an asset")
			}

			created, err := investments.ResolveSyncedAsset(f.ctx, f.conn, holding)
			if err != nil {
				t.Fatalf("ResolveSyncedAsset: %v", err)
			}
			found, ok, err := investments.FindSyncedAsset(f.ctx, f.conn, holding)
			if err != nil || !ok {
				t.Fatalf("FindSyncedAsset after resolve = found %v, err %v; want the created asset", ok, err)
			}
			if found.ID != created.ID || found.CanonicalKey != created.CanonicalKey {
				t.Fatalf("found %s (%s), want %s (%s)", found.ID, found.CanonicalKey, created.ID, created.CanonicalKey)
			}
			if f.assetCount() != before+1 {
				t.Fatalf("assets = %d, want %d", f.assetCount(), before+1)
			}
		})
	}
}

func TestFindSyncedAssetSharesTheAssetOfHoldingsWithTheSameIdentity(t *testing.T) {
	f := newLedgerFixture(t)
	first, second := equity("10"), equity("3")
	created, err := investments.ResolveSyncedAsset(f.ctx, f.conn, first)
	if err != nil {
		t.Fatalf("ResolveSyncedAsset: %v", err)
	}
	found, ok, err := investments.FindSyncedAsset(f.ctx, f.conn, second)
	if err != nil || !ok || found.ID != created.ID {
		t.Fatalf("FindSyncedAsset = %v (found %v, err %v), want %s", found.ID, ok, err, created.ID)
	}
}

// Listing never writes the catalog, and creating an asset is safe when several
// requests resolve the same new identity at once.
func TestConcurrentReadsAndResolvesOfASyncedAssetDoNotRaceOnItsKey(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		holding := equity("10")
		f.addHolding(holding)
		const workers = 20
		run := func(work func() error) {
			t.Helper()
			errs := make(chan error, workers)
			for i := 0; i < workers; i++ {
				go func() { errs <- work() }()
			}
			for i := 0; i < workers; i++ {
				if err := <-errs; err != nil {
					t.Errorf("concurrent call failed: %v", err)
				}
			}
		}

		before := f.assetCount()
		run(func() error {
			_, err := investments.ListPositions(f.ctx, f.conn, investments.PositionFilter{})
			return err
		})
		if f.assetCount() != before {
			t.Fatalf("listing created assets: %d, was %d", f.assetCount(), before)
		}

		run(func() error {
			_, err := investments.ResolveSyncedAsset(f.ctx, f.conn, holding)
			return err
		})
		if f.assetCount() != before+1 {
			t.Fatalf("assets = %d, want exactly one new", f.assetCount())
		}
	})
}

func TestFindSyncedAssetOfAHoldingWithNoIdentityIsNotFound(t *testing.T) {
	f := newLedgerFixture(t)
	if _, err := investments.ResolveSyncedAsset(f.ctx, f.conn, investments.SyncedHolding{}); err == nil {
		t.Fatal("ResolveSyncedAsset accepted a holding with no identity")
	}
	if _, found, err := investments.FindSyncedAsset(f.ctx, f.conn, investments.SyncedHolding{}); err != nil || found {
		t.Fatalf("FindSyncedAsset = found %v, err %v; want not found", found, err)
	}
}

// A holding whose asset does not exist yet has no quotes: reading its yield
// says so instead of creating the asset.
func TestPositionYieldDoesNotCreateTheAssetOfASyncedHolding(t *testing.T) {
	f := newLedgerFixture(t)
	id := f.addHolding(equity("10"))
	f.addMovement(id, "inflow", "1000", "10", day(1))
	before := f.assetCount()

	if reason := f.yieldReason(id); reason != investments.YieldReasonNoPrice {
		t.Fatalf("reason = %s, want %s", reason, investments.YieldReasonNoPrice)
	}
	if _, err := investments.DailyYield(f.ctx, f.conn, id, day(1), day(10)); err != nil {
		t.Fatalf("DailyYield: %v", err)
	}
	if f.assetCount() != before {
		t.Fatalf("assets = %d, want %d: a read created one", f.assetCount(), before)
	}
}

// quoteLoadCounter counts the queries that read a price series.
type quoteLoadCounter struct {
	investments.Querier
	loads int
}

func (c *quoteLoadCounter) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if strings.Contains(query, "FROM investment_asset_quotes") {
		c.loads++
	}
	return c.Querier.QueryContext(ctx, query, args...)
}

// Two lots of one stock are two holdings over one price series.
func twoLotsOfOneStock(f *ledgerFixture) (first, second string) {
	f.t.Helper()
	holding := equity("10")
	first = f.addHolding(holding)
	f.addMovement(first, "inflow", "1000", "10", day(1))
	second = f.addHolding(equity("10"))
	f.addMovement(second, "inflow", "900", "10", day(2))
	f.recordQuote(holding, "2026-09-20T12:00:00Z", "1050")
	f.recordQuote(holding, "2026-09-28T12:00:00Z", "1100")
	return first, second
}

func TestYieldCacheLoadsAnAssetsSeriesOncePerRequest(t *testing.T) {
	f := newLedgerFixture(t)
	first, second := twoLotsOfOneStock(f)
	to := calendarDay("2026-09-28")

	counter := &quoteLoadCounter{Querier: f.conn}
	cache := investments.NewYieldCache()
	for id, want := range map[string]string{first: "100", second: "200"} {
		got, err := investments.PositionYieldWith(f.ctx, counter, cache, id, nil, to)
		if err != nil {
			t.Fatalf("PositionYieldWith: %v", err)
		}
		if got.Value.String() != want {
			t.Fatalf("yield of %s = %s, want %s", id, got.Value, want)
		}
	}
	if counter.loads != 1 {
		t.Fatalf("price series loaded %d times for two holdings of one asset, want 1", counter.loads)
	}

	// Without a cache every holding loads its own, as PositionYield does.
	uncached := &quoteLoadCounter{Querier: f.conn}
	for _, id := range []string{first, second} {
		if _, err := investments.PositionYield(f.ctx, uncached, id, nil, to); err != nil {
			t.Fatalf("PositionYield: %v", err)
		}
	}
	if uncached.loads != 2 {
		t.Fatalf("uncached loads = %d, want 2", uncached.loads)
	}
}

// The series is loaded only through the requested day, and a cache holding a
// longer one answers an earlier day without another query.
func TestYieldCacheServesEarlierDaysFromWhatItHoldsAndReloadsForLaterOnes(t *testing.T) {
	f := newLedgerFixture(t)
	first, _ := twoLotsOfOneStock(f)
	counter := &quoteLoadCounter{Querier: f.conn}
	cache := investments.NewYieldCache()
	yield := func(to string) string {
		t.Helper()
		got, err := investments.PositionYieldWith(f.ctx, counter, cache, first, nil, calendarDay(to))
		if err != nil {
			t.Fatalf("PositionYieldWith(%s): %v", to, err)
		}
		return got.Value.String()
	}

	if got := yield("2026-09-22"); got != "50" || counter.loads != 1 {
		t.Fatalf("to 22nd = %s after %d loads, want 50 after 1: the 28th's quote must not be read", got, counter.loads)
	}
	if got := yield("2026-09-28"); got != "100" || counter.loads != 2 {
		t.Fatalf("to 28th = %s after %d loads, want 100 after 2", got, counter.loads)
	}
	if got := yield("2026-09-22"); got != "50" || counter.loads != 2 {
		t.Fatalf("to 22nd again = %s after %d loads, want 50 after 2: the held series covers it", got, counter.loads)
	}
}
