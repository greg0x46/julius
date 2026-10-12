package investments

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
)

// ErrOperationHasReconciliations refuses to drop an operation a bank line is
// still explained by: the user has to undo the link first, so the bank
// movement goes back to ordinary reporting deliberately rather than as a side
// effect of deleting the ledger entry.
var ErrOperationHasReconciliations = errors.New("investment operation has reconciliations")

// operationColumns is the column order scanOperation expects.
const operationColumns = `
	id, account_id, position_id, transfer_id, kind, occurred_on, amount, quantity,
	unit_price, fees, taxes, notes, source, created_at, updated_at`

// operationOrder is the replay order. Listing shares it so what the user
// reads back is the same sequence the ledger was computed from.
const operationOrder = ` ORDER BY occurred_on, created_at, id`

type OperationFilter struct {
	AccountID           *string
	PositionID          *string
	Source              *string
	SourceID            *string
	PortfolioID         *string
	ReconciliationState *string
	From, To            *time.Time
}

func ListOperations(ctx context.Context, q Querier, filter OperationFilter) ([]Operation, error) {
	clauses := []string{}
	args := []any{}
	if filter.AccountID != nil {
		clauses = append(clauses, `account_id = ?`)
		args = append(args, *filter.AccountID)
	}
	if filter.PositionID != nil {
		clauses = append(clauses, `position_id = ?`)
		args = append(args, *filter.PositionID)
	}
	if filter.From != nil {
		clauses = append(clauses, `occurred_on >= ?`)
		args = append(args, formatDate(*filter.From))
	}
	if filter.To != nil {
		clauses = append(clauses, `occurred_on <= ?`)
		args = append(args, formatDate(*filter.To))
	}
	if filter.Source != nil {
		clauses = append(clauses, `source = ?`)
		args = append(args, *filter.Source)
	}
	if filter.SourceID != nil {
		clauses = append(clauses, `account_id IN (SELECT id FROM investment_accounts WHERE source_id = ?)`)
		args = append(args, *filter.SourceID)
	}
	if filter.PortfolioID != nil {
		clauses = append(clauses, `position_id IN (SELECT manual_position_id FROM investment_position_portfolios WHERE portfolio_id = ?)`)
		args = append(args, *filter.PortfolioID)
	}
	if filter.ReconciliationState != nil {
		switch *filter.ReconciliationState {
		case "linked":
			clauses = append(clauses, `EXISTS (SELECT 1 FROM investment_reconciliations r WHERE r.operation_id = investment_operations.id)`)
		case "unlinked":
			clauses = append(clauses, `NOT EXISTS (SELECT 1 FROM investment_reconciliations r WHERE r.operation_id = investment_operations.id)`)
		default:
			return nil, ErrInvalidInput
		}
	}
	query := `SELECT ` + operationColumns + ` FROM investment_operations`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	rows, err := q.QueryContext(ctx, query+operationOrder, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Operation{}
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, rows.Err()
}

func GetOperation(ctx context.Context, q Querier, id string) (Operation, error) {
	op, err := scanOperation(q.QueryRowContext(ctx, `SELECT `+operationColumns+` FROM investment_operations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Operation{}, ErrOperationNotFound
	}
	return op, err
}

// requirePositionOfAccount keeps an operation inside the book it belongs to.
// investment_positions holds manual holdings only, so a provider-synced
// holding simply is not there — it is reported as non-manual rather than as a
// typo, because no local operation can ever apply to it.
func requirePositionOfAccount(ctx context.Context, q Querier, accountID string, positionID *string) error {
	if positionID == nil || *positionID == "" {
		return nil
	}
	var owner string
	err := q.QueryRowContext(ctx, `SELECT account_id FROM investment_positions WHERE id = ?`, *positionID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		var synced bool
		if err := q.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM financial_investments WHERE id = ?)`, *positionID).Scan(&synced); err != nil {
			return err
		}
		if synced {
			return ErrNotManual
		}
		return ErrInvalidInput
	}
	if err != nil {
		return err
	}
	if owner != accountID {
		return ErrInvalidInput
	}
	return nil
}

// mutateLedger applies one write and replays every account it touched in the
// same transaction. The replay is the only judge of whether the resulting
// history is valid, so a correction that would make cash or a position go
// negative on any later day rolls the write back instead of being half
// applied.
func mutateLedger(ctx context.Context, conn *sql.DB, mutate func(tx *sql.Tx) ([]string, error)) error {
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	accountIDs, err := mutate(tx)
	if err != nil {
		return err
	}
	replayed := map[string]bool{}
	for _, accountID := range accountIDs {
		if replayed[accountID] {
			continue
		}
		replayed[accountID] = true
		if _, err := replayAccount(ctx, tx, accountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func CreateOperation(ctx context.Context, conn *sql.DB, in OperationInput) (Operation, error) {
	if !in.Kind.Valid() {
		return Operation{}, ErrInvalidInput
	}
	operations, err := CreateOperations(ctx, conn, []OperationInput{in}, nil)
	if err != nil {
		return Operation{}, err
	}
	return operations[0], nil
}

// CreateTransfer moves units between two custody positions of the same asset.
// The paired ledger entries share an id and carry the source cost basis; cash
// is untouched, so a custody move can never masquerade as a sale and purchase.
//
// The cost basis is the source holding's as of the transfer date, not as of
// today: a transfer dated before a later purchase carries the earlier cost.
// It is persisted on the transfer_in, so a later correction to the source
// purchases does not propagate to the destination.
func CreateTransfer(ctx context.Context, conn *sql.DB, in TransferInput) ([]Operation, error) {
	if in.SourcePositionID == "" || in.DestinationPositionID == "" || in.SourcePositionID == in.DestinationPositionID || !in.Quantity.IsPositive() || in.OccurredOn.IsZero() {
		return nil, ErrInvalidInput
	}
	ids := []string{uuid.NewString(), uuid.NewString()}
	err := mutateLedger(ctx, conn, func(tx *sql.Tx) ([]string, error) {
		var sourceAccount, destinationAccount, sourceAsset, destinationAsset string
		if err := tx.QueryRowContext(ctx, `SELECT account_id, asset_id FROM investment_positions WHERE id = ?`, in.SourcePositionID).Scan(&sourceAccount, &sourceAsset); errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPositionNotFound
		} else if err != nil {
			return nil, err
		}
		if err := tx.QueryRowContext(ctx, `SELECT account_id, asset_id FROM investment_positions WHERE id = ?`, in.DestinationPositionID).Scan(&destinationAccount, &destinationAsset); errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPositionNotFound
		} else if err != nil {
			return nil, err
		}
		if sourceAsset != destinationAsset || sourceAccount == destinationAccount {
			return nil, ErrInvalidInput
		}
		if _, err := requireManualAccount(ctx, tx, sourceAccount); err != nil {
			return nil, err
		}
		if _, err := requireManualAccount(ctx, tx, destinationAccount); err != nil {
			return nil, err
		}
		occurredOn := Day(in.OccurredOn)
		ledger, err := replayAccountUntil(ctx, tx, sourceAccount, &occurredOn)
		if err != nil {
			return nil, err
		}
		state := ledger.position(in.SourcePositionID)
		if state.quantity.LessThan(in.Quantity) {
			return nil, ErrNegativePosition
		}
		cost := state.costOf(in.Quantity)
		unitCost := state.averageCost()
		transferID, now := uuid.NewString(), db.FormatTime(time.Now())
		for index, row := range []struct {
			account, position string
			kind              OperationKind
		}{{sourceAccount, in.SourcePositionID, OperationTransferOut}, {destinationAccount, in.DestinationPositionID, OperationTransferIn}} {
			_, err := tx.ExecContext(ctx, `INSERT INTO investment_operations (id, account_id, position_id, transfer_id, kind, occurred_on, amount, quantity, unit_price, fees, taxes, notes, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '0', '0', ?, 'manual', ?, ?)`, ids[index], row.account, row.position, transferID, string(row.kind), formatDate(in.OccurredOn), money.CanonicalDecimal(cost), money.CanonicalDecimal(in.Quantity), money.CanonicalDecimal(unitCost), nullableString(trimOptional(in.Notes)), now, now)
			if err != nil {
				return nil, err
			}
		}
		return []string{sourceAccount, destinationAccount}, nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]Operation, 0, 2)
	for _, id := range ids {
		op, err := GetOperation(ctx, conn, id)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, nil
}

// CreateOperations commits a compound action and its optional bank link as one
// transaction. A failed purchase or reconciliation leaves no orphan deposit.
func CreateOperations(ctx context.Context, conn *sql.DB, inputs []OperationInput, link *ReconciliationInput) ([]Operation, error) {
	if len(inputs) == 0 || len(inputs) > 100 {
		return nil, ErrInvalidInput
	}
	ids := make([]string, 0, len(inputs))
	err := mutateLedger(ctx, conn, func(tx *sql.Tx) ([]string, error) {
		accounts := []string{}
		for _, in := range inputs {
			if err := validateOperationAccount(ctx, tx, in); err != nil {
				return nil, err
			}
			if err := requirePositionOfAccount(ctx, tx, in.AccountID, in.PositionID); err != nil {
				return nil, err
			}
			id := uuid.NewString()
			now := db.FormatTime(time.Now())
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operations (
    id, account_id, position_id, kind, occurred_on, amount, quantity, unit_price,
    fees, taxes, notes, source, created_at, updated_at
   ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'manual', ?, ?)`,
				id, in.AccountID, nullableString(in.PositionID), string(in.Kind), formatDate(in.OccurredOn),
				money.CanonicalDecimal(in.Amount), nullableDecimal(in.Quantity), nullableDecimal(in.UnitPrice),
				money.CanonicalDecimal(in.Fees), money.CanonicalDecimal(in.Taxes), nullableString(trimOptional(in.Notes)), now, now); err != nil {
				return nil, err
			}
			ids = append(ids, id)
			accounts = append(accounts, in.AccountID)
		}
		if link != nil {
			input := *link
			input.OperationID = ids[0]
			if _, err := createReconciliation(ctx, tx, input); err != nil {
				return nil, err
			}
		}
		return accounts, nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]Operation, 0, len(ids))
	for _, id := range ids {
		op, err := GetOperation(ctx, conn, id)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, nil
}

// positionLedgerKind reports the kinds a manual holding of an integrated
// custody may record. They only move the holding, never the provider's cash.
func positionLedgerKind(kind OperationKind) bool {
	switch kind {
	case OperationInitialBalance, OperationBuy, OperationSell, OperationValuation:
		return true
	default:
		return false
	}
}

func validateOperationAccount(ctx context.Context, q Querier, in OperationInput) error {
	account, err := getRawAccount(ctx, q, in.AccountID)
	if err != nil {
		return err
	}
	if account.Kind == AccountKindIntegrated {
		cashNote := reconcilableKind(in.Kind) && in.PositionID == nil
		holding := positionLedgerKind(in.Kind) && in.PositionID != nil && *in.PositionID != ""
		if !cashNote && !holding {
			return ErrIntegratedReadOnly
		}
	}
	return validateOperationShape(Operation{AccountID: in.AccountID, PositionID: in.PositionID,
		Kind: in.Kind, OccurredOn: in.OccurredOn, Amount: in.Amount, Quantity: in.Quantity,
		UnitPrice: in.UnitPrice, Fees: in.Fees, Taxes: in.Taxes})
}

// UpdateOperation rewrites a manual operation in place, keeping created_at so
// the correction stays where it was in the replay order.
func UpdateOperation(ctx context.Context, conn *sql.DB, id string, in OperationInput) (Operation, error) {
	if !in.Kind.Valid() {
		return Operation{}, ErrInvalidInput
	}
	err := mutateLedger(ctx, conn, func(tx *sql.Tx) ([]string, error) {
		current, err := GetOperation(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if current.TransferID != nil {
			return nil, ErrTransferAtomic
		}
		if current.Source != "manual" {
			return nil, ErrNotManual
		}
		if err := validateOperationAccount(ctx, tx, in); err != nil {
			return nil, err
		}
		if err := requirePositionOfAccount(ctx, tx, in.AccountID, in.PositionID); err != nil {
			return nil, err
		}
		updated := current
		updated.AccountID = in.AccountID
		updated.PositionID = in.PositionID
		updated.Kind = in.Kind
		updated.OccurredOn = Day(in.OccurredOn)
		updated.Amount = in.Amount
		updated.Fees = in.Fees
		updated.Taxes = in.Taxes
		if err := validateOperationLinks(ctx, tx, updated); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE investment_operations
			SET account_id = ?, position_id = ?, kind = ?, occurred_on = ?, amount = ?,
			    quantity = ?, unit_price = ?, fees = ?, taxes = ?, notes = ?, updated_at = ?
			WHERE id = ? AND source = 'manual'`,
			in.AccountID, nullableString(in.PositionID), string(in.Kind), formatDate(in.OccurredOn),
			money.CanonicalDecimal(in.Amount), nullableDecimal(in.Quantity), nullableDecimal(in.UnitPrice),
			money.CanonicalDecimal(in.Fees), money.CanonicalDecimal(in.Taxes),
			nullableString(trimOptional(in.Notes)), db.FormatTime(time.Now()), id); err != nil {
			return nil, err
		}
		return []string{in.AccountID, current.AccountID}, nil
	})
	if err != nil {
		return Operation{}, err
	}
	return GetOperation(ctx, conn, id)
}

func DeleteOperation(ctx context.Context, conn *sql.DB, id string) error {
	return mutateLedger(ctx, conn, func(tx *sql.Tx) ([]string, error) {
		current, err := GetOperation(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if current.TransferID != nil {
			return nil, ErrTransferAtomic
		}
		if current.Source != "manual" {
			return nil, ErrNotManual
		}
		var linked bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM investment_reconciliations WHERE operation_id = ?)`, id).Scan(&linked); err != nil {
			return nil, err
		}
		if linked {
			return nil, ErrOperationHasReconciliations
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM investment_operations WHERE id = ?`, id); err != nil {
			return nil, err
		}
		return []string{current.AccountID}, nil
	})
}

func DeleteTransfer(ctx context.Context, conn *sql.DB, transferID string) error {
	if transferID == "" {
		return ErrInvalidInput
	}
	return mutateLedger(ctx, conn, func(tx *sql.Tx) ([]string, error) {
		rows, err := tx.QueryContext(ctx, `SELECT account_id FROM investment_operations WHERE transfer_id = ? ORDER BY id`, transferID)
		if err != nil {
			return nil, err
		}
		accounts := []string{}
		for rows.Next() {
			var account string
			if err := rows.Scan(&account); err != nil {
				rows.Close()
				return nil, err
			}
			accounts = append(accounts, account)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if len(accounts) != 2 {
			return nil, ErrOperationNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM investment_operations WHERE transfer_id = ?`, transferID); err != nil {
			return nil, err
		}
		return accounts, nil
	})
}
