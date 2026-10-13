package quotes

import (
	"context"
	"fmt"
	"github.com/greg0x46/julius/internal/settings"
	"strings"
	"time"
)

// Schedule is a daily wall-clock time at which RefreshAll runs.
//
// Copied locally from internal/worker/schedule.go's Schedule/ParseSchedule/
// Next rather than imported: internal/worker is Pluggy-specific end to end
// (Run/RunSchedule reach into sync_runs and block on Pluggy credentials),
// and importing it just for this one small type would tie this package to
// that one unrelated to quotes.
type Schedule struct {
	Hour, Minute int
	Location     *time.Location
}

// ParseSchedule reads JULIUS_QUOTES_SCHEDULE: "HH:MM" in the process's
// local time, optionally followed by an IANA zone ("06:00 America/Sao_Paulo").
// An empty value disables the schedule.
func ParseSchedule(value string) (Schedule, bool, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return Schedule{}, false, nil
	}
	if len(fields) > 2 {
		return Schedule{}, false, fmt.Errorf("JULIUS_QUOTES_SCHEDULE deve ser \"HH:MM\" ou \"HH:MM Zona/IANA\"")
	}
	clock, err := time.Parse("15:04", fields[0])
	if err != nil {
		return Schedule{}, false, fmt.Errorf("JULIUS_QUOTES_SCHEDULE: horário inválido %q (use HH:MM)", fields[0])
	}
	s := Schedule{Hour: clock.Hour(), Minute: clock.Minute(), Location: time.Local}
	if len(fields) == 2 {
		if s.Location, err = time.LoadLocation(fields[1]); err != nil {
			return Schedule{}, false, fmt.Errorf("JULIUS_QUOTES_SCHEDULE: fuso inválido %q", fields[1])
		}
	}
	return s, true, nil
}

func (s Schedule) at(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), s.Hour, s.Minute, 0, 0, s.Location)
}

// Previous is the most recent occurrence at or before now.
func (s Schedule) Previous(now time.Time) time.Time {
	now = now.In(s.Location)
	if due := s.at(now); !due.After(now) {
		return due
	}
	return s.at(now.AddDate(0, 0, -1))
}

// Next is the first occurrence strictly after now.
func (s Schedule) Next(now time.Time) time.Time {
	return s.Previous(now).AddDate(0, 0, 1)
}

// InitializeSchedule consults the legacy environment only before the first save.
func InitializeSchedule(ctx context.Context, q settings.Querier, legacy string) (settings.QuoteRefreshSettings, error) {
	config, found, err := settings.GetQuoteRefresh(ctx, q)
	if err != nil || found {
		return config, err
	}
	if strings.TrimSpace(legacy) != "" {
		schedule, _, err := ParseSchedule(legacy)
		if err != nil {
			return config, err
		}
		zone := "Local"
		if fields := strings.Fields(legacy); len(fields) == 2 {
			zone = fields[1]
		}
		config = settings.QuoteRefreshSettings{Enabled: true, Time: fmt.Sprintf("%02d:%02d", schedule.Hour, schedule.Minute), Timezone: zone}
	}
	return settings.InitializeQuoteRefresh(ctx, q, config)
}

func configuredSchedule(config settings.QuoteRefreshSettings) (Schedule, error) {
	if err := config.Validate(); err != nil {
		return Schedule{}, err
	}
	clock, _ := time.Parse("15:04", config.Time)
	zone, _ := time.LoadLocation(config.Timezone)
	return Schedule{Hour: clock.Hour(), Minute: clock.Minute(), Location: zone}, nil
}
