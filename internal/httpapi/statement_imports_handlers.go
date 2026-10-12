package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/statementimport"
)

type importCounts struct {
	Total     int `json:"total"`
	New       int `json:"new"`
	Duplicate int `json:"duplicate"`
	Ambiguous int `json:"ambiguous"`
	Invalid   int `json:"invalid"`
}
type importPreview struct {
	statementimport.Parsed
	Filename           string       `json:"filename"`
	Counts             importCounts `json:"counts"`
	PreviewFingerprint string       `json:"preview_fingerprint"`
}
type importAccount struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CurrencyCode string `json:"currency_code"`
}
type importHistory struct {
	RunID       string       `json:"run_id"`
	AccountID   string       `json:"account_id"`
	AccountName string       `json:"account_name"`
	Filename    string       `json:"filename"`
	Format      string       `json:"format"`
	CreatedAt   string       `json:"created_at"`
	Counts      importCounts `json:"counts"`
}

func readStatementUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, statementimport.MaxBytes+1<<16)
	if err := r.ParseMultipartForm(statementimport.MaxBytes + 1<<16); err != nil {
		// Only the size cap is "too large"; a body that is not multipart, or is
		// malformed or cut short, is a bad request whatever its size.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(w, 413, "upload-too-large", "Arquivo muito grande", "O limite é 2 MB.")
		} else {
			writeProblem(w, 422, "invalid-upload", "Envio inválido", "Não foi possível ler o arquivo enviado.")
		}
		return nil, "", false
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		writeProblem(w, 422, "missing-file", "Arquivo ausente", "Selecione um CSV.")
		return nil, "", false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, statementimport.MaxBytes+1))
	if err != nil {
		writeProblem(w, 422, "invalid-upload", "Envio inválido", "Não foi possível ler o arquivo enviado.")
		return nil, "", false
	}
	if len(data) > statementimport.MaxBytes {
		writeProblem(w, 413, "upload-too-large", "Arquivo muito grande", "O limite é 2 MB.")
		return nil, "", false
	}
	return data, h.Filename, true
}

// statementParseError marks a failure of statementimport.Parse. Its message is
// a safe pt-BR sentence meant for the user (422); every other error on the
// preview/confirm path comes from the database and must not reach the client.
type statementParseError struct{ err error }

func (e *statementParseError) Error() string { return e.err.Error() }
func (e *statementParseError) Unwrap() error { return e.err }

// statementParseProblem answers a Parse failure as 422 and reports whether err
// was one; it writes nothing for any other error.
func statementParseProblem(w http.ResponseWriter, err error) bool {
	var parseErr *statementParseError
	if !errors.As(err, &parseErr) {
		return false
	}
	writeProblem(w, 422, "invalid-statement", "Extrato inválido", parseErr.Error())
	return true
}

// previewFailure answers an infrastructure failure during a preview. The
// underlying error is only logged: driver text is not for the UI, and nothing
// from the statement rows goes into the log line.
func previewFailure(w http.ResponseWriter, err error) {
	log.Printf("statement_preview_failed: %v", err)
	writeProblem(w, 503, "preview-unavailable", "Prévia indisponível", "Tente novamente em instantes.")
}

func loadFileAccount(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (string, error) {
	var source string
	err := q.QueryRowContext(ctx, `SELECT fa.source_id FROM financial_accounts fa JOIN data_sources ds ON ds.id=fa.source_id WHERE fa.id=? AND ds.provider='file' AND fa.currency_code='BRL'`, id).Scan(&source)
	return source, err
}
func digest(parts ...string) string {
	s := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(s[:])
}

// warnBalanceDiffers flags a same-looking movement with a different balance.
// The balance separates distinct movements; the preview still calls attention
// to a possible correction of an earlier file.
const warnBalanceDiffers = "saldo difere da importação anterior; revisão necessária"
const warnOtherFileMatch = "movimentação idêntica em outro arquivo; confira se é uma nova compra"

func sameStatementBalance(stored, incoming string) bool {
	a, aErr := decimal.NewFromString(stored)
	b, bErr := decimal.NewFromString(incoming)
	return aErr == nil && bErr == nil && a.Equal(b)
}

// amountSpellings lists the text forms one statement amount can have in the
// TEXT amount columns. The file side is always fixed at two decimals
// ("-49.90"), but Pluggy and manual rows are written through
// money.CanonicalDecimal, which keeps the scale of the source ("-49.9",
// "-10"), so comparing against a single spelling misses them. The result runs
// from the shortest form (trailing zeros trimmed, an integer when the value is
// whole) up to two decimals, without duplicates: "-10.00" gives "-10", "-10.0"
// and "-10.00", and "1533.33" only itself. Each spelling is matched exactly,
// so "-10" never matches "-100". An unparseable input is returned as is.
func amountSpellings(amount string) []string {
	d, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil {
		return []string{amount}
	}
	shortest := d.String()
	places := int32(0)
	if dot := strings.IndexByte(shortest, '.'); dot >= 0 {
		places = int32(len(shortest) - dot - 1)
	}
	out := []string{}
	for p := places; p <= max(places, 2); p++ {
		out = append(out, d.StringFixed(p))
	}
	return out
}
func classifyStatement(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, p *importPreview, accountID string) error {
	seen := map[string]int{}
	legacySeen := map[string]int{}
	p.Counts = importCounts{Total: len(p.Rows)}
	for i := range p.Rows {
		row := &p.Rows[i]
		if row.Status == "invalid" {
			p.Counts.Invalid++
			continue
		}
		normalized := strings.ToLower(strings.Join(strings.Fields(row.Description), " "))
		base := digest(row.OccurredAt, normalized, row.Amount, strings.ToLower(row.PaymentMethod))
		legacySeen[base]++
		// A Flash timestamp has only minute precision. Two genuine purchases
		// can therefore have identical text, amount and payment method. Their
		// resulting balances distinguish them, while an exact replay retains
		// the same identity. Count equal balances separately so the identity
		// does not depend on which statement was imported first.
		key := digest(base, row.Balance)
		seen[key]++
		row.Identity = digest("statement-v2", accountID, key, fmt.Sprint(seen[key]))
		row.ContentHash = digest(row.Identity, row.Balance)
		row.Status = "new"
		if accountID != "" {
			matchedID := row.Identity
			var oldBalance sql.NullString
			forcedID := digest("statement-v3", accountID, p.SHA256, fmt.Sprint(row.LineNumber))
			err := q.QueryRowContext(ctx, `SELECT balance_after FROM financial_transactions WHERE account_id=? AND external_id=?`, accountID, forcedID).Scan(&oldBalance)
			if err == nil {
				matchedID = forcedID
			} else if err == sql.ErrNoRows {
				err = q.QueryRowContext(ctx, `SELECT balance_after FROM financial_transactions WHERE account_id=? AND external_id=?`, accountID, row.Identity).Scan(&oldBalance)
			}
			if err == sql.ErrNoRows {
				// Statements imported before v2 used an identity without balance.
				// Match those only when the balance agrees; otherwise keep the
				// new movement and show the ambiguity in the preview.
				legacyID := digest("statement-v1", accountID, base, fmt.Sprint(legacySeen[base]))
				err = q.QueryRowContext(ctx, `SELECT balance_after FROM financial_transactions WHERE account_id=? AND external_id=?`, accountID, legacyID).Scan(&oldBalance)
				if err == nil && (!oldBalance.Valid || !sameStatementBalance(oldBalance.String, row.Balance)) {
					row.Warnings = append(row.Warnings, warnBalanceDiffers)
					err = sql.ErrNoRows
				} else if err == nil {
					matchedID = legacyID
				}
			}
			if err == nil {
				row.Status = "duplicate"
				var originalFileHash string
				err = q.QueryRowContext(ctx, `SELECT ri.payload_sha256 FROM financial_transactions ft JOIN raw_imports ri ON ri.id=ft.current_raw_import_id WHERE ft.account_id=? AND ft.external_id=?`, accountID, matchedID).Scan(&originalFileHash)
				if err != nil {
					return err
				}
				if originalFileHash != p.SHA256 {
					row.Warnings = append(row.Warnings, warnOtherFileMatch)
					row.Status = "ambiguous"
				}
			} else if err != sql.ErrNoRows {
				return err
			}
		}
		if row.Status == "duplicate" {
			p.Counts.Duplicate++
		} else if row.Status == "ambiguous" {
			p.Counts.Ambiguous++
		} else {
			p.Counts.New++
			if accountID != "" && !slices.Contains(row.Warnings, warnBalanceDiffers) {
				var similarFile int
				// File imports store balance_after at exactly two decimals.
				err := q.QueryRowContext(ctx, `SELECT 1 FROM financial_transactions WHERE account_id=? AND origin='synced' AND occurred_at=? AND amount=? AND LOWER(TRIM(description))=? AND LOWER(COALESCE(source_category,''))=? AND balance_after<>? LIMIT 1`,
					accountID, row.OccurredAt, row.Amount, normalized, strings.ToLower(row.PaymentMethod), row.Balance).Scan(&similarFile)
				if err == nil {
					row.Warnings = append(row.Warnings, warnBalanceDiffers)
				} else if err != sql.ErrNoRows {
					return err
				}
			}
			// The amount is matched under every spelling it may be stored with;
			// occurred_at stays an equality so the lookup remains selective.
			in, amountArgs := db.InClause(amountSpellings(row.Amount))
			var similarPluggy int
			args := append([]any{row.OccurredAt}, amountArgs...)
			err := q.QueryRowContext(ctx, `SELECT 1 FROM financial_transactions ft JOIN data_sources ds ON ds.id=ft.source_id WHERE ds.provider='pluggy' AND ft.deleted_at IS NULL AND ft.occurred_at=? AND ft.amount IN (`+in+`) AND LOWER(TRIM(ft.description))=LOWER(TRIM(?)) LIMIT 1`, append(args, row.Description)...).Scan(&similarPluggy)
			if err == nil {
				row.Warnings = append(row.Warnings, "possível correspondência com conexão automática")
			} else if err != sql.ErrNoRows {
				return err
			}
			if accountID != "" {
				var similar int
				args = append([]any{accountID, row.OccurredAt}, amountArgs...)
				err = q.QueryRowContext(ctx, `SELECT 1 FROM financial_transactions WHERE account_id=? AND origin='manual' AND deleted_at IS NULL AND occurred_at=? AND amount IN (`+in+`) AND LOWER(TRIM(description))=LOWER(TRIM(?)) LIMIT 1`, append(args, row.Description)...).Scan(&similar)
				if err == nil {
					row.Warnings = append(row.Warnings, "possível correspondência com lançamento manual")
				} else if err != sql.ErrNoRows {
					return err
				}
			}
			if slices.Contains(row.Warnings, warnBalanceDiffers) {
				row.Status = "ambiguous"
				p.Counts.New--
				p.Counts.Ambiguous++
			}
		}
	}
	// Sort by timestamp and compare only unambiguous adjacent movements.
	order := make([]int, 0, len(p.Rows))
	states := make([]string, 0, len(p.Rows))
	for i := range p.Rows {
		states = append(states, p.Rows[i].Status+"|"+strings.Join(p.Rows[i].Warnings, "|"))
		if p.Rows[i].Status != "invalid" {
			order = append(order, i)
		}
	}
	p.PreviewFingerprint = digest(append([]string{accountID}, states...)...)
	sort.Slice(order, func(i, j int) bool { return p.Rows[order[i]].OccurredAt < p.Rows[order[j]].OccurredAt })
	for i := 1; i < len(order); i++ {
		prev := &p.Rows[order[i-1]]
		next := &p.Rows[order[i]]
		if prev.OccurredAt == next.OccurredAt || (i+1 < len(order) && p.Rows[order[i+1]].OccurredAt == next.OccurredAt) || (i > 1 && p.Rows[order[i-2]].OccurredAt == prev.OccurredAt) {
			continue
		}
		ab, e1 := decimal.NewFromString(next.Balance)
		bb, e2 := decimal.NewFromString(prev.Balance)
		amount, e3 := decimal.NewFromString(next.Amount)
		if e1 == nil && e2 == nil && e3 == nil && !ab.Sub(bb).Equal(amount) {
			next.Warnings = append(next.Warnings, "continuidade do saldo não confere")
		}
	}

	return nil
}
func makeStatementPreview(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, data []byte, filename, format, accountID string) (importPreview, error) {
	parsed, err := statementimport.Parse(data, format)
	if err != nil {
		return importPreview{}, &statementParseError{err}
	}
	p := importPreview{Parsed: parsed, Filename: filename}
	if err := classifyStatement(ctx, q, &p, accountID); err != nil {
		return importPreview{}, err
	}
	return p, nil
}
func handleStatementPreview(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, name, ok := readStatementUpload(w, r)
		if !ok {
			return
		}
		accountID := strings.TrimSpace(r.FormValue("account_id"))
		if accountID != "" {
			if _, err := loadFileAccount(r.Context(), conn, accountID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeProblem(w, 422, "invalid-account", "Conta inválida", "Escolha uma conta de arquivo em BRL.")
				} else {
					previewFailure(w, err)
				}
				return
			}
		}
		p, err := makeStatementPreview(r.Context(), conn, data, name, r.FormValue("format"), accountID)
		if err != nil {
			if !statementParseProblem(w, err) {
				previewFailure(w, err)
			}
			return
		}
		writeJSON(w, 200, p)
	}
}
func handleStatementAccounts(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := conn.QueryContext(r.Context(), `SELECT fa.id, COALESCE(fa.name,''), fa.currency_code FROM financial_accounts fa JOIN data_sources ds ON ds.id=fa.source_id WHERE ds.provider='file' AND fa.currency_code='BRL' ORDER BY fa.name,fa.id`)
		if err != nil {
			writeProblem(w, 503, "accounts-unavailable", "Contas indisponíveis", "")
			return
		}
		defer rows.Close()
		out := []importAccount{}
		for rows.Next() {
			var a importAccount
			if err := rows.Scan(&a.ID, &a.Name, &a.CurrencyCode); err != nil {
				writeProblem(w, 503, "accounts-unavailable", "Contas indisponíveis", "")
				return
			}
			out = append(out, a)
		}
		if rows.Err() != nil {
			writeProblem(w, 503, "accounts-unavailable", "Contas indisponíveis", "")
			return
		}
		writeJSON(w, 200, out)
	}
}
func handleStatementHistory(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := conn.QueryContext(r.Context(), `SELECT si.sync_run_id,si.account_id,COALESCE(fa.name,''),si.filename,si.format,si.created_at,si.rows_total,sr.transactions_inserted,si.rows_duplicate,si.rows_invalid FROM statement_imports si JOIN sync_runs sr ON sr.id=si.sync_run_id JOIN financial_accounts fa ON fa.id=si.account_id ORDER BY si.created_at DESC LIMIT 100`)
		if err != nil {
			writeProblem(w, 503, "imports-unavailable", "Importações indisponíveis", "")
			return
		}
		defer rows.Close()
		out := []importHistory{}
		for rows.Next() {
			var h importHistory
			if err := rows.Scan(&h.RunID, &h.AccountID, &h.AccountName, &h.Filename, &h.Format, &h.CreatedAt, &h.Counts.Total, &h.Counts.New, &h.Counts.Duplicate, &h.Counts.Invalid); err != nil {
				writeProblem(w, 503, "imports-unavailable", "Importações indisponíveis", "")
				return
			}
			out = append(out, h)
		}
		if rows.Err() != nil {
			writeProblem(w, 503, "imports-unavailable", "Importações indisponíveis", "")
			return
		}
		writeJSON(w, 200, out)
	}
}
func handleStatementConfirm(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, name, ok := readStatementUpload(w, r)
		if !ok {
			return
		}
		accountID := strings.TrimSpace(r.FormValue("account_id"))
		newName := strings.TrimSpace(r.FormValue("new_account_name"))
		if (accountID == "") == (newName == "") || len(newName) > 120 {
			writeProblem(w, 422, "invalid-account", "Conta inválida", "Escolha uma conta ou informe um nome para a nova conta.")
			return
		}
		if r.FormValue("expected_sha256") == "" || r.FormValue("expected_format") == "" || r.FormValue("expected_format_version") == "" || r.FormValue("expected_preview_fingerprint") == "" {
			writeProblem(w, 422, "missing-preview", "Prévia necessária", "Confira a prévia antes de importar.")
			return
		}
		tx, err := conn.BeginTx(r.Context(), nil)
		if err != nil {
			importFailure(w, err)
			return
		}
		defer tx.Rollback()
		sourceID := ""
		if accountID != "" {
			sourceID, err = loadFileAccountTx(r.Context(), tx, accountID)
			if errors.Is(err, sql.ErrNoRows) {
				writeProblem(w, 422, "invalid-account", "Conta inválida", "Escolha uma conta de arquivo em BRL.")
				return
			} else if err != nil {
				importFailure(w, err)
				return
			}
		}
		p, err := makeStatementPreviewTx(r.Context(), tx, data, name, r.FormValue("format"), accountID)
		if err != nil {
			if !statementParseProblem(w, err) {
				importFailure(w, err)
			}
			return
		}
		if p.SHA256 != r.FormValue("expected_sha256") || p.Format != r.FormValue("expected_format") || p.FormatVersion != r.FormValue("expected_format_version") {
			writeProblem(w, 409, "preview-changed", "Arquivo alterado", "Gere uma nova prévia antes de importar.")
			return
		}
		if p.PreviewFingerprint != r.FormValue("expected_preview_fingerprint") {
			writeProblem(w, 409, "preview-changed", "Prévia alterada", "As movimentações da conta mudaram. Gere uma nova prévia.")
			return
		}
		for i := range p.Rows {
			row := &p.Rows[i]
			if row.Status != "ambiguous" {
				continue
			}
			switch r.FormValue(fmt.Sprintf("ambiguous_line_%d", row.LineNumber)) {
			case "import":
				row.Status = "new"
				row.Identity = digest("statement-v3", accountID, p.SHA256, fmt.Sprint(row.LineNumber))
				row.ContentHash = digest(row.Identity, row.Balance)
				p.Counts.New++
			case "ignore":
				row.Status = "duplicate"
				p.Counts.Duplicate++
			default:
				writeProblem(w, 422, "ambiguous-row", "Movimentação ambígua", "Escolha importar ou ignorar cada movimentação ambígua.")
				return
			}
		}
		p.Counts.Ambiguous = 0
		if p.Counts.New == 0 && newName != "" {
			writeProblem(w, 422, "empty-import", "Nada para importar", "O extrato não contém movimentações válidas.")
			return
		}
		if p.Counts.Invalid > 0 && r.FormValue("allow_partial") != "true" {
			writeProblem(w, 422, "invalid-rows", "Linhas inválidas", "Revise as linhas ou autorize importar somente as válidas.")
			return
		}
		now := db.FormatTime(time.Now().UTC())
		runID := uuid.NewString()
		rawID := uuid.NewString()
		if accountID == "" {
			sourceID = uuid.NewString()
			accountID = uuid.NewString()
			_, err = tx.ExecContext(r.Context(), `INSERT INTO data_sources (id,provider,external_item_id,display_name,label,created_at,updated_at) VALUES (?,'file',?,?,?,?,?)`, sourceID, sourceID, newName, newName, now, now)
			if err != nil {
				importFailure(w, err)
				return
			}
			if err = classifyStatementTx(r.Context(), tx, &p, accountID); err != nil {
				importFailure(w, err)
				return
			}
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO sync_runs (id,source_id,run_type,status,started_at,finished_at,accounts_processed,transactions_inserted) VALUES (?,?,'file_import','completed',?,?,1,?)`, runID, sourceID, now, now, p.Counts.New); err != nil {
			importFailure(w, err)
			return
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO raw_imports (id,sync_run_id,source_id,scope,external_account_id,page_sequence,request_attempt,payload,payload_sha256,received_at) VALUES (?,?,?,'file',?,1,1,?,?,?)`, rawID, runID, sourceID, accountID, data, p.SHA256, now); err != nil {
			importFailure(w, err)
			return
		}
		if newName != "" {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO financial_accounts (id,source_id,external_id,institution,name,account_type,account_subtype,currency_code,current_raw_import_id,normalized_hash,created_at,updated_at) VALUES (?,?,?,?,?,'BANK','CHECKING_ACCOUNT','BRL',?,?,?,?)`, accountID, sourceID, accountID, p.Institution, newName, rawID, digest(accountID, newName), now, now)
			if err != nil {
				importFailure(w, err)
				return
			}
		}
		inserted := 0
		duplicates := 0
		for _, row := range p.Rows {
			if row.Status == "invalid" {
				continue
			}
			if row.Status == "duplicate" {
				duplicates++
				continue
			}
			id := uuid.NewString()
			move := "DEBIT"
			if strings.HasPrefix(row.Amount, "-") == false {
				move = "CREDIT"
			}
			res, e := tx.ExecContext(r.Context(), `INSERT INTO financial_transactions (id,source_id,account_id,external_id,description,description_raw,amount,amount_in_account_currency,balance_after,currency_code,occurred_at,provider_status,movement_type,source_category,current_raw_import_id,normalized_hash,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?, 'BRL',?,'POSTED',?,?,?,?,?,?) ON CONFLICT (source_id,external_id) DO NOTHING`, id, sourceID, accountID, row.Identity, row.Description, row.Description, row.Amount, row.Amount, row.Balance, row.OccurredAt, move, row.PaymentMethod, rawID, row.ContentHash, now, now)
			if e != nil {
				importFailure(w, e)
				return
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				writeProblem(w, 409, "preview-changed", "Prévia alterada", "As movimentações da conta mudaram. Gere uma nova prévia.")
				return
			}
			inserted++
			if _, e = tx.ExecContext(r.Context(), `INSERT INTO normalization_events (id,sync_run_id,raw_import_id,entity_type,transaction_id,external_id,outcome,normalized_hash,created_at) VALUES (?,?,?,'transaction',?,?,'inserted',?,?)`, uuid.NewString(), runID, rawID, id, row.Identity, row.ContentHash, now); e != nil {
				importFailure(w, e)
				return
			}
			if e = ledger.ApplyNewTransactionDecisions(r.Context(), tx, id, nil, onIgnoredHook); e != nil {
				importFailure(w, e)
				return
			}
		}
		if asOf, balance := latestStatementBalance(p.Rows); asOf != "" {
			_, err = tx.ExecContext(r.Context(), `UPDATE financial_accounts SET balance=?,balance_as_of=?,updated_at=? WHERE id=? AND (balance_as_of IS NULL OR balance_as_of < ?)`, balance, asOf, now, accountID, asOf)
			if err != nil {
				importFailure(w, err)
				return
			}
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE sync_runs SET transactions_inserted=? WHERE id=?`, inserted, runID); err != nil {
			importFailure(w, err)
			return
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO statement_imports (id,sync_run_id,source_id,account_id,format,adapter_version,filename,file_sha256,rows_total,rows_invalid,rows_duplicate,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), runID, sourceID, accountID, p.Format, p.FormatVersion, name, p.SHA256, p.Counts.Total, p.Counts.Invalid, duplicates, now); err != nil {
			importFailure(w, err)
			return
		}
		if err = tx.Commit(); err != nil {
			importFailure(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"run_id": runID, "account_id": accountID, "counts": importCounts{Total: p.Counts.Total, New: inserted, Duplicate: duplicates, Invalid: p.Counts.Invalid}})
	}
}

// importFailure answers an infrastructure failure during confirmation. As with
// previewFailure, the underlying error is logged and never sent to the client.
func importFailure(w http.ResponseWriter, err error) {
	log.Printf("statement_import_failed: %v", err)
	writeProblem(w, 503, "import-unavailable", "Importação indisponível", "Tente novamente em instantes.")
}

// latestStatementBalance picks the balance the file leaves the account with:
// that of its newest valid row, together with that row's instant. It returns
// empty strings, meaning "leave the account balance alone", whenever the
// figure could be stale or ambiguous:
//   - several rows share the newest instant, so their order is unknown;
//   - the newest row is a duplicate whose stored balance differs from the file's;
//   - an invalid row (only imported under allow_partial) may be newer: its date
//     is unreadable, or it is not older than the newest valid row.
//
// Older invalid rows do not matter. The caller still imports the valid rows
// and never moves balance_as_of backwards.
func latestStatementBalance(rows []statementimport.Row) (asOf, balance string) {
	newest := 0
	for _, row := range rows {
		if row.Status == "invalid" {
			continue
		}
		switch {
		case row.OccurredAt > asOf:
			asOf, balance, newest = row.OccurredAt, row.Balance, 1
			if slices.Contains(row.Warnings, warnBalanceDiffers) {
				balance = ""
			}
		case row.OccurredAt == asOf:
			newest++
		}
	}
	if newest != 1 || balance == "" {
		return "", ""
	}
	for _, row := range rows {
		if row.Status == "invalid" && (row.OccurredAt == "" || row.OccurredAt >= asOf) {
			return "", ""
		}
	}
	return asOf, balance
}

// sql.Tx.QueryRowContext returns *sql.Row too; these wrappers keep the
// preview and confirmation on the same database transaction.
func loadFileAccountTx(ctx context.Context, tx *sql.Tx, id string) (string, error) {
	var s string
	err := tx.QueryRowContext(ctx, `SELECT fa.source_id FROM financial_accounts fa JOIN data_sources ds ON ds.id=fa.source_id WHERE fa.id=? AND ds.provider='file' AND fa.currency_code='BRL'`, id).Scan(&s)
	return s, err
}
func makeStatementPreviewTx(ctx context.Context, tx *sql.Tx, data []byte, name, format, account string) (importPreview, error) {
	p, e := statementimport.Parse(data, format)
	if e != nil {
		return importPreview{}, &statementParseError{e}
	}
	v := importPreview{Parsed: p, Filename: name}
	e = classifyStatementTx(ctx, tx, &v, account)
	return v, e
}
func classifyStatementTx(ctx context.Context, tx *sql.Tx, p *importPreview, account string) error {
	return classifyStatement(ctx, txQuery{tx}, p, account)
}

type txQuery struct{ *sql.Tx }

func (t txQuery) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, q, args...)
}
