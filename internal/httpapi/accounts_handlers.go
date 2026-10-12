package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
)

// accountDTO exposes a financial_accounts row — a bank account or, when
// AccountType is "CREDIT", a credit card. A card is a single account here
// even when the institution issues several physical cards against it; the
// individual card numbers live only in transaction metadata and are served
// separately by handleListAccountCards.
type accountDTO struct {
	ID                string  `json:"id"`
	Origin            string  `json:"origin"`
	OpeningBalance    *string `json:"opening_balance"`
	ArchivedAt        *string `json:"archived_at"`
	HasMovements      bool    `json:"has_movements"`
	ExternalID        *string `json:"external_id"`
	SourceDisplayName *string `json:"source_display_name"`
	Institution       *string `json:"institution"`
	// InstitutionName is the display-safe institution: the user's own label
	// for the connection when set, otherwise the provider's reported
	// institution with Pluggy's own proxy connector name (never a real bank)
	// filtered out. Institution stays as the raw provider value for anything
	// that still needs it; new UI should read this field instead.
	InstitutionName      *string    `json:"institution_name"`
	Name                 *string    `json:"name"`
	Number               *string    `json:"number"`
	AccountType          *string    `json:"account_type"`
	AccountSubtype       *string    `json:"account_subtype"`
	Balance              *string    `json:"balance"`
	CreditLimit          *string    `json:"credit_limit"`
	AvailableCreditLimit *string    `json:"available_credit_limit"`
	CreditUsageRatio     *string    `json:"credit_usage_ratio"`
	CurrencyCode         *string    `json:"currency_code"`
	BalanceCloseDate     *time.Time `json:"balance_close_date"`
	BalanceDueDate       *time.Time `json:"balance_due_date"`
	ClosingDay           *int       `json:"closing_day"`
	ClosingDaySource     *string    `json:"closing_day_source"` // "manual" | "informado" | "estimado"
	ProviderUpdatedAt    *time.Time `json:"provider_updated_at"`
	UpdatedAt            *time.Time `json:"updated_at"`

	// manualClosingDay is the raw override as stored. It stays unexported —
	// and so out of the JSON — because it isn't the answer on its own:
	// applyClosingDay ranks it against the provider's sources and publishes
	// the winner as ClosingDay/ClosingDaySource.
	manualClosingDay *int
	// connectionLabel is the user's own name for this connection
	// (data_sources.label), which wins over the provider-reported
	// institution — see applyInstitutionName.
	connectionLabel *string
}

const accountSelectColumns = `
	fa.id, fa.external_id, ` + connectionNameColumn + `, fa.institution, fa.name, fa.number,
	fa.account_type, fa.account_subtype, fa.balance, fa.credit_limit,
	fa.available_credit_limit, fa.currency_code, fa.balance_close_date,
	fa.balance_due_date, fa.manual_closing_day, fa.provider_updated_at, fa.updated_at,
	NULLIF(ds.label, ''), fa.origin, fa.opening_balance, fa.archived_at,
 EXISTS(SELECT 1 FROM financial_transactions ft WHERE ft.account_id = fa.id) OR
 EXISTS(SELECT 1 FROM account_balance_adjustments aba WHERE aba.account_id = fa.id)`

const accountSelectFrom = `
	FROM financial_accounts fa
	LEFT JOIN data_sources ds ON ds.id = fa.source_id`

func accountsUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "accounts-unavailable", "Contas temporariamente indisponíveis", "Tente novamente em instantes.")
}

func accountNotFoundProblem(w http.ResponseWriter) {
	writeProblem(w, 404, "account-not-found", "Conta não encontrada", "")
}

func scanAccount(row interface{ Scan(...any) error }) (accountDTO, error) {
	var (
		d              accountDTO
		closeRaw       sql.NullString
		dueRaw         sql.NullString
		manualClosing  sql.NullInt64
		providerUpdRaw sql.NullString
		updatedRaw     sql.NullString
		connectionLbl  sql.NullString
	)
	if err := row.Scan(dbtypes.ID(&d.ID), dbtypes.ID(&d.ExternalID), &d.SourceDisplayName, &d.Institution, &d.Name, &d.Number,
		&d.AccountType, &d.AccountSubtype, dbtypes.Decimal(&d.Balance), dbtypes.Decimal(&d.CreditLimit),
		dbtypes.Decimal(&d.AvailableCreditLimit), &d.CurrencyCode, dbtypes.Time(&closeRaw),
		dbtypes.Time(&dueRaw), &manualClosing, dbtypes.Time(&providerUpdRaw), dbtypes.Time(&updatedRaw), &connectionLbl, &d.Origin, dbtypes.Decimal(&d.OpeningBalance), dbtypes.Time(&d.ArchivedAt), dbtypes.Bool(&d.HasMovements)); err != nil {
		return accountDTO{}, err
	}
	if manualClosing.Valid {
		day := int(manualClosing.Int64)
		d.manualClosingDay = &day
	}
	if connectionLbl.Valid {
		d.connectionLabel = &connectionLbl.String
	}
	applyInstitutionName(&d)
	var err error
	if d.BalanceCloseDate, err = dbtypes.ParseNullTime(closeRaw); err != nil {
		return accountDTO{}, err
	}
	if d.BalanceDueDate, err = dbtypes.ParseNullTime(dueRaw); err != nil {
		return accountDTO{}, err
	}
	if d.ProviderUpdatedAt, err = dbtypes.ParseNullTime(providerUpdRaw); err != nil {
		return accountDTO{}, err
	}
	if d.UpdatedAt, err = dbtypes.ParseNullTime(updatedRaw); err != nil {
		return accountDTO{}, err
	}
	applyCreditUsage(&d)
	return d, nil
}

// isCreditAccount reports whether the row is a credit card. Everything that
// only makes sense for a card — the usage ratio, the closing day, bills —
// gates on it, so the spelling lives in one place.
func isCreditAccount(d *accountDTO) bool {
	return d.AccountType != nil && *d.AccountType == "CREDIT"
}

// applyInstitutionName fills InstitutionName, the display-safe institution:
// the user's own connection label always wins (it exists precisely to
// override a provider-reported name that isn't trustworthy — see
// 00031_data_source_connections.sql), otherwise the provider's institution
// with Pluggy's own proxy connector name filtered out by
// transactions.SanitizeInstitution. Left nil rather than falling back to
// something that isn't actually a bank name.
func applyInstitutionName(d *accountDTO) {
	if d.connectionLabel != nil {
		label := strings.TrimSpace(*d.connectionLabel)
		if label != "" {
			d.InstitutionName = &label
			return
		}
	}
	d.InstitutionName = transactions.SanitizeInstitution(d.Institution)
}

// applyCreditUsage fills CreditUsageRatio as balance / credit_limit for
// credit accounts. It deliberately derives usage from balance rather than
// from credit_limit - available_credit_limit: Pluggy's balance is the
// authoritative "what's still owed on this card" (see handleDebtTotalOwed),
// while availableCreditLimit can lag or be absent entirely. The ratio isn't
// clamped — above 1.0 means the limit was exceeded, which is real
// information the UI should be able to show.
func applyCreditUsage(d *accountDTO) {
	if !isCreditAccount(d) || d.Balance == nil || d.CreditLimit == nil {
		return
	}
	balance, err := decimal.NewFromString(*d.Balance)
	if err != nil {
		return
	}
	limit, err := decimal.NewFromString(*d.CreditLimit)
	if err != nil || !limit.IsPositive() {
		return
	}
	ratio := money.CanonicalDecimal(balance.Div(limit).Round(4))
	d.CreditUsageRatio = &ratio
}

const (
	closingDaySourceManual    = "manual"    // the user stated it
	closingDaySourceReported  = "informado" // creditData.balanceCloseDate
	closingDaySourceEstimated = "estimado"  // day of the most recent closed bill
)

// latestBillClosingDay maps account_id to the day of month its most recent
// closed bill closed on, for the accounts named in accountIDs. It's the
// last-resort source for a card's closing day: connectors that omit
// creditData.balanceCloseDate often still return bills, and the day a card
// closes barely moves month to month.
//
// The max is taken in Go rather than with a window function or LATERAL join
// because none of those spell the same across SQLite and Postgres, and every
// query in this app is shared verbatim between the two engines. Taking it in
// Go is why the account filter matters: without it this would read every
// bill ever synced to answer for one card.
func latestBillClosingDay(ctx context.Context, conn *sql.DB, accountIDs []string) (map[string]int, error) {
	if len(accountIDs) == 0 {
		return map[string]int{}, nil
	}
	in, args := db.InClause(accountIDs)
	rows, err := conn.QueryContext(ctx, `
		SELECT account_id, closing_date FROM financial_bills
		WHERE closing_date IS NOT NULL AND account_id IN (`+in+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("query bill closing dates: %w", err)
	}
	defer rows.Close()

	newest := map[string]time.Time{}
	for rows.Next() {
		var accountID string
		var closingRaw sql.NullString
		if err := rows.Scan(dbtypes.ID(&accountID), dbtypes.Time(&closingRaw)); err != nil {
			return nil, err
		}
		closing, err := dbtypes.ParseNullTime(closingRaw)
		if err != nil || closing == nil {
			continue
		}
		if seen, ok := newest[accountID]; !ok || closing.After(seen) {
			newest[accountID] = *closing
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	days := make(map[string]int, len(newest))
	for accountID, closing := range newest {
		days[accountID] = closing.Day()
	}
	return days, nil
}

// applyClosingDay fills ClosingDay for credit accounts, in decreasing order of
// trust: what the user stated, then the provider's own balanceCloseDate, then
// the most recent closed bill. Left nil when none of the three is available,
// which is exactly the case the manual override exists to cover.
//
// All three sources are ranked here, behind a single account-type gate, so a
// bank account can never report a closing day — not even a manual override
// left behind by an account that used to be a card.
func applyClosingDay(d *accountDTO, billDays map[string]int) {
	if !isCreditAccount(d) {
		return
	}
	switch {
	case d.manualClosingDay != nil:
		setClosingDay(d, *d.manualClosingDay, closingDaySourceManual)
	case d.BalanceCloseDate != nil:
		setClosingDay(d, d.BalanceCloseDate.Day(), closingDaySourceReported)
	default:
		if day, ok := billDays[d.ID]; ok {
			setClosingDay(d, day, closingDaySourceEstimated)
		}
	}
}

// billClosingDayCandidates names the accounts whose closing day can only come
// from a bill: credit accounts with neither an override nor a date from the
// provider. Everything else already has a better answer, so querying bills
// for it would be work whose result applyClosingDay discards.
func billClosingDayCandidates(accounts []accountDTO) []string {
	ids := []string{}
	for _, d := range accounts {
		if isCreditAccount(&d) && d.manualClosingDay == nil && d.BalanceCloseDate == nil {
			ids = append(ids, d.ID)
		}
	}
	return ids
}

func setClosingDay(d *accountDTO, day int, source string) {
	value, from := day, source
	d.ClosingDay, d.ClosingDaySource = &value, &from
}

func handleListAccounts(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := conn.QueryContext(r.Context(), `
			SELECT `+accountSelectColumns+accountSelectFrom+`
			ORDER BY fa.account_type, fa.institution, fa.name, fa.id`)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		defer rows.Close()

		result := []accountDTO{}
		for rows.Next() {
			d, err := scanAccount(rows)
			if err != nil {
				accountsUnavailableProblem(w)
				return
			}
			result = append(result, d)
		}
		if err := rows.Err(); err != nil {
			accountsUnavailableProblem(w)
			return
		}

		rows.Close()
		for i := range result {
			result[i].Balance, err = transactions.AccountBalance(r.Context(), conn, result[i].ID)
			if err != nil {
				accountsUnavailableProblem(w)
				return
			}
		}
		billDays, err := latestBillClosingDay(r.Context(), conn, billClosingDayCandidates(result))
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		for i := range result {
			applyClosingDay(&result[i], billDays)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleGetAccount(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row := conn.QueryRowContext(r.Context(), `
			SELECT `+accountSelectColumns+accountSelectFrom+`
			WHERE fa.id = ?`, r.PathValue("id"))
		d, err := scanAccount(row)
		if err == sql.ErrNoRows {
			accountNotFoundProblem(w)
			return
		}
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		billDays, err := latestBillClosingDay(r.Context(), conn, billClosingDayCandidates([]accountDTO{d}))
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		d.Balance, err = transactions.AccountBalance(r.Context(), conn, d.ID)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		applyClosingDay(&d, billDays)
		writeJSON(w, http.StatusOK, d)
	}
}

type accountClosingDayRequest struct {
	// A null closing_day clears the override and lets the provider sources
	// take over again.
	ClosingDay *int `json:"closing_day"`
}

// handleSetAccountClosingDay records the user's own closing day for a card.
// Restricted to credit accounts: a bank account has no invoice to close.
func handleSetAccountClosingDay(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		var req accountClosingDayRequest
		if err := decodeStrict(r, &req); err != nil || (req.ClosingDay != nil && (*req.ClosingDay < 1 || *req.ClosingDay > 31)) {
			writeProblem(w, 422, "invalid-account-closing-day", "Dia de fechamento inválido", "Informe um dia entre 1 e 31.")
			return
		}

		var accountType sql.NullString
		err := conn.QueryRowContext(r.Context(),
			`SELECT account_type FROM financial_accounts WHERE id = ?`, accountID).Scan(&accountType)
		if err == sql.ErrNoRows {
			accountNotFoundProblem(w)
			return
		}
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		if accountType.String != "CREDIT" {
			writeProblem(w, 422, "invalid-account-closing-day", "Dia de fechamento inválido", "Só cartões de crédito têm dia de fechamento.")
			return
		}

		if _, err := conn.ExecContext(r.Context(),
			`UPDATE financial_accounts SET manual_closing_day = ? WHERE id = ?`,
			req.ClosingDay, accountID); err != nil {
			accountsUnavailableProblem(w)
			return
		}

		row := conn.QueryRowContext(r.Context(), `
			SELECT `+accountSelectColumns+accountSelectFrom+`
			WHERE fa.id = ?`, accountID)
		d, err := scanAccount(row)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		billDays, err := latestBillClosingDay(r.Context(), conn, billClosingDayCandidates([]accountDTO{d}))
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		d.Balance, err = transactions.AccountBalance(r.Context(), conn, d.ID)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		applyClosingDay(&d, billDays)
		writeJSON(w, http.StatusOK, d)
	}
}

// accountExists guards the sub-resource handlers so an unknown id gets a 404
// instead of an empty list.
func accountExists(ctx context.Context, conn *sql.DB, id string) (bool, error) {
	var exists bool
	err := conn.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM financial_accounts WHERE id = ?)`, id).Scan(dbtypes.Bool(&exists))
	return exists, err
}

type accountCardDTO struct {
	CardNumber        string     `json:"card_number"`
	TransactionCount  int        `json:"transaction_count"`
	LastTransactionAt *time.Time `json:"last_transaction_at"`
}

// cardNumberMetadata types only the one key this handler needs out of
// financial_transactions.credit_card_metadata. Package transactions and
// package automation each decode their own subset of the same opaque blob —
// duplicating the struct is the established convention here, not an
// oversight.
type cardNumberMetadata struct {
	CardNumber *string `json:"cardNumber"`
}

// handleListAccountCards groups an account's transactions by the card number
// stamped on each one, so a credit account can show which physical cards
// spend against it. The grouping happens in Go rather than in SQL because
// JSON extraction has no portable spelling — json_extract is SQLite-only and
// ->> is Postgres-only, and every other query in this app is shared verbatim
// between the two engines.
//
// No per-card total is reported: on credit accounts Pluggy mixes purchases,
// payments and refunds in the same amount column, so an all-history sum would
// be a meaningless number wearing an authoritative face.
func handleListAccountCards(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		exists, err := accountExists(r.Context(), conn, accountID)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		if !exists {
			accountNotFoundProblem(w)
			return
		}

		rows, err := conn.QueryContext(r.Context(), `
			SELECT credit_card_metadata, occurred_at
			FROM financial_transactions
			WHERE account_id = ? AND credit_card_metadata IS NOT NULL`, accountID)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		defer rows.Close()

		byNumber := map[string]*accountCardDTO{}
		for rows.Next() {
			var metadata string
			var occurredRaw sql.NullString
			if err := rows.Scan(&metadata, dbtypes.Time(&occurredRaw)); err != nil {
				accountsUnavailableProblem(w)
				return
			}
			var parsed cardNumberMetadata
			// The blob is opaque provider data: metadata that doesn't decode
			// or carries no card number is skipped, never fatal.
			if err := json.Unmarshal([]byte(metadata), &parsed); err != nil {
				continue
			}
			if parsed.CardNumber == nil || *parsed.CardNumber == "" {
				continue
			}
			occurredAt, err := dbtypes.ParseNullTime(occurredRaw)
			if err != nil {
				occurredAt = nil
			}
			card, seen := byNumber[*parsed.CardNumber]
			if !seen {
				card = &accountCardDTO{CardNumber: *parsed.CardNumber}
				byNumber[*parsed.CardNumber] = card
			}
			card.TransactionCount++
			if occurredAt != nil && (card.LastTransactionAt == nil || occurredAt.After(*card.LastTransactionAt)) {
				card.LastTransactionAt = occurredAt
			}
		}
		if err := rows.Err(); err != nil {
			accountsUnavailableProblem(w)
			return
		}

		result := make([]accountCardDTO, 0, len(byNumber))
		for _, card := range byNumber {
			result = append(result, *card)
		}
		sort.Slice(result, func(i, j int) bool {
			li, lj := result[i].LastTransactionAt, result[j].LastTransactionAt
			if li != nil && lj != nil && !li.Equal(*lj) {
				return li.After(*lj)
			}
			if (li == nil) != (lj == nil) {
				return li != nil
			}
			return result[i].CardNumber < result[j].CardNumber
		})
		writeJSON(w, http.StatusOK, result)
	}
}

type accountBillDTO struct {
	ID                   string     `json:"id"`
	ExternalID           string     `json:"external_id"`
	DueDate              *time.Time `json:"due_date"`
	ClosingDate          *time.Time `json:"closing_date"`
	TotalAmount          *string    `json:"total_amount"`
	CurrencyCode         *string    `json:"currency_code"`
	MinimumPaymentAmount *string    `json:"minimum_payment_amount"`
}

// handleListAccountBills serves the faturas already synced for an account.
// Pluggy only exposes closed and overdue bills, so the currently open fatura
// is never here — it's represented by the account's balance plus its
// balance_close_date/balance_due_date.
func handleListAccountBills(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("id")
		exists, err := accountExists(r.Context(), conn, accountID)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		if !exists {
			accountNotFoundProblem(w)
			return
		}

		rows, err := conn.QueryContext(r.Context(), `
			SELECT id, external_id, due_date, closing_date, total_amount,
				currency_code, minimum_payment_amount
			FROM financial_bills
			WHERE account_id = ?
			ORDER BY due_date DESC, id DESC`, accountID)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		defer rows.Close()

		result := []accountBillDTO{}
		for rows.Next() {
			var (
				d          accountBillDTO
				dueRaw     sql.NullString
				closingRaw sql.NullString
			)
			if err := rows.Scan(dbtypes.ID(&d.ID), dbtypes.ID(&d.ExternalID), dbtypes.Time(&dueRaw), dbtypes.Time(&closingRaw),
				dbtypes.Decimal(&d.TotalAmount), &d.CurrencyCode, dbtypes.Decimal(&d.MinimumPaymentAmount)); err != nil {
				accountsUnavailableProblem(w)
				return
			}
			if d.DueDate, err = dbtypes.ParseNullTime(dueRaw); err != nil {
				accountsUnavailableProblem(w)
				return
			}
			if d.ClosingDate, err = dbtypes.ParseNullTime(closingRaw); err != nil {
				accountsUnavailableProblem(w)
				return
			}
			result = append(result, d)
		}
		if err := rows.Err(); err != nil {
			accountsUnavailableProblem(w)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}
