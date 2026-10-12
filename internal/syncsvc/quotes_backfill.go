package syncsvc

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/pluggy"
)

// HoldingFromSnapshot is the provider holding as investments needs it to
// resolve its asset and record its price.
func HoldingFromSnapshot(snapshot pluggy.InvestmentSnapshot) investments.SyncedHolding {
	return investments.SyncedHolding{
		ExternalID: snapshot.ExternalID, Name: snapshot.Name, Code: snapshot.Code, ISIN: snapshot.ISIN,
		InvestmentType: snapshot.InvestmentType, Subtype: snapshot.Subtype, CurrencyCode: snapshot.CurrencyCode,
		IssuerCNPJ: snapshot.IssuerCNPJ, Rate: snapshot.Rate, RateType: snapshot.RateType,
		IssueDate: snapshot.IssueDate, PurchaseDate: snapshot.PurchaseDate, DueDate: snapshot.DueDate,
		Quantity: snapshot.Quantity, Value: snapshot.Value, Amount: snapshot.Amount,
		AmountOriginal: snapshot.AmountOriginal, AsOfDate: snapshot.AsOfDate,
	}
}

// BackfillAssetQuotes rebuilds the price series of synced holdings from the
// investments payloads raw_imports has kept since the first sync. It also
// fills issue_date/purchase_date/issuer_cnpj on holdings stored before those
// columns existed, so they resolve to their per-title asset right away
// rather than on their next sync.
//
// It runs once: after the first pass the live sync keeps the series current,
// so any stored sync quote means there is nothing left to rebuild. It runs
// in one transaction so an interrupted pass is retried whole on next start.
func BackfillAssetQuotes(ctx context.Context, conn *sql.DB) (int, error) {
	var done bool
	if err := conn.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM investment_asset_quotes WHERE origin = ?)`, string(investments.QuoteOriginSync)).
		Scan(dbtypes.Bool(&done)); err != nil {
		return 0, err
	}
	if done {
		return 0, nil
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	type payloadRow struct {
		id, sourceID string
		payload      []byte
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, source_id, payload FROM raw_imports
		WHERE scope = 'investments' AND http_status BETWEEN 200 AND 299
		ORDER BY received_at, id`)
	if err != nil {
		return 0, err
	}
	payloads := []payloadRow{}
	for rows.Next() {
		var row payloadRow
		if err := rows.Scan(dbtypes.ID(&row.id), dbtypes.ID(&row.sourceID), &row.payload); err != nil {
			rows.Close()
			return 0, err
		}
		payloads = append(payloads, row)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	type holdingKey struct{ sourceID, externalID string }
	latest := map[holdingKey]pluggy.InvestmentSnapshot{}
	processed := 0
	for _, row := range payloads {
		// A payload the live sync could not read yielded no holdings then
		// either; skipping it keeps the backfill to what sync accepted.
		snapshots, err := pluggy.ParseInvestmentsPayload(row.payload)
		if err != nil {
			continue
		}
		rawImportID := row.id
		for _, snapshot := range snapshots {
			latest[holdingKey{row.sourceID, snapshot.ExternalID}] = snapshot
			if err := investments.RecordSyncedQuotes(ctx, tx, HoldingFromSnapshot(snapshot), &rawImportID); err != nil {
				return 0, fmt.Errorf("record quotes of %s from %s: %w", snapshot.ExternalID, row.id, err)
			}
			processed++
		}
	}

	for key, snapshot := range latest {
		if _, err := tx.ExecContext(ctx, `
			UPDATE financial_investments SET issue_date = ?, purchase_date = ?, issuer_cnpj = ?
			WHERE source_id = ? AND external_id = ? AND issue_date IS NULL AND purchase_date IS NULL AND issuer_cnpj IS NULL`,
			db.FormatTimePtr(snapshot.IssueDate), db.FormatTimePtr(snapshot.PurchaseDate), snapshot.IssuerCNPJ,
			key.sourceID, key.externalID); err != nil {
			return 0, err
		}
	}
	if _, err := investments.PruneUnusedSyncedAssets(ctx, tx); err != nil {
		return 0, err
	}
	return processed, tx.Commit()
}
