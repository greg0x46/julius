package investments

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// positionLedger is deliberately private: quantities, weighted costs and
// valuations are derived state. Persisting them would make a correction to a
// historical operation leave stale balances behind.
//
// It keeps totals, not unit figures: the cost basis is the exact sum paid and a
// valuation is the exact amount the user typed, together with the quantity it
// priced. Unit cost and unit quote are derived only for display, so a holding
// of 3 units bought for R$ 100 reports R$ 100 at cost rather than the
// 99.9999999999999999 that quantity × (100 ÷ 3) would give.
type positionLedger struct {
	quantity  decimal.Decimal
	totalCost decimal.Decimal // exact cost basis of the units currently held
	// valuationTotal is the last manual quote, preserved across trades, and
	// valuedQuantity the quantity it priced. The quote per unit is their ratio.
	valuationTotal *decimal.Decimal
	valuedQuantity decimal.Decimal
	valuedOn       *timeValue
}

// averageCost is the display-only unit cost. It is zero for an empty holding.
func (p *positionLedger) averageCost() decimal.Decimal {
	if !p.quantity.IsPositive() {
		return decimal.Zero
	}
	return p.totalCost.Div(p.quantity)
}

// costOf is the exact cost basis carried by quantity units of the holding:
// the whole totalCost when the whole quantity is asked for, a proportional
// share otherwise.
func (p *positionLedger) costOf(quantity decimal.Decimal) decimal.Decimal {
	if !p.quantity.IsPositive() || quantity.IsZero() {
		return decimal.Zero
	}
	if quantity.Equal(p.quantity) {
		return p.totalCost
	}
	return p.totalCost.Mul(quantity).Div(p.quantity)
}

// valuedValue is the value of the current quantity at the last manual quote:
// exactly the amount typed while the quantity is unchanged, otherwise that
// amount scaled to the quantity now held.
func (p *positionLedger) valuedValue() decimal.Decimal {
	if p.valuationTotal == nil || !p.valuedQuantity.IsPositive() {
		return decimal.Zero
	}
	if p.quantity.Equal(p.valuedQuantity) {
		return *p.valuationTotal
	}
	return p.valuationTotal.Mul(p.quantity).Div(p.valuedQuantity)
}

// valuationUnitPrice is the display-only quote per unit.
func (p *positionLedger) valuationUnitPrice() decimal.Decimal {
	if p.valuationTotal == nil || !p.valuedQuantity.IsPositive() {
		return decimal.Zero
	}
	return p.valuationTotal.Div(p.valuedQuantity)
}

// valuedDay is the day of the last manual valuation, nil while the holding
// was never valued.
func (p *positionLedger) valuedDay() (*time.Time, error) {
	if p.valuedOn == nil {
		return nil, nil
	}
	day, err := parseDate(p.valuedOn.value)
	if err != nil {
		return nil, err
	}
	return &day, nil
}

// removeUnits lowers the quantity and the proportional share of the cost
// basis, so what remains is still an exact total rather than a re-multiplied
// unit cost.
func (p *positionLedger) removeUnits(quantity decimal.Decimal) {
	remaining := p.quantity.Sub(quantity)
	p.totalCost = p.costOf(remaining)
	p.quantity = remaining
	if p.quantity.IsZero() {
		p.totalCost = decimal.Zero
	}
}

// timeValue keeps a date in the replay state without exporting an internal
// implementation detail to callers.
type timeValue struct{ value string }

type accountLedger struct {
	cash      decimal.Decimal
	positions map[string]*positionLedger
	// cashless marks an integrated custody: its cash is the provider's, so
	// local trades never move or validate it.
	cashless bool
}

func newAccountLedger() accountLedger {
	return accountLedger{positions: map[string]*positionLedger{}}
}

func (l *accountLedger) position(id string) *positionLedger {
	p := l.positions[id]
	if p == nil {
		p = &positionLedger{}
		l.positions[id] = p
	}
	return p
}

func validateOperationShape(op Operation) error {
	if !op.Kind.Valid() || op.AccountID == "" || op.OccurredOn.IsZero() || op.Amount.IsNegative() ||
		op.Fees.IsNegative() || op.Taxes.IsNegative() {
		return ErrInvalidInput
	}
	hasPosition := op.PositionID != nil && *op.PositionID != ""
	hasQuantity := op.Quantity != nil
	hasUnitPrice := op.UnitPrice != nil
	if hasQuantity && !op.Quantity.IsPositive() {
		return ErrInvalidInput
	}
	if hasUnitPrice && op.UnitPrice.IsNegative() {
		return ErrInvalidInput
	}

	noExtras := func() bool {
		return !hasQuantity && !hasUnitPrice && op.Fees.IsZero() && op.Taxes.IsZero()
	}
	valid := false
	switch op.Kind {
	case OperationInitialBalance:
		if hasPosition {
			valid = hasQuantity && op.Amount.GreaterThanOrEqual(decimal.Zero) && !op.Fees.IsNegative() && !op.Taxes.IsNegative()
			break
		}
		valid = !hasQuantity && !hasUnitPrice && op.Fees.IsZero() && op.Taxes.IsZero() && op.Amount.IsPositive()
	case OperationDeposit, OperationWithdrawal, OperationIncome, OperationFee, OperationTax:
		valid = !hasPosition && op.Amount.IsPositive() && noExtras()
	case OperationBuy, OperationSell:
		valid = hasPosition && hasQuantity && op.Amount.IsPositive()
	case OperationTransferOut, OperationTransferIn:
		valid = hasPosition && hasQuantity && op.TransferID != nil && op.Amount.GreaterThanOrEqual(decimal.Zero) && op.Fees.IsZero() && op.Taxes.IsZero()
	case OperationValuation:
		valid = hasPosition && op.Amount.GreaterThanOrEqual(decimal.Zero) && noExtras()
	}
	if !valid {
		return ErrInvalidInput
	}
	return nil
}

// replayAccount replays one account in stable event order. It is invoked
// inside every operation mutation transaction, so an invalid correction rolls
// the UPDATE/DELETE/INSERT back rather than leaving a partially changed book.
func replayAccount(ctx context.Context, q Querier, accountID string) (accountLedger, error) {
	return replayAccountUntil(ctx, q, accountID, nil)
}

// replayAccountUntil replays the account only through the given day, so a
// retroactive operation can read the book as it stood on its own date rather
// than as it stands today. A nil day replays the whole history.
func replayAccountUntil(ctx context.Context, q Querier, accountID string, until *time.Time) (accountLedger, error) {
	account, err := getRawAccount(ctx, q, accountID)
	if err != nil {
		return accountLedger{}, err
	}
	query := `SELECT ` + operationColumns + ` FROM investment_operations WHERE account_id = ?`
	args := []any{accountID}
	ledger := newAccountLedger()
	// An integrated custody only replays its manual holdings: local cash
	// annotations never increment or decrement provider balances.
	if account.Kind == AccountKindIntegrated {
		query += ` AND position_id IS NOT NULL`
		ledger.cashless = true
	}
	if until != nil {
		query += ` AND occurred_on <= ?`
		args = append(args, formatDate(*until))
	}
	rows, err := q.QueryContext(ctx, query+operationOrder, args...)
	if err != nil {
		return accountLedger{}, err
	}
	defer rows.Close()

	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return accountLedger{}, err
		}
		if err := applyOperation(&ledger, op); err != nil {
			return accountLedger{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return accountLedger{}, err
	}
	if ledger.cashless {
		ledger.cash = decimal.Zero
	}
	return ledger, nil
}

func applyOperation(ledger *accountLedger, op Operation) error {
	if err := validateOperationShape(op); err != nil {
		return fmt.Errorf("operation %s: %w", op.ID, err)
	}
	position := func() (*positionLedger, error) {
		if op.PositionID == nil || *op.PositionID == "" {
			return nil, ErrInvalidInput
		}
		return ledger.position(*op.PositionID), nil
	}
	addCost := func(p *positionLedger, quantity, totalCost decimal.Decimal) {
		p.quantity = p.quantity.Add(quantity)
		p.totalCost = p.totalCost.Add(totalCost)
	}

	switch op.Kind {
	case OperationInitialBalance:
		if op.PositionID == nil {
			ledger.cash = ledger.cash.Add(op.Amount)
			break
		}
		p, err := position()
		if err != nil {
			return err
		}
		addCost(p, *op.Quantity, op.Amount.Add(op.Fees).Add(op.Taxes))
	case OperationDeposit:
		ledger.cash = ledger.cash.Add(op.Amount)
	case OperationWithdrawal:
		ledger.cash = ledger.cash.Sub(op.Amount)
	case OperationBuy:
		p, err := position()
		if err != nil {
			return err
		}
		ledger.cash = ledger.cash.Sub(op.Amount.Add(op.Fees).Add(op.Taxes))
		addCost(p, *op.Quantity, op.Amount.Add(op.Fees).Add(op.Taxes))
	case OperationSell:
		p, err := position()
		if err != nil {
			return err
		}
		if p.quantity.LessThan(*op.Quantity) {
			return fmt.Errorf("operation %s: %w", op.ID, ErrNegativePosition)
		}
		p.removeUnits(*op.Quantity)
		ledger.cash = ledger.cash.Add(op.Amount.Sub(op.Fees).Sub(op.Taxes))
	case OperationTransferOut:
		p, err := position()
		if err != nil {
			return err
		}
		if p.quantity.LessThan(*op.Quantity) {
			return fmt.Errorf("operation %s: %w", op.ID, ErrNegativePosition)
		}
		p.removeUnits(*op.Quantity)
	case OperationTransferIn:
		p, err := position()
		if err != nil {
			return err
		}
		addCost(p, *op.Quantity, op.Amount)
	case OperationIncome:
		ledger.cash = ledger.cash.Add(op.Amount)
	case OperationFee, OperationTax:
		ledger.cash = ledger.cash.Sub(op.Amount)
	case OperationValuation:
		p, err := position()
		if err != nil {
			return err
		}
		if !p.quantity.IsPositive() {
			return ErrInvalidInput
		}
		total := op.Amount
		p.valuationTotal = &total
		p.valuedQuantity = p.quantity
		p.valuedOn = &timeValue{value: op.OccurredOn.Format(DateLayout)}
	}
	if !ledger.cashless && ledger.cash.IsNegative() {
		return fmt.Errorf("operation %s: %w", op.ID, ErrNegativeCash)
	}
	return nil
}

// scanOperation is shared by the write path and listing API. Operations are
// stored as exact decimal text; parse failures are surfaced instead of being
// silently rounded into a misleading ledger.
func scanOperation(row interface{ Scan(...any) error }) (Operation, error) {
	var (
		op                                                          Operation
		positionID, transferID, quantityRaw, unitPriceRaw, notesRaw sql.NullString
		occurredOnRaw, amountRaw, feesRaw, taxesRaw                 string
		createdAtRaw, updatedAtRaw                                  string
	)
	if err := row.Scan(&op.ID, &op.AccountID, &positionID, &transferID, &op.Kind, &occurredOnRaw, &amountRaw,
		&quantityRaw, &unitPriceRaw, &feesRaw, &taxesRaw, &notesRaw, &op.Source,
		&createdAtRaw, &updatedAtRaw); err != nil {
		return Operation{}, err
	}
	if positionID.Valid {
		v := positionID.String
		op.PositionID = &v
	}
	if transferID.Valid {
		v := transferID.String
		op.TransferID = &v
	}
	if notesRaw.Valid {
		v := notesRaw.String
		op.Notes = &v
	}
	var err error
	if op.OccurredOn, err = parseDate(occurredOnRaw); err != nil {
		return Operation{}, err
	}
	for _, item := range []struct {
		raw string
		dst *decimal.Decimal
	}{
		{amountRaw, &op.Amount}, {feesRaw, &op.Fees}, {taxesRaw, &op.Taxes},
	} {
		if *item.dst, err = decimal.NewFromString(item.raw); err != nil {
			return Operation{}, err
		}
	}
	if quantityRaw.Valid {
		v, err := decimal.NewFromString(quantityRaw.String)
		if err != nil {
			return Operation{}, err
		}
		op.Quantity = &v
	}
	if unitPriceRaw.Valid {
		v, err := decimal.NewFromString(unitPriceRaw.String)
		if err != nil {
			return Operation{}, err
		}
		op.UnitPrice = &v
	}
	if op.CreatedAt, err = parseTimestamp(createdAtRaw); err != nil {
		return Operation{}, err
	}
	if op.UpdatedAt, err = parseTimestamp(updatedAtRaw); err != nil {
		return Operation{}, err
	}
	op.IsEditable = op.Source == "manual" && op.TransferID == nil
	return op, nil
}
