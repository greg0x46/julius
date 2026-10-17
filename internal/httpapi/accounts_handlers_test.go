package httpapi_test

import (
	"database/sql"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
)

// accountSeed carries the ids insertAccount created, so follow-up helpers can
// hang transactions and bills off the same sync-schema chain.
type accountSeed struct {
	sourceID    string
	syncRunID   string
	rawImportID string
	accountID   string
}

// insertAccount inserts the minimal sync-schema chain plus one
// financial_accounts row directly, so HTTP tests can exercise the read
// endpoints without a full sync pipeline. Money columns take the raw storage
// strings (nil for NULL) so tests can pin the credit-usage edge cases.
func insertAccount(t *testing.T, conn *sql.DB, accountType, name string, balance, creditLimit *string) accountSeed {
	t.Helper()
	now := db.FormatTime(time.Now())
	seed := accountSeed{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO data_sources (id, provider, external_item_id, display_name, created_at, updated_at)
		VALUES (?, 'pluggy', ?, 'Banco Exemplo', ?, ?)`, seed.sourceID, seed.sourceID, now, now)
	exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, seed.syncRunID, seed.sourceID, now, now)
	exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'accounts', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		seed.rawImportID, seed.syncRunID, seed.sourceID, now)
	exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, institution, name, account_type, balance, credit_limit,
			currency_code, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, 'Banco Exemplo', ?, ?, ?, ?, 'BRL', ?, 'hash', ?, ?)`,
		seed.accountID, seed.sourceID, seed.accountID, name, accountType, balance, creditLimit,
		seed.rawImportID, now, now)
	return seed
}

// insertAccountTransaction hangs one financial_transactions row off a seeded
// account, with credit_card_metadata passed through verbatim so tests can
// feed the card aggregator malformed blobs.
func insertAccountTransaction(t *testing.T, conn *sql.DB, seed accountSeed, metadata *string, occurredAt time.Time) {
	t.Helper()
	now := db.FormatTime(time.Now())
	txID := uuid.NewString()
	_, err := conn.Exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, credit_card_metadata,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'Compra', '-10.00', '-10.00', 'BRL', ?, 'POSTED', 'DEBIT', ?, ?, 'hash', ?, ?)`,
		txID, seed.sourceID, seed.accountID, txID, db.FormatTime(occurredAt), metadata, seed.rawImportID, now, now)
	if err != nil {
		t.Fatalf("insert transaction: %v", err)
	}
}

func insertBill(t *testing.T, conn *sql.DB, seed accountSeed, dueDate time.Time, totalAmount string) {
	t.Helper()
	now := db.FormatTime(time.Now())
	billID := uuid.NewString()
	_, err := conn.Exec(`INSERT INTO financial_bills (
			id, source_id, account_id, external_id, due_date, closing_date, total_amount,
			currency_code, minimum_payment_amount, current_raw_import_id, normalized_hash,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'BRL', '120.00', ?, 'hash', ?, ?)`,
		billID, seed.sourceID, seed.accountID, billID, dueDate.Format("2006-01-02"),
		dueDate.AddDate(0, 0, -7).Format("2006-01-02"), totalAmount, seed.rawImportID, now, now)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}
}

func setBalanceCloseDate(t *testing.T, conn *sql.DB, seed accountSeed, closeDate time.Time) {
	t.Helper()
	if _, err := conn.Exec(`UPDATE financial_accounts SET balance_close_date = ? WHERE id = ?`,
		closeDate.Format("2006-01-02"), seed.accountID); err != nil {
		t.Fatalf("set balance_close_date: %v", err)
	}
}

func getAccountJSON(t *testing.T, baseURL, accountID string) map[string]any {
	t.Helper()
	resp, err := testGet(t, baseURL+"/api/accounts/"+accountID)
	if err != nil {
		t.Fatalf("GET /api/accounts/{id}: %v", err)
	}
	var got map[string]any
	decodeJSON(t, resp, &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	return got
}

func strPtr(s string) *string { return &s }

func TestListAccountsReturnsBankAndCreditAccounts(t *testing.T) {
	srv, conn := newTestServer(t)
	insertAccount(t, conn, "BANK", "Conta Corrente", strPtr("500.00"), nil)
	insertAccount(t, conn, "CREDIT", "Cartão Platinum", strPtr("1234.56"), strPtr("5000.00"))

	resp, err := testGet(t, srv.URL+"/api/accounts")
	if err != nil {
		t.Fatalf("GET /api/accounts: %v", err)
	}
	var accounts []map[string]any
	decodeJSON(t, resp, &accounts)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(accounts) != 2 {
		t.Fatalf("len(accounts) = %d, want 2", len(accounts))
	}
	// ORDER BY account_type puts BANK before CREDIT.
	if accounts[0]["account_type"] != "BANK" || accounts[0]["name"] != "Conta Corrente" {
		t.Errorf("first account = %+v", accounts[0])
	}
	if accounts[0]["source_display_name"] != "Banco Exemplo" || accounts[0]["balance"] != "500.00" {
		t.Errorf("first account = %+v", accounts[0])
	}
	if accounts[1]["account_type"] != "CREDIT" || accounts[1]["credit_limit"] != "5000.00" {
		t.Errorf("second account = %+v", accounts[1])
	}
}

func TestListAccountsReturnsEmptyArrayNotNull(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := testGet(t, srv.URL+"/api/accounts")
	if err != nil {
		t.Fatalf("GET /api/accounts: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "[]\n" {
		t.Errorf("body = %q, want an empty JSON array", body)
	}
}

func TestListAccountsComputesCreditUsageRatio(t *testing.T) {
	srv, conn := newTestServer(t)
	insertAccount(t, conn, "CREDIT", "Cartão", strPtr("1234.56"), strPtr("5000.00"))

	resp, err := testGet(t, srv.URL+"/api/accounts")
	if err != nil {
		t.Fatalf("GET /api/accounts: %v", err)
	}
	var accounts []map[string]any
	decodeJSON(t, resp, &accounts)
	if got := accounts[0]["credit_usage_ratio"]; got != "0.2469" {
		t.Errorf("credit_usage_ratio = %v, want 0.2469", got)
	}
}

func TestListAccountsOmitsUsageRatioWhenNotComputable(t *testing.T) {
	cases := []struct {
		name        string
		accountType string
		balance     *string
		creditLimit *string
	}{
		{"limite ausente", "CREDIT", strPtr("100.00"), nil},
		{"limite zerado", "CREDIT", strPtr("100.00"), strPtr("0")},
		{"saldo ausente", "CREDIT", nil, strPtr("5000.00")},
		{"conta bancária", "BANK", strPtr("100.00"), strPtr("5000.00")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, conn := newTestServer(t)
			insertAccount(t, conn, tc.accountType, "Conta", tc.balance, tc.creditLimit)

			resp, err := testGet(t, srv.URL+"/api/accounts")
			if err != nil {
				t.Fatalf("GET /api/accounts: %v", err)
			}
			var accounts []map[string]any
			decodeJSON(t, resp, &accounts)
			if got := accounts[0]["credit_usage_ratio"]; got != nil {
				t.Errorf("credit_usage_ratio = %v, want null", got)
			}
		})
	}
}

func TestGetAccountReturnsAccount(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))

	resp, err := testGet(t, srv.URL+"/api/accounts/"+seed.accountID)
	if err != nil {
		t.Fatalf("GET /api/accounts/{id}: %v", err)
	}
	var got map[string]any
	decodeJSON(t, resp, &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got["id"] != seed.accountID || got["credit_usage_ratio"] != "0.1000" {
		t.Errorf("account = %+v", got)
	}
}

func TestGetAccountReturns404WhenMissing(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := testGet(t, srv.URL+"/api/accounts/"+uuid.NewString())
	if err != nil {
		t.Fatalf("GET /api/accounts/{id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestListAccountCardsGroupsDistinctCardNumbers(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
	older := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	insertAccountTransaction(t, conn, seed, strPtr(`{"cardNumber":"1111"}`), older)
	insertAccountTransaction(t, conn, seed, strPtr(`{"cardNumber":"1111"}`), newer)
	insertAccountTransaction(t, conn, seed, strPtr(`{"cardNumber":"2222"}`), older)

	resp, err := testGet(t, srv.URL+"/api/accounts/"+seed.accountID+"/cards")
	if err != nil {
		t.Fatalf("GET cards: %v", err)
	}
	var cards []map[string]any
	decodeJSON(t, resp, &cards)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(cards) != 2 {
		t.Fatalf("len(cards) = %d, want 2", len(cards))
	}
	// Most recently used card first.
	if cards[0]["card_number"] != "1111" || cards[0]["transaction_count"] != float64(2) {
		t.Errorf("first card = %+v", cards[0])
	}
	if cards[1]["card_number"] != "2222" || cards[1]["transaction_count"] != float64(1) {
		t.Errorf("second card = %+v", cards[1])
	}
}

func TestListAccountCardsSkipsMalformedMetadata(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
	occurred := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	insertAccountTransaction(t, conn, seed, strPtr(`nao e json`), occurred)
	insertAccountTransaction(t, conn, seed, strPtr(`{"installmentNumber":1}`), occurred)
	insertAccountTransaction(t, conn, seed, strPtr(`{"cardNumber":"3333"}`), occurred)

	resp, err := testGet(t, srv.URL+"/api/accounts/"+seed.accountID+"/cards")
	if err != nil {
		t.Fatalf("GET cards: %v", err)
	}
	var cards []map[string]any
	decodeJSON(t, resp, &cards)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(cards) != 1 || cards[0]["card_number"] != "3333" {
		t.Errorf("cards = %+v, want only the well-formed card", cards)
	}
}

func TestListAccountCardsReturns404WhenAccountMissing(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := testGet(t, srv.URL+"/api/accounts/"+uuid.NewString()+"/cards")
	if err != nil {
		t.Fatalf("GET cards: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestListAccountBillsReturnsClosedBillsNewestFirst(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
	insertBill(t, conn, seed, time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), "800.00")
	insertBill(t, conn, seed, time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), "1234.56")

	resp, err := testGet(t, srv.URL+"/api/accounts/"+seed.accountID+"/bills")
	if err != nil {
		t.Fatalf("GET bills: %v", err)
	}
	var bills []map[string]any
	decodeJSON(t, resp, &bills)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(bills) != 2 {
		t.Fatalf("len(bills) = %d, want 2", len(bills))
	}
	if bills[0]["total_amount"] != "1234.56" || bills[1]["total_amount"] != "800.00" {
		t.Errorf("bills = %+v, want newest due date first", bills)
	}
	if bills[0]["minimum_payment_amount"] != "120.00" || bills[0]["closing_date"] == nil {
		t.Errorf("first bill = %+v", bills[0])
	}
}

func TestAccountClosingDayFallsBackFromProviderToBillToNothing(t *testing.T) {
	t.Run("informado pelo provedor", func(t *testing.T) {
		srv, conn := newTestServer(t)
		seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
		setBalanceCloseDate(t, conn, seed, time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC))
		// A bill exists too, but the provider's own date outranks it.
		insertBill(t, conn, seed, time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC), "800.00")

		got := getAccountJSON(t, srv.URL, seed.accountID)
		if got["closing_day"] != float64(3) || got["closing_day_source"] != "informado" {
			t.Errorf("account = %+v", got)
		}
	})

	t.Run("estimado pela última fatura", func(t *testing.T) {
		srv, conn := newTestServer(t)
		seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
		// insertBill sets closing_date to due_date minus 7 days.
		insertBill(t, conn, seed, time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC), "800.00")
		insertBill(t, conn, seed, time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), "900.00")

		got := getAccountJSON(t, srv.URL, seed.accountID)
		if got["closing_day"] != float64(7) || got["closing_day_source"] != "estimado" {
			t.Errorf("account = %+v, want day 7 from the newest bill", got)
		}
	})

	t.Run("indisponível", func(t *testing.T) {
		srv, conn := newTestServer(t)
		seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))

		got := getAccountJSON(t, srv.URL, seed.accountID)
		if got["closing_day"] != nil || got["closing_day_source"] != nil {
			t.Errorf("account = %+v", got)
		}
	})
}

func TestSetAccountClosingDayOverridesEveryProviderSource(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
	setBalanceCloseDate(t, conn, seed, time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC))

	resp := doJSON(t, http.MethodPut, srv.URL+"/api/accounts/"+seed.accountID+"/closing-day",
		map[string]any{"closing_day": 18})
	var got map[string]any
	decodeJSON(t, resp, &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got["closing_day"] != float64(18) || got["closing_day_source"] != "manual" {
		t.Errorf("account = %+v", got)
	}

	reread := getAccountJSON(t, srv.URL, seed.accountID)
	if reread["closing_day"] != float64(18) || reread["closing_day_source"] != "manual" {
		t.Errorf("account = %+v, want the override to stick", reread)
	}
}

// The override lives on financial_accounts, which the sync pipeline rewrites
// every run — this pins that upsertAccount leaves the user's column alone.
func TestSetAccountClosingDaySurvivesAnAccountResync(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
	doJSON(t, http.MethodPut, srv.URL+"/api/accounts/"+seed.accountID+"/closing-day",
		map[string]any{"closing_day": 18}).Body.Close()

	// Exactly the columns upsertAccount touches on an "updated" outcome.
	if _, err := conn.Exec(`
		UPDATE financial_accounts SET institution = ?, name = ?, number = ?, account_type = ?,
			account_subtype = ?, balance = ?, credit_limit = ?, available_credit_limit = ?,
			balance_close_date = ?, balance_due_date = ?, currency_code = ?,
			provider_updated_at = ?, normalized_hash = ?, updated_at = ?
		WHERE id = ?`,
		"Outro Banco", "Cartão renomeado", nil, "CREDIT", nil, "99.00", "100.00", nil,
		nil, nil, "BRL", nil, "novo-hash", db.FormatTime(time.Now()), seed.accountID); err != nil {
		t.Fatalf("simulate resync: %v", err)
	}

	got := getAccountJSON(t, srv.URL, seed.accountID)
	if got["closing_day"] != float64(18) || got["closing_day_source"] != "manual" {
		t.Errorf("account = %+v, want the manual closing day to survive a resync", got)
	}
}

// A card that comes back from the provider as a plain account keeps its
// manual_closing_day row value — nothing deletes it. It must not surface: a
// bank account has no invoice, so every source is gated on the type.
func TestAccountClosingDayIsNeverReportedForABankAccount(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
	doJSON(t, http.MethodPut, srv.URL+"/api/accounts/"+seed.accountID+"/closing-day",
		map[string]any{"closing_day": 18}).Body.Close()

	if _, err := conn.Exec(`UPDATE financial_accounts SET account_type = 'BANK' WHERE id = ?`,
		seed.accountID); err != nil {
		t.Fatalf("change account type: %v", err)
	}

	got := getAccountJSON(t, srv.URL, seed.accountID)
	if got["closing_day"] != nil || got["closing_day_source"] != nil {
		t.Errorf("account = %+v, want no closing day on a bank account", got)
	}
}

func TestSetAccountClosingDayToNullRestoresTheProviderValue(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("10.00"), strPtr("100.00"))
	setBalanceCloseDate(t, conn, seed, time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC))
	url := srv.URL + "/api/accounts/" + seed.accountID + "/closing-day"

	doJSON(t, http.MethodPut, url, map[string]any{"closing_day": 18}).Body.Close()
	resp := doJSON(t, http.MethodPut, url, map[string]any{"closing_day": nil})
	var got map[string]any
	decodeJSON(t, resp, &got)
	if got["closing_day"] != float64(3) || got["closing_day_source"] != "informado" {
		t.Errorf("account = %+v", got)
	}
}

func TestSetAccountClosingDayRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name        string
		accountType string
		body        map[string]any
		wantStatus  int
	}{
		{"dia zero", "CREDIT", map[string]any{"closing_day": 0}, 422},
		{"dia acima de 31", "CREDIT", map[string]any{"closing_day": 32}, 422},
		{"conta bancária", "BANK", map[string]any{"closing_day": 10}, 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, conn := newTestServer(t)
			seed := insertAccount(t, conn, tc.accountType, "Conta", strPtr("10.00"), strPtr("100.00"))

			resp := doJSON(t, http.MethodPut, srv.URL+"/api/accounts/"+seed.accountID+"/closing-day", tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestSetAccountClosingDayReturns404WhenAccountMissing(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPut, srv.URL+"/api/accounts/"+uuid.NewString()+"/closing-day",
		map[string]any{"closing_day": 10})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestListAccountBillsReturnsEmptyArrayForBankAccount(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "BANK", "Conta Corrente", strPtr("500.00"), nil)

	resp, err := testGet(t, srv.URL+"/api/accounts/"+seed.accountID+"/bills")
	if err != nil {
		t.Fatalf("GET bills: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "[]\n" {
		t.Errorf("body = %q, want an empty JSON array", body)
	}
}

func TestAccountCalendarDates(t *testing.T) {
	srv, conn := newTestServer(t)
	seed := insertAccount(t, conn, "CREDIT", "Cartão", strPtr("0"), strPtr("1000"))
	if _, err := conn.Exec(`UPDATE financial_accounts SET balance_close_date = ?, balance_due_date = ? WHERE id = ?`, "2026-04-03", "2026-04-10", seed.accountID); err != nil {
		t.Fatal(err)
	}
	got := getAccountJSON(t, srv.URL, seed.accountID)
	if got["balance_close_date"] != "2026-04-03T00:00:00Z" || got["balance_due_date"] != "2026-04-10T00:00:00Z" || got["closing_day"] != float64(3) {
		t.Fatalf("calendar dates = %+v", got)
	}
}
