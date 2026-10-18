package httpapi

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/text/currency"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
)

type manualAccountRequest struct {
	Name           string `json:"name"`
	CurrencyCode   string `json:"currency_code"`
	OpeningBalance string `json:"opening_balance"`
	Archived       bool   `json:"archived"`
}

func handleAccountCurrencies(provider exchange.CoverageProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		coverage := provider.Currencies(r.Context())
		currencies := make([]exchange.Currency, 0, len(coverage.Currencies))
		for _, c := range coverage.Currencies {
			if validManualAccountCurrency(c.Code) {
				currencies = append(currencies, c)
			}
		}
		coverage.Currencies = currencies
		writeJSON(w, 200, coverage)
	}
}

func validManualAccountCurrency(code string) bool {
	_, err := currency.ParseISO(code)
	return err == nil && code == strings.ToUpper(code) && code != "XXX"
}

func invalidManualAccount(w http.ResponseWriter, detail string) {
	writeProblem(w, 422, "invalid-manual-account", "Conta manual inválida", detail)
}

func handleWriteManualAccount(conn *sql.DB, create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req manualAccountRequest
		if err := decodeStrict(r, &req); err != nil {
			invalidManualAccount(w, "Informe nome, moeda e saldo inicial.")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.CurrencyCode == "" {
			req.CurrencyCode = "BRL"
		}
		if req.OpeningBalance == "" {
			req.OpeningBalance = "0"
		}
		opening, err := decimal.NewFromString(req.OpeningBalance)
		if req.Name == "" || len(req.Name) > 200 || !validManualAccountCurrency(req.CurrencyCode) || err != nil {
			invalidManualAccount(w, "Informe um nome, uma moeda ISO 4217 e um saldo inicial decimal válido.")
			return
		}
		tx, err := conn.BeginTx(r.Context(), nil)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		defer tx.Rollback()
		id := r.PathValue("id")
		now := db.FormatTime(time.Now())
		openingRaw := money.CanonicalDecimal(opening)
		var archived any
		if req.Archived {
			archived = now
		}
		if create {
			id = uuid.NewString()
			_, err = tx.ExecContext(r.Context(), `INSERT INTO financial_accounts
   (id, origin, name, account_type, currency_code, opening_balance, archived_at, created_at, updated_at)
   VALUES (?, 'manual', ?, 'BANK', ?, ?, ?, ?, ?)`, id, req.Name, req.CurrencyCode, openingRaw, archived, now, now)
		} else {
			// Take the same row lock as manual transaction creation before checking currency.
			result, lockErr := tx.ExecContext(r.Context(), `UPDATE financial_accounts SET updated_at = updated_at WHERE id = ?`, id)
			if lockErr != nil {
				accountsUnavailableProblem(w)
				return
			}
			n, lockErr := result.RowsAffected()
			if lockErr != nil {
				accountsUnavailableProblem(w)
				return
			}
			if n == 0 {
				accountNotFoundProblem(w)
				return
			}
			var origin, oldCurrency string
			var oldOpening decimal.Decimal
			var hasMovements bool
			err = tx.QueryRowContext(r.Context(), `SELECT origin, COALESCE(currency_code, ''), COALESCE(opening_balance, '0'),
    EXISTS(SELECT 1 FROM financial_transactions WHERE account_id = ?) OR
    EXISTS(SELECT 1 FROM account_balance_adjustments WHERE account_id = ?)
    FROM financial_accounts WHERE id = ?`, id, id, id).Scan(&origin, &oldCurrency, dbtypes.Decimal(&oldOpening), dbtypes.Bool(&hasMovements))
			if err != nil {
				accountsUnavailableProblem(w)
				return
			}
			if origin != "manual" {
				writeProblem(w, 409, "account-not-manual", "Conta sincronizada", "Os dados desta conta são definidos pela fonte de dados.")
				return
			}
			if hasMovements && (oldCurrency != req.CurrencyCode || !oldOpening.Equal(opening)) {
				writeProblem(w, 409, "account-has-movements", "Conta com movimentações", "A moeda e o saldo inicial não podem ser alterados após movimentações. Use um ajuste para corrigir o saldo.")
				return
			}
			_, err = tx.ExecContext(r.Context(), `UPDATE financial_accounts SET name = ?, currency_code = ?, opening_balance = ?, archived_at = ?, updated_at = ? WHERE id = ?`, req.Name, req.CurrencyCode, openingRaw, archived, now, id)
		}
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		if err := tx.Commit(); err != nil {
			accountsUnavailableProblem(w)
			return
		}
		if create {
			w.Header().Set("Location", "/api/accounts/"+id)
		}
		row := conn.QueryRowContext(r.Context(), `SELECT `+accountSelectColumns+accountSelectFrom+` WHERE fa.id = ?`, id)
		d, err := scanAccount(row)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		d.Balance, err = transactions.AccountBalance(r.Context(), conn, id)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		status := 200
		if create {
			status = 201
		}
		writeJSON(w, status, d)
	}
}

func handleAdjustManualAccount(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Amount      string `json:"amount"`
			Description string `json:"description"`
		}
		if err := decodeStrict(r, &req); err != nil {
			invalidManualAccount(w, "Informe valor e motivo do ajuste.")
			return
		}
		amount, err := decimal.NewFromString(req.Amount)
		req.Description = strings.TrimSpace(req.Description)
		if err != nil || amount.IsZero() || req.Description == "" || len(req.Description) > 200 {
			invalidManualAccount(w, "Informe um valor decimal diferente de zero e o motivo do ajuste.")
			return
		}
		tx, err := conn.BeginTx(r.Context(), nil)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		defer tx.Rollback()
		id := r.PathValue("id")
		result, err := tx.ExecContext(r.Context(), `UPDATE financial_accounts SET updated_at = ? WHERE id = ? AND origin = 'manual' AND archived_at IS NULL`, db.FormatTime(time.Now()), id)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		n, err := result.RowsAffected()
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		if n == 0 {
			var exists bool
			err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM financial_accounts WHERE id = ?)`, id).Scan(dbtypes.Bool(&exists))
			if err != nil {
				accountsUnavailableProblem(w)
				return
			}
			if !exists {
				accountNotFoundProblem(w)
				return
			}
			writeProblem(w, 409, "account-not-active-manual", "Ajuste indisponível", "Ajustes exigem uma conta manual ativa.")
			return
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO account_balance_adjustments (id, account_id, amount, description, created_at) VALUES (?, ?, ?, ?, ?)`, uuid.NewString(), id, money.CanonicalDecimal(amount), req.Description, db.FormatTime(time.Now()))
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		if err := tx.Commit(); err != nil {
			accountsUnavailableProblem(w)
			return
		}
		handleGetAccount(conn)(w, r)
	}
}
