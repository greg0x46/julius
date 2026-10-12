package quotes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/marketdata"
)

func TestRefreshStatus(t *testing.T) {
	for _, tc := range []struct {
		summary Summary
		err     error
		want    RefreshStatus
	}{
		{Summary{}, nil, RefreshSucceeded},
		{Summary{AssetsConsidered: 2, PricesFetched: 2}, nil, RefreshSucceeded},
		{Summary{AssetsConsidered: 2, PricesFetched: 1, PricesDeferred: 1}, nil, RefreshDeferred},
		{Summary{AssetsConsidered: 1, PricesDeferred: 1}, nil, RefreshDeferred},
		{Summary{AssetsConsidered: 2, PricesFetched: 1, PriceFailures: 1}, nil, RefreshPartial},
		{Summary{AssetsConsidered: 2, PriceFailures: 1, PricesDeferred: 1}, nil, RefreshPartial},
		{Summary{AssetsConsidered: 1, PriceFailures: 1}, nil, RefreshFailed},
		{Summary{}, errors.New("db down"), RefreshFailed},
	} {
		if got := refreshStatus(tc.summary, tc.err); got != tc.want {
			t.Errorf("refreshStatus(%+v, %v) = %s, want %s", tc.summary, tc.err, got, tc.want)
		}
	}
}

func TestRefreshNowRecordsOutcomeAndKeepsStoredPrices(t *testing.T) {
	ctx := context.Background()
	conn := newJobTestConn(t)
	asset := mustCreateQuotedAsset(t, ctx, conn, "Bitcoin", "crypto", "crypto", "BTC")
	mustCreateManualPosition(t, ctx, conn, "wallet", asset, 1)

	if last, err := GetLastRefresh(ctx, conn); err != nil || last != nil {
		t.Fatalf("before any run: %+v %v", last, err)
	}
	ok := marketdata.New(&fakeConnector{prices: map[string]decimal.Decimal{"BTC": decimal.NewFromInt(100)}})
	last, err := RefreshNow(ctx, conn, ok)
	if err != nil || last.Status != RefreshSucceeded || last.Trigger != "manual" || last.Fetched != 1 {
		t.Fatalf("success: %+v %v", last, err)
	}
	stored, err := GetLastRefresh(ctx, conn)
	if err != nil || stored == nil || *stored != last {
		t.Fatalf("persisted: %+v %v", stored, err)
	}

	failing := marketdata.New(&fakeConnector{err: errors.New("provider down")})
	if last, err = RefreshNow(ctx, conn, failing); err != nil || last.Status != RefreshFailed || last.Failures != 1 {
		t.Fatalf("failure: %+v %v", last, err)
	}
	if prices := quotesOf(t, ctx, conn, asset.ID); len(prices) != 1 || prices[0].Price.String() != "100" {
		t.Fatalf("failure removed stored price: %+v", prices)
	}

	limited := marketdata.New(&fakeConnector{err: &marketdata.RateLimitedError{Provider: "yahoo", RetryAfter: time.Minute}})
	if last, err = RefreshNow(ctx, conn, limited); err != nil || last.Status != RefreshDeferred || last.Deferred != 1 {
		t.Fatalf("rate limited: %+v %v", last, err)
	}
}

func TestRefreshNowRefusesWhileAnotherRefreshRuns(t *testing.T) {
	conn := newJobTestConn(t)
	spotRefresh.Lock()
	defer spotRefresh.Unlock()
	if _, err := RefreshNow(context.Background(), conn, marketdata.New(&fakeConnector{})); !errors.Is(err, ErrRefreshRunning) {
		t.Fatalf("err = %v, want ErrRefreshRunning", err)
	}
	if last, _ := GetLastRefresh(context.Background(), conn); last != nil {
		t.Fatalf("refused run recorded: %+v", last)
	}
}
