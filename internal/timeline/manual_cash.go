package timeline

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

// Manual adjustments and future posted movements are already in the cash anchor.
// They belong in the balance walk without becoming additional reporting entries.
func anchoredManualCash(ctx context.Context, q Querier, params BuildParams) ([]Entry, error) {
	query := `SELECT aba.id, aba.account_id, aba.amount, aba.created_at, 'adjustment'
 FROM account_balance_adjustments aba JOIN financial_accounts fa ON fa.id = aba.account_id
 WHERE fa.origin = 'manual' AND fa.currency_code = 'BRL'
 UNION ALL SELECT ft.id, ft.account_id, ft.amount_in_account_currency, ft.occurred_at, 'transaction'
 FROM financial_transactions ft JOIN financial_accounts fa ON fa.id = ft.account_id
 WHERE fa.origin = 'manual' AND fa.currency_code = 'BRL'
 AND ft.deleted_at IS NULL AND ft.provider_status = 'POSTED'`
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	from, reference := dates.Day(params.From), dates.Day(params.ReferenceDate)
	for rows.Next() {
		var id, accountID, rawAmount, rawDate, kind string
		if err := rows.Scan(dbtypes.ID(&id), dbtypes.ID(&accountID), dbtypes.Decimal(&rawAmount), dbtypes.Time(&rawDate), &kind); err != nil {
			return nil, err
		}
		if len(params.AccountIDs) > 0 && !contains(params.AccountIDs, accountID) {
			continue
		}
		occurredAt, err := dbtypes.ParseTime(rawDate)
		if err != nil {
			return nil, err
		}
		day := dates.Day(occurredAt)
		if day.Before(from) || (kind == "transaction" && !day.After(reference)) {
			continue
		}
		amount, err := decimal.NewFromString(rawAmount)
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Date: day, Amount: amount, Tier: TierRealizado,
			Source: SourceReal, EventKey: kind + ":" + id})
	}
	return entries, rows.Err()
}
