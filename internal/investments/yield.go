package investments

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

// Rendimento is derived from one formula for every holding, manual or
// synced, over any period:
//
//	Rendimento(de, até) = V(até) − V(de) − FluxoLíquido(de, até]
//
// V(d) is the holding's value at the end of day d, and FluxoLíquido the
// money that went into the holding (positive) or came out of it (negative)
// during the period, so what is left is what the holding earned. "Since
// inception" is the same formula with de on the day before the first
// purchase, where V is zero.
//
// For a synced holding V(d) = quantity(d) × the asset's price on d (its last
// quote on or before d, from investment_asset_quotes), which is the gross
// value — IR/IOF the institution provisions stay for the caller to deduct.
// For a manual one V(d) is the ledger replayed through d, valued at the last
// typed valuation — or, when its asset has a price series, at quantity(d) ×
// the price on d, unless a valuation as recent as that price was typed.

// Reasons a yield cannot be stated. They match the API's
// yield_unavailable_reason vocabulary.
const (
	YieldReasonNoHistory         = "sem_historico"
	YieldReasonIncompleteHistory = "historico_incompleto"
	YieldReasonNoPrice           = "saldo_indisponivel"
)

// YieldUnavailableError says why a yield was not computed.
type YieldUnavailableError struct{ Reason string }

func (e *YieldUnavailableError) Error() string { return "yield unavailable: " + e.Reason }

func unavailable(reason string) error { return &YieldUnavailableError{Reason: reason} }

// YieldResult is the rendimento of one holding over [From, To]. From and To
// are the days actually used: Partial reports that the requested start had
// no price and the period starts on the first day that does.
type YieldResult struct {
	Value      decimal.Decimal
	StartValue decimal.Decimal
	EndValue   decimal.Decimal
	NetFlows   decimal.Decimal
	From       time.Time
	To         time.Time
	Partial    bool
}

// DayYield is the rendimento of one day: V(Day) − V(Day−1) − flows on Day.
type DayYield struct {
	Day      time.Time
	Value    decimal.Decimal
	EndValue decimal.Decimal
	NetFlows decimal.Decimal
}

// valueSeries is a holding's value and cash flows over time.
type valueSeries interface {
	// value is V at the end of day d; false when no price covers d.
	value(d time.Time) (decimal.Decimal, bool)
	// flows sums the net money put in during (from, to].
	flows(from, to time.Time) decimal.Decimal
	// inception is the last day before the holding existed, where V is zero;
	// false when the history cannot reach back that far, and then
	// missingInception says why.
	inception() (time.Time, bool)
	missingInception() string
	// flowEvents are the dated money movements, sorted by day.
	flowEvents() []flowEvent
}

// PositionYield is the rendimento of a holding between from and to. A nil
// from means since inception.
func PositionYield(ctx context.Context, q Querier, positionID string, from *time.Time, to time.Time) (YieldResult, error) {
	return PositionYieldWith(ctx, q, nil, positionID, from, to)
}

// PositionYieldWith is PositionYield reading price series through cache, so
// the holdings of one request that share an asset load its series once. A nil
// cache loads every time.
func PositionYieldWith(ctx context.Context, q Querier, cache *YieldCache, positionID string, from *time.Time, to time.Time) (YieldResult, error) {
	to = Day(to)
	series, err := loadSeries(ctx, q, cache, positionID, to)
	if err != nil {
		return YieldResult{}, err
	}
	return yieldOver(series, from, to)
}

func yieldOver(series valueSeries, from *time.Time, to time.Time) (YieldResult, error) {
	var start time.Time
	if from == nil {
		inception, ok := series.inception()
		if !ok {
			return YieldResult{}, unavailable(series.missingInception())
		}
		start = inception
	} else {
		start = Day(*from)
	}
	if start.After(to) {
		return YieldResult{}, ErrInvalidInput
	}
	endValue, ok := series.value(to)
	if !ok {
		return YieldResult{}, unavailable(YieldReasonNoPrice)
	}
	result := YieldResult{To: to, EndValue: endValue}
	startValue, ok := series.value(start)
	for !ok {
		// No price on the requested start: begin on the first day that has
		// one rather than inventing a starting value.
		start = start.AddDate(0, 0, 1)
		if start.After(to) {
			return YieldResult{}, unavailable(YieldReasonNoPrice)
		}
		result.Partial = true
		startValue, ok = series.value(start)
	}
	result.From, result.StartValue = start, startValue
	result.NetFlows = series.flows(start, to)
	result.Value = endValue.Sub(startValue).Sub(result.NetFlows)
	return result, nil
}

// DailyYield is the rendimento of each day in (from, to] that has a price on
// both itself and the day before. Days without one are left out rather than
// reported as zero.
func DailyYield(ctx context.Context, q Querier, positionID string, from, to time.Time) ([]DayYield, error) {
	series, err := loadSeries(ctx, q, nil, positionID, Day(to))
	if err != nil {
		return nil, err
	}
	return dailyOver(series, Day(from), Day(to)), nil
}

func dailyOver(series valueSeries, from, to time.Time) []DayYield {
	days := []DayYield{}
	previous, havePrevious := series.value(from)
	for day := from.AddDate(0, 0, 1); !day.After(to); day = day.AddDate(0, 0, 1) {
		current, ok := series.value(day)
		if ok && havePrevious {
			flows := series.flows(day.AddDate(0, 0, -1), day)
			days = append(days, DayYield{
				Day: day, Value: current.Sub(previous).Sub(flows), EndValue: current, NetFlows: flows,
			})
		}
		previous, havePrevious = current, ok
	}
	return days
}

// loadSeries builds the value series of a holding. A synced holding's prices
// later than until are never read, so its series is loaded only through it.
func loadSeries(ctx context.Context, q Querier, cache *YieldCache, positionID string, until time.Time) (valueSeries, error) {
	var row syncedHoldingRow
	err := q.QueryRowContext(ctx, `SELECT `+syncedHoldingColumns+` FROM financial_investments fi WHERE fi.id = ?`, positionID).
		Scan(row.dests()...)
	switch {
	case err == nil:
		holding, err := row.holding()
		if err != nil {
			return nil, err
		}
		return loadSyncedSeries(ctx, q, cache, positionID, holding, until)
	case errors.Is(err, sql.ErrNoRows):
		return loadManualSeries(ctx, q, positionID)
	default:
		return nil, err
	}
}

// flowEvent is one dated change to a holding: units in (+) or out (−) and
// the money that moved with them, in (+) or out (−).
type flowEvent struct {
	day      time.Time
	quantity decimal.Decimal
	cash     decimal.Decimal
	inflow   bool
}

func sumFlows(events []flowEvent, from, to time.Time) decimal.Decimal {
	total := decimal.Zero
	for _, event := range events {
		if event.day.After(from) && !event.day.After(to) {
			total = total.Add(event.cash)
		}
	}
	return total
}

// maxQuoteAge is how long a quote keeps pricing the days after it: enough to
// bridge weekends, holidays and a few missed syncs. Carrying it further
// would let a title's purchase PU price it two years later and report the
// whole accrual as that period's rendimento.
const maxQuoteAge = 10 * 24 * time.Hour

// latestQuoteOn is the last quote on or before d, however old.
func latestQuoteOn(quotes []AssetQuote, d time.Time) (AssetQuote, bool) {
	i := sort.Search(len(quotes), func(i int) bool { return quotes[i].QuotedOn.After(d) })
	if i == 0 {
		return AssetQuote{}, false
	}
	return quotes[i-1], true
}

// quoteOn is the last quote on or before d, if recent enough to price d.
func quoteOn(quotes []AssetQuote, d time.Time) (AssetQuote, bool) {
	quote, ok := latestQuoteOn(quotes, d)
	if !ok || d.Sub(quote.QuotedOn) > maxQuoteAge {
		return AssetQuote{}, false
	}
	return quote, true
}

// priceOn is the price of quoteOn.
func priceOn(quotes []AssetQuote, d time.Time) (decimal.Decimal, bool) {
	quote, ok := quoteOn(quotes, d)
	return quote.Price, ok
}

// syncedSeries values a provider holding from its asset's quotes. The
// provider only states today's quantity, so quantity on an earlier day is
// rebuilt backwards: today's, minus what the movements after that day added.
type syncedSeries struct {
	// quantityKnown is false when the provider sent no quantity: the holding
	// then cannot be valued from a price at all.
	quantityKnown bool
	quantityNow   decimal.Decimal
	events        []flowEvent // sorted by day
	quotes        []AssetQuote
	firstInflow   *time.Time
	complete      bool
	// movements counts every movement the provider sent, including ones
	// that could not be placed and so are not among events.
	movements int
}

func loadSyncedSeries(ctx context.Context, q Querier, cache *YieldCache, id string, holding SyncedHolding, until time.Time) (*syncedSeries, error) {
	// Reading never creates the asset: a holding whose asset does not exist
	// yet has no quotes, which yieldOver reports as no price.
	var quotes []AssetQuote
	asset, found, err := FindSyncedAsset(ctx, q, holding)
	if err != nil {
		return nil, err
	}
	if found {
		if quotes, err = cache.assetQuotes(ctx, q, asset.ID, until); err != nil {
			return nil, err
		}
	}
	rows, err := q.QueryContext(ctx, `
		SELECT direction, quantity, amount, occurred_at, trade_date
		FROM financial_investment_transactions WHERE investment_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movements []syncedMovement
	for rows.Next() {
		var m syncedMovement
		if err := m.scan(rows); err != nil {
			return nil, err
		}
		movements = append(movements, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return newSyncedSeries(holding, quotes, movements)
}

// syncedMovement is one provider movement as stored, before it is placed.
type syncedMovement struct {
	direction, quantity, amount, occurred sql.NullString
	tradeDate                             *time.Time
}

// scan reads the columns direction, quantity, amount, occurred_at, trade_date.
func (m *syncedMovement) scan(row interface{ Scan(...any) error }) error {
	return row.Scan(&m.direction, dbtypes.Decimal(&m.quantity), dbtypes.Decimal(&m.amount), dbtypes.Time(&m.occurred), dbtypes.Date(&m.tradeDate))
}

// newSyncedSeries places a holding's movements over its price series.
func newSyncedSeries(holding SyncedHolding, quotes []AssetQuote, movements []syncedMovement) (*syncedSeries, error) {
	s := &syncedSeries{quotes: quotes, complete: true}
	if holding.Quantity != nil {
		s.quantityNow, s.quantityKnown = *holding.Quantity, true
	}

	soldQty, boughtQty := decimal.Zero, decimal.Zero
	for _, m := range movements {
		s.movements++
		inflow := m.direction.String == "inflow"
		amount, amountErr := decimal.NewFromString(m.amount.String)
		when := m.tradeDate
		if when == nil {
			var err error
			if when, err = dbtypes.ParseNullTime(m.occurred); err != nil {
				return nil, err
			}
		}
		// A movement that cannot be placed — no direction, no amount, no
		// date — leaves the history unable to account for the holding, the
		// same evidence rule the API applied before quotes existed.
		if !m.direction.Valid || (m.direction.String != "inflow" && m.direction.String != "outflow") ||
			!m.amount.Valid || amountErr != nil || when == nil {
			s.complete = false
			continue
		}
		event := flowEvent{day: MovementDay(*when), cash: amount, inflow: inflow}
		if !inflow {
			event.cash = amount.Neg()
		}
		// A movement without quantity (a dividend paid out) moves money,
		// not units.
		if quantity, err := decimal.NewFromString(m.quantity.String); m.quantity.Valid && err == nil {
			if inflow {
				event.quantity = quantity
				boughtQty = boughtQty.Add(quantity)
			} else {
				event.quantity = quantity.Neg()
				soldQty = soldQty.Add(quantity)
			}
		}
		s.events = append(s.events, event)
	}
	sort.SliceStable(s.events, func(i, j int) bool { return s.events[i].day.Before(s.events[j].day) })

	hasInflow := false
	for _, event := range s.events {
		hasInflow = hasInflow || event.inflow
	}
	// A fixed income title bought before the connection's window arrives
	// without its purchase. The provider still states the principal and the
	// purchase date, so the purchase is restored from them: the units held on
	// that day, bought for amountOriginal.
	if !hasInflow {
		if _, boughtOn, ok := holding.issuePrice(); ok {
			purchase := flowEvent{day: boughtOn, cash: *holding.AmountOriginal, inflow: true}
			purchase.quantity = s.quantityAt(boughtOn)
			s.events = append([]flowEvent{purchase}, s.events...)
			sort.SliceStable(s.events, func(i, j int) bool { return s.events[i].day.Before(s.events[j].day) })
			hasInflow = true
		}
	}
	if !hasInflow {
		s.complete = false
	}
	// Units sold can never exceed units bought. An excess means purchases
	// are missing (BBAS3: 120 sold against 80 bought). A CDB redeems slightly
	// more units than it bought as it accrues (50000.098 against 50000), so
	// fixed income is only flagged past a 1% excess — a redemption of a whole
	// title bought before the connection's window is far past that.
	excess := soldQty.Sub(boughtQty)
	switch {
	case !excess.IsPositive():
	case holding.isFixedIncome():
		if excess.GreaterThan(boughtQty.Div(decimal.NewFromInt(100))) {
			s.complete = false
		}
	default:
		s.complete = false
	}
	for _, event := range s.events {
		if event.inflow {
			day := event.day
			s.firstInflow = &day
			break
		}
	}
	return s, nil
}

// MovementDay is the calendar day of a provider movement. Pluggy sends
// movement dates as date-only values stored at UTC midnight; any other
// instant is read on the provider's wall clock like as-of dates are.
func MovementDay(t time.Time) time.Time {
	utc := t.UTC()
	if utc.Hour() == 0 && utc.Minute() == 0 && utc.Second() == 0 && utc.Nanosecond() == 0 {
		return Day(utc)
	}
	return ProviderDay(t)
}

// quantityAt is the units held at the end of day d.
func (s *syncedSeries) quantityAt(d time.Time) decimal.Decimal {
	quantity := s.quantityNow
	for _, event := range s.events {
		if event.day.After(d) {
			quantity = quantity.Sub(event.quantity)
		}
	}
	return quantity
}

func (s *syncedSeries) value(d time.Time) (decimal.Decimal, bool) {
	if s.complete && s.firstInflow != nil && d.Before(*s.firstInflow) {
		return decimal.Zero, true
	}
	if !s.quantityKnown {
		return decimal.Zero, false
	}
	quantity := s.quantityAt(d)
	if !quantity.IsPositive() {
		return decimal.Zero, true
	}
	price, ok := priceOn(s.quotes, d)
	if !ok {
		return decimal.Zero, false
	}
	return quantity.Mul(price).Round(2), true
}

func (s *syncedSeries) flows(from, to time.Time) decimal.Decimal { return sumFlows(s.events, from, to) }

func (s *syncedSeries) flowEvents() []flowEvent { return s.events }

func (s *syncedSeries) inception() (time.Time, bool) {
	if !s.complete || s.firstInflow == nil {
		return time.Time{}, false
	}
	return s.firstInflow.AddDate(0, 0, -1), true
}

// missingInception tells a holding with no movements at all apart from one
// whose movements do not reach back to its purchase.
func (s *syncedSeries) missingInception() string {
	if s.movements == 0 {
		return YieldReasonNoHistory
	}
	return YieldReasonIncompleteHistory
}

// manualSeries values a manual holding from its account's ledger. The ledger
// is complete by construction, so inception is always known.
type manualSeries struct {
	events []flowEvent
	// checkpoints is the holding's value after the last operation of each
	// day that has one; valued is false while no valuation priced it.
	checkpoints []manualCheckpoint
	// quotes is the asset's price series, loaded only when the asset has a
	// quote market configured: a holding the user asked to be priced
	// automatically is valued on any day from quantity × that day's price,
	// not only on the days a valuation operation happens to exist. A valuation
	// a person typed on the quote's day or later outranks the quote (see
	// quotedValue).
	quotes   []AssetQuote
	timeline quantityTimeline
}

type manualCheckpoint struct {
	day    time.Time
	value  decimal.Decimal
	valued bool
	// valuedOn is the day of the last typed valuation as of this checkpoint.
	valuedOn *time.Time
}

func loadManualSeries(ctx context.Context, q Querier, positionID string) (*manualSeries, error) {
	var accountID, assetID string
	var quoteSource sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT p.account_id, p.asset_id, a.quote_source
		FROM investment_positions p JOIN investment_assets a ON a.id = p.asset_id
		WHERE p.id = ?`, positionID).Scan(dbtypes.ID(&accountID), dbtypes.ID(&assetID), &quoteSource)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPositionNotFound
	}
	if err != nil {
		return nil, err
	}
	operations, cashless, err := accountOperations(ctx, q, accountID)
	if err != nil {
		return nil, err
	}
	var quotes []AssetQuote
	if quoteSource.Valid && quoteSource.String != "" {
		if quotes, err = ListAssetQuotes(ctx, q, assetID, nil); err != nil {
			return nil, err
		}
	}
	return newManualSeries(operations, cashless, positionID, quotes)
}

// accountOperations is the account's operations in replay order. An
// integrated custody only replays the ones that carry a position and never
// computes cash, exactly like replayAccountUntil.
func accountOperations(ctx context.Context, q Querier, accountID string) ([]Operation, bool, error) {
	account, err := getRawAccount(ctx, q, accountID)
	if err != nil {
		return nil, false, err
	}
	query := `SELECT ` + operationColumns + ` FROM investment_operations WHERE account_id = ?`
	cashless := account.Kind == AccountKindIntegrated
	if cashless {
		query += ` AND position_id IS NOT NULL`
	}
	rows, err := q.QueryContext(ctx, query+operationOrder, accountID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	operations := []Operation{}
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, false, err
		}
		operations = append(operations, op)
	}
	return operations, cashless, rows.Err()
}

// newManualSeries replays an account's operations and keeps the value of one
// holding after each day. quotes is the asset's price series when it has a
// quote market, nil otherwise.
func newManualSeries(operations []Operation, cashless bool, positionID string, quotes []AssetQuote) (*manualSeries, error) {
	s := &manualSeries{}
	ledger := newAccountLedger()
	ledger.cashless = cashless
	for i, op := range operations {
		if err := applyOperation(&ledger, op); err != nil {
			return nil, err
		}
		if op.PositionID != nil && *op.PositionID == positionID {
			if event, ok := manualFlow(op); ok {
				s.events = append(s.events, event)
			}
		}
		lastOfDay := i == len(operations)-1 || !operations[i+1].OccurredOn.Equal(op.OccurredOn)
		if !lastOfDay {
			continue
		}
		checkpoint := manualCheckpoint{day: Day(op.OccurredOn), valued: true}
		if state := ledger.positions[positionID]; state != nil && state.quantity.IsPositive() {
			var err error
			checkpoint.value, checkpoint.valued = state.valuedValue(), state.valuationTotal != nil
			if checkpoint.valuedOn, err = state.valuedDay(); err != nil {
				return nil, err
			}
		}
		s.checkpoints = append(s.checkpoints, checkpoint)
	}
	if len(quotes) > 0 {
		s.quotes = quotes
		s.timeline = newQuantityTimeline(s.events)
	}
	return s, nil
}

// manualFlow is the money an operation moved into (+) or out of (−) the
// holding. Buying costs the amount plus its fees and taxes; selling returns
// the amount net of them.
func manualFlow(op Operation) (flowEvent, bool) {
	event := flowEvent{day: Day(op.OccurredOn)}
	if op.Quantity != nil {
		event.quantity = *op.Quantity
	}
	switch op.Kind {
	case OperationInitialBalance, OperationBuy:
		event.cash, event.inflow = op.Amount.Add(op.Fees).Add(op.Taxes), true
	case OperationTransferIn:
		event.cash, event.inflow = op.Amount, true
	case OperationSell:
		event.cash, event.quantity = op.Amount.Sub(op.Fees).Sub(op.Taxes).Neg(), event.quantity.Neg()
	case OperationTransferOut:
		event.cash, event.quantity = op.Amount.Neg(), event.quantity.Neg()
	default:
		return flowEvent{}, false
	}
	return event, true
}

func (s *manualSeries) value(d time.Time) (decimal.Decimal, bool) {
	i := sort.Search(len(s.checkpoints), func(i int) bool { return s.checkpoints[i].day.After(d) })
	if i == 0 {
		return decimal.Zero, true
	}
	checkpoint := s.checkpoints[i-1]
	switch value, verdict := s.quotedValue(d, checkpoint); verdict {
	case quoteUsed:
		return value, true
	case quoteStale:
		return decimal.Zero, false
	}
	return checkpoint.value, checkpoint.valued
}

// quoteVerdict is what the asset's price series says about a day.
type quoteVerdict int

const (
	// quoteDeclined: the series does not price the day, and the ledger's own
	// valuation answers instead.
	quoteDeclined quoteVerdict = iota
	// quoteUsed: the day is priced at quantity × the quote.
	quoteUsed
	// quoteStale: a quote is the latest word on the price but too old to
	// price the day. The day is left unpriced rather than answered by a
	// valuation that is older than the quote.
	quoteStale
)

// quotedValue is the units held at the end of d times the asset's price,
// rounded to cents like a synced holding's. The latest statement about the
// price wins, and on the same day a valuation a person typed does: it
// declines when the asset has no price series, nothing was held, no quote
// precedes d, or the typed valuation is as recent as the latest quote. When
// the latest quote is more than maxQuoteAge old it prices nothing, and the
// day is stale. checkpoint is the ledger's state as of d.
func (s *manualSeries) quotedValue(d time.Time, checkpoint manualCheckpoint) (decimal.Decimal, quoteVerdict) {
	if len(s.quotes) == 0 {
		return decimal.Zero, quoteDeclined
	}
	held := s.timeline.at(d)
	if !held.IsPositive() {
		return decimal.Zero, quoteDeclined
	}
	quote, ok := latestQuoteOn(s.quotes, d)
	if !ok {
		return decimal.Zero, quoteDeclined
	}
	if checkpoint.valuedOn != nil && !checkpoint.valuedOn.Before(quote.QuotedOn) {
		return decimal.Zero, quoteDeclined
	}
	if d.Sub(quote.QuotedOn) > maxQuoteAge {
		return decimal.Zero, quoteStale
	}
	return held.Mul(quote.Price).Round(2), quoteUsed
}

func (s *manualSeries) flows(from, to time.Time) decimal.Decimal { return sumFlows(s.events, from, to) }

func (s *manualSeries) flowEvents() []flowEvent { return s.events }

func (s *manualSeries) inception() (time.Time, bool) {
	for _, event := range s.events {
		if event.inflow {
			return event.day.AddDate(0, 0, -1), true
		}
	}
	return time.Time{}, false
}

func (s *manualSeries) missingInception() string { return YieldReasonNoHistory }

// quantityTimeline is how many units a position held at the end of each day
// it moved, from its own operations: what a price on a later day multiplies.
type quantityTimeline struct {
	days []time.Time
	held []decimal.Decimal
}

func newQuantityTimeline(events []flowEvent) quantityTimeline {
	var timeline quantityTimeline
	running := decimal.Zero
	for _, event := range events {
		running = running.Add(event.quantity)
		if n := len(timeline.days); n > 0 && timeline.days[n-1].Equal(event.day) {
			timeline.held[n-1] = running
			continue
		}
		timeline.days = append(timeline.days, event.day)
		timeline.held = append(timeline.held, running)
	}
	return timeline
}

// at is the quantity held at the end of day d.
func (t quantityTimeline) at(d time.Time) decimal.Decimal {
	held := decimal.Zero
	for i, day := range t.days {
		if day.After(d) {
			break
		}
		held = t.held[i]
	}
	return held
}
