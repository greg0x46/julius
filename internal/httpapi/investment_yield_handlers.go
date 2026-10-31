package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
)

type investmentDayYieldDTO struct {
	Day      string `json:"day"`
	Value    string `json:"value"`
	EndValue string `json:"end_value"`
	NetFlows string `json:"net_flows"`
}

// investmentYieldDTO is a holding's rendimento over a period. Value is nil
// only when UnavailableReason says why; From/To are the days actually used,
// which Partial reports differ from the requested start.
type investmentYieldDTO struct {
	PositionID        string                  `json:"position_id"`
	From              *string                 `json:"from"`
	To                string                  `json:"to"`
	Partial           bool                    `json:"partial"`
	Value             *string                 `json:"value"`
	StartValue        *string                 `json:"start_value"`
	EndValue          *string                 `json:"end_value"`
	NetFlows          *string                 `json:"net_flows"`
	UnavailableReason *string                 `json:"unavailable_reason"`
	Days              []investmentDayYieldDTO `json:"days,omitempty"`
}

// handleGetInvestmentPositionYield answers
// GET /api/investment-positions/{id}/yield?from=YYYY-MM-DD&to=YYYY-MM-DD&granularity=day.
// Without from it is the rendimento since inception; without to, through
// today. granularity=day adds each day's rendimento in (from, to].
func handleGetInvestmentPositionYield(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		var from *time.Time
		if raw := query.Get("from"); raw != "" {
			parsed, err := time.Parse(investments.DateLayout, raw)
			if err != nil {
				writeInvestmentInvalid(w, "from deve estar no formato AAAA-MM-DD.")
				return
			}
			from = &parsed
		}
		to := investments.ProviderDay(time.Now())
		if raw := query.Get("to"); raw != "" {
			parsed, err := time.Parse(investments.DateLayout, raw)
			if err != nil {
				writeInvestmentInvalid(w, "to deve estar no formato AAAA-MM-DD.")
				return
			}
			to = parsed
		}
		granularity := query.Get("granularity")
		if granularity != "" && granularity != "day" {
			writeInvestmentInvalid(w, "granularity aceita apenas day.")
			return
		}
		if from != nil && from.After(to) {
			writeInvestmentInvalid(w, "from não pode ser posterior a to.")
			return
		}
		if from != nil && dates.ExceedsMaxSpan(*from, to) {
			writeInvestmentRangeTooLong(w)
			return
		}

		id := r.PathValue("id")
		dto := investmentYieldDTO{PositionID: id, To: to.Format(investments.DateLayout)}
		result, err := investments.PositionYield(r.Context(), conn, id, from, to)
		var reason *investments.YieldUnavailableError
		switch {
		case err == nil:
			fromDay := result.From.Format(investments.DateLayout)
			value, start, end, flows := money.CanonicalDecimal(result.Value), money.CanonicalDecimal(result.StartValue),
				money.CanonicalDecimal(result.EndValue), money.CanonicalDecimal(result.NetFlows)
			dto.From, dto.Partial = &fromDay, result.Partial
			dto.Value, dto.StartValue, dto.EndValue, dto.NetFlows = &value, &start, &end, &flows
		case errors.As(err, &reason):
			dto.UnavailableReason = &reason.Reason
		default:
			writeInvestmentProblem(w, err)
			return
		}

		if granularity == "day" && dto.From != nil {
			// Without from, the series starts at the holding's inception.
			if dates.ExceedsMaxSpan(result.From, to) {
				writeInvestmentRangeTooLong(w)
				return
			}
			days, err := investments.DailyYield(r.Context(), conn, id, result.From, to)
			if err != nil {
				writeInvestmentProblem(w, err)
				return
			}
			dto.Days = make([]investmentDayYieldDTO, len(days))
			for i, day := range days {
				dto.Days[i] = investmentDayYieldDTO{
					Day: day.Day.Format(investments.DateLayout), Value: money.CanonicalDecimal(day.Value),
					EndValue: money.CanonicalDecimal(day.EndValue), NetFlows: money.CanonicalDecimal(day.NetFlows),
				}
			}
		}
		writeJSON(w, http.StatusOK, dto)
	}
}
