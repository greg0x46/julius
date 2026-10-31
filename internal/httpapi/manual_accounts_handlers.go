package httpapi

import (
	"database/sql"
	"net/http"
	"strings"

	"errors"
	"github.com/shopspring/decimal"
	"golang.org/x/text/currency"

	"github.com/greg0x46/julius/internal/exchange"

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
		if !validMoneyAmount(opening) {
			invalidManualAccount(w, "Saldo inicial inválido. "+invalidAmountDetail)
			return
		}
		tx, err := conn.BeginTx(r.Context(), nil)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		defer tx.Rollback()
		id, err := transactions.WriteManualAccount(r.Context(), tx, r.PathValue("id"), transactions.ManualAccountInput{Name: req.Name, CurrencyCode: req.CurrencyCode, OpeningBalance: opening, Archived: req.Archived}, create)
		if err != nil {
			manualAccountWriteProblem(w, err)
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
		if !validMoneyAmount(amount) {
			invalidManualAccount(w, "Valor do ajuste inválido. "+invalidAmountDetail)
			return
		}
		tx, err := conn.BeginTx(r.Context(), nil)
		if err != nil {
			accountsUnavailableProblem(w)
			return
		}
		defer tx.Rollback()
		if err := transactions.AdjustManualAccount(r.Context(), tx, r.PathValue("id"), amount, req.Description); err != nil {
			manualAccountWriteProblem(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			accountsUnavailableProblem(w)
			return
		}
		handleGetAccount(conn)(w, r)
	}
}

func manualAccountWriteProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, transactions.ErrAccountNotFound):
		accountNotFoundProblem(w)
	case errors.Is(err, transactions.ErrAccountNotManual):
		writeProblem(w, 409, "account-not-manual", "Conta sincronizada", "Os dados desta conta são definidos pela fonte de dados.")
	case errors.Is(err, transactions.ErrAccountHasMovements):
		writeProblem(w, 409, "account-has-movements", "Conta com movimentações", "A moeda e o saldo inicial não podem ser alterados após movimentações. Use um ajuste para corrigir o saldo.")
	case errors.Is(err, transactions.ErrAccountNotActiveManual):
		writeProblem(w, 409, "account-not-active-manual", "Ajuste indisponível", "Ajustes exigem uma conta manual ativa.")
	default:
		accountsUnavailableProblem(w)
	}
}
