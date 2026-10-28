package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/goals"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/projections"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/reporting"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/valuation"
)

const mcpMaxOutput = 256 << 10

var errMCPOutput = errors.New("result exceeds 256 KiB; reduce limit, date range or account scope")

var errMCPInput = errors.New("invalid arguments: use calendar dates YYYY-MM-DD, ordered ranges of at most 366 days, and documented filters")

type mcpPage struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Maximum items, 1–100; default 50."`
	Offset int `json:"offset,omitempty" jsonschema:"Zero-based offset; default 0."`
}

func (p mcpPage) bounds(n int) (int, int, *int, error) {
	if p.Limit < 0 || p.Limit > 100 || p.Offset < 0 {
		return 0, 0, nil, errMCPInput
	}
	limit := p.Limit
	if limit == 0 {
		limit = 50
	}
	start := min(p.Offset, n)
	end := min(start+limit, n)
	var next *int
	if end < n {
		next = &end
	}
	return start, end, next, nil
}

type mcpItems[T any] struct {
	CurrencyCode string `json:"currency_code"`
	Items        []T    `json:"items"`
	NextOffset   *int   `json:"next_offset"`
	Assumptions  string `json:"assumptions"`
}

func mcpPaginate[T any](items []T, page mcpPage, assumptions string) (mcpItems[T], error) {
	start, end, next, err := page.bounds(len(items))
	if err != nil {
		return mcpItems[T]{}, err
	}
	return mcpItems[T]{CurrencyCode: "BRL", Items: append([]T{}, items[start:end]...), NextOffset: next, Assumptions: assumptions}, nil
}

type mcpRange struct {
	From string `json:"from" jsonschema:"First calendar day, inclusive, YYYY-MM-DD."`
	To   string `json:"to" jsonschema:"Last calendar day, inclusive; at most 366 days from from."`
}

func (r mcpRange) days() (time.Time, time.Time, error) {
	from, e1 := time.Parse(time.DateOnly, r.From)
	to, e2 := time.Parse(time.DateOnly, r.To)
	if e1 != nil || e2 != nil || to.Before(from) || to.After(from.AddDate(0, 0, 365)) {
		return time.Time{}, time.Time{}, errMCPInput
	}
	return from, to, nil
}

// RegisterMCPPlanningTools shares the REST presentation boundary and domain
// engines. No HTTP dispatch, credentials, or write handlers are exposed.
func RegisterMCPPlanningTools(server *sdk.Server, conn *sql.DB) {
	addPlanningTool(server, "get_investment_portfolio", "Investment positions and returns as of to. Modified Dietz rates; costs counted once; unavailable positions excluded with reasons. Native figures and dated FX retained. No scenarios. Narrow account_id if output exceeds 256 KiB.", func(ctx context.Context, in mcpPortfolioInput) (investmentPeriodDTO, error) {
		return mcpPortfolio(ctx, conn, in)
	})
	addPlanningTool(server, "list_goals", "Goals with current progress and required end-of-month contributions, in BRL. Derived on read; no scenarios. Projection assumes no new contributions, a trailing measured monthly return (zero when null); return_partial discloses incomplete history.", func(ctx context.Context, in mcpPage) (mcpItems[goalDTO], error) {
		list, err := goals.List(ctx, conn)
		if err != nil {
			return mcpItems[goalDTO]{}, err
		}
		items := make([]goalDTO, len(list))
		for i, g := range list {
			items[i] = goalToDTO(g)
		}
		return mcpPaginate(items, in, "Current invested value; trailing 12-month return with at least 30 days of history; deposits at month end. No hypothetical data.")
	})
	addPlanningTool(server, "get_net_worth", "Realized cash plus investments; excludes liabilities, future bills and scenarios. Historical days use their own FX. Unavailable days carry no figures. Range is inclusive, at most 366 days and cannot extend beyond today; latest is always today.", func(ctx context.Context, in mcpRange) (mcpWealth, error) { return mcpNetWorth(ctx, conn, in) })
	addPlanningTool(server, "list_payables", "Debts and receivables with authoritative settled/remaining amounts in BRL. Excludes credit-card bills. Actual settlements only; no simulated repayments.", func(ctx context.Context, in mcpPayablesInput) (mcpItems[payableDTO], error) {
		var kind *payables.Kind
		if in.Kind != "" {
			k := payables.Kind(in.Kind)
			if k != payables.KindDebt && k != payables.KindReceivable {
				return mcpItems[payableDTO]{}, errMCPInput
			}
			kind = &k
		}
		list, err := payables.List(ctx, conn, kind)
		if err != nil {
			return mcpItems[payableDTO]{}, err
		}
		summaries, err := payables.SummarizeAll(ctx, conn, list)
		if err != nil {
			return mcpItems[payableDTO]{}, err
		}
		items := make([]payableDTO, len(list))
		for i, p := range list {
			items[i] = payableToDTO(p, summaries[p.ID])
		}
		return mcpPaginate(items, in.mcpPage, "BRL obligations; remaining is clamped to [0,total]; credit-card bills are separate.")
	})
	addPlanningTool(server, "list_recurring_commitments", "Recurring schedules, including inactive ones, in BRL. Schedule amounts are planned commitments, not realized spending. Use list_projection_events for dated occurrences and reconciliation.", func(ctx context.Context, in mcpPage) (mcpItems[recurringCommitmentDTO], error) {
		list, err := recurrences.List(ctx, conn)
		if err != nil {
			return mcpItems[recurringCommitmentDTO]{}, err
		}
		items := make([]recurringCommitmentDTO, len(list))
		for i, c := range list {
			items[i] = toRecurringCommitmentDTO(c)
		}
		return mcpPaginate(items, in, "Schedules only; realized occurrences are suppressed in timeline projections.")
	})
	addPlanningTool(server, "list_scenarios", "Discover scenario IDs and kinds without applying them to any calculation. Includes inactive scenarios. Use IDs explicitly in projection tools to include hypothetical data.", func(ctx context.Context, in mcpPage) (mcpItems[scenarioDTO], error) {
		list, err := scenarios.ListScenarios(ctx, conn, scenarios.ListFilter{})
		if err != nil {
			return mcpItems[scenarioDTO]{}, err
		}
		items := make([]scenarioDTO, len(list))
		for i, s := range list {
			items[i] = scenarioToDTO(s)
		}
		return mcpPaginate(items, in, "Discovery does not activate scenarios.")
	})
	addPlanningTool(server, "list_projection_events", "Dated plan installments and recurring occurrences with realization state, in BRL. Defaults to active non-hypothetical commitments. Standalone scenarios enter only via scenario_ids; supplying [] adds none. Inclusive range, maximum 366 days.", func(ctx context.Context, in mcpProjectionInput) (mcpItems[mcpProjectionEvent], error) {
		return mcpEvents(ctx, conn, in)
	})
	addPlanningTool(server, "get_timeline", "Daily cash projection and income/expense totals. Active plans and recurring commitments are included; standalone hypotheticals only via scenario_ids. Actual figures are never changed. Returns selected IDs and reporting/FX coverage. At most 366 days; narrow dates if output exceeds 256 KiB.", func(ctx context.Context, in mcpTimelineInput) (mcpTimeline, error) {
		return mcpBuildTimeline(ctx, conn, in)
	})
}

func addPlanningTool[In, Out any](server *sdk.Server, name, description string, read func(context.Context, In) (Out, error)) {
	schema, err := jsonschema.For[Out](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[decimal.Decimal](): {Type: "string", Description: "Exact decimal string."},
	}})
	if err != nil {
		panic(fmt.Sprintf("MCP %s schema: %v", name, err))
	}
	sdk.AddTool(server, &sdk.Tool{Name: name, Description: description, OutputSchema: schema, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}}, func(ctx context.Context, _ *sdk.CallToolRequest, input In) (*sdk.CallToolResult, Out, error) {
		out, err := read(ctx, input)
		if err == nil {
			var data []byte
			data, err = json.Marshal(out)
			if err == nil && len(data) > mcpMaxOutput {
				err = errMCPOutput
			}
		}
		if err != nil {
			var zero Out
			return nil, zero, planningToolError(err)
		}
		return nil, out, nil
	})
}

func planningToolError(err error) error {
	message := "financial data unavailable; retry later"
	var incomplete *reporting.IncompleteError
	switch {
	case errors.Is(err, errMCPInput):
		message = errMCPInput.Error()
	case errors.Is(err, valuation.ErrMissingRate), errors.Is(err, exchange.ErrUnavailable):
		message = "exchange-unavailable: required FX valuation is unavailable; no balances were dropped"
	case errors.As(err, &incomplete):
		data, _ := json.Marshal(struct {
			Code      string             `json:"code"`
			Reporting reporting.Summary  `json:"reporting"`
			Totals    []reporting.Totals `json:"totals"`
		}{"reporting-fx-unavailable", incomplete.Coverage, incomplete.Totals})
		message = string(data)
	default:
		if errors.Is(err, errMCPOutput) {
			message = err.Error()
		}
	}
	if len(message) > mcpMaxOutput {
		message = "reporting-fx-unavailable: coverage exceeds 256 KiB; reduce the date range"
	}
	return errors.New(message)
}

type mcpPayablesInput struct {
	mcpPage
	Kind string `json:"kind,omitempty" jsonschema:"Optional debt or receivable; omitted returns both."`
}

type mcpPortfolioInput struct {
	From      string `json:"from,omitempty" jsonschema:"Inclusive YYYY-MM-DD; omitted means inception."`
	To        string `json:"to" jsonschema:"Inclusive YYYY-MM-DD; future dates are clamped to today."`
	GroupBy   string `json:"group_by,omitempty" jsonschema:"none (default), account, or class. Does not alter portfolio totals."`
	Source    string `json:"source,omitempty" jsonschema:"manual or synced; omitted measures both."`
	AccountID string `json:"account_id,omitempty" jsonschema:"Narrows displayed positions only; totals remain portfolio-wide."`
}

func mcpReference(ctx context.Context, conn *sql.DB, day time.Time) (valuationDTO, error) {
	code, err := settings.ReferenceCurrency(ctx, conn)
	if err != nil {
		return valuationDTO{}, err
	}
	rate, err := exchange.New(conn, nil).On(ctx, "BRL", code, day)
	return valuationDTO{CurrencyCode: code, FX: rate}, err
}

func mcpPortfolio(ctx context.Context, conn *sql.DB, in mcpPortfolioInput) (investmentPeriodDTO, error) {
	to, err := time.Parse(time.DateOnly, in.To)
	if err != nil {
		return investmentPeriodDTO{}, errMCPInput
	}
	today := investments.ProviderDay(time.Now())
	clamped := to.After(today)
	if clamped {
		to = today
	}
	q := investments.PortfolioQuery{To: to, GroupBy: investments.PortfolioGroupNone}
	if in.From != "" {
		from, err := time.Parse(time.DateOnly, in.From)
		if err != nil || from.After(to) {
			return investmentPeriodDTO{}, errMCPInput
		}
		q.From = &from
	}
	if in.GroupBy != "" {
		q.GroupBy = investments.PortfolioGroupBy(in.GroupBy)
	}
	if q.GroupBy != investments.PortfolioGroupNone && q.GroupBy != investments.PortfolioGroupAccount && q.GroupBy != investments.PortfolioGroupClass {
		return investmentPeriodDTO{}, errMCPInput
	}
	if in.Source != "" {
		source := investments.PositionSource(in.Source)
		if source != investments.PositionSourceManual && source != investments.PositionSourceSynced {
			return investmentPeriodDTO{}, errMCPInput
		}
		q.Source = &source
	}
	if in.AccountID != "" {
		q.AccountID = &in.AccountID
		if _, err := investments.GetAccount(ctx, conn, in.AccountID); err != nil {
			if errors.Is(err, investments.ErrNotFound) {
				return investmentPeriodDTO{}, errMCPInput
			}
			return investmentPeriodDTO{}, err
		}
	}
	reference, err := mcpReference(ctx, conn, to)
	if err != nil {
		return investmentPeriodDTO{}, err
	}
	p, err := investments.BuildPortfolio(ctx, conn, q)
	if err != nil {
		return investmentPeriodDTO{}, err
	}
	dto := investmentPeriodToDTO(p, reference)
	dto.ToClamped = clamped
	return dto, nil
}

type mcpWealthPoint struct {
	Date            string                `json:"date"`
	ValuationStatus string                `json:"valuation_status"`
	Valuation       *valuation.Metadata   `json:"valuation"`
	FX              *exchange.Rate        `json:"fx"`
	Breakdown       *netWorthBreakdownDTO `json:"breakdown"`
}
type mcpWealth struct {
	CurrencyCode string           `json:"currency_code"`
	Assumptions  string           `json:"assumptions"`
	Series       []mcpWealthPoint `json:"series"`
	Latest       mcpWealthPoint   `json:"latest"`
}

func mcpNetWorth(ctx context.Context, conn *sql.DB, in mcpRange) (mcpWealth, error) {
	from, to, err := in.days()
	if err != nil || to.After(dates.Day(time.Now())) {
		return mcpWealth{}, errMCPInput
	}
	points, err := networth.Series(ctx, conn, time.Now())
	if err != nil {
		return mcpWealth{}, err
	}
	code, err := settings.ReferenceCurrency(ctx, conn)
	if err != nil {
		return mcpWealth{}, err
	}
	out := mcpWealth{CurrencyCode: code, Series: []mcpWealthPoint{}, Assumptions: "Realized cash plus investments; excludes debts, receivables, credit cards and hypothetical data. Historical valuations use each day's FX. History starts at the first cash transaction; unavailable days have null breakdown."}
	fx := exchange.New(conn, nil)
	convert := func(p networth.Point) (mcpWealthPoint, error) {
		dto := mcpWealthPoint{Date: p.Day.Format(time.DateOnly), ValuationStatus: p.Status, Valuation: p.Valuation}
		if p.Status == networth.ValuationUnavailable {
			return dto, nil
		}
		rate, err := fx.On(ctx, "BRL", code, fxDay(p))
		if err != nil {
			return dto, err
		}
		dto.FX = &rate
		b := toNetWorthBreakdownDTO(p.Breakdown)
		valuationDTO{CurrencyCode: code, FX: rate}.breakdown(&b)
		dto.Breakdown = &b
		return dto, nil
	}
	for _, p := range points {
		if !p.Day.Before(from) && !p.Day.After(to) {
			dto, err := convert(p)
			if err != nil {
				return mcpWealth{}, err
			}
			out.Series = append(out.Series, dto)
		}
	}
	out.Latest, err = convert(points[len(points)-1])
	return out, err
}

// Explicit selection retains active commitments, but never implicitly includes
// a standalone hypothetical, even if it was marked active in the application.
func mcpScenarioIDs(ctx context.Context, conn *sql.DB, selected []string) ([]string, error) {
	if len(selected) > 20 {
		return nil, errMCPInput
	}
	list, err := scenarios.ListScenarios(ctx, conn, scenarios.ListFilter{})
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	included := map[string]bool{}
	ids := []string{}
	for _, s := range list {
		known[s.ID] = true
		if s.IsActive && s.Kind != scenarios.KindStandalone {
			ids = append(ids, s.ID)
			included[s.ID] = true
		}
	}
	for _, id := range selected {
		if !known[id] {
			return nil, errMCPInput
		}
		if !included[id] {
			ids = append(ids, id)
			included[id] = true
		}
	}
	return ids, nil
}

type mcpProjectionInput struct {
	mcpRange
	mcpPage
	ScenarioIDs []string `json:"scenario_ids,omitempty" jsonschema:"Explicit additional scenario IDs, maximum 20. Omitted or empty adds no hypotheticals."`
}
type mcpProjectionEvent struct {
	ScenarioID        string  `json:"scenario_id"`
	EventKey          string  `json:"event_key"`
	Kind              string  `json:"kind"`
	Date              string  `json:"date"`
	Description       string  `json:"description"`
	Amount            string  `json:"amount"`
	CategoryID        *string `json:"category_id"`
	Tier              string  `json:"tier"`
	Source            string  `json:"source"`
	PayableID         *string `json:"payable_id"`
	Realized          bool    `json:"realized"`
	RealizationOrigin string  `json:"realization_origin"`
	Detached          bool    `json:"detached"`
}

func mcpEvents(ctx context.Context, conn *sql.DB, in mcpProjectionInput) (mcpItems[mcpProjectionEvent], error) {
	from, to, err := in.days()
	if err != nil {
		return mcpItems[mcpProjectionEvent]{}, err
	}
	ids, err := mcpScenarioIDs(ctx, conn, in.ScenarioIDs)
	if err != nil {
		return mcpItems[mcpProjectionEvent]{}, err
	}
	list, err := projections.List(ctx, conn, projections.ProjectionQuery{From: from, To: to, Selection: projections.SelectionExplicit, IDs: ids})
	if err != nil {
		return mcpItems[mcpProjectionEvent]{}, err
	}
	items := make([]mcpProjectionEvent, len(list))
	for i, e := range list {
		items[i] = mcpProjectionEvent{e.ScenarioID, e.EventKey, string(e.ScenarioKind), e.Date.Format(time.DateOnly), e.Description, e.Amount.String(), e.CategoryID, string(e.Tier), string(e.Source), e.PayableID, e.Realized, e.RealizationOrigin, e.Detached}
	}
	return mcpPaginate(items, in.mcpPage, "Active non-standalone commitments plus explicit scenario_ids. Signed BRL amounts; realized events remain visible here but are suppressed in timeline projections.")
}

type mcpTimelineInput struct {
	mcpRange
	ReferenceDate string   `json:"reference_date" jsonschema:"Cash anchor date, YYYY-MM-DD, as in the application timeline."`
	Timezone      string   `json:"timezone,omitempty" jsonschema:"IANA timezone for transaction calendar days; default UTC."`
	ScenarioIDs   []string `json:"scenario_ids,omitempty" jsonschema:"Explicit additional scenario IDs, maximum 20; empty adds no hypotheticals."`
}
type mcpTimeline struct {
	valuationDTO
	ScenarioIDs  []string          `json:"scenario_ids"`
	Assumptions  string            `json:"assumptions"`
	Series       timelineSeriesDTO `json:"series"`
	PeriodTotals periodTotalsDTO   `json:"period_totals"`
}

func mcpBuildTimeline(ctx context.Context, conn *sql.DB, in mcpTimelineInput) (mcpTimeline, error) {
	from, to, err := in.days()
	if err != nil {
		return mcpTimeline{}, err
	}
	reference, err := time.Parse(time.DateOnly, in.ReferenceDate)
	if err != nil {
		return mcpTimeline{}, errMCPInput
	}
	if in.Timezone != "" {
		if _, err := time.LoadLocation(in.Timezone); err != nil {
			return mcpTimeline{}, errMCPInput
		}
	}
	ids, err := mcpScenarioIDs(ctx, conn, in.ScenarioIDs)
	if err != nil {
		return mcpTimeline{}, err
	}
	s, err := timeline.BuildSeries(ctx, conn, timeline.BuildParams{From: from, To: to, ReferenceDate: reference, Timezone: in.Timezone, ScenarioIDs: ids, ScenarioSelection: projections.SelectionExplicit})
	if err != nil {
		return mcpTimeline{}, err
	}
	v, err := mcpReference(ctx, conn, time.Now().UTC())
	if err != nil {
		return mcpTimeline{}, err
	}
	out := mcpTimeline{valuationDTO: v, ScenarioIDs: ids, Assumptions: "Active non-standalone commitments plus explicit scenario_ids. Cash anchored at reference_date; end-of-day points. Current reference-currency FX applies across the projection; not an FX forecast.", Series: timelineSeriesToDTO(s), PeriodTotals: periodTotalsToDTO(timeline.TotalsForPeriod(s))}
	out.PeriodTotals.Totals = s.Totals
	out.PeriodTotals.Reporting = &out.Series.Reporting
	v.series(&out.Series)
	v.fields(&out.PeriodTotals.Income, &out.PeriodTotals.Expense, &out.PeriodTotals.Result)
	return out, nil
}
