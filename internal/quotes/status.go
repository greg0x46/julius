package quotes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/settings"
)

const keyLastRefresh = "quotes.last_refresh"

type RefreshStatus string

const (
	RefreshSucceeded RefreshStatus = "success"
	RefreshPartial   RefreshStatus = "partial"
	RefreshDeferred  RefreshStatus = "deferred"
	RefreshFailed    RefreshStatus = "failed"
)

// LastRefresh is the outcome of the most recent full spot refresh, scheduled
// or manual.
type LastRefresh struct {
	FinishedAt time.Time     `json:"finished_at"`
	Trigger    string        `json:"trigger"`
	Status     RefreshStatus `json:"status"`
	Assets     int           `json:"assets"`
	Fetched    int           `json:"fetched"`
	Failures   int           `json:"failures"`
	Deferred   int           `json:"deferred"`
}

var ErrRefreshRunning = errors.New("quote refresh already running")

// spotRefresh serializes spot refreshes between the scheduler and RefreshNow.
var spotRefresh sync.Mutex

// RefreshNow runs RefreshAll right away and asks the scheduler to fill in
// missing history afterwards. It refuses instead of queueing while another
// spot refresh is running.
func RefreshNow(ctx context.Context, conn *sql.DB, service *marketdata.Service) (LastRefresh, error) {
	if !spotRefresh.TryLock() {
		return LastRefresh{}, ErrRefreshRunning
	}
	defer spotRefresh.Unlock()
	summary, err := RefreshAll(ctx, conn, service, investments.ProviderDay(time.Now()))
	if err != nil {
		log.Printf("quote_refresh_failed: %v", err)
	}
	last := recordRefresh(ctx, conn, "manual", summary, err)
	RequestBackfill()
	return last, nil
}

func GetLastRefresh(ctx context.Context, q settings.Querier) (*LastRefresh, error) {
	value, ok, err := settings.Get(ctx, q, keyLastRefresh, nil)
	if err != nil || !ok {
		return nil, err
	}
	var last LastRefresh
	if err := json.Unmarshal([]byte(value), &last); err != nil {
		return nil, err
	}
	return &last, nil
}

func recordRefresh(ctx context.Context, conn *sql.DB, trigger string, summary Summary, refreshErr error) LastRefresh {
	last := LastRefresh{
		FinishedAt: time.Now().UTC(), Trigger: trigger, Status: refreshStatus(summary, refreshErr),
		Assets: summary.AssetsConsidered, Fetched: summary.PricesFetched,
		Failures: summary.PriceFailures, Deferred: summary.PricesDeferred,
	}
	value, err := json.Marshal(last)
	if err == nil {
		err = settings.Set(context.WithoutCancel(ctx), conn, keyLastRefresh, string(value), false, nil)
	}
	if err != nil {
		log.Printf("quote_refresh_status_failed: %v", err)
	}
	return last
}

func refreshStatus(summary Summary, err error) RefreshStatus {
	switch {
	case err != nil:
		return RefreshFailed
	case summary.PricesFetched == summary.AssetsConsidered:
		return RefreshSucceeded
	case summary.PriceFailures == 0:
		return RefreshDeferred
	case summary.PricesFetched == 0 && summary.PricesDeferred == 0:
		return RefreshFailed
	default:
		return RefreshPartial
	}
}
