package categories

import (
	"context"
	"database/sql"
	"errors"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"strings"
)

// CardPaymentCategoryID is the seeded "Pagamento de Fatura de Cartão"
// transfer-kind category (00034_card_payment_category.sql /
// 00033 on postgres). Assigning it to both legs of paying a credit card
// bill with the user's own bank account gives them the same treatment
// SamePersonTransferLabel already has: excluded from income/expense totals
// (money.Eligibility), but still counted as real cash movement
// (money.MovedCash's ReasonTransferCategory) — see mapping.go's comment for
// why that combination matters.
const CardPaymentCategoryID = "dd10c680-fb35-4457-8595-4e51c8d279a7"

// IsCardPaymentTransaction recognizes either leg of paying a credit card
// bill from the user's own account, by the two provider fields confirmed
// against real production data:
//   - the bank debit that leaves the paying account carries
//     operation_type_additional_info = "PAGAMENTO_FATURA";
//   - the card credit that receives it carries source_category_id
//     "05100000" (Pluggy's own numeric code) or the source_category label
//     "Credit card payment".
//
// movementType alone tells the two apart — a purchase is also a DEBIT, but
// never on the paying bank account with that additional-info value, so it
// can't match the first branch; it also never carries the card-payment
// source_category_id, so it can't match the second.
//
// Exported so transactions.CreditCardTransactionTotal can recognize the same
// leg directly from the raw provider fields it already has — that
// computation intentionally bypasses category-based exclusion (see its own
// doc comment), so it needs this signal to exclude a bill-payment CREDIT
// from the *current* cycle by identity rather than by date. See the doc
// comment on cardTransactionBelongsToCurrentCycle in cardtotal.go for why a
// payment's date is not a reliable cycle signal.
func IsCardPaymentTransaction(movementType, sourceCategory, sourceCategoryID, additionalInfo string) bool {
	switch strings.ToUpper(strings.TrimSpace(movementType)) {
	case "DEBIT":
		return strings.ToUpper(strings.TrimSpace(additionalInfo)) == "PAGAMENTO_FATURA"
	case "CREDIT":
		return strings.TrimSpace(sourceCategoryID) == "05100000" ||
			strings.EqualFold(strings.TrimSpace(sourceCategory), "Credit card payment")
	default:
		return false
	}
}

// ApplyAutomaticCardPayment assigns CardPaymentCategoryID to transactionID
// when it recognizes one leg of a credit-card bill payment (see
// IsCardPaymentTransaction), mirroring ApplyAutomatic's guarantees: a no-op
// when the fields don't match, when the transaction already has a category
// decision, or when the seeded category is missing.
func ApplyAutomaticCardPayment(ctx context.Context, q Querier, transactionID, movementType, sourceCategory, sourceCategoryID, additionalInfo string) error {
	if !IsCardPaymentTransaction(movementType, sourceCategory, sourceCategoryID, additionalInfo) {
		return nil
	}
	existing, err := getDecision(ctx, q, transactionID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	if _, err := Get(ctx, q, CardPaymentCategoryID); errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	_, _, err = writeDecision(ctx, q, transactionID, CardPaymentCategoryID, OriginAutomatic)
	return err
}

// BackfillAutomaticCardPayment mirrors BackfillAutomatic for card-payment
// legs synced before this rule existed — same "never touches an
// already-categorized row" guarantee, since it re-uses
// ApplyAutomaticCardPayment per row instead of writing decisions directly.
// Returns how many transactions were newly categorized.
func BackfillAutomaticCardPayment(ctx context.Context, conn *sql.DB) (int, error) {
	if _, err := Get(ctx, conn, CardPaymentCategoryID); errors.Is(err, ErrNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	// The movement_type/operation_type_additional_info/source_category
	// comparisons below are wrapped in UPPER(TRIM(...)) to match
	// IsCardPaymentTransaction's own case/whitespace-insensitive comparison —
	// a plain '=' here would silently exclude rows the Go check would have
	// matched (e.g. a lowercase movement_type or a trailing-space label).
	rows, err := conn.QueryContext(ctx, `
		SELECT ft.id, ft.movement_type, ft.source_category, ft.source_category_id, ft.operation_type_additional_info
		FROM financial_transactions ft
		LEFT JOIN transaction_category_decisions d ON d.transaction_id = ft.id
		WHERE d.transaction_id IS NULL
		  AND ft.deleted_at IS NULL
		  AND (
		    (UPPER(TRIM(ft.movement_type)) = 'DEBIT' AND UPPER(TRIM(ft.operation_type_additional_info)) = 'PAGAMENTO_FATURA')
		    OR (UPPER(TRIM(ft.movement_type)) = 'CREDIT' AND (TRIM(ft.source_category_id) = '05100000' OR UPPER(TRIM(ft.source_category)) = 'CREDIT CARD PAYMENT'))
		  )`)
	if err != nil {
		return 0, err
	}

	type candidate struct {
		id, movementType, sourceCategory, sourceCategoryID, additionalInfo string
	}
	var candidates []candidate
	for rows.Next() {
		var (
			c                                                              candidate
			movementType, sourceCategory, sourceCategoryID, additionalInfo sql.NullString
		)
		if err := rows.Scan(dbtypes.ID(&c.id), &movementType, &sourceCategory, dbtypes.ID(&sourceCategoryID), &additionalInfo); err != nil {
			rows.Close()
			return 0, err
		}
		c.movementType, c.sourceCategory, c.sourceCategoryID, c.additionalInfo =
			movementType.String, sourceCategory.String, sourceCategoryID.String, additionalInfo.String
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	// SQLite runs on a single connection, so the rows above must be drained
	// and closed before ApplyAutomaticCardPayment can write.
	if err := rows.Close(); err != nil {
		return 0, err
	}

	applied := 0
	for _, c := range candidates {
		if err := ApplyAutomaticCardPayment(ctx, conn, c.id, c.movementType, c.sourceCategory, c.sourceCategoryID, c.additionalInfo); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}
