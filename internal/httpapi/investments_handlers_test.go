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

// insertInvestment inserts the minimal sync-schema chain plus one
// financial_investments row directly, so HTTP tests can exercise the read
// endpoint without a full sync pipeline.
func insertInvestment(t *testing.T, conn *sql.DB) string {
	t.Helper()
	now := db.FormatTime(time.Now())
	sourceID, syncRunID, rawImportID, investmentID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO data_sources (id, provider, external_item_id, display_name, created_at, updated_at)
		VALUES (?, 'pluggy', ?, 'Banco Exemplo', ?, ?)`, sourceID, sourceID, now, now)
	exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, syncRunID, sourceID, now, now)
	exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'investments', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		rawImportID, syncRunID, sourceID, now)
	exec(`INSERT INTO financial_investments (
			id, source_id, external_id, investment_type, name, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, 'MUTUAL_FUND', 'Fundo XYZ', '1000.50', 'BRL', ?, 'hash', ?, ?)`,
		investmentID, sourceID, investmentID, rawImportID, now, now)
	return investmentID
}

// insertInvestmentTransaction inserts a financial_investment_transactions
// row (plus the raw_import it references) for an investment created by
// insertInvestment, so yield tests can build a buy/sell history.
func insertInvestmentTransaction(t *testing.T, conn *sql.DB, investmentID, movementType, amount string) {
	insertInvestmentMovement(t, conn, investmentID, movementType, directionFor(movementType), amount, nil)
}

// directionFor mirrors what the sync pipeline normalizes into the direction
// column, so the helpers above can stay in the provider's BUY/SELL vocabulary.
func directionFor(movementType string) string {
	if movementType == "SELL" || movementType == "REDEMPTION" || movementType == "INTEREST" {
		return "outflow"
	}
	return "inflow"
}

// nullableDirection lets a test build a movement whose direction the sync
// could not establish, which is what an unrecognized provider movementType
// leaves behind.
func nullableDirection(direction string) any {
	if direction == "" {
		return nil
	}
	return direction
}

func insertInvestmentMovement(t *testing.T, conn *sql.DB, investmentID, movementType, direction, amount string, quantity *string) {
	t.Helper()
	now := db.FormatTime(time.Now())
	var sourceID, syncRunID string
	if err := conn.QueryRow(`SELECT source_id FROM financial_investments WHERE id = ?`, investmentID).Scan(&sourceID); err != nil {
		t.Fatalf("lookup source_id: %v", err)
	}
	if err := conn.QueryRow(`SELECT sync_run_id FROM raw_imports WHERE id = (
		SELECT current_raw_import_id FROM financial_investments WHERE id = ?)`, investmentID).Scan(&syncRunID); err != nil {
		t.Fatalf("lookup sync_run_id: %v", err)
	}
	var pageSequence int
	if err := conn.QueryRow(`SELECT count(*) FROM raw_imports WHERE sync_run_id = ? AND scope = 'investment_transactions'`, syncRunID).Scan(&pageSequence); err != nil {
		t.Fatalf("count raw_imports: %v", err)
	}
	pageSequence++
	rawImportID := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'investment_transactions', ?, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		rawImportID, syncRunID, sourceID, pageSequence, now); err != nil {
		t.Fatalf("insert raw_import: %v", err)
	}
	transactionID := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO financial_investment_transactions (
			id, source_id, investment_id, external_id, movement_type, direction, quantity, value, amount,
			occurred_at, trade_date, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, '1.00', ?, ?, ?, ?, 'hash', ?, ?)`,
		transactionID, sourceID, investmentID, transactionID, movementType, nullableDirection(direction),
		quantity, amount, now, now, rawImportID, now, now); err != nil {
		t.Fatalf("insert investment transaction: %v", err)
	}
}

// insertInvestmentWithTaxFields inserts a financial_investments row with an
// explicit investment_type plus the amount_original/taxes/taxes2 columns
// Feature 1 added, without touching insertInvestment's fixed MUTUAL_FUND
// shape the other tests in this file depend on. Nil arguments leave the
// columns NULL, matching how Pluggy actually sends EQUITY holdings.
func insertInvestmentWithTaxFields(t *testing.T, conn *sql.DB, investmentType string, amountOriginal, taxes, taxes2 any) string {
	t.Helper()
	now := db.FormatTime(time.Now())
	sourceID, syncRunID, rawImportID, investmentID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO data_sources (id, provider, external_item_id, display_name, created_at, updated_at)
		VALUES (?, 'pluggy', ?, 'Banco Exemplo', ?, ?)`, sourceID, sourceID, now, now)
	exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, syncRunID, sourceID, now, now)
	exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'investments', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		rawImportID, syncRunID, sourceID, now)
	exec(`INSERT INTO financial_investments (
			id, source_id, external_id, investment_type, name, balance, currency_code,
			amount_original, taxes, taxes2,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'Fundo XYZ', '1000.50', 'BRL', ?, ?, ?, ?, 'hash', ?, ?)`,
		investmentID, sourceID, investmentID, investmentType, amountOriginal, taxes, taxes2, rawImportID, now, now)
	return investmentID
}

// TestListInvestmentsRoundTripsTaxFieldsForFixedIncomeOnly covers
// amount_original/taxes/taxes2 end to end through GET /api/investments: a
// FIXED_INCOME holding (what real Nubank CDB/LCA data sends) must expose all
// three, while an EQUITY holding (which Pluggy never populates them for)
// must come back null rather than some zero-value placeholder.
func TestListInvestmentsRoundTripsTaxFieldsForFixedIncomeOnly(t *testing.T) {
	srv, conn := newTestServer(t)
	fixedIncomeID := insertInvestmentWithTaxFields(t, conn, "FIXED_INCOME", "900.00", "15.00", "2.50")
	equityID := insertInvestmentWithTaxFields(t, conn, "EQUITY", nil, nil, nil)

	resp, err := testGet(t, srv.URL+"/api/investments")
	if err != nil {
		t.Fatalf("GET /api/investments: %v", err)
	}
	var investments []map[string]any
	decodeJSON(t, resp, &investments)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	byID := map[string]map[string]any{}
	for _, inv := range investments {
		byID[inv["id"].(string)] = inv
	}

	fixedIncome := byID[fixedIncomeID]
	if fixedIncome["amount_original"] != "900.00" || fixedIncome["taxes"] != "15.00" || fixedIncome["taxes2"] != "2.50" {
		t.Errorf("FIXED_INCOME tax fields = amount_original=%v taxes=%v taxes2=%v, want 900.00/15.00/2.50",
			fixedIncome["amount_original"], fixedIncome["taxes"], fixedIncome["taxes2"])
	}

	equity := byID[equityID]
	if equity["amount_original"] != nil || equity["taxes"] != nil || equity["taxes2"] != nil {
		t.Errorf("EQUITY tax fields = amount_original=%v taxes=%v taxes2=%v, want all null",
			equity["amount_original"], equity["taxes"], equity["taxes2"])
	}
}

func TestListInvestmentsReturnsSyncedHoldings(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)

	resp, err := testGet(t, srv.URL+"/api/investments")
	if err != nil {
		t.Fatalf("GET /api/investments: %v", err)
	}
	var investments []map[string]any
	decodeJSON(t, resp, &investments)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(investments) != 1 {
		t.Fatalf("len(investments) = %d, want 1", len(investments))
	}
	got := investments[0]
	if got["id"] != investmentID {
		t.Errorf("id = %v, want %v", got["id"], investmentID)
	}
	if got["name"] != "Fundo XYZ" || got["balance"] != "1000.50" || got["source_display_name"] != "Banco Exemplo" {
		t.Errorf("investment = %+v", got)
	}
}

func TestListInvestmentsReturnsEmptyArrayNotNull(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := testGet(t, srv.URL+"/api/investments")
	if err != nil {
		t.Fatalf("GET /api/investments: %v", err)
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

func TestGetInvestmentReturnsHolding(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)

	resp, err := testGet(t, srv.URL+"/api/investments/"+investmentID)
	if err != nil {
		t.Fatalf("GET /api/investments/{id}: %v", err)
	}
	var got map[string]any
	decodeJSON(t, resp, &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got["id"] != investmentID || got["name"] != "Fundo XYZ" {
		t.Errorf("investment = %+v", got)
	}
}

func TestGetInvestmentReturns404WhenMissing(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := testGet(t, srv.URL+"/api/investments/"+uuid.NewString())
	if err != nil {
		t.Fatalf("GET /api/investments/{id}: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestListInvestmentTransactionsReturnsHistory(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	insertInvestmentTransaction(t, conn, investmentID, "BUY", "1000.00")

	resp, err := testGet(t, srv.URL+"/api/investments/"+investmentID+"/transactions")
	if err != nil {
		t.Fatalf("GET /api/investments/{id}/transactions: %v", err)
	}
	var got []map[string]any
	decodeJSON(t, resp, &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(got) != 1 || got[0]["movement_type"] != "BUY" || got[0]["amount"] != "1000.00" {
		t.Errorf("transactions = %+v", got)
	}
}

// Regression test: Pluggy's investments.amount field for renda fixa holdings
// turned out to be quantity × value (current gross mark value), not the
// invested principal, so balance - amount produced a false negative yield on
// healthy CDBs. The yield must come from the actual buy/sell history instead.
func TestListInvestmentsComputesYieldFromTransactionHistory(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn) // balance = 1000.50, amount_profit unset
	insertInvestmentTransaction(t, conn, investmentID, "BUY", "1200.00")
	insertInvestmentTransaction(t, conn, investmentID, "SELL", "300.00")
	// net contributed = 1200.00 - 300.00 = 900.00; yield = balance - net = 100.50

	resp, err := testGet(t, srv.URL+"/api/investments")
	if err != nil {
		t.Fatalf("GET /api/investments: %v", err)
	}
	var investments []map[string]any
	decodeJSON(t, resp, &investments)
	if resp.StatusCode != http.StatusOK || len(investments) != 1 {
		t.Fatalf("status = %d, investments = %+v", resp.StatusCode, investments)
	}
	got := investments[0]
	if got["yield_value"] != "100.50" || got["yield_source"] != "calculado" {
		t.Errorf("yield_value = %v, yield_source = %v, want 100.50/calculado", got["yield_value"], got["yield_source"])
	}
}

func TestListInvestmentsPrefersProviderAmountProfitOverHistory(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	insertInvestmentTransaction(t, conn, investmentID, "BUY", "1200.00")
	if _, err := conn.Exec(`UPDATE financial_investments SET amount_profit = '42.00' WHERE id = ?`, investmentID); err != nil {
		t.Fatalf("set amount_profit: %v", err)
	}

	resp, err := testGet(t, srv.URL+"/api/investments")
	if err != nil {
		t.Fatalf("GET /api/investments: %v", err)
	}
	var investments []map[string]any
	decodeJSON(t, resp, &investments)
	got := investments[0]
	if got["yield_value"] != "42.00" || got["yield_source"] != "informado" {
		t.Errorf("yield_value = %v, yield_source = %v, want 42.00/informado", got["yield_value"], got["yield_source"])
	}
}

func TestListInvestmentsOmitsYieldWithoutProfitOrHistory(t *testing.T) {
	srv, conn := newTestServer(t)
	insertInvestment(t, conn)

	resp, err := testGet(t, srv.URL+"/api/investments")
	if err != nil {
		t.Fatalf("GET /api/investments: %v", err)
	}
	var investments []map[string]any
	decodeJSON(t, resp, &investments)
	got := investments[0]
	if got["yield_value"] != nil || got["yield_source"] != nil {
		t.Errorf("yield_value = %v, yield_source = %v, want both nil", got["yield_value"], got["yield_source"])
	}
}

// Regression test: an investment whose only captured history is a
// SELL/REDEMPTION (the original BUY was never synced — closed position from
// before the sync window, or the provider's history is otherwise partial)
// must not report the entire redemption as yield. Reported in production as
// a CDB with balance=0 and a single SELL of 11370.20 showing "Rendimento:
// R$ 11.370,20" instead of "Não disponível".
func TestListInvestmentsOmitsCalculatedYieldWhenHistoryHasNoInflow(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	if _, err := conn.Exec(`UPDATE financial_investments SET balance = '0' WHERE id = ?`, investmentID); err != nil {
		t.Fatalf("set balance: %v", err)
	}
	insertInvestmentTransaction(t, conn, investmentID, "SELL", "11370.20")

	resp, err := testGet(t, srv.URL+"/api/investments")
	if err != nil {
		t.Fatalf("GET /api/investments: %v", err)
	}
	var investments []map[string]any
	decodeJSON(t, resp, &investments)
	if resp.StatusCode != http.StatusOK || len(investments) != 1 {
		t.Fatalf("status = %d, investments = %+v", resp.StatusCode, investments)
	}
	got := investments[0]
	if got["yield_value"] != nil || got["yield_source"] != nil {
		t.Errorf("yield_value = %v, yield_source = %v, want both nil", got["yield_value"], got["yield_source"])
	}
}

// Regression test: a dividend/JCP payout (Pluggy "INTEREST", movementType
// DEBIT) leaves the investment, so it is a return, not a further
// contribution. Counting it as an aporte inflated net contributed and made
// profitable positions report a loss — HGLG11 in production showed
// "Rendimento: -R$ 26,12" after four dividend credits.
func TestListInvestmentsTreatsDividendPayoutAsReturnNotContribution(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	if _, err := conn.Exec(`UPDATE financial_investments SET balance = '0' WHERE id = ?`, investmentID); err != nil {
		t.Fatalf("set balance: %v", err)
	}
	// Bought for 800, sold for 790, plus 20 of dividends: a 10.00 gain.
	insertInvestmentTransaction(t, conn, investmentID, "BUY", "800.00")
	insertInvestmentTransaction(t, conn, investmentID, "SELL", "790.00")
	insertInvestmentTransaction(t, conn, investmentID, "INTEREST", "20.00")

	got := firstInvestment(t, srv.URL)
	if got["yield_value"] != "10.00" || got["yield_source"] != "calculado" {
		t.Errorf("yield_value = %v, yield_source = %v, want 10.00/calculado", got["yield_value"], got["yield_source"])
	}
}

// Regression test: renda variável cannot sell more shares than it bought, so
// an excess means purchases are missing from the window the provider served,
// even though some were captured. BBAS3 in production had 120 cotas sold
// against 80 bought and reported the uncovered proceeds as R$ 1.286,84 of
// yield; the no-inflow guard alone did not catch it.
func TestListInvestmentsOmitsCalculatedYieldWhenEquitySoldMoreThanBought(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	if _, err := conn.Exec(
		`UPDATE financial_investments SET balance = '0', investment_type = 'EQUITY' WHERE id = ?`, investmentID,
	); err != nil {
		t.Fatalf("set balance: %v", err)
	}
	insertInvestmentMovement(t, conn, investmentID, "BUY", "inflow", "1600.00", strPtr("80"))
	insertInvestmentMovement(t, conn, investmentID, "SELL", "outflow", "2900.00", strPtr("120"))

	got := firstInvestment(t, srv.URL)
	if got["yield_value"] != nil || got["yield_source"] != nil {
		t.Errorf("yield_value = %v, yield_source = %v, want both nil", got["yield_value"], got["yield_source"])
	}
	if got["yield_unavailable_reason"] != "historico_incompleto" {
		t.Errorf("yield_unavailable_reason = %v, want historico_incompleto", got["yield_unavailable_reason"])
	}
}

// A renda fixa "quantity" accrues with interest, so redeeming slightly more
// units than were bought is what a healthy CDB does — the quantity guard
// above must not fire on it.
func TestListInvestmentsKeepsCalculatedYieldWhenFixedIncomeUnitsAccrue(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	if _, err := conn.Exec(
		`UPDATE financial_investments SET balance = '0', investment_type = 'FIXED_INCOME' WHERE id = ?`, investmentID,
	); err != nil {
		t.Fatalf("set balance: %v", err)
	}
	insertInvestmentMovement(t, conn, investmentID, "BUY", "inflow", "1000.00", strPtr("50000"))
	insertInvestmentMovement(t, conn, investmentID, "SELL", "outflow", "1009.74", strPtr("50000.098"))

	got := firstInvestment(t, srv.URL)
	if got["yield_value"] != "9.74" || got["yield_source"] != "calculado" {
		t.Errorf("yield_value = %v, yield_source = %v, want 9.74/calculado", got["yield_value"], got["yield_source"])
	}
}

// A movement whose direction the sync could not establish (an unrecognized
// provider movementType) makes the whole netting unreliable: it is unknown
// which side of the sum it belongs on. Refusing the yield beats guessing.
func TestListInvestmentsOmitsCalculatedYieldWhenAMovementHasNoDirection(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	insertInvestmentTransaction(t, conn, investmentID, "BUY", "1200.00")
	insertInvestmentMovement(t, conn, investmentID, "SOMETHING_NEW", "", "300.00", nil)

	got := firstInvestment(t, srv.URL)
	if got["yield_value"] != nil || got["yield_unavailable_reason"] != "historico_incompleto" {
		t.Errorf("yield_value = %v, reason = %v, want nil/historico_incompleto",
			got["yield_value"], got["yield_unavailable_reason"])
	}
}

// A holding with no movements at all is a different story from one whose
// history arrived partial, and the UI says so — "Sem histórico sincronizado"
// against "Histórico incompleto".
func TestListInvestmentsReportsNoHistorySeparatelyFromPartialHistory(t *testing.T) {
	srv, conn := newTestServer(t)
	insertInvestment(t, conn)

	got := firstInvestment(t, srv.URL)
	if got["yield_unavailable_reason"] != "sem_historico" {
		t.Errorf("yield_unavailable_reason = %v, want sem_historico", got["yield_unavailable_reason"])
	}
}

// Regression test: netContributed stopped filtering on amount IS NOT NULL
// when it started reading the direction column, which let a movement with no
// amount count as evidence of a complete history while contributing nothing
// to the sum. A single amountless aplicação then reported the entire balance
// as profit.
func TestListInvestmentsOmitsCalculatedYieldWhenAMovementHasNoAmount(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	if _, err := conn.Exec(`UPDATE financial_investments SET balance = '5000' WHERE id = ?`, investmentID); err != nil {
		t.Fatalf("set balance: %v", err)
	}
	insertInvestmentTransaction(t, conn, investmentID, "BUY", "1000.00")
	if _, err := conn.Exec(
		`UPDATE financial_investment_transactions SET amount = NULL WHERE investment_id = ?`, investmentID,
	); err != nil {
		t.Fatalf("null the amount: %v", err)
	}

	got := firstInvestment(t, srv.URL)
	if got["yield_value"] != nil || got["yield_unavailable_reason"] != "historico_incompleto" {
		t.Errorf("yield_value = %v, reason = %v, want nil/historico_incompleto",
			got["yield_value"], got["yield_unavailable_reason"])
	}
}

// A complete history with nothing to net it against is its own answer: the
// UI would otherwise show a bare "Não disponível" next to a full movement
// list, which reads as a bug rather than as missing provider data.
func TestListInvestmentsReportsAMissingBalanceAsItsOwnReason(t *testing.T) {
	srv, conn := newTestServer(t)
	investmentID := insertInvestment(t, conn)
	if _, err := conn.Exec(`UPDATE financial_investments SET balance = NULL WHERE id = ?`, investmentID); err != nil {
		t.Fatalf("clear balance: %v", err)
	}
	insertInvestmentTransaction(t, conn, investmentID, "BUY", "1000.00")

	got := firstInvestment(t, srv.URL)
	if got["yield_unavailable_reason"] != "saldo_indisponivel" {
		t.Errorf("yield_unavailable_reason = %v, want saldo_indisponivel", got["yield_unavailable_reason"])
	}
}

// firstInvestment GETs the list endpoint and returns the only holding in it.
func firstInvestment(t *testing.T, baseURL string) map[string]any {
	t.Helper()
	resp, err := testGet(t, baseURL+"/api/investments")
	if err != nil {
		t.Fatalf("GET /api/investments: %v", err)
	}
	var investments []map[string]any
	decodeJSON(t, resp, &investments)
	if resp.StatusCode != http.StatusOK || len(investments) != 1 {
		t.Fatalf("status = %d, investments = %+v", resp.StatusCode, investments)
	}
	return investments[0]
}

func TestListInvestmentTransactionsReturns404WhenInvestmentMissing(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := testGet(t, srv.URL+"/api/investments/"+uuid.NewString()+"/transactions")
	if err != nil {
		t.Fatalf("GET /api/investments/{id}/transactions: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestInvestmentCalendarDates(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertInvestment(t, conn)
	if _, err := conn.Exec(`UPDATE financial_investments SET due_date = ? WHERE id = ?`, "2026-12-15", id); err != nil {
		t.Fatal(err)
	}
	insertInvestmentTransaction(t, conn, id, "BUY", "100")
	if _, err := conn.Exec(`UPDATE financial_investment_transactions SET trade_date = ? WHERE investment_id = ?`, "2026-04-03", id); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/investments", "/api/investments/" + id} {
		resp, err := testGet(t, srv.URL+path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", path, resp.StatusCode)
		}
		var got map[string]any
		if path == "/api/investments" {
			var rows []map[string]any
			decodeJSON(t, resp, &rows)
			if len(rows) != 1 {
				t.Fatalf("rows = %+v", rows)
			}
			got = rows[0]
		} else {
			decodeJSON(t, resp, &got)
		}
		if got["due_date"] != "2026-12-15T00:00:00Z" {
			t.Fatalf("due date = %+v", got)
		}
	}
	resp, err := testGet(t, srv.URL+"/api/investments/"+id+"/transactions")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("transactions status = %d", resp.StatusCode)
	}
	var rows []map[string]any
	decodeJSON(t, resp, &rows)
	if len(rows) != 1 || rows[0]["trade_date"] != "2026-04-03T00:00:00Z" {
		t.Fatalf("trade date = %+v", rows)
	}
}

func TestInvestmentTransactionsWithCorruptTimestampReturn503(t *testing.T) {
	srv, conn := newTestServer(t)
	id := insertInvestment(t, conn)
	insertInvestmentTransaction(t, conn, id, "BUY", "100")
	if _, err := conn.Exec(`UPDATE financial_investment_transactions SET occurred_at = 'not-a-time' WHERE investment_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	resp, err := testGet(t, srv.URL+"/api/investments/"+id+"/transactions")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}
