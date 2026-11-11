package investments

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

// SyncedMovementsSource is every movement of the provider's holdings: what the
// provider sent plus what the owner recorded for it. Readers that rebuild a
// holding's history select from it instead of the provider table alone.
const SyncedMovementsSource = `(
	SELECT investment_id, direction, quantity, amount, occurred_at, trade_date FROM financial_investment_transactions
	UNION ALL
	SELECT investment_id, direction, quantity, amount, NULL, trade_date FROM manual_investment_movements
)`

// ManualMovementKind uses the provider's movement vocabulary so a manual
// movement reads like the ones the sync stores.
type ManualMovementKind string

const (
	ManualMovementBuy      ManualMovementKind = "BUY"
	ManualMovementSell     ManualMovementKind = "SELL"
	ManualMovementInterest ManualMovementKind = "INTEREST"
)

func (k ManualMovementKind) Valid() bool {
	return k == ManualMovementBuy || k == ManualMovementSell || k == ManualMovementInterest
}

// direction is the side of the holding the money moves on: a purchase puts
// money in, a sale or a provento takes it out.
func (k ManualMovementKind) direction() string {
	if k == ManualMovementBuy {
		return "inflow"
	}
	return "outflow"
}

// ManualMovement is a movement the owner recorded for a provider holding the
// provider did not report. It is never written by the sync.
type ManualMovement struct {
	ID           string
	InvestmentID string
	Kind         ManualMovementKind
	Direction    string
	TradeDate    time.Time
	Quantity     *decimal.Decimal
	UnitPrice    *decimal.Decimal
	Amount       decimal.Decimal
	Notes        *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ManualMovementInput is the amount in reais, costs included, and the units
// moved. A provento carries no units.
type ManualMovementInput struct {
	InvestmentID string
	Kind         ManualMovementKind
	TradeDate    time.Time
	Quantity     *decimal.Decimal
	UnitPrice    *decimal.Decimal
	Amount       decimal.Decimal
	Notes        *string
}

const manualMovementColumns = `id, investment_id, movement_type, direction, trade_date, quantity, unit_price, amount, notes, created_at, updated_at`

func (in ManualMovementInput) validate(today time.Time) error {
	if !in.Kind.Valid() || in.TradeDate.IsZero() || Day(in.TradeDate).After(Day(today)) || !in.Amount.IsPositive() {
		return ErrInvalidInput
	}
	if in.Kind == ManualMovementInterest {
		return nil
	}
	if in.Quantity == nil || !in.Quantity.IsPositive() {
		return ErrInvalidInput
	}
	if in.UnitPrice != nil && !in.UnitPrice.IsPositive() {
		return ErrInvalidInput
	}
	return nil
}

func requireProviderInvestment(ctx context.Context, q Querier, id string) error {
	var found string
	err := q.QueryRowContext(ctx, `SELECT id FROM financial_investments WHERE id = ?`, id).Scan(dbtypes.ID(&found))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func CreateManualMovement(ctx context.Context, conn *sql.DB, in ManualMovementInput) (ManualMovement, error) {
	if err := in.validate(time.Now()); err != nil {
		return ManualMovement{}, err
	}
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	err := mutateLedgerTransaction(ctx, conn, func(tx *sql.Tx) error {
		if err := requireProviderInvestment(ctx, tx, in.InvestmentID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO manual_investment_movements (`+manualMovementColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, in.InvestmentID, string(in.Kind), in.Kind.direction(), formatDate(in.TradeDate),
			nullableDecimal(in.Quantity), nullableDecimal(in.UnitPrice), money.CanonicalDecimal(in.Amount),
			nullableString(trimOptional(in.Notes)), now, now)
		return err
	})
	if err != nil {
		return ManualMovement{}, err
	}
	return GetManualMovement(ctx, conn, id)
}

// UpdateManualMovement replaces the movement's data; the holding it belongs to
// cannot change.
func UpdateManualMovement(ctx context.Context, conn *sql.DB, id string, in ManualMovementInput) (ManualMovement, error) {
	if err := in.validate(time.Now()); err != nil {
		return ManualMovement{}, err
	}
	err := mutateLedgerTransaction(ctx, conn, func(tx *sql.Tx) error {
		current, err := GetManualMovement(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.InvestmentID != in.InvestmentID {
			return ErrInvalidInput
		}
		_, err = tx.ExecContext(ctx, `UPDATE manual_investment_movements
			SET movement_type = ?, direction = ?, trade_date = ?, quantity = ?, unit_price = ?, amount = ?, notes = ?, updated_at = ?
			WHERE id = ?`,
			string(in.Kind), in.Kind.direction(), formatDate(in.TradeDate), nullableDecimal(in.Quantity), nullableDecimal(in.UnitPrice),
			money.CanonicalDecimal(in.Amount), nullableString(trimOptional(in.Notes)), db.FormatTime(time.Now()), id)
		return err
	})
	if err != nil {
		return ManualMovement{}, err
	}
	return GetManualMovement(ctx, conn, id)
}

func DeleteManualMovement(ctx context.Context, conn *sql.DB, id string) error {
	result, err := conn.ExecContext(ctx, `DELETE FROM manual_investment_movements WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

func GetManualMovement(ctx context.Context, q Querier, id string) (ManualMovement, error) {
	movements, err := queryManualMovements(ctx, q, `WHERE id = ?`, id)
	if err != nil {
		return ManualMovement{}, err
	}
	if len(movements) == 0 {
		return ManualMovement{}, ErrNotFound
	}
	return movements[0], nil
}

// ListManualMovements returns the movements recorded for one provider holding,
// oldest first.
func ListManualMovements(ctx context.Context, q Querier, investmentID string) ([]ManualMovement, error) {
	return queryManualMovements(ctx, q, `WHERE investment_id = ?`, investmentID)
}

func queryManualMovements(ctx context.Context, q Querier, where string, args ...any) ([]ManualMovement, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+manualMovementColumns+` FROM manual_investment_movements `+where+` ORDER BY trade_date, created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ManualMovement
	for rows.Next() {
		var (
			m    ManualMovement
			kind string
		)
		if err := rows.Scan(dbtypes.ID(&m.ID), dbtypes.ID(&m.InvestmentID), &kind, &m.Direction, dbtypes.Date(&m.TradeDate),
			dbtypes.Decimal(&m.Quantity), dbtypes.Decimal(&m.UnitPrice), dbtypes.Decimal(&m.Amount), &m.Notes,
			dbtypes.Time(&m.CreatedAt), dbtypes.Time(&m.UpdatedAt)); err != nil {
			return nil, err
		}
		m.Kind = ManualMovementKind(kind)
		result = append(result, m)
	}
	return result, rows.Err()
}
