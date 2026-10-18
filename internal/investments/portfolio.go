package investments

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/valuation"
)

// The period view of the investment book. For a period [from, to] it states,
// for every holding that existed at some point of it, the holding as of `to`
// and what it earned, by the same formula for manual and synced holdings:
//
//	Yield = V(to) − V(from−1) − NetFlows[from, to]
//	V(to) = V(from−1) + Contributions − Withdrawals + Yield        (identity)
//
// V(d) is the value at the end of day d (see yield.go), the opening value is
// the one at the end of the day before `from`, and a flow dated d moves the
// holding at the end of d. Nothing is stored: every figure is recomputed on
// read from the ledger replay, the movements and the price series.
//
// The percentage is Modified Dietz: Yield / (V0 + Σ F × w), where each flow
// weighs the share of the period left after it, w = (to − day) / (to − start).
// Aggregates (group, total) sum the components — V0, weighted flows, yield —
// and divide once; they never combine child percentages.
//
// A holding whose value cannot be stated is unavailable with a reason. It is
// counted, flagged and excluded from every sum, never taken as zero.
//
// Costs are counted once: a manual purchase's fees and taxes are inside the
// money that went in and a sale's are netted from the money that came out
// (manualFlow), so `Costs` is informational and never subtracted again. The
// yield of a synced holding is gross of the IR/IOF the institution provisions
// (`ProvisionedTaxes`, informational); a manual holding has no such provision.
//
// Foreign holdings: V0, V1 and every flow are converted with the same rate
// (one per currency, valued on `to`), so the identity holds in BRL and the
// yield excludes exchange variation inside the period.

// PortfolioGroupBy selects how the positions are subtotaled.
type PortfolioGroupBy string

const (
	PortfolioGroupNone    PortfolioGroupBy = "none"
	PortfolioGroupAccount PortfolioGroupBy = "account"
	PortfolioGroupClass   PortfolioGroupBy = "class"
)

// AssetClassUnclassified is the class of a position without a catalog asset.
const AssetClassUnclassified AssetClass = "unclassified"

// YieldReasonNoRate: the position is in a currency with no rate for the day.
const YieldReasonNoRate = "cambio_indisponivel"

// PortfolioQuery scopes a period view. Source narrows what is measured at all
// (totals included); AccountID only narrows which positions are listed.
type PortfolioQuery struct {
	// From is the first day of the period; nil means since each position's
	// inception.
	From      *time.Time
	To        time.Time
	Source    *PositionSource
	AccountID *string
	GroupBy   PortfolioGroupBy
}

// PortfolioNative keeps the amounts of a position in its own currency.
type PortfolioNative struct {
	StartValue    decimal.Decimal
	EndValue      decimal.Decimal
	Contributions decimal.Decimal
	Withdrawals   decimal.Decimal
	Yield         decimal.Decimal
	Invested      *decimal.Decimal
}

// PortfolioPosition is one holding over the period, in BRL unless Native.
type PortfolioPosition struct {
	ID           string
	Source       PositionSource
	AccountID    string
	AccountName  string
	AssetID      *string
	Name         string
	Ticker       *string
	AssetType    string
	AssetClass   AssetClass
	CurrencyCode string
	// Quantity is the units held at `to`; nil when the provider sent none.
	Quantity *decimal.Decimal
	// ClosedAsOfTo: nothing is held at the end of `to`; the position is kept
	// because it moved or was worth something inside the period.
	ClosedAsOfTo bool
	// UnavailableReason is empty when the period figures below are valid.
	UnavailableReason string

	// OpeningOn is the day whose end value opens the period; PartialStart says
	// it is later than requested because the requested day had no price.
	OpeningOn       time.Time
	PartialStart    bool
	HistoryComplete bool

	StartValue    decimal.Decimal
	EndValue      decimal.Decimal
	Contributions decimal.Decimal
	Withdrawals   decimal.Decimal
	Yield         decimal.Decimal
	// CapitalBase is the Dietz denominator over the position's own period.
	CapitalBase decimal.Decimal
	YieldRate   *decimal.Decimal
	// Share is the position's end value over the portfolio's; nil when
	// unavailable or when the portfolio's end value is not positive.
	Share  *decimal.Decimal
	Native *PortfolioNative

	// Invested is the cost of what is held at `to`; nil says why in
	// InvestedUnavailableReason.
	Invested                  *decimal.Decimal
	InvestedUnavailableReason string
	// Costs is the fees and taxes of the position's own operations in the
	// period (manual only), informational. ProvisionedTaxes is the IR/IOF the
	// provider provisions (synced only). ReportedProfit is the provider's own
	// profit figure in the native currency, for conference only.
	Costs            *decimal.Decimal
	ProvisionedTaxes *decimal.Decimal
	ReportedProfit   *decimal.Decimal
	ReportedAsOf     *time.Time

	openingBRL decimal.Decimal
	flowsBRL   []datedFlow
}

type datedFlow struct {
	day  time.Time
	cash decimal.Decimal
}

// PortfolioPart is the Dietz components of a subset of positions.
type PortfolioPart struct {
	StartValue    decimal.Decimal
	EndValue      decimal.Decimal
	Contributions decimal.Decimal
	Withdrawals   decimal.Decimal
	Yield         decimal.Decimal
	CapitalBase   decimal.Decimal
	YieldRate     *decimal.Decimal
}

// PortfolioTotals are the components of a set of positions, summed. Held and
// Closed split them by whether the position is still held at the end of `to`
// (the yield of what is held vs the realized result of what is closed); they
// add up to the totals.
type PortfolioTotals struct {
	Held   PortfolioPart
	Closed PortfolioPart

	StartValue    decimal.Decimal
	EndValue      decimal.Decimal
	Contributions decimal.Decimal
	Withdrawals   decimal.Decimal
	Yield         decimal.Decimal
	CapitalBase   decimal.Decimal
	YieldRate     *decimal.Decimal
	// Invested sums the positions whose cost is known; InvestedUnknown counts
	// the others.
	Invested         decimal.Decimal
	InvestedUnknown  int
	Costs            decimal.Decimal
	ProvisionedTaxes decimal.Decimal

	PositionsTotal        int
	PositionsUnavailable  int
	PositionsPartialStart int
}

// Partial says some position was left out of the sums.
func (t PortfolioTotals) Partial() bool { return t.PositionsUnavailable > 0 }

type PortfolioGroup struct {
	Key       string
	Label     string
	AccountID *string
	// AssetClass is set for class groups.
	AssetClass *AssetClass
	Totals     PortfolioTotals
	// Share is the group's end value over the portfolio's; nil when that is
	// not positive.
	Share     *decimal.Decimal
	Positions []PortfolioPosition
}

type Portfolio struct {
	From      *time.Time
	To        time.Time
	GroupBy   PortfolioGroupBy
	Source    *PositionSource
	AccountID *string
	Totals    PortfolioTotals
	Groups    []PortfolioGroup
	Valuation valuation.Metadata
}

// BuildPortfolio values foreign holdings with the rate of the day `to`.
func BuildPortfolio(ctx context.Context, q Querier, query PortfolioQuery) (Portfolio, error) {
	return BuildPortfolioValued(ctx, q, query, valuation.New(exchange.New(q, nil), query.To))
}

func BuildPortfolioValued(ctx context.Context, q Querier, query PortfolioQuery, valued *valuation.Valuation) (Portfolio, error) {
	if query.GroupBy == "" {
		query.GroupBy = PortfolioGroupNone
	}
	switch query.GroupBy {
	case PortfolioGroupNone, PortfolioGroupAccount, PortfolioGroupClass:
	default:
		return Portfolio{}, ErrInvalidInput
	}
	to := Day(query.To)
	var opening *time.Time
	if query.From != nil {
		from := Day(*query.From)
		if from.After(to) {
			return Portfolio{}, ErrInvalidInput
		}
		before := from.AddDate(0, 0, -1)
		opening = &before
	}

	filter := PositionFilter{IncludeClosed: true}
	if query.Source != nil {
		source := string(*query.Source)
		filter.Source = &source
	}
	listed, err := ListPositions(ctx, q, filter)
	if err != nil {
		return Portfolio{}, err
	}
	assets, err := ListAssets(ctx, q)
	if err != nil {
		return Portfolio{}, err
	}
	assetByID := make(map[string]Asset, len(assets))
	for _, asset := range assets {
		assetByID[asset.ID] = asset
	}
	accounts, err := listRawAccounts(ctx, q)
	if err != nil {
		return Portfolio{}, err
	}
	accountName := make(map[string]string, len(accounts))
	for _, account := range accounts {
		accountName[account.ID] = account.Name
	}

	loader := &portfolioLoader{
		q: q, cache: NewYieldCache(), to: to, assets: assetByID,
		books: map[string]*accountBook{},
	}
	for _, position := range listed {
		if position.Source == PositionSourceSynced {
			if err := loader.loadSynced(ctx); err != nil {
				return Portfolio{}, err
			}
			break
		}
	}

	positions := make([]*PortfolioPosition, 0, len(listed))
	for _, listedPosition := range listed {
		built, include, err := loader.position(ctx, valued, listedPosition, opening, query.From == nil, to)
		if err != nil {
			return Portfolio{}, err
		}
		if !include {
			continue
		}
		built.AccountName = accountName[built.AccountID]
		if built.AssetID != nil {
			if asset, ok := assetByID[*built.AssetID]; ok {
				built.AssetClass = asset.AssetClass
			}
		}
		if built.AssetClass == "" {
			built.AssetClass = AssetClassUnclassified
		}
		positions = append(positions, built)
	}

	portfolio := Portfolio{
		From: query.From, To: to, GroupBy: query.GroupBy, Source: query.Source, AccountID: query.AccountID,
		Totals: aggregatePositions(positions, opening, to), Groups: []PortfolioGroup{}, Valuation: valued.Metadata,
	}
	if portfolio.Totals.EndValue.IsPositive() {
		for _, p := range positions {
			if p.UnavailableReason == "" {
				share := p.EndValue.DivRound(portfolio.Totals.EndValue, 10)
				p.Share = &share
			}
		}
	}
	portfolio.Groups = groupPositions(positions, query, opening, to, portfolio.Totals.EndValue)
	return portfolio, nil
}

// accountBook is an account's operations and the ledger as it stood on `to`.
type accountBook struct {
	operations []Operation
	cashless   bool
	atTo       accountLedger
}

// portfolioLoader holds what the positions of one request share, so each
// account's operations, each asset's price series and the provider's holdings
// and movements are read once.
type portfolioLoader struct {
	q      Querier
	cache  *YieldCache
	to     time.Time
	assets map[string]Asset
	books  map[string]*accountBook

	holdings  map[string]syncedHoldingExtras
	movements map[string][]syncedMovement
}

type syncedHoldingExtras struct {
	holding        SyncedHolding
	taxes, taxes2  *decimal.Decimal
	reportedProfit *decimal.Decimal
}

func (l *portfolioLoader) loadSynced(ctx context.Context) error {
	l.holdings, l.movements = map[string]syncedHoldingExtras{}, map[string][]syncedMovement{}
	rows, err := l.q.QueryContext(ctx, `SELECT fi.id, `+syncedHoldingColumns+`, fi.taxes, fi.taxes2, fi.amount_profit FROM financial_investments fi`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id                          string
			row                         syncedHoldingRow
			taxes, taxes2, profitReport sql.NullString
		)
		if err := rows.Scan(append(append([]any{dbtypes.ID(&id)}, row.dests()...),
			dbtypes.Decimal(&taxes), dbtypes.Decimal(&taxes2), dbtypes.Decimal(&profitReport))...); err != nil {
			return err
		}
		holding, err := row.holding()
		if err != nil {
			return err
		}
		extras := syncedHoldingExtras{holding: holding}
		for _, item := range []struct {
			raw sql.NullString
			dst **decimal.Decimal
		}{{taxes, &extras.taxes}, {taxes2, &extras.taxes2}, {profitReport, &extras.reportedProfit}} {
			if !item.raw.Valid || item.raw.String == "" {
				continue
			}
			value, err := decimal.NewFromString(item.raw.String)
			if err != nil {
				return err
			}
			*item.dst = &value
		}
		l.holdings[id] = extras
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	moves, err := l.q.QueryContext(ctx, `
		SELECT investment_id, direction, quantity, amount, occurred_at, trade_date
		FROM financial_investment_transactions`)
	if err != nil {
		return err
	}
	defer moves.Close()
	for moves.Next() {
		var (
			id string
			m  syncedMovement
		)
		if err := moves.Scan(dbtypes.ID(&id), &m.direction, dbtypes.Decimal(&m.quantity), dbtypes.Decimal(&m.amount), dbtypes.Time(&m.occurred), dbtypes.Date(&m.tradeDate)); err != nil {
			return err
		}
		l.movements[id] = append(l.movements[id], m)
	}
	return moves.Err()
}

func (l *portfolioLoader) book(ctx context.Context, accountID string) (*accountBook, error) {
	if book, ok := l.books[accountID]; ok {
		return book, nil
	}
	operations, cashless, err := accountOperations(ctx, l.q, accountID)
	if err != nil {
		return nil, err
	}
	ledger := newAccountLedger()
	ledger.cashless = cashless
	for _, op := range operations {
		if op.OccurredOn.After(l.to) {
			break
		}
		if err := applyOperation(&ledger, op); err != nil {
			return nil, err
		}
	}
	book := &accountBook{operations: operations, cashless: cashless, atTo: ledger}
	l.books[accountID] = book
	return book, nil
}

// position measures one holding. include is false for a holding that did not
// exist inside the period: it neither held nor moved anything.
func (l *portfolioLoader) position(ctx context.Context, valued *valuation.Valuation, listed Position, opening *time.Time, sinceInception bool, to time.Time) (*PortfolioPosition, bool, error) {
	result := &PortfolioPosition{
		ID: listed.ID, Source: listed.Source, AccountID: listed.AccountID, AssetID: listed.AssetID,
		Name: listed.Name, Ticker: listed.Ticker, AssetType: listed.AssetType, CurrencyCode: valuation.Currency(listed.CurrencyCode),
		HistoryComplete: true,
	}

	var (
		series   valueSeries
		quantity *decimal.Decimal
		invested *decimal.Decimal
		extras   syncedHoldingExtras
		book     *accountBook
	)
	switch listed.Source {
	case PositionSourceManual:
		var err error
		if book, err = l.book(ctx, listed.AccountID); err != nil {
			return nil, false, err
		}
		var quotes []AssetQuote
		if listed.AssetID != nil {
			if asset := l.assets[*listed.AssetID]; asset.QuoteSource != nil && *asset.QuoteSource != "" {
				if quotes, err = l.cache.assetQuotes(ctx, l.q, asset.ID, to); err != nil {
					return nil, false, err
				}
			}
		}
		if series, err = newManualSeries(book.operations, book.cashless, listed.ID, quotes); err != nil {
			return nil, false, err
		}
		held, cost := decimal.Zero, decimal.Zero
		if state := book.atTo.positions[listed.ID]; state != nil {
			held, cost = state.quantity, state.totalCost
		}
		quantity, invested = &held, &cost
	default:
		var ok bool
		if extras, ok = l.holdings[listed.ID]; !ok {
			return nil, false, ErrPositionNotFound
		}
		var quotes []AssetQuote
		if listed.AssetID != nil {
			var err error
			if quotes, err = l.cache.assetQuotes(ctx, l.q, *listed.AssetID, to); err != nil {
				return nil, false, err
			}
		}
		synced, err := newSyncedSeries(extras.holding, quotes, l.movements[listed.ID])
		if err != nil {
			return nil, false, err
		}
		series = synced
		result.HistoryComplete = synced.complete
		if synced.quantityKnown {
			held := synced.quantityAt(to)
			quantity = &held
		}
		if synced.complete && synced.firstInflow != nil {
			net := decimal.Zero
			if quantity == nil || quantity.IsPositive() {
				for _, event := range synced.events {
					if !event.day.After(to) {
						net = net.Add(event.cash)
					}
				}
			}
			invested = &net
		} else {
			result.InvestedUnavailableReason = synced.missingInception()
		}
		result.ReportedProfit = extras.reportedProfit
		if extras.holding.AsOfDate != nil {
			asOf := ProviderDay(*extras.holding.AsOfDate)
			result.ReportedAsOf = &asOf
			if extras.taxes != nil || extras.taxes2 != nil {
				if !asOf.After(to) && (quantity == nil || quantity.IsPositive()) {
					provisioned := decimal.Zero
					for _, tax := range []*decimal.Decimal{extras.taxes, extras.taxes2} {
						if tax != nil {
							provisioned = provisioned.Add(*tax)
						}
					}
					result.ProvisionedTaxes = &provisioned
				}
			}
		}
	}
	result.Quantity = quantity

	if book != nil && len(series.flowEvents()) == 0 {
		// A manual holding opened empty has nothing to measure yet.
		return nil, false, nil
	}
	if sinceInception {
		// A holding whose first purchase is after `to` did not exist yet.
		if inception, ok := series.inception(); ok && !inception.Before(to) {
			return nil, false, nil
		}
	}
	period, err := yieldOver(series, opening, to)
	var reason *YieldUnavailableError
	switch {
	case errors.As(err, &reason):
		result.UnavailableReason = reason.Reason
		result.ProvisionedTaxes = nil
		result.Invested = convertOrNil(ctx, valued, invested, result.CurrencyCode)
		return result, true, nil
	case err != nil:
		return nil, false, err
	}

	var flows []flowEvent
	for _, event := range series.flowEvents() {
		if event.day.After(period.From) && !event.day.After(to) {
			flows = append(flows, event)
		}
	}
	if period.StartValue.IsZero() && period.EndValue.IsZero() && len(flows) == 0 {
		return nil, false, nil
	}

	result.OpeningOn, result.PartialStart = period.From, period.Partial
	native := &PortfolioNative{StartValue: period.StartValue, EndValue: period.EndValue}
	for _, event := range flows {
		if event.cash.IsPositive() {
			native.Contributions = native.Contributions.Add(event.cash)
		} else {
			native.Withdrawals = native.Withdrawals.Add(event.cash.Neg())
		}
	}
	native.Yield = period.EndValue.Sub(period.StartValue).Sub(native.Contributions).Add(native.Withdrawals)
	native.Invested = invested
	result.Native = native
	result.ClosedAsOfTo = period.EndValue.IsZero() && (quantity == nil || !quantity.IsPositive())

	if book != nil {
		costs := decimal.Zero
		for _, op := range book.operations {
			if op.PositionID == nil || *op.PositionID != listed.ID || op.OccurredOn.After(to) || !op.OccurredOn.After(period.From) {
				continue
			}
			switch op.Kind {
			case OperationInitialBalance, OperationBuy, OperationSell:
				costs = costs.Add(op.Fees).Add(op.Taxes)
			}
		}
		result.Costs = &costs
	}

	// Every figure goes to BRL at the same rate, then the yield is derived from
	// the converted parts so the identity is exact in BRL too.
	convert := func(amount decimal.Decimal) (decimal.Decimal, error) {
		return valued.Convert(ctx, amount, result.CurrencyCode)
	}
	var convErr error
	conv := func(amount decimal.Decimal) decimal.Decimal {
		value, err := convert(amount)
		if err != nil && convErr == nil {
			convErr = err
		}
		return value
	}
	result.StartValue, result.EndValue = conv(period.StartValue), conv(period.EndValue)
	result.openingBRL = result.StartValue
	for _, event := range flows {
		cash := conv(event.cash)
		result.flowsBRL = append(result.flowsBRL, datedFlow{day: event.day, cash: cash})
		if cash.IsPositive() {
			result.Contributions = result.Contributions.Add(cash)
		} else {
			result.Withdrawals = result.Withdrawals.Add(cash.Neg())
		}
	}
	result.Invested = convertOrNil(ctx, valued, invested, result.CurrencyCode)
	if result.ProvisionedTaxes != nil {
		taxes := conv(*result.ProvisionedTaxes)
		result.ProvisionedTaxes = &taxes
	}
	if convErr != nil {
		if !errors.Is(convErr, valuation.ErrMissingRate) {
			return nil, false, convErr
		}
		return &PortfolioPosition{
			ID: result.ID, Source: result.Source, AccountID: result.AccountID, AssetID: result.AssetID, Name: result.Name,
			Ticker: result.Ticker, AssetType: result.AssetType, CurrencyCode: result.CurrencyCode, Quantity: result.Quantity,
			HistoryComplete: result.HistoryComplete, UnavailableReason: YieldReasonNoRate,
			InvestedUnavailableReason: YieldReasonNoRate, ReportedProfit: result.ReportedProfit, ReportedAsOf: result.ReportedAsOf,
		}, true, nil
	}
	result.Yield = result.EndValue.Sub(result.StartValue).Sub(result.Contributions).Add(result.Withdrawals)
	result.CapitalBase, result.YieldRate = dietz([]*PortfolioPosition{result}, result.OpeningOn, to)
	return result, true, nil
}

// convertOrNil converts an optional amount; a missing rate leaves it unknown.
func convertOrNil(ctx context.Context, valued *valuation.Valuation, amount *decimal.Decimal, currency string) *decimal.Decimal {
	if amount == nil {
		return nil
	}
	converted, err := valued.Convert(ctx, *amount, currency)
	if err != nil {
		return nil
	}
	return &converted
}

// dietz returns the Modified Dietz capital base and rate of positions over
// the period [start, to]: the opening value weighs the days from its own
// opening day, each flow the days left after it. The rate is nil when the
// base is not positive.
func dietz(positions []*PortfolioPosition, start, to time.Time) (decimal.Decimal, *decimal.Decimal) {
	periodDays := int64(to.Sub(start) / (24 * time.Hour))
	if periodDays <= 0 {
		periodDays = 1
	}
	weighted, yield := decimal.Zero, decimal.Zero
	for _, p := range positions {
		remaining := int64(to.Sub(p.OpeningOn) / (24 * time.Hour))
		if to.Sub(start) <= 0 {
			remaining = 1
		}
		weighted = weighted.Add(p.openingBRL.Mul(decimal.NewFromInt(remaining)))
		for _, flow := range p.flowsBRL {
			left := int64(to.Sub(flow.day) / (24 * time.Hour))
			weighted = weighted.Add(flow.cash.Mul(decimal.NewFromInt(left)))
		}
		yield = yield.Add(p.Yield)
	}
	days := decimal.NewFromInt(periodDays)
	base := weighted.DivRound(days, 10)
	if !weighted.IsPositive() {
		return base, nil
	}
	rate := yield.Mul(days).DivRound(weighted, 10)
	return base, &rate
}

// aggregatePositions sums the components of the available positions. opening
// is the requested opening day; nil (since inception) uses the earliest day a
// position opens on.
func aggregatePositions(positions []*PortfolioPosition, opening *time.Time, to time.Time) PortfolioTotals {
	var totals PortfolioTotals
	var available []*PortfolioPosition
	for _, p := range positions {
		totals.PositionsTotal++
		if p.UnavailableReason != "" {
			totals.PositionsUnavailable++
			continue
		}
		available = append(available, p)
		if p.PartialStart {
			totals.PositionsPartialStart++
		}
		totals.StartValue = totals.StartValue.Add(p.StartValue)
		totals.EndValue = totals.EndValue.Add(p.EndValue)
		totals.Contributions = totals.Contributions.Add(p.Contributions)
		totals.Withdrawals = totals.Withdrawals.Add(p.Withdrawals)
		totals.Yield = totals.Yield.Add(p.Yield)
		if p.Invested != nil {
			totals.Invested = totals.Invested.Add(*p.Invested)
		} else {
			totals.InvestedUnknown++
		}
		if p.Costs != nil {
			totals.Costs = totals.Costs.Add(*p.Costs)
		}
		if p.ProvisionedTaxes != nil {
			totals.ProvisionedTaxes = totals.ProvisionedTaxes.Add(*p.ProvisionedTaxes)
		}
	}
	if len(available) == 0 {
		return totals
	}
	start := available[0].OpeningOn
	if opening != nil {
		start = *opening
	} else {
		for _, p := range available {
			if p.OpeningOn.Before(start) {
				start = p.OpeningOn
			}
		}
	}
	var held, closed []*PortfolioPosition
	for _, p := range available {
		if p.ClosedAsOfTo {
			closed = append(closed, p)
		} else {
			held = append(held, p)
		}
	}
	totals.Held, totals.Closed = portfolioPart(held, start, to), portfolioPart(closed, start, to)
	// The base is the parts' sum so that held + closed equals the total; the
	// rate divides the exact weighted capital.
	totals.CapitalBase = totals.Held.CapitalBase.Add(totals.Closed.CapitalBase)
	_, totals.YieldRate = dietz(available, start, to)
	return totals
}

func portfolioPart(positions []*PortfolioPosition, start, to time.Time) PortfolioPart {
	var part PortfolioPart
	for _, p := range positions {
		part.StartValue = part.StartValue.Add(p.StartValue)
		part.EndValue = part.EndValue.Add(p.EndValue)
		part.Contributions = part.Contributions.Add(p.Contributions)
		part.Withdrawals = part.Withdrawals.Add(p.Withdrawals)
		part.Yield = part.Yield.Add(p.Yield)
	}
	if len(positions) > 0 {
		part.CapitalBase, part.YieldRate = dietz(positions, start, to)
	}
	return part
}

func groupPositions(positions []*PortfolioPosition, query PortfolioQuery, opening *time.Time, to time.Time, portfolioEnd decimal.Decimal) []PortfolioGroup {
	classLabels := map[AssetClass]string{AssetClassUnclassified: "Sem classe"}
	for _, definition := range AssetClassification() {
		classLabels[definition.Class] = definition.Label
	}

	type bucket struct {
		group     PortfolioGroup
		positions []*PortfolioPosition
	}
	buckets := map[string]*bucket{}
	var order []string
	for _, p := range positions {
		if query.AccountID != nil && p.AccountID != *query.AccountID {
			continue
		}
		var key, label string
		var accountID *string
		var class *AssetClass
		switch query.GroupBy {
		case PortfolioGroupAccount:
			id := p.AccountID
			key, label, accountID = id, p.AccountName, &id
		case PortfolioGroupClass:
			c := p.AssetClass
			key, label, class = string(c), classLabels[c], &c
			if label == "" {
				label = string(c)
			}
		default:
			key, label = "all", "Carteira"
		}
		b := buckets[key]
		if b == nil {
			b = &bucket{group: PortfolioGroup{Key: key, Label: label, AccountID: accountID, AssetClass: class}}
			buckets[key] = b
			order = append(order, key)
		}
		b.positions = append(b.positions, p)
	}

	groups := make([]PortfolioGroup, 0, len(order))
	for _, key := range order {
		b := buckets[key]
		b.group.Totals = aggregatePositions(b.positions, opening, to)
		if portfolioEnd.IsPositive() {
			share := b.group.Totals.EndValue.DivRound(portfolioEnd, 10)
			b.group.Share = &share
		}
		sort.SliceStable(b.positions, func(i, j int) bool {
			if !b.positions[i].EndValue.Equal(b.positions[j].EndValue) {
				return b.positions[i].EndValue.GreaterThan(b.positions[j].EndValue)
			}
			if b.positions[i].Name != b.positions[j].Name {
				return b.positions[i].Name < b.positions[j].Name
			}
			return b.positions[i].ID < b.positions[j].ID
		})
		for _, p := range b.positions {
			b.group.Positions = append(b.group.Positions, *p)
		}
		groups = append(groups, b.group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if !groups[i].Totals.EndValue.Equal(groups[j].Totals.EndValue) {
			return groups[i].Totals.EndValue.GreaterThan(groups[j].Totals.EndValue)
		}
		return groups[i].Key < groups[j].Key
	})
	return groups
}
