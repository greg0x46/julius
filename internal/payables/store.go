// Package payables ports app/debts/eligibility.py and app/debts/service.py
// (feature 007, since extended to also cover receivables), unified: payable
// CRUD, and linking a payable to the transaction(s) that paid it down or
// settled it. Every amount beyond a link's own snapshot (settled_amount,
// remaining_amount, status, and even a link's "current" amount) is computed
// on read from the linked transactions' live effective money — never
// persisted — so a later sync correcting a transaction's amount is
// reflected automatically instead of leaving stale totals.
package payables

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
)

// ErrNotFound is returned by Get/Update/Delete when a payable id has no
// matching row.
var ErrNotFound = errors.New("payable not found")

// ErrLinkNotFound is returned by GetLink when a payable_transaction_links
// id has no matching row.
var ErrLinkNotFound = errors.New("payable link not found")

// Querier is satisfied by both *sql.DB and *sql.Tx, and structurally by
// transactions.Querier too (same three methods), so it can also be passed
// wherever that's expected — see UnlinkIfPresent.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Create mirrors create_debt/create_receivable.
func Create(ctx context.Context, q Querier, kind Kind, name string, totalAmount, initialRemainingAmount decimal.Decimal) (Payable, error) {
	now := time.Now().UTC()
	p := Payable{
		ID: uuid.NewString(), Kind: kind, Name: name, TotalAmount: totalAmount,
		StartingSettledAmount: totalAmount.Sub(initialRemainingAmount),
		CreatedAt:             now, UpdatedAt: now,
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO payables (id, kind, name, total_amount, starting_settled_amount, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, string(p.Kind), p.Name, money.CanonicalDecimal(p.TotalAmount), money.CanonicalDecimal(p.StartingSettledAmount),
		db.FormatTime(now), db.FormatTime(now),
	)
	if err != nil {
		return Payable{}, err
	}
	return p, nil
}

// Update mirrors update_debt/update_receivable (name and total_amount only
// — starting_settled_amount is set once at creation and never edited
// directly afterward; kind is immutable).
func Update(ctx context.Context, q Querier, id, name string, totalAmount decimal.Decimal) (Payable, error) {
	now := db.FormatTime(time.Now())
	res, err := q.ExecContext(ctx, `UPDATE payables SET name = ?, total_amount = ?, updated_at = ? WHERE id = ?`,
		name, money.CanonicalDecimal(totalAmount), now, id)
	if err != nil {
		return Payable{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Payable{}, ErrNotFound
	}
	return Get(ctx, q, id)
}

// Delete mirrors delete_debt/delete_receivable (payable_transaction_links
// cascades via the schema's ON DELETE CASCADE).
func Delete(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM payables WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanPayable(row *sql.Row) (Payable, error) {
	var (
		p                                        Payable
		kindRaw                                  string
		totalAmountRaw, startingSettledAmountRaw string
		createdAtRaw, updatedAtRaw               string
	)
	if err := row.Scan(dbtypes.ID(&p.ID), &kindRaw, &p.Name, dbtypes.Decimal(&totalAmountRaw), dbtypes.Decimal(&startingSettledAmountRaw), dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return Payable{}, err
	}
	p.Kind = Kind(kindRaw)
	var err error
	if p.TotalAmount, err = decimal.NewFromString(totalAmountRaw); err != nil {
		return Payable{}, err
	}
	if p.StartingSettledAmount, err = decimal.NewFromString(startingSettledAmountRaw); err != nil {
		return Payable{}, err
	}
	if p.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return Payable{}, err
	}
	if p.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
		return Payable{}, err
	}
	return p, nil
}

// Get mirrors get_debt/get_receivable (without eager-loaded links — the
// links reach callers on Summary.Links instead, so not every caller pays
// for loading them).
func Get(ctx context.Context, q Querier, id string) (Payable, error) {
	row := q.QueryRowContext(ctx, `SELECT id, kind, name, total_amount, starting_settled_amount, created_at, updated_at FROM payables WHERE id = ?`, id)
	p, err := scanPayable(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Payable{}, ErrNotFound
	}
	return p, err
}

// List mirrors list_debts/list_receivables, ordered by created_at. A nil
// kind lists both; a non-nil kind filters to just that one.
func List(ctx context.Context, q Querier, kind *Kind) ([]Payable, error) {
	query := `SELECT id, kind, name, total_amount, starting_settled_amount, created_at, updated_at FROM payables`
	args := []any{}
	if kind != nil {
		query += ` WHERE kind = ?`
		args = append(args, string(*kind))
	}
	query += ` ORDER BY created_at`

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Payable
	for rows.Next() {
		var (
			p                                        Payable
			kindRaw                                  string
			totalAmountRaw, startingSettledAmountRaw string
			createdAtRaw, updatedAtRaw               string
		)
		if err := rows.Scan(dbtypes.ID(&p.ID), &kindRaw, &p.Name, dbtypes.Decimal(&totalAmountRaw), dbtypes.Decimal(&startingSettledAmountRaw), dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
			return nil, err
		}
		p.Kind = Kind(kindRaw)
		if p.TotalAmount, err = decimal.NewFromString(totalAmountRaw); err != nil {
			return nil, err
		}
		if p.StartingSettledAmount, err = decimal.NewFromString(startingSettledAmountRaw); err != nil {
			return nil, err
		}
		if p.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
			return nil, err
		}
		if p.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// linksFor mirrors accessing payable.links for a whole set of payables at
// once, keyed by payable_id and newest-first within each key (matching the
// reference's order_by(...linked_at.desc())).
//
// There is deliberately no single-payable form: every read path here — the
// HTTP list endpoint, RemainingTotal — summarizes a
// set, and offering a convenient per-payable loader is what invites the
// query-per-payable loop this shape exists to prevent. Unexported because
// the rows reach anyone outside this package on Summary.Links.
//
// A payable with no links is simply absent from the map; ranging over a nil
// slice is what callers do anyway.
type loadedLink struct {
	Link
	countsAsSettlement bool
}

func linksFor(ctx context.Context, q Querier, payableIDs []string) (map[string][]loadedLink, error) {
	result := make(map[string][]loadedLink, len(payableIDs))
	if len(payableIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(payableIDs)
	rows, err := q.QueryContext(ctx, `
		SELECT l.id, l.payable_id, l.transaction_id, l.linked_amount, l.linked_at,
		       CASE
		         WHEN NOT EXISTS (
					SELECT 1 FROM scenarios s0
					WHERE s0.payable_id = l.payable_id AND s0.is_accounting_source = TRUE
				 ) THEN 1
		         WHEN EXISTS (
					SELECT 1
					FROM scenario_realizations sr
					JOIN scenarios s ON s.id = sr.scenario_id
					WHERE s.payable_id = l.payable_id
					  AND s.is_accounting_source = TRUE
					  AND sr.relation_type = 'settlement'
					  AND sr.state = 'linked'
					  AND sr.transaction_id = l.transaction_id
				 ) THEN 1
		         ELSE 0
		       END AS counts_as_settlement
		FROM payable_transaction_links l
		WHERE l.payable_id IN (`+in+`) ORDER BY l.linked_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l Link
		var countsAsSettlement int
		var linkedAmountRaw, linkedAtRaw string
		if err := rows.Scan(dbtypes.ID(&l.ID), dbtypes.ID(&l.PayableID), dbtypes.ID(&l.TransactionID), dbtypes.Decimal(&linkedAmountRaw), dbtypes.Time(&linkedAtRaw), dbtypes.Bool(&countsAsSettlement)); err != nil {
			return nil, err
		}
		if l.LinkedAmount, err = decimal.NewFromString(linkedAmountRaw); err != nil {
			return nil, err
		}
		if l.LinkedAt, err = dbtypes.ParseTime(linkedAtRaw); err != nil {
			return nil, err
		}
		result[l.PayableID] = append(result[l.PayableID], loadedLink{Link: l, countsAsSettlement: countsAsSettlement != 0})
	}
	return result, rows.Err()
}

// GetLink mirrors reading a single payable_transaction_links row by id,
// regardless of which payable it belongs to — callers that need to scope it
// to a specific payable (e.g. DeleteLink) check Link.PayableID themselves.
func GetLink(ctx context.Context, q Querier, id string) (Link, error) {
	var l Link
	var linkedAmountRaw, linkedAtRaw string
	err := q.QueryRowContext(ctx,
		`SELECT id, payable_id, transaction_id, linked_amount, linked_at FROM payable_transaction_links WHERE id = ?`, id,
	).Scan(dbtypes.ID(&l.ID), dbtypes.ID(&l.PayableID), dbtypes.ID(&l.TransactionID), dbtypes.Decimal(&linkedAmountRaw), dbtypes.Time(&linkedAtRaw))
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrLinkNotFound
	}
	if err != nil {
		return Link{}, err
	}
	if l.LinkedAmount, err = decimal.NewFromString(linkedAmountRaw); err != nil {
		return Link{}, err
	}
	if l.LinkedAt, err = dbtypes.ParseTime(linkedAtRaw); err != nil {
		return Link{}, err
	}
	return l, nil
}

// linkEffectiveAmount mirrors link_effective_amount: how much of a linked
// transaction counts toward settled_amount, recomputed from the
// transaction's *current* amount/currency rather than the link's stored
// snapshot — see the package doc comment. A non-BRL transaction contributes
// nothing: every payable total is BRL, and converting here would invent a
// rate the app never stored.
//
// It takes the already-loaded row rather than an id because summarize is
// the only caller and holds the snapshot — reloading it per link is what
// Summary.Links exists to avoid.
func (s transactionSnapshot) linkEffectiveAmount() decimal.Decimal {
	eff := money.SelectEffectiveMoney(s.amountInAcct, s.acctCurrency, s.amount, s.currencyCode)
	if eff == nil || eff.CurrencyCode != "BRL" {
		return decimal.Zero
	}
	return eff.Value.Abs()
}

// LinkedTransactionSummary is what a link's read paths need off the
// transaction behind it, beyond the effective amount: the fields a
// PayableLinkedTransaction response renders, plus OccurredAt, which is not
// display-only — an entry with no occurred_at is dated by nothing. It reaches callers on
// LinkSummary.Transaction, filled from the same row the link's effective
// amount came from.
type LinkedTransactionSummary struct {
	OccurredAt  *time.Time
	Description *string
}

type transactionSnapshot struct {
	description    *string
	occurredAt     *time.Time
	movementType   *string
	amount         *decimal.Decimal
	amountInAcct   *decimal.Decimal
	currencyCode   *string
	acctCurrency   *string
	acctName       *string
	inclusionState *string
}

// snapshotScan holds the nullable columns a transactionSnapshot is built
// from, so the single-row and batch loaders below share one column list and
// one conversion instead of keeping two copies in sync.
type snapshotScan struct {
	description, occurredAt, movementType  sql.NullString
	amount, amountInAcct, currencyCode     sql.NullString
	acctCurrency, acctName, inclusionState sql.NullString
}

const transactionSnapshotColumns = `ft.description, ft.occurred_at, ft.movement_type, ft.amount,
	ft.amount_in_account_currency, ft.currency_code, fa.currency_code, fa.name, tid.state`

const transactionSnapshotFrom = `FROM financial_transactions ft
	JOIN financial_accounts fa ON fa.id = ft.account_id
	LEFT JOIN transaction_inclusion_decisions tid ON tid.transaction_id = ft.id`

func (sc *snapshotScan) dest() []any {
	return []any{&sc.description, dbtypes.Time(&sc.occurredAt), &sc.movementType, dbtypes.Decimal(&sc.amount),
		dbtypes.Decimal(&sc.amountInAcct), &sc.currencyCode, &sc.acctCurrency, &sc.acctName, &sc.inclusionState}
}

func (sc *snapshotScan) snapshot() (transactionSnapshot, error) {
	var s transactionSnapshot
	if sc.description.Valid {
		s.description = &sc.description.String
	}
	if sc.movementType.Valid {
		s.movementType = &sc.movementType.String
	}
	if sc.currencyCode.Valid {
		s.currencyCode = &sc.currencyCode.String
	}
	if sc.acctCurrency.Valid {
		s.acctCurrency = &sc.acctCurrency.String
	}
	if sc.acctName.Valid {
		s.acctName = &sc.acctName.String
	}
	if sc.inclusionState.Valid {
		s.inclusionState = &sc.inclusionState.String
	}
	if sc.occurredAt.Valid {
		t, err := dbtypes.ParseTime(sc.occurredAt.String)
		if err != nil {
			return transactionSnapshot{}, err
		}
		s.occurredAt = &t
	}
	if sc.amount.Valid {
		d, err := decimal.NewFromString(sc.amount.String)
		if err != nil {
			return transactionSnapshot{}, err
		}
		s.amount = &d
	}
	if sc.amountInAcct.Valid {
		d, err := decimal.NewFromString(sc.amountInAcct.String)
		if err != nil {
			return transactionSnapshot{}, err
		}
		s.amountInAcct = &d
	}
	return s, nil
}

func loadTransaction(ctx context.Context, q Querier, transactionID string) (transactionSnapshot, bool, error) {
	var sc snapshotScan
	err := q.QueryRowContext(ctx,
		`SELECT `+transactionSnapshotColumns+` `+transactionSnapshotFrom+` WHERE ft.id = ? AND ft.deleted_at IS NULL`, transactionID,
	).Scan(sc.dest()...)
	if errors.Is(err, sql.ErrNoRows) {
		return transactionSnapshot{}, false, nil
	}
	if err != nil {
		return transactionSnapshot{}, false, err
	}
	s, err := sc.snapshot()
	if err != nil {
		return transactionSnapshot{}, false, err
	}
	return s, true, nil
}

// loadTransactions is loadTransaction for many ids at once: one query for
// the whole set instead of one per id, which is what lets summarize cost a
// fixed number of round trips no matter how many links a payable has.
//
// An id with no row is simply absent from the result — the same "no data,
// not an error" treatment loadTransaction gives a missing transaction, with
// the map's comma-ok standing in for its bool.
func loadTransactions(ctx context.Context, q Querier, transactionIDs []string) (map[string]transactionSnapshot, error) {
	result := make(map[string]transactionSnapshot, len(transactionIDs))
	if len(transactionIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(transactionIDs)
	rows, err := q.QueryContext(ctx,
		`SELECT ft.id, `+transactionSnapshotColumns+` `+transactionSnapshotFrom+
			` WHERE ft.deleted_at IS NULL AND ft.id IN (`+in+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var sc snapshotScan
		if err := rows.Scan(append([]any{dbtypes.ID(&id)}, sc.dest()...)...); err != nil {
			return nil, err
		}
		s, err := sc.snapshot()
		if err != nil {
			return nil, err
		}
		result[id] = s
	}
	return result, rows.Err()
}

func inclusionStateFor(s transactionSnapshot) money.InclusionState {
	if s.inclusionState != nil && *s.inclusionState == string(money.Ignored) {
		return money.Ignored
	}
	return money.Considered
}

// EligibleTransaction mirrors EligibleTransactionRow.
type EligibleTransaction struct {
	ID             string
	OccurredAt     *time.Time
	Description    *string
	AccountName    *string
	EffectiveMoney money.EffectiveMoney
}

// ListEligibleTransactions mirrors list_eligible_transactions: every
// not-yet-linked transaction eligible to pay down a debt or settle a
// receivable (per kind), optionally filtered by a case-insensitive
// description search, newest first. "Not-yet-linked" is checked against
// payable_transaction_links as a whole (not scoped to kind), since a
// transaction settling a debt can never simultaneously settle a
// receivable — one link per transaction, regardless of kind.
func ListEligibleTransactions(ctx context.Context, q Querier, kind Kind, search *string, limit int) ([]EligibleTransaction, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT ft.id, ft.description, ft.occurred_at, ft.movement_type, ft.amount, ft.amount_in_account_currency,
			ft.currency_code, fa.currency_code, fa.name, tid.state
		FROM financial_transactions ft
		JOIN financial_accounts fa ON fa.id = ft.account_id
		LEFT JOIN transaction_inclusion_decisions tid ON tid.transaction_id = ft.id
		WHERE ft.deleted_at IS NULL
		  AND ft.id NOT IN (SELECT transaction_id FROM payable_transaction_links)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var needle string
	if search != nil {
		needle = strings.ToLower(strings.TrimSpace(*search))
	}

	type candidate struct {
		row EligibleTransaction
		key time.Time
	}
	var candidates []candidate
	for rows.Next() {
		var (
			id                                      string
			description, movementType, currencyCode sql.NullString
			amountRaw, amountInAcctRaw              sql.NullString
			occurredAtRaw                           sql.NullString
			acctCurrency, acctName                  sql.NullString
			inclusionState                          sql.NullString
		)
		if err := rows.Scan(dbtypes.ID(&id), &description, dbtypes.Time(&occurredAtRaw), &movementType, dbtypes.Decimal(&amountRaw), dbtypes.Decimal(&amountInAcctRaw),
			&currencyCode, &acctCurrency, &acctName, &inclusionState); err != nil {
			return nil, err
		}
		s := transactionSnapshot{}
		if description.Valid {
			s.description = &description.String
		}
		if movementType.Valid {
			s.movementType = &movementType.String
		}
		if currencyCode.Valid {
			s.currencyCode = &currencyCode.String
		}
		if acctCurrency.Valid {
			s.acctCurrency = &acctCurrency.String
		}
		if acctName.Valid {
			s.acctName = &acctName.String
		}
		if inclusionState.Valid {
			s.inclusionState = &inclusionState.String
		}
		if occurredAtRaw.Valid {
			t, err := dbtypes.ParseTime(occurredAtRaw.String)
			if err != nil {
				return nil, err
			}
			s.occurredAt = &t
		}
		if amountRaw.Valid {
			d, err := decimal.NewFromString(amountRaw.String)
			if err != nil {
				return nil, err
			}
			s.amount = &d
		}
		if amountInAcctRaw.Valid {
			d, err := decimal.NewFromString(amountInAcctRaw.String)
			if err != nil {
				return nil, err
			}
			s.amountInAcct = &d
		}

		classification := money.Classify(s.movementType)
		eff := money.SelectEffectiveMoney(s.amountInAcct, s.acctCurrency, s.amount, s.currencyCode)
		eligibility := EligibilityForLink(kind, classification, inclusionStateFor(s), eff, false)
		if !eligibility.Eligible {
			continue
		}
		if needle != "" && (s.description == nil || !strings.Contains(strings.ToLower(*s.description), needle)) {
			continue
		}
		orderKey := time.Time{}
		if s.occurredAt != nil {
			orderKey = *s.occurredAt
		}
		candidates = append(candidates, candidate{
			row: EligibleTransaction{ID: id, OccurredAt: s.occurredAt, Description: s.description, AccountName: s.acctName, EffectiveMoney: *eff},
			key: orderKey,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].key.After(candidates[j].key) })
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	result := make([]EligibleTransaction, len(candidates))
	for i, c := range candidates {
		result[i] = c.row
	}
	return result, nil
}

// CreateLinkStatus mirrors CreateLinkResult.status.
type CreateLinkStatus string

const (
	StatusCreated             CreateLinkStatus = "created"
	StatusPayableNotFound     CreateLinkStatus = "payable_not_found"
	StatusTransactionNotFound CreateLinkStatus = "transaction_not_found"
	StatusIneligible          CreateLinkStatus = "ineligible"
	StatusConflict            CreateLinkStatus = "conflict"
)

// CreateLinkResult mirrors CreateLinkResult.
type CreateLinkResult struct {
	Status CreateLinkStatus
	Link   *Link
	Reason *LinkIneligibilityReason
}

// CreateLink mirrors create_link. payableID must already be known to have
// kind — callers look the payable up first (they need it for other reasons
// anyway) and pass its Kind through rather than this function re-fetching
// it.
func CreateLink(ctx context.Context, conn *sql.DB, kind Kind, payableID, transactionID string) (CreateLinkResult, error) {
	if _, err := Get(ctx, conn, payableID); errors.Is(err, ErrNotFound) {
		return CreateLinkResult{Status: StatusPayableNotFound}, nil
	} else if err != nil {
		return CreateLinkResult{}, err
	}

	snapshot, found, err := loadTransaction(ctx, conn, transactionID)
	if err != nil {
		return CreateLinkResult{}, err
	}
	if !found {
		return CreateLinkResult{Status: StatusTransactionNotFound}, nil
	}

	var existingLinkID sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT id FROM payable_transaction_links WHERE transaction_id = ?`, transactionID).Scan(dbtypes.ID(&existingLinkID)); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return CreateLinkResult{}, err
	}
	alreadyLinked := existingLinkID.Valid

	classification := money.Classify(snapshot.movementType)
	eff := money.SelectEffectiveMoney(snapshot.amountInAcct, snapshot.acctCurrency, snapshot.amount, snapshot.currencyCode)
	eligibility := EligibilityForLink(kind, classification, inclusionStateFor(snapshot), eff, alreadyLinked)
	if eligibility.Reason != nil && *eligibility.Reason == ReasonAlreadyLinked {
		return CreateLinkResult{Status: StatusConflict}, nil
	}
	if !eligibility.Eligible {
		return CreateLinkResult{Status: StatusIneligible, Reason: eligibility.Reason}, nil
	}

	link := Link{ID: uuid.NewString(), PayableID: payableID, TransactionID: transactionID, LinkedAmount: eff.Value.Abs(), LinkedAt: time.Now().UTC()}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return CreateLinkResult{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO payable_transaction_links (id, payable_id, transaction_id, linked_amount, linked_at)
		VALUES (?, ?, ?, ?, ?)`,
		link.ID, link.PayableID, link.TransactionID, money.CanonicalDecimal(link.LinkedAmount), db.FormatTime(link.LinkedAt),
	)
	if err != nil {
		// A UNIQUE violation here (transaction_id) means a concurrent
		// caller won the race despite the already-linked check above.
		// Any other failure is not a conflict the caller can resolve.
		if db.IsUniqueViolation(err) {
			return CreateLinkResult{Status: StatusConflict}, nil
		}
		return CreateLinkResult{}, err
	}
	if err := createSettlement(ctx, tx, link); err != nil {
		if db.IsUniqueViolationOn(err,
			db.ConstraintOneCompleteEventTransactionSQLite,
			db.ConstraintOneCompleteEventTransactionPostgres) {
			return CreateLinkResult{Status: StatusConflict}, nil
		}
		return CreateLinkResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreateLinkResult{}, err
	}
	return CreateLinkResult{Status: StatusCreated, Link: &link}, nil
}

// createSettlement mirrors a payable link in the accounting scenario. A
// payable has no scenario until someone creates a plan for it, so a link made
// before that has nothing to settle against yet and writes no settlement row;
// scenarios.CreateScenario catches those up when the plan appears.
func createSettlement(ctx context.Context, q Querier, link Link) error {
	var scenarioID string
	err := q.QueryRowContext(ctx, `
		SELECT id FROM scenarios
		WHERE payable_id = ? AND is_accounting_source = TRUE`, link.PayableID).Scan(dbtypes.ID(&scenarioID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO scenario_realizations (
			id, scenario_id, transaction_id, relation_type, state, origin,
			linked_amount, created_at
		) VALUES (?, ?, ?, 'settlement', 'linked', 'manual', ?, ?)`,
		uuid.NewString(), scenarioID, link.TransactionID,
		money.CanonicalDecimal(link.LinkedAmount), db.FormatTime(link.LinkedAt),
	)
	return err
}

// DeleteLink mirrors delete_link: returns found=false when linkID doesn't
// exist under payableID (a 404 either way at the HTTP layer, but keeps the
// scoping check explicit here rather than delegating it to a WHERE clause
// that would silently no-op on a mismatched payable_id).
func DeleteLink(ctx context.Context, conn *sql.DB, payableID, linkID string) (bool, error) {
	var storedPayableID, transactionID string
	err := conn.QueryRowContext(ctx, `SELECT payable_id, transaction_id FROM payable_transaction_links WHERE id = ?`, linkID).Scan(dbtypes.ID(&storedPayableID), dbtypes.ID(&transactionID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && storedPayableID != payableID) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM payable_transaction_links WHERE id = ?`, linkID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM scenario_realizations
		WHERE relation_type = 'settlement' AND transaction_id = ?`, transactionID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM scenario_realizations
		WHERE relation_type = 'allocation' AND transaction_id = ?
		  AND scenario_id IN (SELECT id FROM scenarios WHERE payable_id = ?)`, transactionID, payableID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// UnlinkIfPresent mirrors unlink_if_present. Its signature matches
// transactions.OnIgnoredHook exactly, so it can be passed directly wherever
// one is needed (transactions.SetInclusion, automation's apply paths, and
// the sync pipeline via automation.NewTransactionHook) without an adapter.
func UnlinkIfPresent(ctx context.Context, q transactions.Querier, transactionID string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM payable_transaction_links WHERE transaction_id = ?`, transactionID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `
		DELETE FROM scenario_realizations
		WHERE relation_type = 'settlement' AND transaction_id = ?`, transactionID)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		DELETE FROM scenario_realizations
		WHERE relation_type = 'allocation' AND transaction_id = ?`, transactionID)
	return err
}
