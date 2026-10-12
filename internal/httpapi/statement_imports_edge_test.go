package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/statementimport"
)

// Synthetic Flash rows (invented data). Every helper below builds on them.
const (
	flashHeaderLine = "\ufeffData,Hora,Movimentação,Valor,Meio de Pagamento,Saldo\n"
	rowCafe         = `09/09/2026,12:00,Café,"-R$ 10,00",Cartão,"R$ 90,00"`
	rowDeposit      = `08/09/2026,12:00,Depósito,"R$ 100,00",Depósito,"R$ 100,00"`
	rowSeed         = `01/09/2026,09:00,Saldo inicial,"R$ 50,00",Depósito,"R$ 50,00"`

	// 2026-09-09 12:00 in America/Sao_Paulo, as stored by the importer.
	cafeOccurredAt = "2026-09-09T15:00:00.000000000Z"

	warnPluggyMatch = "possível correspondência com conexão automática"
	warnManualMatch = "possível correspondência com lançamento manual"
)

func flashFile(rows ...string) string {
	return flashHeaderLine + strings.Join(rows, "\n") + "\n"
}

func newStatementTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

type statementPreviewBody struct {
	Format             string       `json:"format"`
	FormatVersion      string       `json:"format_version"`
	SHA256             string       `json:"sha256"`
	PreviewFingerprint string       `json:"preview_fingerprint"`
	Counts             importCounts `json:"counts"`
	Rows               []struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings"`
	} `json:"rows"`
}

// hasWarning reports whether any row of the preview carries the warning.
func (p statementPreviewBody) hasWarning(warning string) bool {
	for _, row := range p.Rows {
		for _, w := range row.Warnings {
			if w == warning {
				return true
			}
		}
	}
	return false
}

type problemBody struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func decodeProblem(t *testing.T, rr *httptest.ResponseRecorder) problemBody {
	t.Helper()
	var p problemBody
	if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
		t.Fatalf("problem body %q: %v", rr.Body.String(), err)
	}
	return p
}

func requireProblem(t *testing.T, rr *httptest.ResponseRecorder, status int, kind string) problemBody {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status %d, want %d: %s", rr.Code, status, rr.Body.String())
	}
	p := decodeProblem(t, rr)
	if p.Type != "/problems/"+kind || p.Status != status {
		t.Fatalf("problem %+v, want type %s status %d", p, kind, status)
	}
	return p
}

// previewStatementFile runs the preview handler and requires a 200. fields may
// carry account_id.
func previewStatementFile(t *testing.T, conn *sql.DB, file string, fields map[string]string) statementPreviewBody {
	t.Helper()
	rr := uploadStatement(t, handleStatementPreview(conn), file, fields)
	if rr.Code != 200 {
		t.Fatalf("preview %d: %s", rr.Code, rr.Body.String())
	}
	var p statementPreviewBody
	if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// confirmStatementFile posts the confirmation with the expectations taken from
// preview p. fields carries account_id or new_account_name and allow_partial.
func confirmStatementFile(t *testing.T, conn *sql.DB, file string, p statementPreviewBody, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	all := map[string]string{
		"expected_sha256":              p.SHA256,
		"expected_format":              p.Format,
		"expected_format_version":      p.FormatVersion,
		"expected_preview_fingerprint": p.PreviewFingerprint,
	}
	for k, v := range fields {
		all[k] = v
	}
	return uploadStatement(t, handleStatementConfirm(conn), file, all)
}

// importStatementFile previews and confirms file (requiring 201) and returns
// the account id that received it. fields carries account_id or
// new_account_name and, optionally, allow_partial.
func importStatementFile(t *testing.T, conn *sql.DB, file string, fields map[string]string) (string, importCounts) {
	t.Helper()
	p := previewStatementFile(t, conn, file, map[string]string{"account_id": fields["account_id"]})
	rr := confirmStatementFile(t, conn, file, p, fields)
	if rr.Code != 201 {
		t.Fatalf("confirm %d: %s", rr.Code, rr.Body.String())
	}
	var out struct {
		AccountID string       `json:"account_id"`
		Counts    importCounts `json:"counts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.AccountID, out.Counts
}

// newFileAccount imports a single old row into a fresh file account.
func newFileAccount(t *testing.T, conn *sql.DB) string {
	t.Helper()
	id, _ := importStatementFile(t, conn, flashFile(rowSeed), map[string]string{"new_account_name": "Conta de teste"})
	return id
}

func accountBalance(t *testing.T, conn *sql.DB, accountID string) (balance, asOf sql.NullString) {
	t.Helper()
	if err := conn.QueryRow(`SELECT balance, balance_as_of FROM financial_accounts WHERE id=?`, accountID).Scan(&balance, &asOf); err != nil {
		t.Fatal(err)
	}
	return balance, asOf
}

func countWhere(t *testing.T, conn *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// insertManualStatementRow stores a manual transaction with the amount text
// exactly as given, the way money.CanonicalDecimal would have written it.
func insertManualStatementRow(t *testing.T, conn *sql.DB, accountID, description, amount, occurredAt string) {
	t.Helper()
	_, err := conn.Exec(`INSERT INTO financial_transactions (id,account_id,description,amount,amount_in_account_currency,currency_code,occurred_at,origin,created_at,updated_at) VALUES (?,?,?,?,?,'BRL',?,'manual',?,?)`,
		uuid.NewString(), accountID, description, amount, amount, occurredAt, occurredAt, occurredAt)
	if err != nil {
		t.Fatal(err)
	}
}

// insertPluggyStatementRow stores a synced transaction on a Pluggy account,
// with the minimal sync-schema chain around it.
func insertPluggyStatementRow(t *testing.T, conn *sql.DB, description, amount, occurredAt string) {
	t.Helper()
	sourceID, runID, rawID, accountID, txID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`INSERT INTO data_sources (id, provider, external_item_id, display_name, created_at, updated_at)
		VALUES (?, 'pluggy', ?, 'Banco Exemplo', ?, ?)`, sourceID, sourceID, occurredAt, occurredAt)
	exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, runID, sourceID, occurredAt, occurredAt)
	exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'transactions', 1, 1, 'GET', '/x', 200, '{}', ?, 'sha', ?)`,
		rawID, runID, sourceID, []byte{0}, occurredAt)
	exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, currency_code, current_raw_import_id, normalized_hash,
			created_at, updated_at
		) VALUES (?, ?, ?, 'BRL', ?, 'hash', ?, ?)`, accountID, sourceID, accountID, rawID, occurredAt, occurredAt)
	exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'BRL', ?, 'POSTED', 'DEBIT', ?, 'hash', ?, ?)`,
		txID, sourceID, accountID, txID, description, amount, amount, occurredAt, rawID, occurredAt, occurredAt)
}

func TestAmountSpellings(t *testing.T) {
	cases := []struct {
		amount string
		want   []string
	}{
		{"-10.00", []string{"-10", "-10.0", "-10.00"}},
		{"100.00", []string{"100", "100.0", "100.00"}},
		{"-100.00", []string{"-100", "-100.0", "-100.00"}},
		{"-49.90", []string{"-49.9", "-49.90"}},
		{"0.50", []string{"0.5", "0.50"}},
		{"-10.50", []string{"-10.5", "-10.50"}},
		{"1533.33", []string{"1533.33"}},
		{"-0.05", []string{"-0.05"}},
		{"+10.00", []string{"10", "10.0", "10.00"}},
		{"10.00", []string{"10", "10.0", "10.00"}},
		{"1000.00", []string{"1000", "1000.0", "1000.00"}},
		{"0.00", []string{"0", "0.0", "0.00"}},
		{"not a number", []string{"not a number"}},
	}
	for _, c := range cases {
		t.Run(c.amount, func(t *testing.T) {
			got := amountSpellings(c.amount)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("amountSpellings(%q) = %q, want %q", c.amount, got, c.want)
			}
			seen := map[string]bool{}
			for _, s := range got {
				if seen[s] {
					t.Fatalf("duplicate spelling %q in %q", s, got)
				}
				seen[s] = true
			}
		})
	}
	// Exact spellings only: an amount's forms must never equal another amount.
	ten, hundred := amountSpellings("-10.00"), amountSpellings("-100.00")
	for _, a := range ten {
		for _, b := range hundred {
			if a == b {
				t.Fatalf("-10 and -100 share the spelling %q", a)
			}
		}
	}
	for _, a := range amountSpellings("10.00") {
		for _, b := range amountSpellings("-10.00") {
			if a == b {
				t.Fatalf("10 and -10 share the spelling %q", a)
			}
		}
	}
}

func TestStatementSimilarWarningsMatchStoredAmountScale(t *testing.T) {
	const (
		lanche  = `09/09/2026,12:00,Lanche,"-R$ 49,90",Cartão,"R$ 40,10"`
		gorjeta = `09/09/2026,12:00,Gorjeta,"-R$ 10,50",Cartão,"R$ 39,60"`
		milhar  = `09/09/2026,12:00,Aluguel,"-R$ 1.533,33",Cartão,"R$ 20,00"`
	)
	cases := []struct {
		name        string
		pluggy      bool // false: manual entry on the imported account
		description string
		stored      string
		row         string
		want        string // "" means no warning at all
	}{
		{"pluggy whole number", true, "Café", "-10", rowCafe, warnPluggyMatch},
		{"pluggy one decimal", true, "Lanche", "-49.9", lanche, warnPluggyMatch},
		{"pluggy two decimals", true, "Café", "-10.00", rowCafe, warnPluggyMatch},
		{"pluggy one decimal on a whole amount", true, "Café", "-10.0", rowCafe, warnPluggyMatch},
		{"pluggy amount without trimming", true, "Aluguel", "-1533.33", milhar, warnPluggyMatch},
		{"pluggy tenfold amount", true, "Café", "-100", rowCafe, ""},
		{"pluggy opposite sign", true, "Café", "10", rowCafe, ""},
		{"pluggy other description", true, "Padaria", "-10", rowCafe, ""},
		{"manual whole number", false, "Café", "-10", rowCafe, warnManualMatch},
		{"manual one decimal", false, "Gorjeta", "-10.5", gorjeta, warnManualMatch},
		{"manual two decimals", false, "Café", "-10.00", rowCafe, warnManualMatch},
		{"manual tenfold amount", false, "Café", "-100", rowCafe, ""},
		{"manual other amount", false, "Café", "-10.5", rowCafe, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := newStatementTestDB(t)
			accountID := newFileAccount(t, conn)
			if c.pluggy {
				insertPluggyStatementRow(t, conn, c.description, c.stored, cafeOccurredAt)
			} else {
				insertManualStatementRow(t, conn, accountID, c.description, c.stored, cafeOccurredAt)
			}
			p := previewStatementFile(t, conn, flashFile(c.row), map[string]string{"account_id": accountID})
			if p.Counts.New != 1 {
				t.Fatalf("counts %+v", p.Counts)
			}
			for _, w := range []string{warnPluggyMatch, warnManualMatch} {
				if got, want := p.hasWarning(w), w == c.want; got != want {
					t.Fatalf("warning %q present=%v, want %v (rows %+v)", w, got, want, p.Rows)
				}
			}
		})
	}
}

func TestStatementPluggyWarningWithoutAccount(t *testing.T) {
	conn := newStatementTestDB(t)
	insertPluggyStatementRow(t, conn, "Café", "-10", cafeOccurredAt)
	p := previewStatementFile(t, conn, flashFile(rowCafe), nil)
	if !p.hasWarning(warnPluggyMatch) || p.hasWarning(warnManualMatch) {
		t.Fatalf("rows %+v", p.Rows)
	}
}

func TestStatementParseErrorsAre422WithMessage(t *testing.T) {
	const notAStatement = "isto não é um extrato\n"
	_, parseErr := statementimport.Parse([]byte(notAStatement), "")
	if parseErr == nil {
		t.Fatal("expected a parse error")
	}
	conn := newStatementTestDB(t)

	rr := uploadStatement(t, handleStatementPreview(conn), notAStatement, nil)
	if p := requireProblem(t, rr, 422, "invalid-statement"); p.Detail != parseErr.Error() {
		t.Fatalf("preview detail %q, want %q", p.Detail, parseErr.Error())
	}

	fields := map[string]string{
		"new_account_name": "Conta", "expected_sha256": "x", "expected_format": "x",
		"expected_format_version": "x", "expected_preview_fingerprint": "x",
	}
	rr = uploadStatement(t, handleStatementConfirm(conn), notAStatement, fields)
	if p := requireProblem(t, rr, 422, "invalid-statement"); p.Detail != parseErr.Error() {
		t.Fatalf("confirm detail %q, want %q", p.Detail, parseErr.Error())
	}
	if n := countWhere(t, conn, `SELECT COUNT(*) FROM statement_imports`); n != 0 {
		t.Fatalf("%d imports recorded", n)
	}
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &buf
}

func TestStatementDatabaseFailuresAre503WithoutDriverText(t *testing.T) {
	confirmFields := func(p statementPreviewBody, extra map[string]string) map[string]string {
		fields := map[string]string{
			"expected_sha256": p.SHA256, "expected_format": p.Format,
			"expected_format_version": p.FormatVersion, "expected_preview_fingerprint": p.PreviewFingerprint,
		}
		for k, v := range extra {
			fields[k] = v
		}
		return fields
	}
	// breakQueries makes every classification query fail while transactions
	// still begin, so the error surfaces from inside the confirmation.
	breakQueries := func(t *testing.T, conn *sql.DB) {
		t.Helper()
		for _, stmt := range []string{`PRAGMA foreign_keys=OFF`, `DROP TABLE financial_transactions`} {
			if _, err := conn.Exec(stmt); err != nil {
				t.Fatalf("break the schema: %v", err)
			}
		}
	}
	cases := []struct {
		name  string
		event string
		kind  string
		run   func(t *testing.T) *httptest.ResponseRecorder
	}{
		{"preview, database closed", "statement_preview_failed", "preview-unavailable", func(t *testing.T) *httptest.ResponseRecorder {
			conn := newStatementTestDB(t)
			conn.Close()
			return uploadStatement(t, handleStatementPreview(conn), validFlash, nil)
		}},
		{"preview with account, database closed", "statement_preview_failed", "preview-unavailable", func(t *testing.T) *httptest.ResponseRecorder {
			conn := newStatementTestDB(t)
			conn.Close()
			return uploadStatement(t, handleStatementPreview(conn), validFlash, map[string]string{"account_id": uuid.NewString()})
		}},
		{"preview, query fails", "statement_preview_failed", "preview-unavailable", func(t *testing.T) *httptest.ResponseRecorder {
			conn := newStatementTestDB(t)
			breakQueries(t, conn)
			return uploadStatement(t, handleStatementPreview(conn), validFlash, nil)
		}},
		{"confirm, database closed", "statement_import_failed", "import-unavailable", func(t *testing.T) *httptest.ResponseRecorder {
			conn := newStatementTestDB(t)
			p := previewStatementFile(t, conn, validFlash, nil)
			conn.Close()
			return uploadStatement(t, handleStatementConfirm(conn), validFlash, confirmFields(p, map[string]string{"new_account_name": "Conta"}))
		}},
		{"confirm, query fails", "statement_import_failed", "import-unavailable", func(t *testing.T) *httptest.ResponseRecorder {
			conn := newStatementTestDB(t)
			p := previewStatementFile(t, conn, validFlash, nil)
			breakQueries(t, conn)
			return uploadStatement(t, handleStatementConfirm(conn), validFlash, confirmFields(p, map[string]string{"new_account_name": "Conta"}))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := captureLog(t)
			rr := c.run(t)
			problem := requireProblem(t, rr, 503, c.kind)
			if problem.Detail != "Tente novamente em instantes." {
				t.Fatalf("detail %q", problem.Detail)
			}
			body := strings.ToLower(rr.Body.String())
			for _, leak := range []string{"sql", "closed", "no such table", "database", "sqlite", "driver"} {
				if strings.Contains(body, leak) {
					t.Fatalf("response leaks %q: %s", leak, rr.Body.String())
				}
			}
			logged := logs.String()
			if !strings.Contains(logged, c.event) {
				t.Fatalf("log %q lacks event %s", logged, c.event)
			}
			// The log carries the technical error only, never statement content.
			for _, content := range []string{"Café", "Depósito", "10,00", "90,00", "-10.00"} {
				if strings.Contains(logged, content) {
					t.Fatalf("log leaks statement content %q: %s", content, logged)
				}
			}
		})
	}
}

func TestStatementUnknownAccountIs422NotUnavailable(t *testing.T) {
	conn := newStatementTestDB(t)
	rr := uploadStatement(t, handleStatementPreview(conn), validFlash, map[string]string{"account_id": uuid.NewString()})
	requireProblem(t, rr, 422, "invalid-account")
	p := previewStatementFile(t, conn, validFlash, nil)
	rr = uploadStatement(t, handleStatementConfirm(conn), validFlash, map[string]string{
		"account_id": uuid.NewString(), "expected_sha256": p.SHA256, "expected_format": p.Format,
		"expected_format_version": p.FormatVersion, "expected_preview_fingerprint": p.PreviewFingerprint,
	})
	requireProblem(t, rr, 422, "invalid-account")
}

// rawStatementRequest posts a hand-made body, for uploads that uploadStatement
// (always well-formed multipart) cannot express.
func rawStatementRequest(handler http.HandlerFunc, contentType, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/statement-imports", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	handler(rr, r)
	return rr
}

func TestStatementUploadErrors(t *testing.T) {
	conn := newStatementTestDB(t)
	handlers := map[string]http.HandlerFunc{
		"preview": handleStatementPreview(conn),
		"confirm": handleStatementConfirm(conn),
	}
	for name, handler := range handlers {
		t.Run(name+"/body over the request cap", func(t *testing.T) {
			huge := strings.Repeat("a", statementimport.MaxBytes+1<<16+1024)
			requireProblem(t, uploadStatement(t, handler, huge, nil), 413, "upload-too-large")
		})
		t.Run(name+"/file over the size limit", func(t *testing.T) {
			big := strings.Repeat("a", statementimport.MaxBytes+1)
			requireProblem(t, uploadStatement(t, handler, big, nil), 413, "upload-too-large")
		})
		t.Run(name+"/not multipart", func(t *testing.T) {
			rr := rawStatementRequest(handler, "application/json", `{"file":"x"}`)
			requireProblem(t, rr, 422, "invalid-upload")
		})
		t.Run(name+"/no content type", func(t *testing.T) {
			requireProblem(t, rawStatementRequest(handler, "", "x"), 422, "invalid-upload")
		})
		t.Run(name+"/malformed multipart", func(t *testing.T) {
			rr := rawStatementRequest(handler, "multipart/form-data; boundary=XYZ", "this is not a multipart body")
			requireProblem(t, rr, 422, "invalid-upload")
		})
		t.Run(name+"/truncated multipart", func(t *testing.T) {
			body := "--XYZ\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.csv\"\r\nContent-Type: text/csv\r\n\r\nabc"
			rr := rawStatementRequest(handler, "multipart/form-data; boundary=XYZ", body)
			requireProblem(t, rr, 422, "invalid-upload")
		})
		t.Run(name+"/missing file field", func(t *testing.T) {
			body := "--XYZ\r\nContent-Disposition: form-data; name=\"account_id\"\r\n\r\nabc\r\n--XYZ--\r\n"
			rr := rawStatementRequest(handler, "multipart/form-data; boundary=XYZ", body)
			requireProblem(t, rr, 422, "missing-file")
		})
	}
}

func TestStatementOlderFileDoesNotRegressBalance(t *testing.T) {
	conn := newStatementTestDB(t)
	accountID, counts := importStatementFile(t, conn, validFlash, map[string]string{"new_account_name": "Flash refeições"})
	if counts.New != 2 {
		t.Fatalf("counts %+v", counts)
	}
	balance, asOf := accountBalance(t, conn, accountID)
	if balance.String != "90.00" || asOf.String != cafeOccurredAt {
		t.Fatalf("balance %v as of %v", balance, asOf)
	}

	older := flashFile(
		`03/09/2026,10:00,Padaria,"-R$ 5,00",Cartão,"R$ 40,00"`,
		`02/09/2026,10:00,Depósito,"R$ 45,00",Depósito,"R$ 45,00"`,
	)
	_, counts = importStatementFile(t, conn, older, map[string]string{"account_id": accountID})
	if counts.New != 2 || counts.Duplicate != 0 {
		t.Fatalf("older counts %+v", counts)
	}
	gotBalance, gotAsOf := accountBalance(t, conn, accountID)
	if gotBalance != balance || gotAsOf != asOf {
		t.Fatalf("older file moved the balance to %v as of %v", gotBalance, gotAsOf)
	}
	if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id=?`, accountID); n != 4 {
		t.Fatalf("%d transactions", n)
	}
}

func TestStatementRowOrderDoesNotChangeIdentity(t *testing.T) {
	descending := flashFile(rowCafe, rowDeposit)
	ascending := flashFile(rowDeposit, rowCafe)
	for _, tc := range []struct {
		name         string
		first, again string
	}{
		{"descending then ascending", descending, ascending},
		{"ascending then descending", ascending, descending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := newStatementTestDB(t)
			accountID, counts := importStatementFile(t, conn, tc.first, map[string]string{"new_account_name": "Flash refeições"})
			if counts.New != 2 {
				t.Fatalf("counts %+v", counts)
			}
			balance, _ := accountBalance(t, conn, accountID)
			if balance.String != "90.00" {
				t.Fatalf("balance %v", balance)
			}
			p := previewStatementFile(t, conn, tc.again, map[string]string{"account_id": accountID})
			if p.Counts.Ambiguous != 2 || p.Counts.New != 0 {
				t.Fatalf("counts %+v", p.Counts)
			}
			for _, row := range p.Rows {
				if row.Status != "ambiguous" || !p.hasWarning(warnOtherFileMatch) {
					t.Fatalf("row %+v", row)
				}
			}
			_, counts = importStatementFile(t, conn, tc.again, map[string]string{
				"account_id": accountID, "ambiguous_line_2": "ignore", "ambiguous_line_3": "ignore",
			})
			if counts.New != 0 || counts.Duplicate != 2 {
				t.Fatalf("second import %+v", counts)
			}
		})
	}
}

func TestStatementAllowPartial(t *testing.T) {
	invalidNewest := `10/09/2026,12:00,,"-R$ 5,00",Cartão,"R$ 85,00"`
	file := flashFile(invalidNewest, rowCafe, rowDeposit)
	conn := newStatementTestDB(t)
	p := previewStatementFile(t, conn, file, nil)
	if p.Counts.Invalid != 1 || p.Counts.New != 2 || p.Counts.Total != 3 {
		t.Fatalf("preview counts %+v", p.Counts)
	}

	rr := confirmStatementFile(t, conn, file, p, map[string]string{"new_account_name": "Flash refeições"})
	requireProblem(t, rr, 422, "invalid-rows")
	rr = confirmStatementFile(t, conn, file, p, map[string]string{"new_account_name": "Flash refeições", "allow_partial": "false"})
	requireProblem(t, rr, 422, "invalid-rows")
	if n := countWhere(t, conn, `SELECT COUNT(*) FROM statement_imports`); n != 0 {
		t.Fatalf("%d imports recorded by a rejected confirmation", n)
	}
	if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_accounts`); n != 0 {
		t.Fatalf("%d accounts created by a rejected confirmation", n)
	}

	rr = confirmStatementFile(t, conn, file, p, map[string]string{"new_account_name": "Flash refeições", "allow_partial": "true"})
	if rr.Code != 201 {
		t.Fatalf("confirm %d: %s", rr.Code, rr.Body.String())
	}
	var out struct {
		AccountID string       `json:"account_id"`
		Counts    importCounts `json:"counts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Counts != (importCounts{Total: 3, New: 2, Duplicate: 0, Invalid: 1}) {
		t.Fatalf("counts %+v", out.Counts)
	}
	if n := countWhere(t, conn, `SELECT rows_invalid FROM statement_imports WHERE account_id=?`, out.AccountID); n != 1 {
		t.Fatalf("rows_invalid %d", n)
	}
	if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id=?`, out.AccountID); n != 2 {
		t.Fatalf("%d transactions", n)
	}
}

// With allow_partial the valid rows are always imported; the account balance
// only follows the file when no invalid row could be newer than the newest
// valid one.
func TestStatementPartialImportBalanceIsConservative(t *testing.T) {
	cases := []struct {
		name        string
		invalid     string
		wantBalance string // "" means the balance stays untouched (NULL)
	}{
		{"invalid row newer than the latest valid one", `10/09/2026,12:00,,"-R$ 5,00",Cartão,"R$ 85,00"`, ""},
		{"invalid row at the same instant as the latest valid one", `09/09/2026,12:00,,"-R$ 1,00",Cartão,"R$ 89,00"`, ""},
		{"invalid row with an unreadable date", `xx/09/2026,12:00,Sem data,"-R$ 5,00",Cartão,"R$ 85,00"`, ""},
		{"invalid row older than the latest valid one", `07/09/2026,12:00,,"-R$ 5,00",Cartão,"R$ 95,00"`, "90.00"},
		{"invalid row older than every valid row", `01/01/2026,08:00,,"-R$ 5,00",Cartão,"R$ 95,00"`, "90.00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := newStatementTestDB(t)
			file := flashFile(c.invalid, rowCafe, rowDeposit)
			accountID, counts := importStatementFile(t, conn, file, map[string]string{"new_account_name": "Flash refeições", "allow_partial": "true"})
			if counts.New != 2 || counts.Invalid != 1 {
				t.Fatalf("counts %+v", counts)
			}
			if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id=?`, accountID); n != 2 {
				t.Fatalf("%d transactions imported", n)
			}
			balance, asOf := accountBalance(t, conn, accountID)
			if c.wantBalance == "" {
				if balance.Valid || asOf.Valid {
					t.Fatalf("balance %v as of %v, want untouched", balance, asOf)
				}
				return
			}
			if balance.String != c.wantBalance || asOf.String != cafeOccurredAt {
				t.Fatalf("balance %v as of %v", balance, asOf)
			}
		})
	}
}

func TestLatestStatementBalance(t *testing.T) {
	row := func(status, occurredAt, balance string, warnings ...string) statementimport.Row {
		return statementimport.Row{Status: status, OccurredAt: occurredAt, Balance: balance, Warnings: warnings}
	}
	cases := []struct {
		name        string
		rows        []statementimport.Row
		wantAsOf    string
		wantBalance string
	}{
		{"newest valid row wins in any order", []statementimport.Row{row("new", "2026-09-01T10:00:00.000000000Z", "50.00"), row("new", "2026-09-03T10:00:00.000000000Z", "70.00"), row("new", "2026-09-02T10:00:00.000000000Z", "60.00")},
			"2026-09-03T10:00:00.000000000Z", "70.00"},
		{"several rows at the newest instant", []statementimport.Row{row("new", "2026-09-03T10:00:00.000000000Z", "70.00"), row("new", "2026-09-03T10:00:00.000000000Z", "60.00")}, "", ""},
		{"newest duplicate with a differing balance", []statementimport.Row{row("duplicate", "2026-09-03T10:00:00.000000000Z", "70.00", warnBalanceDiffers), row("new", "2026-09-01T10:00:00.000000000Z", "50.00")}, "", ""},
		{"older duplicate with a differing balance does not matter", []statementimport.Row{row("new", "2026-09-03T10:00:00.000000000Z", "70.00"), row("duplicate", "2026-09-01T10:00:00.000000000Z", "50.00", warnBalanceDiffers)},
			"2026-09-03T10:00:00.000000000Z", "70.00"},
		{"invalid row without a date", []statementimport.Row{row("new", "2026-09-03T10:00:00.000000000Z", "70.00"), row("invalid", "", "")}, "", ""},
		{"invalid row newer", []statementimport.Row{row("new", "2026-09-03T10:00:00.000000000Z", "70.00"), row("invalid", "2026-09-04T10:00:00.000000000Z", "80.00")}, "", ""},
		{"invalid row at the same instant", []statementimport.Row{row("new", "2026-09-03T10:00:00.000000000Z", "70.00"), row("invalid", "2026-09-03T10:00:00.000000000Z", "")}, "", ""},
		{"invalid row older", []statementimport.Row{row("invalid", "2026-09-02T10:00:00.000000000Z", ""), row("new", "2026-09-03T10:00:00.000000000Z", "70.00")},
			"2026-09-03T10:00:00.000000000Z", "70.00"},
		{"only invalid rows", []statementimport.Row{row("invalid", "2026-09-02T10:00:00.000000000Z", "")}, "", ""},
		{"no rows", nil, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asOf, balance := latestStatementBalance(c.rows)
			if asOf != c.wantAsOf || balance != c.wantBalance {
				t.Fatalf("got (%q, %q), want (%q, %q)", asOf, balance, c.wantAsOf, c.wantBalance)
			}
		})
	}
}

func TestStatementConfirmRejectsChangedPreview(t *testing.T) {
	t.Run("file or format differs from the preview", func(t *testing.T) {
		conn := newStatementTestDB(t)
		p := previewStatementFile(t, conn, validFlash, nil)
		for name, mutate := range map[string]func(*statementPreviewBody){
			"sha256":         func(p *statementPreviewBody) { p.SHA256 = strings.Repeat("0", 64) },
			"format":         func(p *statementPreviewBody) { p.Format = "other" },
			"format version": func(p *statementPreviewBody) { p.FormatVersion = "999" },
		} {
			changed := p
			mutate(&changed)
			rr := confirmStatementFile(t, conn, validFlash, changed, map[string]string{"new_account_name": "Conta"})
			if got := requireProblem(t, rr, 409, "preview-changed"); got.Title != "Arquivo alterado" {
				t.Fatalf("%s: title %q", name, got.Title)
			}
		}
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM statement_imports`); n != 0 {
			t.Fatalf("%d imports recorded", n)
		}
	})

	t.Run("another file was uploaded instead", func(t *testing.T) {
		conn := newStatementTestDB(t)
		p := previewStatementFile(t, conn, validFlash, nil)
		other := flashFile(rowCafe)
		rr := confirmStatementFile(t, conn, other, p, map[string]string{"new_account_name": "Conta"})
		requireProblem(t, rr, 409, "preview-changed")
	})

	t.Run("a manual entry appeared after the preview", func(t *testing.T) {
		conn := newStatementTestDB(t)
		accountID := newFileAccount(t, conn)
		fields := map[string]string{"account_id": accountID}
		p := previewStatementFile(t, conn, validFlash, fields)
		if p.hasWarning(warnManualMatch) {
			t.Fatalf("unexpected warning before the manual entry: %+v", p.Rows)
		}
		insertManualStatementRow(t, conn, accountID, "Café", "-10", cafeOccurredAt)
		rr := confirmStatementFile(t, conn, validFlash, p, fields)
		if got := requireProblem(t, rr, 409, "preview-changed"); got.Title != "Prévia alterada" {
			t.Fatalf("title %q", got.Title)
		}
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE account_id=?`, accountID); n != 2 {
			t.Fatalf("%d transactions, want the seed row and the manual one", n)
		}
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM statement_imports`); n != 1 {
			t.Fatalf("%d imports recorded", n)
		}
		// A fresh preview sees the change and confirms.
		fresh := previewStatementFile(t, conn, validFlash, fields)
		if !fresh.hasWarning(warnManualMatch) {
			t.Fatalf("fresh preview lacks the manual warning: %+v", fresh.Rows)
		}
		if rr = confirmStatementFile(t, conn, validFlash, fresh, fields); rr.Code != 201 {
			t.Fatalf("confirm %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("rows became duplicates after the preview", func(t *testing.T) {
		conn := newStatementTestDB(t)
		accountID := newFileAccount(t, conn)
		fields := map[string]string{"account_id": accountID}
		stale := previewStatementFile(t, conn, validFlash, fields)
		importStatementFile(t, conn, validFlash, fields)
		rr := confirmStatementFile(t, conn, validFlash, stale, fields)
		requireProblem(t, rr, 409, "preview-changed")
		if n := countWhere(t, conn, `SELECT COUNT(*) FROM statement_imports`); n != 2 {
			t.Fatalf("%d imports recorded", n)
		}
	})
}

// The file name never decides the period: a file called after another month
// still reads its dates from the rows (uploadStatement always sends
// flash_06-2026.csv, whose rows here are from September).
func TestStatementPeriodIgnoresFilename(t *testing.T) {
	conn := newStatementTestDB(t)
	rr := uploadStatement(t, handleStatementPreview(conn), validFlash, nil)
	if rr.Code != 200 {
		t.Fatalf("preview %d: %s", rr.Code, rr.Body.String())
	}
	var preview map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview["filename"] != "flash_06-2026.csv" || preview["period_start"] != "2026-09-08" || preview["period_end"] != "2026-09-09" {
		t.Fatalf("preview %v", preview)
	}
}
