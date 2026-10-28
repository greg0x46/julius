package categories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"strings"
)

// creditCardMetadata is the subset of Pluggy's opaque credit_card_metadata
// JSON that identifies which card purchase an installment belongs to.
type creditCardMetadata struct {
	InstallmentNumber *int    `json:"installmentNumber"`
	TotalInstallments *int    `json:"totalInstallments"`
	CardNumber        *string `json:"cardNumber"`
}

// findInstallmentGroup returns every transaction id sharing the same card
// purchase as transactionID, always including transactionID itself. Pluggy
// splits an installment purchase into one financial_transactions row per
// parcela, each carrying installmentNumber/totalInstallments in
// credit_card_metadata and a description ending in " N/M" — there is no
// explicit purchase id, so siblings are recognized by matching account,
// cardNumber, totalInstallments and the description with that suffix
// stripped. Anything that doesn't fit this exact shape (no metadata, no
// installment fields, description missing the expected suffix) is treated
// as a standalone transaction, matching AssignManual's pre-cascade
// behavior.
// ManualAssignmentIDs returns the complete installment cascade for a manual decision.
func ManualAssignmentIDs(ctx context.Context, q Querier, transactionID string) ([]string, error) {
	return findInstallmentGroup(ctx, q, transactionID)
}

func findInstallmentGroup(ctx context.Context, q Querier, transactionID string) ([]string, error) {
	var (
		accountID   string
		description sql.NullString
		metadataRaw sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT account_id, description, credit_card_metadata FROM financial_transactions WHERE id = ?`,
		transactionID,
	).Scan(dbtypes.ID(&accountID), &description, &metadataRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return []string{transactionID}, nil
	}
	if err != nil {
		return nil, err
	}
	if !description.Valid || !metadataRaw.Valid {
		return []string{transactionID}, nil
	}

	var meta creditCardMetadata
	if err := json.Unmarshal([]byte(metadataRaw.String), &meta); err != nil {
		return []string{transactionID}, nil
	}
	if meta.InstallmentNumber == nil || meta.TotalInstallments == nil || *meta.TotalInstallments <= 1 {
		return []string{transactionID}, nil
	}

	suffix := fmt.Sprintf(" %d/%d", *meta.InstallmentNumber, *meta.TotalInstallments)
	if !strings.HasSuffix(description.String, suffix) {
		return []string{transactionID}, nil
	}
	baseDescription := strings.TrimSuffix(description.String, suffix)

	rows, err := q.QueryContext(ctx,
		`SELECT id, description, credit_card_metadata FROM financial_transactions
		 WHERE account_id = ? AND credit_card_metadata IS NOT NULL`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	group := []string{transactionID}
	for rows.Next() {
		var (
			candidateID          string
			candidateDescription string
			candidateMetadataRaw string
		)
		if err := rows.Scan(dbtypes.ID(&candidateID), &candidateDescription, &candidateMetadataRaw); err != nil {
			return nil, err
		}
		if candidateID == transactionID {
			continue
		}
		var candidateMeta creditCardMetadata
		if err := json.Unmarshal([]byte(candidateMetadataRaw), &candidateMeta); err != nil {
			continue
		}
		if candidateMeta.InstallmentNumber == nil || candidateMeta.TotalInstallments == nil {
			continue
		}
		if *candidateMeta.TotalInstallments != *meta.TotalInstallments {
			continue
		}
		if meta.CardNumber != nil && candidateMeta.CardNumber != nil && *meta.CardNumber != *candidateMeta.CardNumber {
			continue
		}
		wantDescription := fmt.Sprintf("%s %d/%d", baseDescription, *candidateMeta.InstallmentNumber, *candidateMeta.TotalInstallments)
		if candidateDescription != wantDescription {
			continue
		}
		group = append(group, candidateID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return group, nil
}
