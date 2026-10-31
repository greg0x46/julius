package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/settings"
)

var errInvalidTimezone = errors.New("invalid timezone")

// configuredZone reads the calendar zone setting once for a request. A failed
// read answers 503 itself.
func configuredZone(w http.ResponseWriter, r *http.Request, q settings.Querier) (*time.Location, bool) {
	loc, err := settings.Location(r.Context(), q)
	if err != nil {
		preferencesUnavailableProblem(w)
		return nil, false
	}
	return loc, true
}

func preferencesUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "preferences-unavailable", "Preferências temporariamente indisponíveis", "")
}

// writeTimezoneProblem answers a bad explicit timezone parameter of the
// reporting endpoints; any other error is a failed setting read.
func writeTimezoneProblem(w http.ResponseWriter, err error) {
	if errors.Is(err, errInvalidTimezone) {
		writeProblem(w, 400, "invalid-timezone", "Fuso horário inválido", "Informe um nome de fuso IANA reconhecido.")
		return
	}
	preferencesUnavailableProblem(w)
}

// resolveTimezone is the optional timezone parameter of reporting endpoints:
// the explicit IANA name when present (errInvalidTimezone if it is not one),
// else the configured zone. Any other error is a failed setting read.
func resolveTimezone(ctx context.Context, q settings.Querier, explicit string) (string, error) {
	if explicit == "" {
		return settings.Timezone(ctx, q)
	}
	if settings.ValidateTimezone(explicit) != nil {
		return "", errInvalidTimezone
	}
	return explicit, nil
}

// dayIn is the calendar day an instant falls on in loc.
func dayIn(t time.Time, loc *time.Location) time.Time { return dates.Day(t.In(loc)) }
