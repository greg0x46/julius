package httpapi

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/valuation"
)

// The period view of the investment book (see internal/investments/portfolio.go).
// Money is in BRL converted to the reference currency like /api/investment-summary;
// a figure that cannot be stated is null (never zero) and its position says why.

type investmentPeriodNativeDTO struct {
	StartValue    string  `json:"start_value"`
	EndValue      string  `json:"end_value"`
	Contributions string  `json:"contributions"`
	Withdrawals   string  `json:"withdrawals"`
	Yield         string  `json:"yield"`
	Invested      *string `json:"invested"`
}

type investmentPeriodPositionDTO struct {
	ID                string  `json:"id"`
	Source            string  `json:"source"`
	AccountID         string  `json:"account_id"`
	AccountName       string  `json:"account_name"`
	AssetID           *string `json:"asset_id"`
	Name              string  `json:"name"`
	Ticker            *string `json:"ticker"`
	AssetType         string  `json:"asset_type"`
	AssetClass        string  `json:"asset_class"`
	CurrencyCode      string  `json:"currency_code"`
	Quantity          *string `json:"quantity"`
	ClosedAsOfTo      bool    `json:"closed_as_of_to"`
	UnavailableReason *string `json:"unavailable_reason"`
	OpeningOn         *string `json:"opening_on"`
	PartialStart      bool    `json:"partial_start"`
	HistoryComplete   bool    `json:"history_complete"`

	StartValue    *string `json:"start_value"`
	EndValue      *string `json:"end_value"`
	Contributions *string `json:"contributions"`
	Withdrawals   *string `json:"withdrawals"`
	Yield         *string `json:"yield"`
	CapitalBase   *string `json:"capital_base"`
	YieldRate     *string `json:"yield_rate"`
	Share         *string `json:"share"`

	Native                    *investmentPeriodNativeDTO `json:"native"`
	Invested                  *string                    `json:"invested"`
	InvestedUnavailableReason *string                    `json:"invested_unavailable_reason"`
	Costs                     *string                    `json:"costs"`
	ProvisionedTaxes          *string                    `json:"provisioned_taxes"`
	ReportedProfit            *string                    `json:"reported_profit"`
	ReportedAsOf              *string                    `json:"reported_as_of"`
}

// investmentPeriodPartDTO is the part of the totals made of the positions held
// (or closed) as of `to`; the two parts add up to the totals.
type investmentPeriodPartDTO struct {
	StartValue    string  `json:"start_value"`
	EndValue      string  `json:"end_value"`
	Contributions string  `json:"contributions"`
	Withdrawals   string  `json:"withdrawals"`
	Yield         string  `json:"yield"`
	CapitalBase   string  `json:"capital_base"`
	YieldRate     *string `json:"yield_rate"`
}

type investmentPeriodTotalsDTO struct {
	Held                  investmentPeriodPartDTO `json:"held"`
	Closed                investmentPeriodPartDTO `json:"closed"`
	StartValue            string                  `json:"start_value"`
	EndValue              string                  `json:"end_value"`
	Contributions         string                  `json:"contributions"`
	Withdrawals           string                  `json:"withdrawals"`
	Yield                 string                  `json:"yield"`
	CapitalBase           string                  `json:"capital_base"`
	YieldRate             *string                 `json:"yield_rate"`
	Invested              string                  `json:"invested"`
	InvestedUnknown       int                     `json:"invested_unknown_positions"`
	Costs                 string                  `json:"costs"`
	ProvisionedTaxes      string                  `json:"provisioned_taxes"`
	PositionsTotal        int                     `json:"positions_total"`
	PositionsUnavailable  int                     `json:"positions_unavailable"`
	PositionsPartialStart int                     `json:"positions_partial_start"`
	Partial               bool                    `json:"partial"`
}

type investmentPeriodGroupDTO struct {
	Key        string                        `json:"key"`
	Label      string                        `json:"label"`
	AccountID  *string                       `json:"account_id"`
	AssetClass *string                       `json:"asset_class"`
	Share      *string                       `json:"share"`
	Totals     investmentPeriodTotalsDTO     `json:"totals"`
	Positions  []investmentPeriodPositionDTO `json:"positions"`
}

type investmentPeriodRateDTO struct {
	Value       string `json:"value"`
	Inverted    bool   `json:"inverted"`
	EffectiveOn string `json:"effective_on"`
	Source      string `json:"source"`
	Stale       bool   `json:"stale"`
}

type investmentPeriodDTO struct {
	valuationDTO
	From      *string `json:"from"`
	To        string  `json:"to"`
	ToClamped bool    `json:"to_clamped"`
	GroupBy   string  `json:"group_by"`
	Source    *string `json:"source"`
	AccountID *string `json:"account_id"`
	// NativeRates are the rates that took foreign positions to BRL.
	NativeRates map[string]investmentPeriodRateDTO `json:"native_rates"`
	Totals      investmentPeriodTotalsDTO          `json:"totals"`
	Groups      []investmentPeriodGroupDTO         `json:"groups"`
}

// handleGetInvestmentPeriod answers
// GET /api/investment-portfolio?from=&to=&group_by=none|account|class&source=manual|synced&account_id=.
// Without from it is since each holding's inception; without to, through today;
// a future to is clamped to today (the response echoes the effective to and
// to_clamped), while a from after the effective to is a 400.
// from and to are both inside the period. source narrows what is measured;
// account_id only narrows which positions are listed, so the totals stay the
// portfolio's.
func handleGetInvestmentPeriod(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, ok := investmentQueryDate(r, "from")
		if !ok {
			writeInvestmentInvalid(w, "from deve estar no formato AAAA-MM-DD.")
			return
		}
		today := investments.ProviderDay(time.Now())
		to := today
		if raw := investmentQueryFilter(r, "to"); raw != nil {
			parsed, err := time.Parse(investmentDateLayout, *raw)
			if err != nil {
				writeInvestmentInvalid(w, "to deve estar no formato AAAA-MM-DD.")
				return
			}
			to = parsed
		}
		toClamped := to.After(today)
		if toClamped {
			to = today
		}
		if from != nil && from.After(to) {
			writeInvestmentInvalid(w, "from não pode ser posterior a to.")
			return
		}
		query := investments.PortfolioQuery{From: from, To: to, GroupBy: investments.PortfolioGroupNone, AccountID: investmentQueryFilter(r, "account_id")}
		switch groupBy := investmentQueryFilter(r, "group_by"); {
		case groupBy == nil:
		case *groupBy == string(investments.PortfolioGroupNone), *groupBy == string(investments.PortfolioGroupAccount), *groupBy == string(investments.PortfolioGroupClass):
			query.GroupBy = investments.PortfolioGroupBy(*groupBy)
		default:
			writeInvestmentInvalid(w, "group_by aceita none, account ou class.")
			return
		}
		switch source := investmentQueryFilter(r, "source"); {
		case source == nil:
		case *source == string(investments.PositionSourceManual), *source == string(investments.PositionSourceSynced):
			value := investments.PositionSource(*source)
			query.Source = &value
		default:
			writeInvestmentInvalid(w, "source aceita manual ou synced.")
			return
		}
		if query.AccountID != nil {
			if _, err := investments.GetAccount(r.Context(), conn, *query.AccountID); err != nil {
				writeInvestmentProblem(w, err)
				return
			}
		}

		reference, ok := reportValuation(w, r, conn, to)
		if !ok {
			return
		}
		portfolio, err := investments.BuildPortfolio(r.Context(), conn, query)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		dto := investmentPeriodToDTO(portfolio, reference)
		dto.ToClamped = toClamped
		writeJSON(w, http.StatusOK, dto)
	}
}

func investmentPeriodToDTO(p investments.Portfolio, reference valuationDTO) investmentPeriodDTO {
	scale := func(amount decimal.Decimal) string { return money.CanonicalDecimal(reference.amount(amount)) }
	dto := investmentPeriodDTO{
		valuationDTO: reference,
		From:         optionalDay(p.From),
		To:           investments.Day(p.To).Format(investmentDateLayout),
		GroupBy:      string(p.GroupBy),
		AccountID:    p.AccountID,
		NativeRates:  map[string]investmentPeriodRateDTO{},
		Totals:       investmentPeriodTotalsToDTO(p.Totals, scale),
		Groups:       []investmentPeriodGroupDTO{},
	}
	if p.Source != nil {
		source := string(*p.Source)
		dto.Source = &source
	}
	for code, rate := range p.Valuation.Rates {
		dto.NativeRates[code] = investmentPeriodRateToDTO(rate)
	}
	for _, group := range p.Groups {
		g := investmentPeriodGroupDTO{
			Key: group.Key, Label: group.Label, AccountID: group.AccountID,
			Share: optionalMoney(group.Share), Totals: investmentPeriodTotalsToDTO(group.Totals, scale),
			Positions: []investmentPeriodPositionDTO{},
		}
		if group.AssetClass != nil {
			class := string(*group.AssetClass)
			g.AssetClass = &class
		}
		for _, position := range group.Positions {
			g.Positions = append(g.Positions, investmentPeriodPositionToDTO(position, scale))
		}
		dto.Groups = append(dto.Groups, g)
	}
	return dto
}

func investmentPeriodRateToDTO(rate valuation.Rate) investmentPeriodRateDTO {
	return investmentPeriodRateDTO{
		Value: money.CanonicalDecimal(rate.Value), Inverted: rate.Inverted,
		EffectiveOn: investments.Day(rate.EffectiveOn).Format(investmentDateLayout), Source: rate.Source, Stale: rate.Stale,
	}
}

func investmentPeriodPartToDTO(p investments.PortfolioPart, scale func(decimal.Decimal) string) investmentPeriodPartDTO {
	return investmentPeriodPartDTO{
		StartValue: scale(p.StartValue), EndValue: scale(p.EndValue), Contributions: scale(p.Contributions),
		Withdrawals: scale(p.Withdrawals), Yield: scale(p.Yield), CapitalBase: scale(p.CapitalBase), YieldRate: optionalMoney(p.YieldRate),
	}
}

func investmentPeriodTotalsToDTO(t investments.PortfolioTotals, scale func(decimal.Decimal) string) investmentPeriodTotalsDTO {
	return investmentPeriodTotalsDTO{
		Held: investmentPeriodPartToDTO(t.Held, scale), Closed: investmentPeriodPartToDTO(t.Closed, scale),
		StartValue: scale(t.StartValue), EndValue: scale(t.EndValue), Contributions: scale(t.Contributions),
		Withdrawals: scale(t.Withdrawals), Yield: scale(t.Yield), CapitalBase: scale(t.CapitalBase),
		YieldRate: optionalMoney(t.YieldRate), Invested: scale(t.Invested), InvestedUnknown: t.InvestedUnknown,
		Costs: scale(t.Costs), ProvisionedTaxes: scale(t.ProvisionedTaxes),
		PositionsTotal: t.PositionsTotal, PositionsUnavailable: t.PositionsUnavailable,
		PositionsPartialStart: t.PositionsPartialStart, Partial: t.Partial(),
	}
}

func investmentPeriodPositionToDTO(p investments.PortfolioPosition, scale func(decimal.Decimal) string) investmentPeriodPositionDTO {
	text := func(value *decimal.Decimal) *string {
		if value == nil {
			return nil
		}
		converted := scale(*value)
		return &converted
	}
	dto := investmentPeriodPositionDTO{
		ID: p.ID, Source: string(p.Source), AccountID: p.AccountID, AccountName: p.AccountName, AssetID: p.AssetID,
		Name: p.Name, Ticker: p.Ticker, AssetType: p.AssetType, AssetClass: string(p.AssetClass), CurrencyCode: p.CurrencyCode,
		Quantity: optionalMoney(p.Quantity), ClosedAsOfTo: p.ClosedAsOfTo, HistoryComplete: p.HistoryComplete,
		Invested: text(p.Invested), Costs: text(p.Costs), ProvisionedTaxes: text(p.ProvisionedTaxes),
		ReportedProfit: optionalMoney(p.ReportedProfit), ReportedAsOf: optionalDay(p.ReportedAsOf),
	}
	if p.InvestedUnavailableReason != "" {
		reason := p.InvestedUnavailableReason
		dto.InvestedUnavailableReason = &reason
	}
	if p.UnavailableReason != "" {
		reason := p.UnavailableReason
		dto.UnavailableReason = &reason
		return dto
	}
	opening := investments.Day(p.OpeningOn).Format(investmentDateLayout)
	dto.OpeningOn, dto.PartialStart = &opening, p.PartialStart
	dto.StartValue, dto.EndValue = text(&p.StartValue), text(&p.EndValue)
	dto.Contributions, dto.Withdrawals = text(&p.Contributions), text(&p.Withdrawals)
	dto.Yield, dto.CapitalBase = text(&p.Yield), text(&p.CapitalBase)
	dto.YieldRate, dto.Share = optionalMoney(p.YieldRate), optionalMoney(p.Share)
	if p.Native != nil {
		dto.Native = &investmentPeriodNativeDTO{
			StartValue: money.CanonicalDecimal(p.Native.StartValue), EndValue: money.CanonicalDecimal(p.Native.EndValue),
			Contributions: money.CanonicalDecimal(p.Native.Contributions), Withdrawals: money.CanonicalDecimal(p.Native.Withdrawals),
			Yield: money.CanonicalDecimal(p.Native.Yield), Invested: optionalMoney(p.Native.Invested),
		}
	}
	return dto
}
