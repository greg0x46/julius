package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/dates"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

// CreditCardTransactionTotal calculates the card amount from eligible
// transactions instead of using financial_accounts.balance. The provider
// balance does not carry this application's inclusion decisions and can
// therefore include payments or purchases that the user has ignored
// locally. This is the single source of truth for "what's currently owed
// on open credit cards" — every caller that needs that figure (the
// payables total-owed summary, net worth's liability breakdown) must go
// through this function so their numbers agree by construction.
func CreditCardTransactionTotal(ctx context.Context, q Querier, currencyCode string) (decimal.Decimal, error) {
	return CreditCardTransactionTotalAt(ctx, q, currencyCode, time.Now(), time.Local)
}

// CreditCardTransactionTotalAt keeps the clock and location explicit so the
// cycle rule can be tested at calendar boundaries without changing the
// production behavior. time.Local is the application's process-configured
// timezone (for example, the TZ setting); it is intentionally used for
// local calendar days instead of comparing only UTC dates.
func CreditCardTransactionTotalAt(ctx context.Context, q Querier, currencyCode string, now time.Time, loc *time.Location) (decimal.Decimal, error) {
	return creditCardTransactionTotalAt(ctx, q, currencyCode, now, loc, nil)
}

// selected optionally collects exactly the transactions contributing to the total.
func creditCardTransactionTotalAt(ctx context.Context, q Querier, currencyCode string, now time.Time, loc *time.Location, selected map[string]bool) (decimal.Decimal, error) {
	if loc == nil {
		return decimal.Zero, fmt.Errorf("credit card cycle timezone is nil")
	}

	bills, err := fetchCardBillClosingDates(ctx, q, loc)
	if err != nil {
		return decimal.Zero, err
	}

	rows, err := q.QueryContext(ctx, `
		SELECT id FROM financial_accounts
		WHERE account_type = 'CREDIT' AND currency_code = ?`, currencyCode)
	if err != nil {
		return decimal.Zero, fmt.Errorf("query credit card accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]string, 0)
	for rows.Next() {
		var accountID string
		if err := rows.Scan(dbtypes.ID(&accountID)); err != nil {
			return decimal.Zero, err
		}
		accounts = append(accounts, accountID)
	}
	if err := rows.Err(); err != nil {
		return decimal.Zero, err
	}

	total := decimal.Zero
	for _, accountID := range accounts {
		amount, err := consideredCardTransactionTotal(ctx, q, accountID, currencyCode, bills, now, loc, selected)
		if err != nil {
			return decimal.Zero, err
		}
		total = total.Add(amount)
	}
	return total, nil
}

type cardBillReference struct {
	closingDate *time.Time
}

type cardBillClosingDates struct {
	byAccount map[string][]time.Time
	byID      map[string]cardBillReference
}

func cardKey(accountID, billID string) string {
	return accountID + "\x00" + billID
}

func fetchCardBillClosingDates(ctx context.Context, q Querier, loc *time.Location) (cardBillClosingDates, error) {
	result := cardBillClosingDates{
		byAccount: make(map[string][]time.Time),
		byID:      make(map[string]cardBillReference),
	}
	rows, err := q.QueryContext(ctx, `
		SELECT account_id, id, external_id, closing_date
		FROM financial_bills`)
	if err != nil {
		return cardBillClosingDates{}, fmt.Errorf("query card bill closing dates: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accountID, internalID, externalID string
		var closingDate *time.Time
		if err := rows.Scan(dbtypes.ID(&accountID), dbtypes.ID(&internalID), dbtypes.ID(&externalID), dbtypes.Date(&closingDate)); err != nil {
			return cardBillClosingDates{}, err
		}

		if closingDate != nil {
			// Preserve the provider calendar day before creating the boundary
			// in the application's location.
			utcDate := closingDate.UTC()
			boundary := time.Date(utcDate.Year(), utcDate.Month(), utcDate.Day(), 0, 0, 0, 0, loc)
			closingDate = &boundary
			result.byAccount[accountID] = append(result.byAccount[accountID], boundary)
		}

		reference := cardBillReference{closingDate: closingDate}
		result.byID[cardKey(accountID, externalID)] = reference
		// Pluggy's billId is the external id. Keeping the internal id too
		// makes the association safe for locally-created/test data without
		// changing the provider-facing rule.
		result.byID[cardKey(accountID, internalID)] = reference
	}
	if err := rows.Err(); err != nil {
		return cardBillClosingDates{}, err
	}

	for accountID := range result.byAccount {
		sort.Slice(result.byAccount[accountID], func(i, j int) bool {
			return result.byAccount[accountID][i].Before(result.byAccount[accountID][j])
		})
	}
	return result, nil
}

type creditCardCycle struct {
	start time.Time
	end   time.Time
}

func currentCreditCardCycle(bills cardBillClosingDates, accountID string, now time.Time, loc *time.Location) (creditCardCycle, bool) {
	closings := bills.byAccount[accountID]
	if len(closings) == 0 {
		return creditCardCycle{}, false
	}

	today := localDay(now, loc)
	var lastClosing time.Time
	var nextClosing time.Time
	for _, closing := range closings {
		if closing.After(today) {
			if !lastClosing.IsZero() {
				nextClosing = closing
			}
			break
		}
		lastClosing = closing
	}
	if lastClosing.IsZero() {
		// All available bills close in the future, so there is no
		// historical lower bound for the current cycle.
		return creditCardCycle{}, false
	}
	if nextClosing.IsZero() {
		nextClosing = inferredNextClosing(lastClosing, closings, loc)
	}
	if !nextClosing.After(lastClosing) {
		return creditCardCycle{}, false
	}
	return creditCardCycle{start: lastClosing, end: nextClosing}, true
}

func localDay(value time.Time, loc *time.Location) time.Time {
	local := value.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func inferredNextClosing(lastClosing time.Time, closings []time.Time, loc *time.Location) time.Time {
	closingDay := lastClosing.Day()
	// A monthly closing on the 29th, 30th, or 31st can be represented by
	// the month's last day when that day does not exist. If earlier history
	// proves a larger nominal day, retain it for the next month.
	if closingDay == dates.DaysInMonth(lastClosing.Year(), lastClosing.Month()) {
		for _, closing := range closings {
			if closing.Before(lastClosing) && closing.Day() > closingDay {
				closingDay = closing.Day()
			}
		}
	}

	nextMonth := time.Date(lastClosing.Year(), lastClosing.Month()+1, 1, 0, 0, 0, 0, loc)
	day := closingDay
	if maxDay := dates.DaysInMonth(nextMonth.Year(), nextMonth.Month()); day > maxDay {
		day = maxDay
	}
	return time.Date(nextMonth.Year(), nextMonth.Month(), day, 0, 0, 0, 0, loc)
}

type cardDebtTransaction struct {
	id                      string
	amount                  *decimal.Decimal
	amountInAccountCurrency *decimal.Decimal
	currencyCode            *string
	occurredAt              *time.Time
	providerStatus          *string
	movementType            *string
	sourceCategory          *string
	sourceCategoryID        *string
	additionalInfo          *string
	creditCardMetadata      *string
	inclusionState          *string
}

// isCardPaymentLeg reports whether transaction is the card-account credit
// leg of paying a credit card bill (see categories.IsCardPaymentTransaction)
// — never the bank-account debit leg, since fetchCardDebtTransactions only
// ever queries transactions already scoped to a CREDIT account.
func (t cardDebtTransaction) isCardPaymentLeg() bool {
	return categories.IsCardPaymentTransaction(strOrEmpty(t.movementType), strOrEmpty(t.sourceCategory), strOrEmpty(t.sourceCategoryID), strOrEmpty(t.additionalInfo))
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// consideredCardTransactionTotal sums the local transactions that represent
// card cost in the current cycle. A card debit adds its absolute value to the
// positive debt indicator; a credit/refund subtracts its absolute value. An
// ignored bill payment never reaches this sign normalization.
func consideredCardTransactionTotal(
	ctx context.Context,
	q Querier,
	accountID, currencyCode string,
	bills cardBillClosingDates,
	now time.Time,
	loc *time.Location,
	selected map[string]bool,
) (decimal.Decimal, error) {
	cycle, ok := currentCreditCardCycle(bills, accountID, now, loc)
	if !ok {
		// Without a reliable historical closing and a next boundary, the
		// conservative result is no locally calculated card amount. In
		// particular, never substitute due_date or billForecastDate here.
		return decimal.Zero, nil
	}

	transactions, err := fetchCardDebtTransactions(ctx, q, accountID)
	if err != nil {
		return decimal.Zero, fmt.Errorf("query credit card transactions: %w", err)
	}

	accountCurrency := currencyCode
	total := decimal.Zero
	for _, transaction := range transactions {
		// A missing decision has the same domain meaning as considered. Only
		// an explicit ignored decision removes a transaction from the sum.
		if transaction.inclusionState != nil && *transaction.inclusionState != string(money.Considered) {
			continue
		}
		if !cardTransactionBelongsToCurrentCycle(accountID, transaction, bills, cycle) {
			continue
		}

		classification := money.Classify(transaction.movementType)
		effective := money.SelectEffectiveMoney(
			transaction.amountInAccountCurrency, &accountCurrency,
			transaction.amount, transaction.currencyCode,
		)
		// "" (no category kind) is deliberate: paying a card bill is itself a
		// transfer between the user's own accounts, so letting the transfer
		// rule reach here would stop the payment from being subtracted and
		// inflate what the card is owed. The ignored decision was already
		// applied above; Considered is passed for the same reason.
		included, _ := money.Eligibility(classification, transaction.providerStatus, effective, money.Considered, "")
		if !included || effective == nil || effective.CurrencyCode != currencyCode {
			continue
		}
		if selected != nil {
			selected[transaction.id] = true
		}
		switch classification {
		case money.Outflow:
			total = total.Add(effective.Value.Abs())
		case money.Inflow:
			total = total.Sub(effective.Value.Abs())
		}
	}
	return total, nil
}

func cardTransactionIsInCycle(occurredAt *time.Time, cycle creditCardCycle) bool {
	return occurredAt != nil && !occurredAt.Before(cycle.start) && occurredAt.Before(cycle.end)
}

// cardTransactionBelongsToCurrentCycle resolves an ordinary purchase/refund
// by date when its billId is missing or unknown — the best available signal
// for something that genuinely happened on that day. A bill-payment CREDIT
// (see cardDebtTransaction.isCardPaymentLeg) is different in kind: it never
// represents new activity on the currently accruing cycle, it settles an
// already-closed bill, and Pluggy generally does not tag it with a billId at
// all. Its occurred_at is not a reliable substitute — a bill's due date sits
// 7-10 days *after* its closing date, i.e. inside the window this function
// already calls the next, still-open cycle, so a payment made anywhere near
// its own due date (the normal case) would otherwise be misattributed as
// debt reduction on a cycle it has nothing to do with, silently driving the
// total negative. Without a resolvable billId, the safe choice for a
// payment leg is exclusion, not a date guess.
func cardTransactionBelongsToCurrentCycle(accountID string, transaction cardDebtTransaction, bills cardBillClosingDates, cycle creditCardCycle) bool {
	fallback := func() bool {
		if transaction.isCardPaymentLeg() {
			return false
		}
		return cardTransactionIsInCycle(transaction.occurredAt, cycle)
	}

	raw := transaction.creditCardMetadata
	if raw == nil {
		return fallback()
	}
	// cardTransactionMetadata is shared with cardflow.go's ProjectedEntryDate
	// — same Pluggy credit_card_metadata shape, read here only for billId.
	var metadata cardTransactionMetadata
	if err := json.Unmarshal([]byte(*raw), &metadata); err != nil || metadata.BillID == nil || *metadata.BillID == "" {
		return fallback()
	}

	reference, known := bills.byID[cardKey(accountID, *metadata.BillID)]
	if !known {
		// An unknown billId can refer to the provider's still-open bill,
		// which Pluggy does not return through the bills endpoint. The
		// occurrence date remains the only available classification signal
		// for anything that isn't a payment leg.
		return fallback()
	}
	if reference.closingDate == nil {
		// The association is known but its historical closing date is not;
		// do not risk counting it as a current-cycle transaction. This is
		// intentionally not a due_date fallback.
		return false
	}
	// The current-cycle bill is the next closing boundary, which is exclusive
	// for transactions classified only by occurred_at. A transaction already
	// assigned to the bill at that boundary belongs to that bill, even if the
	// provider's posting date is outside the cycle window.
	//
	// This applies identically to a payment leg with a resolved billId — a
	// payment against the still-open current-cycle bill (closingDate ==
	// cycle.end) is a legitimate early/partial payment that should reduce
	// what the cycle will end up owing, unlike the unknown-billId case above,
	// where occurred_at is the only signal and is known to misattribute
	// payments (see this function's doc comment). A known billId is a
	// stronger signal than a date guess regardless of which bill it names.
	return reference.closingDate.After(cycle.start) && !reference.closingDate.After(cycle.end)
}

// billForecastDate is deliberately decoded above only to document the
// provider metadata shape. It identifies an expected month, not a closing
// boundary, so it never participates in the debt indicator's classification.

func fetchCardDebtTransactions(ctx context.Context, q Querier, accountID string) ([]cardDebtTransaction, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT ft.id, ft.amount, ft.amount_in_account_currency, ft.currency_code,
		       ft.occurred_at, ft.provider_status, ft.movement_type,
		       ft.source_category, ft.source_category_id, ft.operation_type_additional_info,
		       ft.credit_card_metadata, tid.state
		FROM financial_transactions ft
		LEFT JOIN transaction_inclusion_decisions tid ON tid.transaction_id = ft.id
		WHERE ft.account_id = ? AND ft.deleted_at IS NULL`, accountID)
	if err != nil {
		return nil, fmt.Errorf("query credit card transactions: %w", err)
	}
	defer rows.Close()

	result := make([]cardDebtTransaction, 0)
	for rows.Next() {
		var transactionID string
		var amountRaw, amountInAccountCurrencyRaw, currencyCodeRaw sql.NullString
		var occurredAtRaw, providerStatusRaw, movementTypeRaw sql.NullString
		var sourceCategoryRaw, sourceCategoryIDRaw, additionalInfoRaw sql.NullString
		var metadataRaw, inclusionStateRaw sql.NullString
		if err := rows.Scan(
			dbtypes.ID(&transactionID), dbtypes.Decimal(&amountRaw), dbtypes.Decimal(&amountInAccountCurrencyRaw), &currencyCodeRaw,
			dbtypes.Time(&occurredAtRaw),
			&providerStatusRaw, &movementTypeRaw,
			&sourceCategoryRaw, dbtypes.ID(&sourceCategoryIDRaw), &additionalInfoRaw,
			&metadataRaw, &inclusionStateRaw,
		); err != nil {
			return nil, err
		}
		amount, err := decimalFromNullString(amountRaw)
		if err != nil {
			return nil, err
		}
		amountInAccountCurrency, err := decimalFromNullString(amountInAccountCurrencyRaw)
		if err != nil {
			return nil, err
		}
		occurredAt, err := dbtypes.ParseNullTime(occurredAtRaw)
		if err != nil {
			return nil, err
		}
		result = append(result, cardDebtTransaction{
			id:                      transactionID,
			amount:                  amount,
			amountInAccountCurrency: amountInAccountCurrency,
			currencyCode:            stringFromNullString(currencyCodeRaw),
			occurredAt:              occurredAt,
			providerStatus:          stringFromNullString(providerStatusRaw),
			movementType:            stringFromNullString(movementTypeRaw),
			sourceCategory:          stringFromNullString(sourceCategoryRaw),
			sourceCategoryID:        stringFromNullString(sourceCategoryIDRaw),
			additionalInfo:          stringFromNullString(additionalInfoRaw),
			creditCardMetadata:      stringFromNullString(metadataRaw),
			inclusionState:          stringFromNullString(inclusionStateRaw),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func decimalFromNullString(value sql.NullString) (*decimal.Decimal, error) {
	if !value.Valid {
		return nil, nil
	}
	amount, err := decimal.NewFromString(value.String)
	if err != nil {
		return nil, fmt.Errorf("parse decimal %q: %w", value.String, err)
	}
	return &amount, nil
}

func stringFromNullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
