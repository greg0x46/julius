package quotes

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/settings"
)

// backfillRequests carries "something that changes what needs pricing was
// just saved" from the HTTP handlers to the scheduler. One slot is enough:
// the scan it triggers looks at everything, so any number of requests that
// arrive while one is pending are the same request.
var backfillRequests = make(chan struct{}, 1)

// backfillDebounce lets a burst of saves (a position, then its operations)
// settle before the scan looks at them.
const backfillDebounce = 2 * time.Second

// RequestBackfill coalesces saves into one scan of missing prices.
func RequestBackfill() {
	select {
	case backfillRequests <- struct{}{}:
	default:
	}
}

var configurationChanges = make(chan struct{}, 1)

func ConfigurationChanged() {
	select {
	case configurationChanges <- struct{}{}:
	default:
	}
}

const configurationPoll = time.Minute

type scheduleState struct {
	config      settings.QuoteRefreshSettings
	initialized bool
	nextDaily   time.Time
	pending     bool
}

// cycle is shared by the timer loop and deterministic scheduler tests.
func (s *scheduleState) cycle(ctx context.Context, conn *sql.DB, service *marketdata.Service, now time.Time, backfill bool) error {
	s.pending = s.pending || backfill
	config, _, err := settings.GetQuoteRefresh(ctx, conn)
	if err != nil {
		return err
	}
	schedule, err := configuredSchedule(config)
	if err != nil {
		return err
	}
	if !s.initialized || config != s.config {
		if !s.initialized || (!s.config.Enabled && config.Enabled) {
			s.pending = true
		}
		s.nextDaily = schedule.Next(now)
		s.config, s.initialized = config, true
	}
	if !config.Enabled || ctx.Err() != nil {
		return ctx.Err()
	}
	daily := !now.Before(s.nextDaily)
	if !daily && !s.pending {
		return nil
	}
	today := investments.ProviderDay(now)
	// A disable committed during spot work takes effect before history starts.
	if daily {
		_, err = RefreshAll(ctx, conn, service, today)
	} else {
		_, err = RefreshMissing(ctx, conn, service, today)
	}
	if err != nil {
		return err
	}
	current, _, err := settings.GetQuoteRefresh(ctx, conn)
	if err != nil {
		return err
	}
	if !current.Enabled || ctx.Err() != nil {
		return ctx.Err()
	}
	if _, err = RefreshHistory(ctx, conn, service, today); err != nil {
		return err
	}
	s.pending = false
	if daily {
		s.nextDaily = schedule.Next(now)
	}
	return nil
}

// RunSchedule rereads persisted settings on notifications and at least every minute.
func RunSchedule(ctx context.Context, conn *sql.DB, service *marketdata.Service) {
	state := scheduleState{}
	backfill := false
	for ctx.Err() == nil {
		now := time.Now()
		err := state.cycle(ctx, conn, service, now, backfill)
		if err != nil && ctx.Err() == nil {
			log.Printf("quote_refresh_failed: %v", err)
		}
		delay := configurationPoll
		if err == nil && state.config.Enabled && state.nextDaily.After(now) {
			if until := time.Until(state.nextDaily); until < delay {
				delay = until
			}
		}
		var running bool
		backfill, running = waitForWork(ctx, delay)
		if !running {
			return
		}
	}
}

func waitForWork(ctx context.Context, delay time.Duration) (backfill, running bool) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false, false
	case <-timer.C:
		return false, true
	case <-configurationChanges:
		return false, true
	case <-backfillRequests:
		sleep(ctx, backfillDebounce)
		select {
		case <-backfillRequests:
		default:
		}
		return true, ctx.Err() == nil
	}
}

// BrapiToken reads the decrypted brapi token from settings, treating
// "not configured" and "server key unavailable" the same way: an empty
// token, which the brapi provider sends unauthenticated (matching brapi's free
// tier) rather than treating as an error.
func BrapiToken(conn *sql.DB, secrets *settings.Secrets) func(context.Context) string {
	return func(ctx context.Context) string {
		key, ok := secrets.Key()
		if !ok {
			return ""
		}
		token, found, err := settings.GetBrapiToken(ctx, conn, key)
		if err != nil || !found {
			return ""
		}
		return token
	}
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
