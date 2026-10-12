package syncsvc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/transactions"
)

// safeMessages mirrors SAFE_MESSAGES: user-facing text never leaks raw
// provider errors, only one of these fixed, safe strings.
var safeMessages = map[string]string{
	"invalid_provider_credentials":   "Financial provider credentials are invalid",
	"provider_unavailable":           "Financial provider is temporarily unavailable",
	"item_invalid_credentials":       "The financial connection requires updated credentials",
	"item_unavailable":               "The financial connection has no safely updated products",
	"provider_partial_result":        "The provider returned only part of the requested data",
	"provider_rate_limited":          "Financial provider rate limit was exceeded",
	"provider_request_failed":        "Financial provider rejected a read request",
	"invalid_provider_payload":       "The provider returned data that could not be processed safely",
	"unsafe_account_association":     "A transaction could not be linked safely to its account",
	"unsafe_investment_association":  "An investment transaction could not be linked safely to its investment",
	"account_transactions_failed":    "Transactions for this account could not be processed",
	"investment_transactions_failed": "Transactions for this investment could not be processed",
	"account_bills_failed":           "Bills for this account could not be processed",
	"internal_error":                 "Synchronization stopped because of an internal error",
	"interrupted":                    "Synchronization was interrupted before completion",
	"worker_unavailable":             "No worker claimed synchronization before the deadline",
	"connection_inactive":            "The financial connection is no longer active",
}

// SafeMessage mirrors safe_message.
func SafeMessage(code string) string {
	if msg, ok := safeMessages[code]; ok {
		return msg
	}
	return "Synchronization encountered a safe processing error"
}

// selectFinalStatus mirrors select_final_status.
func selectFinalStatus(hasFailures, safeProgress, generalFailure bool) string {
	if generalFailure || (hasFailures && !safeProgress) {
		return "failed"
	}
	if hasFailures {
		return "completed_with_failures"
	}
	return "completed"
}

// Provider is the subset of *pluggy.Adapter's behavior Service depends on,
// so tests can supply a fake instead of hitting a real Pluggy sandbox.
type Provider interface {
	GetSource(ctx context.Context) (pluggy.SourceSnapshot, string, error)
	GetAccounts(ctx context.Context) (pluggy.AccountsPage, error)
	IterTransactionPages(ctx context.Context, externalAccountID string, handle func(pluggy.TransactionsPage) error) error
	GetInvestments(ctx context.Context) (pluggy.InvestmentsPage, error)
	GetInvestmentTransactions(ctx context.Context, externalInvestmentID string) (pluggy.InvestmentTransactionsPage, error)
	GetBills(ctx context.Context, externalAccountID string) (pluggy.BillsPage, error)
}

// TransactionUpsertedHook is called with a transaction's id whenever a sync
// run inserts it for the first time, or updates it because a resync changed
// its normalized hash (see the outcome == "inserted" || outcome == "updated"
// call site in upsertTransaction) — not on "unchanged" outcomes, where
// nothing about the transaction could newly match a rule. It exists so
// package automation can (re-)apply active rules without this package
// importing it. The type itself lives on transactions (the Lançamentos
// engine) so automation does not have to import an ingestion provider just
// to name its own entry point. The hook shares the record's transaction so
// normalization and all resulting decisions commit or roll back together.
type TransactionUpsertedHook = transactions.UpsertedHook

// Service mirrors SyncService.
type Service struct {
	DB                    *sql.DB
	Provider              Provider
	SyncRunID             string
	SourceID              string
	OnTransactionUpserted TransactionUpsertedHook
	// WorkerID is the claim owner; when set, finishing the run fails with
	// ErrClaimLost if another worker owns it. Empty skips the check.
	WorkerID string
}

// beginOwnedTx locks the run row before any sync side effect. Recovery and
// heartbeat update that same row, so ownership cannot change before the
// returned transaction commits. Refreshing the heartbeat also prevents a
// slow database write from expiring an otherwise active claim.
func (s *Service) beginOwnedTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	query := `UPDATE sync_runs SET heartbeat_at = ? WHERE id = ? AND status = 'in_progress'`
	args := []any{db.FormatTime(time.Now()), s.SyncRunID}
	if s.WorkerID != "" {
		query += ` AND worker_id = ?`
		args = append(args, s.WorkerID)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err == nil {
		var affected int64
		affected, err = result.RowsAffected()
		if err == nil && affected == 0 {
			err = ErrClaimLost
		}
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (s *Service) execOwned(ctx context.Context, query string, args ...any) error {
	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) recordSyncedQuotes(ctx context.Context, holding investments.SyncedHolding, rawImportID *string) error {
	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := investments.RecordSyncedQuotes(ctx, tx, holding, rawImportID); err != nil {
		return err
	}
	return tx.Commit()
}

// Execute mirrors SyncService.execute.
func (s *Service) Execute(ctx context.Context) error {
	source, itemRawImportID, err := s.Provider.GetSource(ctx)
	if err != nil {
		var provErr *pluggy.ProviderError
		if errors.As(err, &provErr) {
			return s.failGeneral(ctx, provErr)
		}
		log.Printf("sync_run_internal_error run_id=%s: %v", s.SyncRunID, err)
		return s.failGeneral(ctx, &pluggy.ProviderError{Code: "internal_error", Stage: pluggy.StageAccount, SafeMessage: SafeMessage("internal_error")})
	}
	if err := s.updateSourceName(ctx, source.InstitutionName); err != nil {
		return err
	}
	for _, warning := range source.Warnings {
		if err := s.recordFailure(ctx, pluggy.StageItem, warning, &itemRawImportID, nil, nil); err != nil {
			return err
		}
	}
	if !source.SafeProducts["ACCOUNTS"] {
		return s.failGeneral(ctx, &pluggy.ProviderError{Code: "item_unavailable", Stage: pluggy.StageItem, SafeMessage: SafeMessage("item_unavailable"), RawImportID: &itemRawImportID})
	}
	transactionsSafe := source.SafeProducts["TRANSACTIONS"]

	accountsPage, err := s.Provider.GetAccounts(ctx)
	if err != nil {
		var provErr *pluggy.ProviderError
		if errors.As(err, &provErr) {
			return s.failGeneral(ctx, provErr)
		}
		log.Printf("sync_run_internal_error run_id=%s: %v", s.SyncRunID, err)
		return s.failGeneral(ctx, &pluggy.ProviderError{Code: "internal_error", Stage: pluggy.StageAccount, SafeMessage: SafeMessage("internal_error")})
	}

	for _, rejection := range accountsPage.Rejections {
		if err := s.recordRejection(ctx, rejection, accountsPage.RawImportID, nil, nil); err != nil {
			return err
		}
	}

	for _, record := range accountsPage.Accounts {
		accountID, err := s.upsertAccount(ctx, record, accountsPage.RawImportID)
		if err != nil {
			if errors.Is(err, ErrClaimLost) {
				return err
			}
			log.Printf("account_persistence_failed run_id=%s account=%s: %v", s.SyncRunID, record.ExternalID, err)
			extID := record.ExternalID
			if ferr := s.recordFailure(ctx, pluggy.StageAccount, "internal_error", nil, nil, &extID); ferr != nil {
				return ferr
			}
			continue
		}
		if !transactionsSafe {
			continue
		}
		if err := s.incrementAccountsProcessed(ctx); err != nil {
			return err
		}

		extID := record.ExternalID
		err = s.Provider.IterTransactionPages(ctx, record.ExternalID, func(page pluggy.TransactionsPage) error {
			return s.processTransactionPage(ctx, accountID, record.ExternalID, page)
		})
		if err != nil {
			var provErr *pluggy.ProviderError
			if errors.As(err, &provErr) {
				code := "account_transactions_failed"
				if provErr.Code == "provider_rate_limited" || provErr.Code == "invalid_provider_payload" {
					code = provErr.Code
				}
				if ferr := s.recordFailure(ctx, pluggy.StageTransactions, code, provErr.RawImportID, &accountID, &extID); ferr != nil {
					return ferr
				}
			} else {
				log.Printf("account_processing_failed run_id=%s account=%s: %v", s.SyncRunID, record.ExternalID, err)
				if ferr := s.recordFailure(ctx, pluggy.StageTransactions, "account_transactions_failed", nil, &accountID, &extID); ferr != nil {
					return ferr
				}
			}
		}

		if record.AccountType != nil && *record.AccountType == "CREDIT" {
			if err := s.processBills(ctx, accountID, record.ExternalID); err != nil {
				return err
			}
		}
	}

	if source.SafeProducts["INVESTMENTS"] {
		if err := s.processInvestments(ctx); err != nil {
			return err
		}
	}

	return s.finalize(ctx)
}

func (s *Service) processInvestments(ctx context.Context) error {
	investmentsPage, err := s.Provider.GetInvestments(ctx)
	if err != nil {
		var provErr *pluggy.ProviderError
		if errors.As(err, &provErr) {
			if ferr := s.recordInvestmentFailure(ctx, pluggy.StageInvestments, provErr.Code, provErr.RawImportID, nil, nil); ferr != nil {
				return ferr
			}
			return nil
		}
		log.Printf("sync_run_internal_error run_id=%s: %v", s.SyncRunID, err)
		if ferr := s.recordInvestmentFailure(ctx, pluggy.StageInvestments, "internal_error", nil, nil, nil); ferr != nil {
			return ferr
		}
		return nil
	}

	for _, rejection := range investmentsPage.Rejections {
		if err := s.recordInvestmentRejection(ctx, rejection, investmentsPage.RawImportID, nil, nil); err != nil {
			return err
		}
	}

	for _, record := range investmentsPage.Investments {
		investmentID, err := s.upsertInvestment(ctx, record, investmentsPage.RawImportID)
		if err != nil {
			if errors.Is(err, ErrClaimLost) {
				return err
			}
			log.Printf("investment_persistence_failed run_id=%s investment=%s: %v", s.SyncRunID, record.ExternalID, err)
			extID := record.ExternalID
			if ferr := s.recordInvestmentFailure(ctx, pluggy.StageInvestment, "internal_error", nil, nil, &extID); ferr != nil {
				return ferr
			}
			continue
		}
		if err := s.incrementInvestmentsProcessed(ctx); err != nil {
			return err
		}
		// The holding itself is already stored; a quote that fails to record
		// only leaves a gap in the price series, which the next sync fills.
		rawImportID := investmentsPage.RawImportID
		if err := s.recordSyncedQuotes(ctx, HoldingFromSnapshot(record), &rawImportID); err != nil {
			if errors.Is(err, ErrClaimLost) {
				return err
			}
			log.Printf("investment_quote_failed run_id=%s investment=%s: %v", s.SyncRunID, record.ExternalID, err)
		}

		extID := record.ExternalID
		txPage, err := s.Provider.GetInvestmentTransactions(ctx, record.ExternalID)
		if err != nil {
			var provErr *pluggy.ProviderError
			if errors.As(err, &provErr) {
				code := "investment_transactions_failed"
				if provErr.Code == "provider_rate_limited" || provErr.Code == "invalid_provider_payload" {
					code = provErr.Code
				}
				if ferr := s.recordInvestmentFailure(ctx, pluggy.StageInvestmentTransactions, code, provErr.RawImportID, &investmentID, &extID); ferr != nil {
					return ferr
				}
			} else {
				log.Printf("investment_processing_failed run_id=%s investment=%s: %v", s.SyncRunID, record.ExternalID, err)
				if ferr := s.recordInvestmentFailure(ctx, pluggy.StageInvestmentTransactions, "investment_transactions_failed", nil, &investmentID, &extID); ferr != nil {
					return ferr
				}
			}
			continue
		}

		for _, rejection := range txPage.Rejections {
			if err := s.recordInvestmentRejection(ctx, rejection, txPage.RawImportID, &investmentID, &extID); err != nil {
				return err
			}
		}
		for _, txRecord := range txPage.Transactions {
			if err := s.upsertInvestmentTransaction(ctx, investmentID, txRecord, txPage.RawImportID); err != nil {
				if errors.Is(err, ErrClaimLost) {
					return err
				}
				txExtID := txRecord.ExternalID
				code := "internal_error"
				if errors.Is(err, errInvestmentMismatch) {
					code = "unsafe_investment_association"
				}
				if rerr := s.recordInvestmentRejection(ctx, pluggy.RejectedRecord{
					EntityType: "investment_transaction", ExternalID: &txExtID, Code: code, SafeMessage: SafeMessage(code),
				}, txPage.RawImportID, &investmentID, &extID); rerr != nil {
					return rerr
				}
			}
		}
	}
	return nil
}

// processBills fetches and upserts every closed/overdue bill for a credit
// card account. A failure here never aborts the run — bills only refine
// which month a transaction is displayed under (see internal/transactions),
// they never gate transaction data itself — so errors are recorded the same
// non-fatal way processInvestments records a per-investment failure.
func (s *Service) processBills(ctx context.Context, accountID, externalAccountID string) error {
	billsPage, err := s.Provider.GetBills(ctx, externalAccountID)
	if err != nil {
		var provErr *pluggy.ProviderError
		if errors.As(err, &provErr) {
			code := "account_bills_failed"
			if provErr.Code == "provider_rate_limited" || provErr.Code == "invalid_provider_payload" {
				code = provErr.Code
			}
			return s.recordBillFailure(ctx, pluggy.StageBills, code, provErr.RawImportID, &accountID, &externalAccountID)
		}
		log.Printf("account_bills_failed run_id=%s account=%s: %v", s.SyncRunID, externalAccountID, err)
		return s.recordBillFailure(ctx, pluggy.StageBills, "account_bills_failed", nil, &accountID, &externalAccountID)
	}

	for _, rejection := range billsPage.Rejections {
		if err := s.recordBillRejection(ctx, rejection, billsPage.RawImportID, &accountID, &externalAccountID); err != nil {
			return err
		}
	}
	for _, record := range billsPage.Bills {
		if _, err := s.upsertBill(ctx, accountID, record, billsPage.RawImportID); err != nil {
			if errors.Is(err, ErrClaimLost) {
				return err
			}
			log.Printf("bill_persistence_failed run_id=%s account=%s bill=%s: %v", s.SyncRunID, externalAccountID, record.ExternalID, err)
			extID := record.ExternalID
			if ferr := s.recordBillRejection(ctx, pluggy.RejectedRecord{
				EntityType: "bill", ExternalID: &extID, Code: "internal_error", SafeMessage: SafeMessage("internal_error"),
			}, billsPage.RawImportID, &accountID, &externalAccountID); ferr != nil {
				return ferr
			}
			continue
		}
		if err := s.incrementBillsProcessed(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) upsertBill(ctx context.Context, accountID string, snapshot pluggy.BillSnapshot, rawImportID string) (string, error) {
	digest := pluggy.BillHash(snapshot)
	now := db.FormatTime(time.Now())

	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	newID := uuid.NewString()
	var insertedID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO financial_bills (
			id, source_id, account_id, external_id, due_date, closing_date, total_amount,
			currency_code, minimum_payment_amount, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_id, external_id) DO NOTHING
		RETURNING id`,
		newID, s.SourceID, accountID, snapshot.ExternalID, db.FormatTimePtr(snapshot.DueDate), db.FormatTimePtr(snapshot.ClosingDate),
		decimalToStorage(snapshot.TotalAmount), snapshot.CurrencyCode, decimalToStorage(snapshot.MinimumPaymentAmount),
		rawImportID, digest, now, now,
	).Scan(&insertedID)
	inserted := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	var billID, existingHash string
	if err := tx.QueryRowContext(ctx, `SELECT id, normalized_hash FROM financial_bills WHERE source_id = ? AND external_id = ?`,
		s.SourceID, snapshot.ExternalID).Scan(&billID, &existingHash); err != nil {
		return "", err
	}

	outcome := "unchanged"
	switch {
	case inserted:
		outcome = "inserted"
	case existingHash != digest:
		outcome = "updated"
		_, err = tx.ExecContext(ctx, `
			UPDATE financial_bills SET due_date = ?, closing_date = ?, total_amount = ?, currency_code = ?,
				minimum_payment_amount = ?, current_raw_import_id = ?, normalized_hash = ?, updated_at = ?
			WHERE id = ?`,
			db.FormatTimePtr(snapshot.DueDate), db.FormatTimePtr(snapshot.ClosingDate), decimalToStorage(snapshot.TotalAmount),
			snapshot.CurrencyCode, decimalToStorage(snapshot.MinimumPaymentAmount), rawImportID, digest, now, billID,
		)
		if err != nil {
			return "", err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO normalization_events (id, sync_run_id, raw_import_id, entity_type, bill_id, external_id, outcome, normalized_hash, created_at)
		VALUES (?, ?, ?, 'bill', ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, billID, snapshot.ExternalID, outcome, digest, now,
	); err != nil {
		return "", err
	}
	return billID, tx.Commit()
}

func (s *Service) incrementBillsProcessed(ctx context.Context) error {
	err := s.execOwned(ctx,
		`UPDATE sync_runs SET bills_processed = bills_processed + 1 WHERE id = ? AND status = 'in_progress'`,
		s.SyncRunID)
	return err
}

func (s *Service) recordBillRejection(ctx context.Context, rejection pluggy.RejectedRecord, rawImportID string, accountID, externalAccountID *string) error {
	now := db.FormatTime(time.Now())
	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO normalization_events (id, sync_run_id, raw_import_id, entity_type, external_id, outcome, created_at)
		VALUES (?, ?, ?, ?, ?, 'rejected', ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, rejection.EntityType, rejection.ExternalID, now,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sync_failures (
			id, sync_run_id, raw_import_id, account_id, external_account_id, external_bill_id,
			stage, error_code, safe_message, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 'normalize', ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, accountID, externalAccountID, rejection.ExternalID,
		rejection.Code, SafeMessage(rejection.Code), now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) recordBillFailure(ctx context.Context, stage pluggy.FailureStage, code string, rawImportID *string, accountID, externalAccountID *string) error {
	now := db.FormatTime(time.Now())
	err := s.execOwned(ctx, `
		INSERT INTO sync_failures (id, sync_run_id, raw_import_id, account_id, external_account_id, stage, error_code, safe_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, accountID, externalAccountID, string(stage), code, SafeMessage(code), now,
	)
	return err
}

func (s *Service) updateSourceName(ctx context.Context, displayName *string) error {
	err := s.execOwned(ctx, `UPDATE data_sources SET display_name = ?, updated_at = ? WHERE id = ?`,
		displayName, db.FormatTime(time.Now()), s.SourceID)
	return err
}

// decimalToStorage renders an optional decimal for a TEXT column, preserving
// its own scale (see money.CanonicalDecimal) rather than losing trailing
// zeros — nil stays SQL NULL.
func decimalToStorage(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}
	return money.CanonicalDecimal(*value)
}

// upsertAccount writes an explicit column list on purpose: financial_accounts
// also carries manual_closing_day, which the user owns and the provider knows
// nothing about. Any column added here must be provider data — a "manual_"
// prefix means leave it alone.
func (s *Service) upsertAccount(ctx context.Context, snapshot pluggy.AccountSnapshot, rawImportID string) (string, error) {
	digest := pluggy.AccountHash(snapshot)
	now := db.FormatTime(time.Now())

	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	newID := uuid.NewString()
	var insertedID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO financial_accounts (
			id, source_id, external_id, institution, name, number, account_type,
			account_subtype, balance, credit_limit, available_credit_limit,
			balance_close_date, balance_due_date, currency_code, provider_updated_at,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_id, external_id) DO NOTHING
		RETURNING id`,
		newID, s.SourceID, snapshot.ExternalID, snapshot.Institution, snapshot.Name, snapshot.Number,
		snapshot.AccountType, snapshot.AccountSubtype, decimalToStorage(snapshot.Balance), decimalToStorage(snapshot.CreditLimit),
		decimalToStorage(snapshot.AvailableCreditLimit), db.FormatTimePtr(snapshot.BalanceCloseDate), db.FormatTimePtr(snapshot.BalanceDueDate),
		snapshot.CurrencyCode, db.FormatTimePtr(snapshot.ProviderUpdatedAt), rawImportID, digest, now, now,
	).Scan(&insertedID)
	inserted := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	var accountID, existingHash string
	if err := tx.QueryRowContext(ctx, `SELECT id, normalized_hash FROM financial_accounts WHERE source_id = ? AND external_id = ?`,
		s.SourceID, snapshot.ExternalID).Scan(&accountID, &existingHash); err != nil {
		return "", err
	}

	outcome := "unchanged"
	switch {
	case inserted:
		outcome = "inserted"
	case existingHash != digest:
		outcome = "updated"
		_, err = tx.ExecContext(ctx, `
			UPDATE financial_accounts SET institution = ?, name = ?, number = ?, account_type = ?,
				account_subtype = ?, balance = ?, credit_limit = ?, available_credit_limit = ?,
				balance_close_date = ?, balance_due_date = ?, currency_code = ?,
				provider_updated_at = ?, current_raw_import_id = ?, normalized_hash = ?, updated_at = ?
			WHERE id = ?`,
			snapshot.Institution, snapshot.Name, snapshot.Number, snapshot.AccountType,
			snapshot.AccountSubtype, decimalToStorage(snapshot.Balance), decimalToStorage(snapshot.CreditLimit),
			decimalToStorage(snapshot.AvailableCreditLimit), db.FormatTimePtr(snapshot.BalanceCloseDate), db.FormatTimePtr(snapshot.BalanceDueDate),
			snapshot.CurrencyCode, db.FormatTimePtr(snapshot.ProviderUpdatedAt), rawImportID, digest, now, accountID,
		)
		if err != nil {
			return "", err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO normalization_events (id, sync_run_id, raw_import_id, entity_type, account_id, external_id, outcome, normalized_hash, created_at)
		VALUES (?, ?, ?, 'account', ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, accountID, snapshot.ExternalID, outcome, digest, now,
	); err != nil {
		return "", err
	}
	return accountID, tx.Commit()
}

func (s *Service) incrementAccountsProcessed(ctx context.Context) error {
	return s.execOwned(ctx,
		`UPDATE sync_runs SET accounts_processed = accounts_processed + 1 WHERE id = ? AND status = 'in_progress'`,
		s.SyncRunID)
}

func (s *Service) processTransactionPage(ctx context.Context, accountID, externalAccountID string, page pluggy.TransactionsPage) error {
	for _, rejection := range page.Rejections {
		if err := s.recordRejection(ctx, rejection, page.RawImportID, &accountID, &externalAccountID); err != nil {
			return err
		}
	}
	for _, record := range page.Transactions {
		if err := s.upsertTransaction(ctx, accountID, record, page.RawImportID); err != nil {
			extID := record.ExternalID
			code := "internal_error"
			if errors.Is(err, errAccountMismatch) {
				code = "unsafe_account_association"
			}
			if rerr := s.recordRejection(ctx, pluggy.RejectedRecord{
				EntityType: "transaction", ExternalID: &extID, Code: code, SafeMessage: SafeMessage(code),
			}, page.RawImportID, &accountID, &externalAccountID); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}

// errAccountMismatch rejects a record whose (source_id, external_id) already
// belongs to another account: merging it would move user decisions across
// accounts, so the stored row is left untouched instead.
var errAccountMismatch = errors.New("transaction external id belongs to another account")

// upsertTransaction runs in its own transaction per record — a deliberate
// simplification of the reference's per-page transaction with a savepoint
// per record: SQLite's single-writer-connection model (see internal/db)
// already serializes every transaction here, so per-record transactions give
// the same "one bad record doesn't corrupt its neighbors" guarantee with far
// simpler code, at the cost of one more commit per record than batching
// would need — negligible at a single user's transaction volume. Category
// passes, automation, audit events and counters share this boundary: a failed
// record retains its previous hash (or remains absent), making resync retry it.
func (s *Service) upsertTransaction(ctx context.Context, accountID string, snapshot pluggy.TransactionSnapshot, rawImportID string) error {
	digest := pluggy.TransactionHash(snapshot)
	now := db.FormatTime(time.Now())

	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	newID := uuid.NewString()
	var insertedID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, description_raw, amount,
			amount_in_account_currency, balance_after, currency_code, occurred_at, provider_status,
			movement_type, source_category, source_category_id, provider_code, payment_data,
			credit_card_metadata, merchant, operation_type, operation_type_additional_info,
			operation_category, provider_id, provider_order, provider_created_at, provider_updated_at,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_id, external_id) DO NOTHING
		RETURNING id`,
		newID, s.SourceID, accountID, snapshot.ExternalID, snapshot.Description, snapshot.DescriptionRaw,
		decimalToStorage(snapshot.Amount), decimalToStorage(snapshot.AmountInAccountCurrency), decimalToStorage(snapshot.BalanceAfter),
		snapshot.CurrencyCode, db.FormatTimePtr(snapshot.OccurredAt), snapshot.ProviderStatus, snapshot.MovementType,
		snapshot.SourceCategory, snapshot.SourceCategoryID, snapshot.ProviderCode, nullBytes(snapshot.PaymentData),
		nullBytes(snapshot.CreditCardMetadata), nullBytes(snapshot.Merchant), snapshot.OperationType,
		snapshot.OperationTypeAdditionalInfo, snapshot.OperationCategory, snapshot.ProviderID, snapshot.ProviderOrder,
		db.FormatTimePtr(snapshot.ProviderCreatedAt), db.FormatTimePtr(snapshot.ProviderUpdatedAt),
		rawImportID, digest, now, now,
	).Scan(&insertedID)
	inserted := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var transactionID, existingHash, existingAccountID string
	if err := tx.QueryRowContext(ctx, `SELECT id, normalized_hash, account_id FROM financial_transactions WHERE source_id = ? AND external_id = ?`,
		s.SourceID, snapshot.ExternalID).Scan(&transactionID, &existingHash, &existingAccountID); err != nil {
		return err
	}
	if existingAccountID != accountID {
		return errAccountMismatch
	}

	outcome := "unchanged"
	switch {
	case inserted:
		outcome = "inserted"
	case existingHash != digest:
		outcome = "updated"
		_, err = tx.ExecContext(ctx, `
			UPDATE financial_transactions SET description = ?, description_raw = ?, amount = ?,
				amount_in_account_currency = ?, balance_after = ?, currency_code = ?, occurred_at = ?,
				provider_status = ?, movement_type = ?, source_category = ?, source_category_id = ?,
				provider_code = ?, payment_data = ?, credit_card_metadata = ?, merchant = ?,
				operation_type = ?, operation_type_additional_info = ?, operation_category = ?,
				provider_id = ?, provider_order = ?, provider_created_at = ?, provider_updated_at = ?,
				current_raw_import_id = ?, normalized_hash = ?, updated_at = ?
			WHERE id = ?`,
			snapshot.Description, snapshot.DescriptionRaw, decimalToStorage(snapshot.Amount),
			decimalToStorage(snapshot.AmountInAccountCurrency), decimalToStorage(snapshot.BalanceAfter), snapshot.CurrencyCode,
			db.FormatTimePtr(snapshot.OccurredAt), snapshot.ProviderStatus, snapshot.MovementType, snapshot.SourceCategory,
			snapshot.SourceCategoryID, snapshot.ProviderCode, nullBytes(snapshot.PaymentData), nullBytes(snapshot.CreditCardMetadata),
			nullBytes(snapshot.Merchant), snapshot.OperationType, snapshot.OperationTypeAdditionalInfo, snapshot.OperationCategory,
			snapshot.ProviderID, snapshot.ProviderOrder, db.FormatTimePtr(snapshot.ProviderCreatedAt), db.FormatTimePtr(snapshot.ProviderUpdatedAt),
			rawImportID, digest, now, transactionID,
		)
		if err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO normalization_events (id, sync_run_id, raw_import_id, entity_type, transaction_id, external_id, outcome, normalized_hash, created_at)
		VALUES (?, ?, ?, 'transaction', ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, transactionID, snapshot.ExternalID, outcome, digest, now,
	); err != nil {
		return err
	}

	if outcome == "inserted" || outcome == "updated" {
		column := "transactions_updated"
		if outcome == "inserted" {
			column = "transactions_inserted"
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE sync_runs SET %s = %s + 1 WHERE id = ? AND status = 'in_progress'`, column, column),
			s.SyncRunID,
		); err != nil {
			return err
		}
	}
	// Category passes remain insert-only; automation also reacts to updates.
	if outcome == "inserted" {
		// Card-payment goes first: it only ever matches one of the two
		// specific card-bill-payment legs (see IsCardPaymentTransaction), but
		// a card-bill DEBIT leg's source_category can still happen to also
		// be a key in SourceCategoryMapping (e.g. "Services"). Both funcs
		// no-op once a decision exists, so whichever runs first wins — and
		// the more specific card-payment transfer categorization must win
		// over the generic mapping, not the other way around.
		if err := categories.ApplyAutomaticCardPayment(ctx, tx, transactionID,
			stringOrEmpty(snapshot.MovementType), stringOrEmpty(snapshot.SourceCategory),
			stringOrEmpty(snapshot.SourceCategoryID), stringOrEmpty(snapshot.OperationTypeAdditionalInfo),
		); err != nil {
			return err
		}
		if err := categories.ApplyAutomatic(ctx, tx, transactionID, snapshot.SourceCategory); err != nil {
			return err
		}
		// Learned goes last among the insert-only passes: a past manual
		// decision on a same-looking transaction beats either automatic
		// source above. The rules hook below may still override it —
		// precedence is manual > rule > learned > automatic.
		if _, err := categories.ApplyLearned(ctx, tx, transactionID); err != nil {
			return err
		}
	}
	if outcome == "inserted" || outcome == "updated" {
		if s.OnTransactionUpserted != nil {
			if err := s.OnTransactionUpserted(ctx, tx, transactionID, accountID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func nullBytes(b []byte) any {
	if b == nil {
		return nil
	}
	return string(b)
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Service) incrementInvestmentsProcessed(ctx context.Context) error {
	err := s.execOwned(ctx,
		`UPDATE sync_runs SET investments_processed = investments_processed + 1 WHERE id = ? AND status = 'in_progress'`,
		s.SyncRunID)
	return err
}

func (s *Service) upsertInvestment(ctx context.Context, snapshot pluggy.InvestmentSnapshot, rawImportID string) (string, error) {
	digest := pluggy.InvestmentHash(snapshot)
	now := db.FormatTime(time.Now())

	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	newID := uuid.NewString()
	var insertedID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO financial_investments (
			id, source_id, external_id, investment_type, subtype, name, balance, currency_code,
			number, owner, tax_number, due_date, issuer, issuer_code, rate, rate_type,
			fixed_annual_rate, annual_rate, last_twelve_months_rate, quantity, value, amount,
			amount_profit, amount_withdrawal, amount_original, taxes, taxes2, as_of_date,
			issue_date, purchase_date, issuer_cnpj,
			provider_updated_at, isin, code,
			provider_status, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_id, external_id) DO NOTHING
		RETURNING id`,
		newID, s.SourceID, snapshot.ExternalID, snapshot.InvestmentType, snapshot.Subtype, snapshot.Name,
		decimalToStorage(snapshot.Balance), snapshot.CurrencyCode, snapshot.Number, snapshot.Owner, snapshot.TaxNumber,
		db.FormatTimePtr(snapshot.DueDate), snapshot.Issuer, snapshot.IssuerCode, decimalToStorage(snapshot.Rate),
		snapshot.RateType, decimalToStorage(snapshot.FixedAnnualRate), decimalToStorage(snapshot.AnnualRate),
		decimalToStorage(snapshot.LastTwelveMonthsRate), decimalToStorage(snapshot.Quantity), decimalToStorage(snapshot.Value),
		decimalToStorage(snapshot.Amount), decimalToStorage(snapshot.AmountProfit), decimalToStorage(snapshot.AmountWithdrawal),
		decimalToStorage(snapshot.AmountOriginal), decimalToStorage(snapshot.Taxes), decimalToStorage(snapshot.Taxes2),
		db.FormatTimePtr(snapshot.AsOfDate), db.FormatTimePtr(snapshot.IssueDate), db.FormatTimePtr(snapshot.PurchaseDate),
		snapshot.IssuerCNPJ, db.FormatTimePtr(snapshot.ProviderUpdatedAt), snapshot.ISIN, snapshot.Code,
		snapshot.ProviderStatus, rawImportID, digest, now, now,
	).Scan(&insertedID)
	inserted := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	var investmentID, existingHash string
	if err := tx.QueryRowContext(ctx, `SELECT id, normalized_hash FROM financial_investments WHERE source_id = ? AND external_id = ?`,
		s.SourceID, snapshot.ExternalID).Scan(&investmentID, &existingHash); err != nil {
		return "", err
	}

	outcome := "unchanged"
	switch {
	case inserted:
		outcome = "inserted"
	case existingHash != digest:
		outcome = "updated"
		_, err = tx.ExecContext(ctx, `
			UPDATE financial_investments SET investment_type = ?, subtype = ?, name = ?, balance = ?,
				currency_code = ?, number = ?, owner = ?, tax_number = ?, due_date = ?, issuer = ?,
				issuer_code = ?, rate = ?, rate_type = ?, fixed_annual_rate = ?, annual_rate = ?,
				last_twelve_months_rate = ?, quantity = ?, value = ?, amount = ?, amount_profit = ?,
				amount_withdrawal = ?, amount_original = ?, taxes = ?, taxes2 = ?, as_of_date = ?,
				issue_date = ?, purchase_date = ?, issuer_cnpj = ?,
				provider_updated_at = ?, isin = ?, code = ?,
				provider_status = ?, current_raw_import_id = ?, normalized_hash = ?, updated_at = ?
			WHERE id = ?`,
			snapshot.InvestmentType, snapshot.Subtype, snapshot.Name, decimalToStorage(snapshot.Balance),
			snapshot.CurrencyCode, snapshot.Number, snapshot.Owner, snapshot.TaxNumber, db.FormatTimePtr(snapshot.DueDate),
			snapshot.Issuer, snapshot.IssuerCode, decimalToStorage(snapshot.Rate), snapshot.RateType,
			decimalToStorage(snapshot.FixedAnnualRate), decimalToStorage(snapshot.AnnualRate),
			decimalToStorage(snapshot.LastTwelveMonthsRate), decimalToStorage(snapshot.Quantity), decimalToStorage(snapshot.Value),
			decimalToStorage(snapshot.Amount), decimalToStorage(snapshot.AmountProfit), decimalToStorage(snapshot.AmountWithdrawal),
			decimalToStorage(snapshot.AmountOriginal), decimalToStorage(snapshot.Taxes), decimalToStorage(snapshot.Taxes2),
			db.FormatTimePtr(snapshot.AsOfDate), db.FormatTimePtr(snapshot.IssueDate), db.FormatTimePtr(snapshot.PurchaseDate),
			snapshot.IssuerCNPJ, db.FormatTimePtr(snapshot.ProviderUpdatedAt), snapshot.ISIN, snapshot.Code,
			snapshot.ProviderStatus, rawImportID, digest, now, investmentID,
		)
		if err != nil {
			return "", err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO normalization_events (id, sync_run_id, raw_import_id, entity_type, investment_id, external_id, outcome, normalized_hash, created_at)
		VALUES (?, ?, ?, 'investment', ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, investmentID, snapshot.ExternalID, outcome, digest, now,
	); err != nil {
		return "", err
	}
	return investmentID, tx.Commit()
}

var errInvestmentMismatch = errors.New("investment transaction external id belongs to another investment")

// upsertInvestmentTransaction mirrors upsertTransaction, minus the
// categories.ApplyAutomatic/OnTransactionUpserted hooks: those are
// spending-transaction-specific domain logic, and an investment movement
// (buy/sell/dividend) isn't a spending event.
func (s *Service) upsertInvestmentTransaction(ctx context.Context, investmentID string, snapshot pluggy.InvestmentTransactionSnapshot, rawImportID string) error {
	digest := pluggy.InvestmentTransactionHash(snapshot)
	now := db.FormatTime(time.Now())

	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	newID := uuid.NewString()
	var insertedID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO financial_investment_transactions (
			id, source_id, investment_id, external_id, movement_type, direction, quantity, value, amount,
			occurred_at, trade_date, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_id, external_id) DO NOTHING
		RETURNING id`,
		newID, s.SourceID, investmentID, snapshot.ExternalID, snapshot.MovementType, snapshot.Direction,
		decimalToStorage(snapshot.Quantity), decimalToStorage(snapshot.Value), decimalToStorage(snapshot.Amount),
		db.FormatTimePtr(snapshot.OccurredAt), db.FormatTimePtr(snapshot.TradeDate), rawImportID, digest, now, now,
	).Scan(&insertedID)
	inserted := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var transactionID, existingHash, existingInvestmentID string
	if err := tx.QueryRowContext(ctx, `SELECT id, normalized_hash, investment_id FROM financial_investment_transactions WHERE source_id = ? AND external_id = ?`,
		s.SourceID, snapshot.ExternalID).Scan(&transactionID, &existingHash, &existingInvestmentID); err != nil {
		return err
	}
	if existingInvestmentID != investmentID {
		return errInvestmentMismatch
	}

	outcome := "unchanged"
	switch {
	case inserted:
		outcome = "inserted"
	case existingHash != digest:
		outcome = "updated"
		_, err = tx.ExecContext(ctx, `
			UPDATE financial_investment_transactions SET movement_type = ?, direction = ?, quantity = ?, value = ?,
				amount = ?, occurred_at = ?, trade_date = ?, current_raw_import_id = ?, normalized_hash = ?, updated_at = ?
			WHERE id = ?`,
			snapshot.MovementType, snapshot.Direction, decimalToStorage(snapshot.Quantity), decimalToStorage(snapshot.Value),
			decimalToStorage(snapshot.Amount), db.FormatTimePtr(snapshot.OccurredAt), db.FormatTimePtr(snapshot.TradeDate),
			rawImportID, digest, now, transactionID,
		)
		if err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO normalization_events (id, sync_run_id, raw_import_id, entity_type, investment_transaction_id, external_id, outcome, normalized_hash, created_at)
		VALUES (?, ?, ?, 'investment_transaction', ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, transactionID, snapshot.ExternalID, outcome, digest, now,
	); err != nil {
		return err
	}

	if outcome == "inserted" || outcome == "updated" {
		column := "investment_transactions_updated"
		if outcome == "inserted" {
			column = "investment_transactions_inserted"
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE sync_runs SET %s = %s + 1 WHERE id = ? AND status = 'in_progress'`, column, column),
			s.SyncRunID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) recordInvestmentRejection(ctx context.Context, rejection pluggy.RejectedRecord, rawImportID string, investmentID, externalInvestmentID *string) error {
	now := db.FormatTime(time.Now())
	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO normalization_events (id, sync_run_id, raw_import_id, entity_type, external_id, outcome, created_at)
		VALUES (?, ?, ?, ?, ?, 'rejected', ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, rejection.EntityType, rejection.ExternalID, now,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sync_failures (
			id, sync_run_id, raw_import_id, investment_id, external_investment_id,
			stage, error_code, safe_message, created_at
		) VALUES (?, ?, ?, ?, ?, 'normalize', ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, investmentID, externalInvestmentID,
		rejection.Code, SafeMessage(rejection.Code), now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) recordInvestmentFailure(ctx context.Context, stage pluggy.FailureStage, code string, rawImportID *string, investmentID, externalInvestmentID *string) error {
	now := db.FormatTime(time.Now())
	err := s.execOwned(ctx, `
		INSERT INTO sync_failures (id, sync_run_id, raw_import_id, investment_id, external_investment_id, stage, error_code, safe_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, investmentID, externalInvestmentID, string(stage), code, SafeMessage(code), now,
	)
	return err
}

func (s *Service) recordRejection(ctx context.Context, rejection pluggy.RejectedRecord, rawImportID string, accountID, externalAccountID *string) error {
	now := db.FormatTime(time.Now())
	tx, err := s.beginOwnedTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO normalization_events (id, sync_run_id, raw_import_id, entity_type, external_id, outcome, created_at)
		VALUES (?, ?, ?, ?, ?, 'rejected', ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, rejection.EntityType, rejection.ExternalID, now,
	); err != nil {
		return err
	}
	var externalTransactionID *string
	if rejection.EntityType == "transaction" {
		externalTransactionID = rejection.ExternalID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sync_failures (
			id, sync_run_id, raw_import_id, account_id, external_account_id,
			external_transaction_id, stage, error_code, safe_message, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 'normalize', ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, accountID, externalAccountID,
		externalTransactionID, rejection.Code, SafeMessage(rejection.Code), now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) recordFailure(ctx context.Context, stage pluggy.FailureStage, code string, rawImportID *string, accountID, externalAccountID *string) error {
	now := db.FormatTime(time.Now())
	err := s.execOwned(ctx, `
		INSERT INTO sync_failures (id, sync_run_id, raw_import_id, account_id, external_account_id, stage, error_code, safe_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, rawImportID, accountID, externalAccountID, string(stage), code, SafeMessage(code), now,
	)
	return err
}

func (s *Service) failGeneral(ctx context.Context, provErr *pluggy.ProviderError) error {
	log.Printf("sync_run_general_failure run_id=%s stage=%s code=%s", s.SyncRunID, provErr.Stage, provErr.Code)
	now := db.FormatTime(time.Now())
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	message := SafeMessage(provErr.Code)
	err = fencedTerminalUpdate(ctx, tx, s.SyncRunID, s.WorkerID,
		`status = 'failed', finished_at = ?, general_error_code = ?, general_error_message = ?`,
		now, provErr.Code, message)
	if errors.Is(err, errRunFinished) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sync_failures (id, sync_run_id, raw_import_id, external_account_id, stage, error_code, safe_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), s.SyncRunID, provErr.RawImportID, provErr.ExternalAccountID, string(provErr.Stage), provErr.Code, message, now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) finalize(ctx context.Context) error {
	now := db.FormatTime(time.Now())
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var failureCount, safeCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ?`, s.SyncRunID).Scan(&failureCount); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM normalization_events WHERE sync_run_id = ? AND outcome <> 'rejected'`, s.SyncRunID).Scan(&safeCount); err != nil {
		return err
	}
	finalStatus := selectFinalStatus(failureCount > 0, safeCount > 0, false)
	err = fencedTerminalUpdate(ctx, tx, s.SyncRunID, s.WorkerID, `status = ?, finished_at = ?`, finalStatus, now)
	if errors.Is(err, errRunFinished) {
		return nil
	}
	if err != nil {
		return err
	}
	log.Printf("sync_run_completed run_id=%s status=%s", s.SyncRunID, finalStatus)
	return tx.Commit()
}
