package httpapi

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/shopspring/decimal"
)

// valuationDTO travels with converted aggregates, never with native records.
type valuationDTO struct {
	CurrencyCode string        `json:"currency_code"`
	FX           exchange.Rate `json:"fx"`
}

func reportValuation(w http.ResponseWriter, r *http.Request, conn *sql.DB, day time.Time) (valuationDTO, bool) {
	code, err := settings.ReferenceCurrency(r.Context(), conn)
	if err != nil {
		writeProblem(w, 503, "preferences-unavailable", "Preferências temporariamente indisponíveis", "")
		return valuationDTO{}, false
	}
	return valuationForCurrency(w, r, exchange.New(conn, nil), code, day)
}

func valuationForCurrency(w http.ResponseWriter, r *http.Request, fx *exchange.Service, code string, day time.Time) (valuationDTO, bool) {
	rate, err := fx.On(r.Context(), "BRL", code, day)
	if err != nil {
		writeProblem(w, 503, "exchange-unavailable", "Cotação indisponível", "Não foi possível obter a cotação de BRL para "+code+". Os totais não podem ser avaliados.")
		return valuationDTO{}, false
	}
	return valuationDTO{CurrencyCode: code, FX: rate}, true
}

func (v valuationDTO) amount(amount decimal.Decimal) decimal.Decimal { return v.FX.Convert(amount) }
func (v valuationDTO) fields(fields ...*string) {
	for _, field := range fields {
		amount, _ := decimal.NewFromString(*field)
		*field = money.CanonicalDecimal(v.amount(amount))
	}
}

func (v valuationDTO) series(s *timelineSeriesDTO) {
	v.fields(&s.StartingBalance)
	point := func(p *timelineDayPointDTO) { v.fields(&p.Balance, &p.Inflow, &p.Outflow) }
	point(&s.LowestBalance)
	for i := range s.Points {
		point(&s.Points[i])
	}
	for i := range s.Entries {
		v.fields(&s.Entries[i].Amount, &s.Entries[i].ReportableAmount)
	}
}

func (v valuationDTO) breakdown(b *netWorthBreakdownDTO) {
	v.fields(&b.CashBalance, &b.InvestmentBalance, &b.CreditCardBalance, &b.PayablesDebt, &b.TotalAssets, &b.TotalLiabilities, &b.NetWorth)
}
