package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/quotes"
)

// This file is the HTTP face of internal/investments: the custody accounts,
// goals, manual positions, the operations that move them and the links that
// explain a bank line. The preserved provider read API stays in
// investments_handlers.go — it answers a different question (what the bank
// reported) and must keep working untouched.
//
// Every monetary field crosses the wire as canonical decimal text and every
// date as YYYY-MM-DD: the frontend parser rejects anything else, and a float
// would silently round money on the way through.

const investmentDateLayout = "2006-01-02"

// investmentDecimal reads a monetary field from its exact JSON text. A string
// is the shape the frontend sends; a bare JSON number is still parsed from
// its literal text rather than through float64, so no precision is lost
// either way. Absent, null and "" all mean "not informed", which is distinct
// from zero for the optional fields.
type investmentDecimal struct {
	value   decimal.Decimal
	present bool
}

func (d *investmentDecimal) UnmarshalJSON(raw []byte) error {
	text := string(bytes.TrimSpace(raw))
	if text == "null" {
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var quoted string
		if err := json.Unmarshal(raw, &quoted); err != nil {
			return err
		}
		text = strings.TrimSpace(quoted)
		if text == "" {
			return nil
		}
	}
	value, err := decimal.NewFromString(text)
	if err != nil {
		return err
	}
	d.value, d.present = value, true
	return nil
}

func (d investmentDecimal) orZero() decimal.Decimal {
	return d.value
}

func (d investmentDecimal) pointer() *decimal.Decimal {
	if !d.present {
		return nil
	}
	value := d.value
	return &value
}

// investmentText separates "the field was not sent" from "the field was sent
// as null". The distinction is the whole difference between keeping a link or
// a note and deliberately clearing it, and a plain *string collapses the two.
type investmentText struct {
	value   *string
	present bool
}

func (t *investmentText) UnmarshalJSON(raw []byte) error {
	t.present = true
	if string(bytes.TrimSpace(raw)) == "null" {
		t.value = nil
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}
	t.value = &text
	return nil
}

// orCurrent answers what the write should carry: the sent value when the
// caller sent the field, and the value already stored otherwise.
func (t investmentText) orCurrent(current *string) *string {
	if !t.present {
		return current
	}
	return investmentOptionalID(t.value)
}

func (t investmentText) stringOr(current string) string {
	if !t.present || t.value == nil {
		return current
	}
	return *t.value
}

// ---------------------------------------------------------------- responses

type investmentAccountDTO struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Kind               string    `json:"kind"`
	CurrencyCode       string    `json:"currency_code"`
	SourceID           *string   `json:"source_id"`
	SourceDisplayName  *string   `json:"source_display_name"`
	FinancialAccountID *string   `json:"financial_account_id"`
	Active             bool      `json:"active"`
	CashBalance        string    `json:"cash_balance"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// investmentAccountCash reports the cash the account actually holds, the same
// way BuildSummary does: a custody account linked to a bank account reports
// the bank's balance (the local ledger would double it), and an integrated
// grouping holds no local cash at all.
func investmentAccountCash(a investments.Account) decimal.Decimal {
	if a.LinkedCashBalance != nil {
		return *a.LinkedCashBalance
	}
	if a.Kind == investments.AccountKindIntegrated {
		return decimal.Zero
	}
	return a.CashBalance
}

// investmentCurrency keeps currency_code a non-null "BRL" for the frontend:
// integrated groupings carry no currency of their own, and the manual ledger
// is BRL by construction (see migration 00037's CHECK).
func investmentCurrency(code *string) string {
	if code != nil && *code != "" {
		return *code
	}
	return "BRL"
}

func investmentAccountToDTO(a investments.Account) investmentAccountDTO {
	return investmentAccountDTO{
		// The id of an integrated grouping is "integrated:<data source id>".
		// It is emitted verbatim: rewriting it would break the only link back
		// to the connection it groups.
		ID:                 a.ID,
		Name:               a.Name,
		Kind:               string(a.Kind),
		CurrencyCode:       investmentCurrency(a.CurrencyCode),
		SourceID:           a.SourceID,
		SourceDisplayName:  a.SourceDisplayName,
		FinancialAccountID: a.FinancialAccountID,
		Active:             a.Active,
		CashBalance:        money.CanonicalDecimal(investmentAccountCash(a)),
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
}

type investmentPortfolioDTO struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	TargetAmount *string   `json:"target_amount"`
	TargetDate   *string   `json:"target_date"`
	Notes        *string   `json:"notes"`
	CurrentValue string    `json:"current_value"`
	Progress     *string   `json:"progress"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type investmentAssetDTO struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Ticker       *string                `json:"ticker"`
	AssetType    string                 `json:"asset_type"`
	AssetClass   investments.AssetClass `json:"asset_class"`
	CurrencyCode string                 `json:"currency_code"`
	QuoteSource  *string                `json:"quote_source"`
	QuoteSymbol  *string                `json:"quote_symbol"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

func investmentAssetToDTO(asset investments.Asset) investmentAssetDTO {
	return investmentAssetDTO{
		ID:           asset.ID,
		Name:         asset.Name,
		Ticker:       asset.Ticker,
		AssetType:    asset.AssetType,
		AssetClass:   asset.AssetClass,
		CurrencyCode: asset.CurrencyCode,
		QuoteSource:  asset.QuoteSource,
		QuoteSymbol:  asset.QuoteSymbol,
		CreatedAt:    asset.CreatedAt,
		UpdatedAt:    asset.UpdatedAt,
	}
}

func investmentPortfolioToDTO(p investments.Portfolio) investmentPortfolioDTO {
	return investmentPortfolioDTO{
		ID:           p.ID,
		Name:         p.Name,
		TargetAmount: optionalMoney(p.TargetAmount),
		TargetDate:   optionalDay(p.TargetDate),
		Notes:        p.Notes,
		CurrentValue: money.CanonicalDecimal(p.CurrentValue),
		Progress:     optionalMoney(p.Progress),
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}

type investmentPositionDTO struct {
	ID                 string  `json:"id"`
	Source             string  `json:"source"`
	AccountID          string  `json:"account_id"`
	AssetID            *string `json:"asset_id"`
	PortfolioID        *string `json:"portfolio_id"`
	Name               string  `json:"name"`
	Ticker             *string `json:"ticker"`
	AssetType          string  `json:"asset_type"`
	Quantity           string  `json:"quantity"`
	AverageCost        *string `json:"average_cost"`
	CurrentValue       string  `json:"current_value"`
	CurrentUnitPrice   *string `json:"current_unit_price"`
	ValuedOn           *string `json:"valued_on"`
	CurrencyCode       string  `json:"currency_code"`
	Closed             bool    `json:"closed"`
	LinkedInvestmentID *string `json:"linked_investment_id"`
	Notes              *string `json:"notes"`
	ValuationBasis     string  `json:"valuation_basis"`
}

// investmentPositionToDTO passes source and valuation_basis straight through:
// both are the domain's judgement about where a holding's numbers came from,
// and recomputing either here would let the two disagree.
func investmentPositionToDTO(p investments.Position) investmentPositionDTO {
	return investmentPositionDTO{
		ID:                 p.ID,
		Source:             string(p.Source),
		AccountID:          p.AccountID,
		AssetID:            p.AssetID,
		PortfolioID:        p.PortfolioID,
		Name:               p.Name,
		Ticker:             p.Ticker,
		AssetType:          p.AssetType,
		Quantity:           money.CanonicalDecimal(p.Quantity),
		AverageCost:        optionalMoney(p.AverageCost),
		CurrentValue:       money.CanonicalDecimal(p.CurrentValue),
		CurrentUnitPrice:   optionalMoney(p.CurrentUnitPrice),
		ValuedOn:           optionalDay(p.ValuedOn),
		CurrencyCode:       investmentCurrency(p.CurrencyCode),
		Closed:             p.Closed,
		LinkedInvestmentID: p.LinkedInvestmentID,
		Notes:              p.Notes,
		ValuationBasis:     string(p.ValuationBasis),
	}
}

type investmentOperationDTO struct {
	ID         string    `json:"id"`
	AccountID  string    `json:"account_id"`
	PositionID *string   `json:"position_id"`
	TransferID *string   `json:"transfer_id"`
	Kind       string    `json:"kind"`
	OccurredOn string    `json:"occurred_on"`
	Amount     string    `json:"amount"`
	Quantity   *string   `json:"quantity"`
	UnitPrice  *string   `json:"unit_price"`
	Fees       string    `json:"fees"`
	Taxes      string    `json:"taxes"`
	Notes      *string   `json:"notes"`
	Source     string    `json:"source"`
	IsEditable bool      `json:"is_editable"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func investmentOperationToDTO(op investments.Operation) investmentOperationDTO {
	return investmentOperationDTO{
		ID:         op.ID,
		AccountID:  op.AccountID,
		PositionID: op.PositionID,
		TransferID: op.TransferID,
		Kind:       string(op.Kind),
		OccurredOn: investments.Day(op.OccurredOn).Format(investmentDateLayout),
		Amount:     money.CanonicalDecimal(op.Amount),
		Quantity:   optionalMoney(op.Quantity),
		UnitPrice:  optionalMoney(op.UnitPrice),
		Fees:       money.CanonicalDecimal(op.Fees),
		Taxes:      money.CanonicalDecimal(op.Taxes),
		Notes:      op.Notes,
		Source:     op.Source,
		// is_editable is the domain's answer, not a re-derivation: an imported
		// movement is read-only no matter what this layer thinks.
		IsEditable: op.IsEditable,
		CreatedAt:  op.CreatedAt,
		UpdatedAt:  op.UpdatedAt,
	}
}

type investmentReconciliationDTO struct {
	ID                               string    `json:"id"`
	OperationID                      string    `json:"operation_id"`
	FinancialTransactionID           *string   `json:"financial_transaction_id"`
	FinancialInvestmentTransactionID *string   `json:"financial_investment_transaction_id"`
	Amount                           string    `json:"amount"`
	CreatedAt                        time.Time `json:"created_at"`
}

func investmentReconciliationToDTO(r investments.Reconciliation) investmentReconciliationDTO {
	return investmentReconciliationDTO{
		ID:                               r.ID,
		OperationID:                      r.OperationID,
		FinancialTransactionID:           r.FinancialTransactionID,
		FinancialInvestmentTransactionID: r.FinancialInvestmentTransactionID,
		Amount:                           money.CanonicalDecimal(r.Amount),
		CreatedAt:                        r.CreatedAt,
	}
}

type investmentSummaryPortfolioDTO struct {
	PortfolioID  *string `json:"portfolio_id"`
	Name         string  `json:"name"`
	CurrentValue string  `json:"current_value"`
	TargetAmount *string `json:"target_amount"`
	Progress     *string `json:"progress"`
}

type investmentSummaryAccountDTO struct {
	AccountID    string `json:"account_id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	CurrentValue string `json:"current_value"`
	CashBalance  string `json:"cash_balance"`
}

type investmentSummaryDTO struct {
	CurrencyCode   string                          `json:"currency_code"`
	TotalValue     string                          `json:"total_value"`
	ManualValue    string                          `json:"manual_value"`
	SyncedValue    string                          `json:"synced_value"`
	CashBalance    string                          `json:"cash_balance"`
	UnrealizedGain string                          `json:"unrealized_gain"`
	Portfolios     []investmentSummaryPortfolioDTO `json:"portfolios"`
	Accounts       []investmentSummaryAccountDTO   `json:"accounts"`
}

func investmentSummaryToDTO(s investments.Summary) investmentSummaryDTO {
	dto := investmentSummaryDTO{
		CurrencyCode:   investmentCurrency(&s.CurrencyCode),
		TotalValue:     money.CanonicalDecimal(s.TotalValue),
		ManualValue:    money.CanonicalDecimal(s.ManualValue),
		SyncedValue:    money.CanonicalDecimal(s.SyncedValue),
		CashBalance:    money.CanonicalDecimal(s.CashBalance),
		UnrealizedGain: money.CanonicalDecimal(s.UnrealizedGain),
		Portfolios:     []investmentSummaryPortfolioDTO{},
		Accounts:       []investmentSummaryAccountDTO{},
	}
	for _, p := range s.Portfolios {
		dto.Portfolios = append(dto.Portfolios, investmentSummaryPortfolioDTO{
			PortfolioID:  p.PortfolioID,
			Name:         p.Name,
			CurrentValue: money.CanonicalDecimal(p.CurrentValue),
			TargetAmount: optionalMoney(p.TargetAmount),
			Progress:     optionalMoney(p.Progress),
		})
	}
	for _, a := range s.Accounts {
		dto.Accounts = append(dto.Accounts, investmentSummaryAccountDTO{
			AccountID:    a.AccountID,
			Name:         a.Name,
			Kind:         string(a.Kind),
			CurrentValue: money.CanonicalDecimal(a.CurrentValue),
			CashBalance:  money.CanonicalDecimal(a.CashBalance),
		})
	}
	return dto
}

func optionalMoney(value *decimal.Decimal) *string {
	if value == nil {
		return nil
	}
	text := money.CanonicalDecimal(*value)
	return &text
}

func optionalDay(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := investments.Day(*value).Format(investmentDateLayout)
	return &text
}

// writeInvestmentItems keeps every collection in the {"items": [...]} envelope
// the frontend parses, with an empty list serialised as [] rather than null.
func writeInvestmentItems[T any](w http.ResponseWriter, items []T) {
	if items == nil {
		items = []T{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ------------------------------------------------------------------- errors

// writeInvestmentProblem gives every domain refusal its own slug, so the UI
// can tell "this is a provider record" from "this correction would overdraw
// the custody cash" without parsing prose.
func writeInvestmentProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, investments.ErrAccountNotFound):
		writeProblem(w, http.StatusNotFound, "investment-account-not-found",
			"Conta de custódia não encontrada", "A conta de investimento informada não existe.")
	case errors.Is(err, investments.ErrPortfolioNotFound):
		writeProblem(w, http.StatusNotFound, "investment-portfolio-not-found",
			"Objetivo não encontrado", "O objetivo de investimento informado não existe.")
	case errors.Is(err, investments.ErrPositionNotFound):
		writeProblem(w, http.StatusNotFound, "investment-position-not-found",
			"Posição não encontrada", "A posição informada não existe.")
	case errors.Is(err, investments.ErrAssetNotFound):
		writeProblem(w, http.StatusNotFound, "investment-asset-not-found",
			"Ativo não encontrado", "O ativo informado não existe.")
	case errors.Is(err, investments.ErrAssetAlreadyExists):
		writeProblem(w, http.StatusConflict, "investment-asset-already-exists",
			"Ativo já cadastrado", "Já existe um ativo com o mesmo código ou identificação.")
	case errors.Is(err, investments.ErrAssetHasPositions):
		writeProblem(w, http.StatusConflict, "investment-asset-has-positions",
			"Ativo em uso", "Exclua as posições associadas antes de excluir este ativo.")
	case errors.Is(err, investments.ErrPositionAlreadyExists):
		writeProblem(w, http.StatusConflict, "investment-position-already-exists",
			"Ativo já está nesta conta", "Use a posição existente para registrar novas movimentações deste ativo.")
	case errors.Is(err, investments.ErrTransferAtomic):
		writeProblem(w, http.StatusConflict, "investment-transfer-atomic",
			"Transferência indivisível", "Exclua a transferência completa; suas duas movimentações não podem ser alteradas separadamente.")
	case errors.Is(err, investments.ErrOperationNotFound):
		writeProblem(w, http.StatusNotFound, "investment-operation-not-found",
			"Movimentação não encontrada", "A movimentação informada não existe.")
	case errors.Is(err, investments.ErrReconciliationNotFound):
		writeProblem(w, http.StatusNotFound, "investment-reconciliation-not-found",
			"Vínculo não encontrado", "O vínculo informado não existe.")
	case errors.Is(err, investments.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "investment-not-found",
			"Registro de investimento não encontrado", "O registro informado não existe.")
	case errors.Is(err, investments.ErrInvalidInput):
		writeInvestmentInvalid(w, "Revise os campos enviados.")
	case errors.Is(err, investments.ErrIntegratedReadOnly):
		writeProblem(w, http.StatusConflict, "investment-integrated-read-only",
			"Registro integrado é somente leitura",
			"Dados vindos da conexão não podem ser editados aqui; apenas o objetivo e o vínculo com a conta financeira são escolhas locais.")
	case errors.Is(err, investments.ErrNotManual):
		writeProblem(w, http.StatusConflict, "investment-not-manual",
			"Registro não é manual",
			"Apenas registros criados manualmente podem ser alterados ou excluídos; o histórico importado permanece como veio do provedor.")
	case errors.Is(err, investments.ErrAccountHasPositions):
		writeProblem(w, http.StatusConflict, "investment-account-has-positions",
			"Conta com posições", "Exclua ou mova as posições desta conta antes de excluí-la.")
	case errors.Is(err, investments.ErrAccountHasOperations):
		writeProblem(w, http.StatusConflict, "investment-account-has-operations",
			"Conta com movimentações", "Exclua as movimentações desta conta antes de excluí-la.")
	case errors.Is(err, investments.ErrFinancialAccountLinked):
		writeProblem(w, http.StatusConflict, "investment-financial-account-linked",
			"Conta bancária já vinculada",
			"Esta conta bancária já representa o caixa de outra custódia; desvincule-a antes de usá-la aqui.")
	case errors.Is(err, investments.ErrPositionHasOperations):
		writeProblem(w, http.StatusConflict, "investment-position-has-operations",
			"Posição com movimentações", "Exclua as movimentações desta posição antes de excluí-la.")
	case errors.Is(err, investments.ErrOperationHasReconciliations):
		writeProblem(w, http.StatusConflict, "investment-operation-has-reconciliations",
			"Movimentação vinculada a lançamento",
			"Desfaça os vínculos com lançamentos bancários antes de excluir esta movimentação.")
	case errors.Is(err, investments.ErrNegativeCash):
		writeProblem(w, http.StatusConflict, "investment-negative-cash",
			"Caixa ficaria negativo",
			"A correção deixaria o caixa da conta negativo em algum dia do histórico; nada foi gravado.")
	case errors.Is(err, investments.ErrNegativePosition):
		writeProblem(w, http.StatusConflict, "investment-negative-position",
			"Posição ficaria negativa",
			"A correção deixaria a quantidade da posição negativa em algum dia do histórico; nada foi gravado.")
	case errors.Is(err, investments.ErrReconciliationConflict):
		writeProblem(w, http.StatusConflict, "investment-reconciliation-conflict",
			"Vínculo excede o valor disponível",
			"A soma das parcelas vinculadas não pode ultrapassar o valor do lançamento nem o da movimentação.")
	case errors.Is(err, investments.ErrReconciliationDuplicate):
		writeProblem(w, http.StatusConflict, "investment-reconciliation-duplicate",
			"Vínculo já existente", "Este lançamento já está vinculado a esta movimentação.")
	case errors.Is(err, investments.ErrInvalidReconciliationLink):
		writeProblem(w, http.StatusConflict, "investment-invalid-reconciliation-link",
			"Vínculo inválido",
			"O lançamento informado não corresponde a esta movimentação: confira a moeda, a instituição, o sentido do valor e o tipo da operação.")
	default:
		writeProblem(w, http.StatusServiceUnavailable, "investment-unavailable",
			"Investimentos temporariamente indisponíveis", "Tente novamente em instantes.")
	}
}

func writeInvestmentInvalid(w http.ResponseWriter, detail string) {
	writeProblem(w, http.StatusBadRequest, "invalid-investment-input", "Dados de investimento inválidos", detail)
}

// ------------------------------------------------------------ query parsing

func investmentQueryFilter(r *http.Request, key string) *string {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil
	}
	return &value
}

func investmentQueryDate(r *http.Request, key string) (*time.Time, bool) {
	raw := investmentQueryFilter(r, key)
	if raw == nil {
		return nil, true
	}
	day, err := time.Parse(investmentDateLayout, *raw)
	if err != nil {
		return nil, false
	}
	return &day, true
}

func investmentBodyDate(raw *string) (time.Time, bool) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return time.Time{}, false
	}
	day, err := time.Parse(investmentDateLayout, strings.TrimSpace(*raw))
	if err != nil {
		return time.Time{}, false
	}
	return day, true
}

// ----------------------------------------------------------------- accounts

type investmentAccountCreateRequest struct {
	Name               string  `json:"name"`
	Kind               *string `json:"kind"`
	CurrencyCode       *string `json:"currency_code"`
	SourceID           *string `json:"source_id"`
	FinancialAccountID *string `json:"financial_account_id"`
}

type investmentAccountUpdateRequest struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
	// An absent financial_account_id keeps the current link, while an
	// explicit null clears it: the edit form saves a rename without sending
	// the link, and collapsing the two would unlink the bank account.
	FinancialAccountID investmentText `json:"financial_account_id"`
}

func handleListInvestmentAccounts(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accounts, err := investments.ListAccounts(r.Context(), conn)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentAccountDTO, 0, len(accounts))
		for _, account := range accounts {
			items = append(items, investmentAccountToDTO(account))
		}
		writeInvestmentItems(w, items)
	}
}

func handleCreateInvestmentAccount(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentAccountCreateRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		if req.Kind != nil && *req.Kind != string(investments.AccountKindManual) {
			writeInvestmentInvalid(w, "Só é possível criar contas de custódia manuais: as integradas vêm da conexão.")
			return
		}
		if req.SourceID != nil && *req.SourceID != "" {
			writeInvestmentInvalid(w, "A conta de uma conexão é criada automaticamente e não aceita source_id.")
			return
		}
		if req.CurrencyCode != nil && *req.CurrencyCode != "BRL" {
			writeInvestmentInvalid(w, "As contas de custódia manuais são em reais.")
			return
		}
		account, err := investments.CreateAccount(r.Context(), conn, investments.AccountInput{
			Name:               req.Name,
			FinancialAccountID: investmentOptionalID(req.FinancialAccountID),
		})
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentAccountToDTO(account))
	}
}

func handleUpdateInvestmentAccount(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req investmentAccountUpdateRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		current, err := investments.GetAccount(r.Context(), conn, id)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		name := current.Name
		if req.Name != nil {
			name = *req.Name
		}
		financialAccountID := req.FinancialAccountID.orCurrent(current.FinancialAccountID)
		account, err := investments.UpdateAccount(r.Context(), conn, id, investments.AccountInput{
			Name:               name,
			FinancialAccountID: financialAccountID,
			Active:             req.Active,
		})
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		writeJSON(w, http.StatusOK, investmentAccountToDTO(account))
	}
}

func investmentOptionalID(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func handleDeleteInvestmentAccount(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := investments.DeleteAccount(r.Context(), conn, r.PathValue("id")); err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --------------------------------------------------------------- portfolios

type investmentPortfolioRequest struct {
	Name         string            `json:"name"`
	TargetAmount investmentDecimal `json:"target_amount"`
	TargetDate   *string           `json:"target_date"`
	Notes        *string           `json:"notes"`
}

func (req investmentPortfolioRequest) toInput() (investments.PortfolioInput, bool) {
	input := investments.PortfolioInput{
		Name:         req.Name,
		TargetAmount: req.TargetAmount.pointer(),
		Notes:        req.Notes,
	}
	if req.TargetDate != nil && strings.TrimSpace(*req.TargetDate) != "" {
		day, ok := investmentBodyDate(req.TargetDate)
		if !ok {
			return investments.PortfolioInput{}, false
		}
		input.TargetDate = &day
	}
	return input, true
}

func handleListInvestmentPortfolios(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		portfolios, err := investments.ListPortfolios(r.Context(), conn)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentPortfolioDTO, 0, len(portfolios))
		for _, portfolio := range portfolios {
			items = append(items, investmentPortfolioToDTO(portfolio))
		}
		writeInvestmentItems(w, items)
	}
}

func handleCreateInvestmentPortfolio(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentPortfolioRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		input, ok := req.toInput()
		if !ok {
			writeInvestmentInvalid(w, "Informe a data alvo no formato AAAA-MM-DD.")
			return
		}
		portfolio, err := investments.CreatePortfolio(r.Context(), conn, input)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentPortfolioToDTO(portfolio))
	}
}

func handleUpdateInvestmentPortfolio(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentPortfolioRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		input, ok := req.toInput()
		if !ok {
			writeInvestmentInvalid(w, "Informe a data alvo no formato AAAA-MM-DD.")
			return
		}
		portfolio, err := investments.UpdatePortfolio(r.Context(), conn, r.PathValue("id"), input)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		writeJSON(w, http.StatusOK, investmentPortfolioToDTO(portfolio))
	}
}

func handleDeleteInvestmentPortfolio(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := investments.DeletePortfolio(r.Context(), conn, r.PathValue("id")); err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ------------------------------------------------------------------- assets

type investmentAssetRequest struct {
	Name         string                 `json:"name"`
	Ticker       *string                `json:"ticker"`
	AssetType    string                 `json:"asset_type"`
	AssetClass   investments.AssetClass `json:"asset_class"`
	CurrencyCode string                 `json:"currency_code"`
	QuoteSource  *string                `json:"quote_source"`
	QuoteSymbol  *string                `json:"quote_symbol"`
}

func (req investmentAssetRequest) toInput() investments.AssetInput {
	return investments.AssetInput{
		Name: req.Name, Ticker: req.Ticker, AssetType: req.AssetType, AssetClass: req.AssetClass, CurrencyCode: req.CurrencyCode,
		QuoteSource: req.QuoteSource, QuoteSymbol: req.QuoteSymbol,
	}
}

// quoteSourceKnown accepts an empty market or one market marketdata knows.
func quoteSourceKnown(source *string) bool {
	if source == nil || strings.TrimSpace(*source) == "" {
		return true
	}
	_, ok := marketdata.ParseMarket(*source)
	return ok
}

// normalizeQuoteConfig validates the quote configuration of an asset request
// and stores a provider-neutral symbol. An explicit symbol takes precedence
// over the ticker; both are canonicalized offline for the chosen market.
func normalizeQuoteConfig(req *investmentAssetRequest) (string, bool) {
	if req.QuoteSource == nil || strings.TrimSpace(*req.QuoteSource) == "" {
		return "", true
	}
	market, _ := marketdata.ParseMarket(*req.QuoteSource)
	source := string(market)
	req.QuoteSource = &source

	symbol := ""
	if req.QuoteSymbol != nil {
		symbol = strings.TrimSpace(*req.QuoteSymbol)
	}
	if symbol == "" {
		if req.Ticker == nil || strings.TrimSpace(*req.Ticker) == "" {
			return "Informe o ticker do ativo para buscar a cotação.", false
		}
		symbol = *req.Ticker
	}
	instrument, err := marketdata.ParseInstrument(market, symbol)
	if err != nil {
		return quoteConfigMessage(err), false
	}
	req.QuoteSymbol = &instrument.Symbol
	return "", true
}

func quoteConfigMessage(err error) string {
	var symbolErr *marketdata.SymbolError
	if errors.As(err, &symbolErr) {
		return "Ticker " + strconv.Quote(symbolErr.Input) + " não serve para o mercado de cotação escolhido: " + symbolErr.Reason + "."
	}
	return "Mercado de cotação inválido."
}

func handleListInvestmentAssets(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assets, err := investments.ListAssets(r.Context(), conn)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentAssetDTO, 0, len(assets))
		for _, asset := range assets {
			items = append(items, investmentAssetToDTO(asset))
		}
		writeInvestmentItems(w, items)
	}
}

func handleInvestmentAssetClassification(w http.ResponseWriter, r *http.Request) {
	writeInvestmentItems(w, investments.AssetClassification())
}

func handleCreateInvestmentAsset(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentAssetRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		if !quoteSourceKnown(req.QuoteSource) {
			writeInvestmentInvalid(w, "Mercado de cotação desconhecido.")
			return
		}
		if message, ok := normalizeQuoteConfig(&req); !ok {
			writeInvestmentInvalid(w, message)
			return
		}
		asset, err := investments.CreateAsset(r.Context(), conn, req.toInput())
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusCreated, investmentAssetToDTO(asset))
	}
}

func handleUpdateInvestmentAsset(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentAssetRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		if !quoteSourceKnown(req.QuoteSource) {
			writeInvestmentInvalid(w, "Mercado de cotação desconhecido.")
			return
		}
		if message, ok := normalizeQuoteConfig(&req); !ok {
			writeInvestmentInvalid(w, message)
			return
		}
		asset, err := investments.UpdateAsset(r.Context(), conn, r.PathValue("id"), req.toInput())
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusOK, investmentAssetToDTO(asset))
	}
}

func handleDeleteInvestmentAsset(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := investments.DeleteAsset(r.Context(), conn, r.PathValue("id")); err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---------------------------------------------------------------- positions

type investmentPositionCreateRequest struct {
	AccountID       string            `json:"account_id"`
	AssetID         *string           `json:"asset_id"`
	Name            string            `json:"name"`
	Ticker          *string           `json:"ticker"`
	AssetType       *string           `json:"asset_type"`
	PortfolioID     *string           `json:"portfolio_id"`
	InitialQuantity investmentDecimal `json:"initial_quantity"`
	InitialUnitCost investmentDecimal `json:"initial_unit_cost"`
	InitialValue    investmentDecimal `json:"initial_value"`
	OccurredOn      *string           `json:"occurred_on"`
	Notes           *string           `json:"notes"`
}

type investmentPositionUpdateRequest struct {
	Name        *string        `json:"name"`
	Ticker      investmentText `json:"ticker"`
	AssetType   *string        `json:"asset_type"`
	PortfolioID investmentText `json:"portfolio_id"`
	Notes       investmentText `json:"notes"`
}

const defaultInvestmentAssetType = "Ativo"

func handleListInvestmentPositions(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := investments.PositionFilter{
			AccountID:     investmentQueryFilter(r, "account_id"),
			PortfolioID:   investmentQueryFilter(r, "portfolio_id"),
			IncludeClosed: r.URL.Query().Get("include_closed") == "true",
			Source:        investmentQueryFilter(r, "source"),
			SourceID:      investmentQueryFilter(r, "source_id"),
		}
		positions, err := investments.ListPositions(r.Context(), conn, filter)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentPositionDTO, 0, len(positions))
		for _, position := range positions {
			items = append(items, investmentPositionToDTO(position))
		}
		writeInvestmentItems(w, items)
	}
}

func handleGetInvestmentPosition(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		position, err := investments.GetPosition(r.Context(), conn, r.PathValue("id"))
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		writeJSON(w, http.StatusOK, investmentPositionToDTO(position))
	}
}

func handleCreateInvestmentPosition(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentPositionCreateRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		input := investments.PositionInput{
			AccountID:       req.AccountID,
			AssetID:         investmentOptionalID(req.AssetID),
			Name:            req.Name,
			Ticker:          req.Ticker,
			AssetType:       defaultInvestmentAssetType,
			PortfolioID:     investmentOptionalID(req.PortfolioID),
			InitialQuantity: req.InitialQuantity.orZero(),
			InitialUnitCost: req.InitialUnitCost.orZero(),
			InitialValue:    req.InitialValue.pointer(),
			OccurredOn:      time.Now(),
			Notes:           req.Notes,
		}
		if req.AssetType != nil && strings.TrimSpace(*req.AssetType) != "" {
			input.AssetType = strings.TrimSpace(*req.AssetType)
		}
		if req.OccurredOn != nil && strings.TrimSpace(*req.OccurredOn) != "" {
			day, ok := investmentBodyDate(req.OccurredOn)
			if !ok {
				writeInvestmentInvalid(w, "Informe a data no formato AAAA-MM-DD.")
				return
			}
			input.OccurredOn = day
		}
		position, err := investments.CreatePosition(r.Context(), conn, input)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusCreated, investmentPositionToDTO(position))
	}
}

// handleUpdateInvestmentPosition fills every omitted field from the holding as
// it stands. That is what lets a provider-synced position accept a goal
// change: the description it sends back is the provider's own, so the domain
// sees no attempt to rewrite it.
func handleUpdateInvestmentPosition(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req investmentPositionUpdateRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		current, err := investments.GetPosition(r.Context(), conn, id)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		update := investments.PositionUpdate{
			Name:        current.Name,
			Ticker:      req.Ticker.orCurrent(current.Ticker),
			AssetType:   current.AssetType,
			PortfolioID: req.PortfolioID.orCurrent(current.PortfolioID),
			Notes:       req.Notes.orCurrent(current.Notes),
		}
		if req.Name != nil {
			update.Name = *req.Name
		}
		if req.AssetType != nil {
			update.AssetType = *req.AssetType
		}
		position, err := investments.UpdatePosition(r.Context(), conn, id, update)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusOK, investmentPositionToDTO(position))
	}
}

func handleDeleteInvestmentPosition(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := investments.DeletePosition(r.Context(), conn, r.PathValue("id")); err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --------------------------------------------------------------- operations

type investmentOperationRequest struct {
	AccountID  string            `json:"account_id"`
	PositionID *string           `json:"position_id"`
	Kind       string            `json:"kind"`
	OccurredOn *string           `json:"occurred_on"`
	Amount     investmentDecimal `json:"amount"`
	Quantity   investmentDecimal `json:"quantity"`
	UnitPrice  investmentDecimal `json:"unit_price"`
	Fees       investmentDecimal `json:"fees"`
	Taxes      investmentDecimal `json:"taxes"`
	Notes      *string           `json:"notes"`
}

func (req investmentOperationRequest) toInput() (investments.OperationInput, bool) {
	day, ok := investmentBodyDate(req.OccurredOn)
	if !ok {
		return investments.OperationInput{}, false
	}
	amount := req.Amount.orZero()
	if !req.Amount.present && req.Quantity.present && req.UnitPrice.present {
		amount = req.Quantity.value.Mul(req.UnitPrice.value)
	}
	return investments.OperationInput{
		AccountID:  req.AccountID,
		PositionID: investmentOptionalID(req.PositionID),
		Kind:       investments.OperationKind(req.Kind),
		OccurredOn: day,
		Amount:     amount,
		Quantity:   req.Quantity.pointer(),
		UnitPrice:  req.UnitPrice.pointer(),
		Fees:       req.Fees.orZero(),
		Taxes:      req.Taxes.orZero(),
		Notes:      req.Notes,
	}, true
}

func handleListInvestmentOperations(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, ok := investmentQueryDate(r, "from")
		if !ok {
			writeInvestmentInvalid(w, "Informe from no formato AAAA-MM-DD.")
			return
		}
		to, ok := investmentQueryDate(r, "to")
		if !ok {
			writeInvestmentInvalid(w, "Informe to no formato AAAA-MM-DD.")
			return
		}
		operations, err := investments.ListOperations(r.Context(), conn, investments.OperationFilter{
			AccountID:           investmentQueryFilter(r, "account_id"),
			PositionID:          investmentQueryFilter(r, "position_id"),
			Source:              investmentQueryFilter(r, "source"),
			SourceID:            investmentQueryFilter(r, "source_id"),
			PortfolioID:         investmentQueryFilter(r, "portfolio_id"),
			ReconciliationState: investmentQueryFilter(r, "reconciliation_state"),
			From:                from,
			To:                  to,
		})
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentOperationDTO, 0, len(operations))
		for _, operation := range operations {
			items = append(items, investmentOperationToDTO(operation))
		}
		writeInvestmentItems(w, items)
	}
}

func handleCreateInvestmentOperation(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentOperationRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		input, ok := req.toInput()
		if !ok {
			writeInvestmentInvalid(w, "Informe a data da movimentação no formato AAAA-MM-DD.")
			return
		}
		operation, err := investments.CreateOperation(r.Context(), conn, input)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusCreated, investmentOperationToDTO(operation))
	}
}

type investmentTransferRequest struct {
	SourcePositionID      string            `json:"source_position_id"`
	DestinationPositionID string            `json:"destination_position_id"`
	Quantity              investmentDecimal `json:"quantity"`
	OccurredOn            *string           `json:"occurred_on"`
	Notes                 *string           `json:"notes"`
}

func handleCreateInvestmentTransfer(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentTransferRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		day, ok := investmentBodyDate(req.OccurredOn)
		if !ok {
			writeInvestmentInvalid(w, "Informe a data no formato AAAA-MM-DD.")
			return
		}
		operations, err := investments.CreateTransfer(r.Context(), conn, investments.TransferInput{SourcePositionID: req.SourcePositionID, DestinationPositionID: req.DestinationPositionID, Quantity: req.Quantity.orZero(), OccurredOn: day, Notes: req.Notes})
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentOperationDTO, 0, len(operations))
		for _, operation := range operations {
			items = append(items, investmentOperationToDTO(operation))
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusCreated, map[string]any{"items": items})
	}
}

func handleDeleteInvestmentTransfer(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := investments.DeleteTransfer(r.Context(), conn, r.PathValue("id")); err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleUpdateInvestmentOperation(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentOperationRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		input, ok := req.toInput()
		if !ok {
			writeInvestmentInvalid(w, "Informe a data da movimentação no formato AAAA-MM-DD.")
			return
		}
		operation, err := investments.UpdateOperation(r.Context(), conn, r.PathValue("id"), input)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusOK, investmentOperationToDTO(operation))
	}
}

func handleDeleteInvestmentOperation(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := investments.DeleteOperation(r.Context(), conn, r.PathValue("id")); err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---------------------------------------------------------- reconciliations

type investmentReconciliationRequest struct {
	// Absent when the provider movement itself is the destination.
	OperationID                      *string           `json:"operation_id"`
	FinancialTransactionID           *string           `json:"financial_transaction_id"`
	FinancialInvestmentTransactionID *string           `json:"financial_investment_transaction_id"`
	Amount                           investmentDecimal `json:"amount"`
}

func handleListInvestmentReconciliations(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links, err := investments.ListReconciliations(r.Context(), conn, investments.ReconciliationFilter{
			OperationID:            investmentQueryFilter(r, "operation_id"),
			FinancialTransactionID: investmentQueryFilter(r, "financial_transaction_id"),
		})
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentReconciliationDTO, 0, len(links))
		for _, link := range links {
			items = append(items, investmentReconciliationToDTO(link))
		}
		writeInvestmentItems(w, items)
	}
}

// handleCreateInvestmentReconciliation accepts a body carrying the bank line,
// the imported provider movement, or both at once. Both in one row is the
// right shape when the two describe the same full-amount operation: the cap
// counts every link of an operation together, so two full-amount rows would
// be refused as over-allocation.
func handleCreateInvestmentReconciliation(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentReconciliationRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		operationID := ""
		if id := investmentOptionalID(req.OperationID); id != nil {
			operationID = *id
		}
		link, err := investments.CreateReconciliation(r.Context(), conn, investments.ReconciliationInput{
			OperationID:                      operationID,
			FinancialTransactionID:           investmentOptionalID(req.FinancialTransactionID),
			FinancialInvestmentTransactionID: investmentOptionalID(req.FinancialInvestmentTransactionID),
			Amount:                           req.Amount.orZero(),
		})
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentReconciliationToDTO(link))
	}
}

func handleDeleteInvestmentReconciliation(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := investments.DeleteReconciliation(r.Context(), conn, r.PathValue("id")); err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ------------------------------------------------------------------ summary

func handleGetInvestmentSummary(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := investments.BuildSummary(r.Context(), conn)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		writeJSON(w, http.StatusOK, investmentSummaryToDTO(summary))
	}
}

type investmentMonthlyMovementDTO struct {
	Month         string `json:"month"`
	Contributions string `json:"contributions"`
	Withdrawals   string `json:"withdrawals"`
	Income        string `json:"income"`
	Fees          string `json:"fees"`
	Taxes         string `json:"taxes"`
}

func handleListInvestmentMonthlyMovements(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		movements, err := investments.MonthlyMovements(r.Context(), conn, investmentQueryFilter(r, "account_id"))
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentMonthlyMovementDTO, 0, len(movements))
		for _, m := range movements {
			items = append(items, investmentMonthlyMovementDTO{
				Month:         m.Month,
				Contributions: money.CanonicalDecimal(m.Contributions),
				Withdrawals:   money.CanonicalDecimal(m.Withdrawals),
				Income:        money.CanonicalDecimal(m.Income),
				Fees:          money.CanonicalDecimal(m.Fees),
				Taxes:         money.CanonicalDecimal(m.Taxes),
			})
		}
		writeInvestmentItems(w, items)
	}
}

// A compound action is committed together with its optional bank reconciliation.
func handleCreateInvestmentOperations(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Operations     []investmentOperationRequest     `json:"operations"`
			Reconciliation *investmentReconciliationRequest `json:"reconciliation"`
		}
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		inputs := make([]investments.OperationInput, 0, len(req.Operations))
		for _, operation := range req.Operations {
			input, ok := operation.toInput()
			if !ok {
				writeInvestmentInvalid(w, "Informe a data no formato AAAA-MM-DD.")
				return
			}
			inputs = append(inputs, input)
		}
		var link *investments.ReconciliationInput
		if req.Reconciliation != nil {
			link = &investments.ReconciliationInput{
				FinancialTransactionID:           investmentOptionalID(req.Reconciliation.FinancialTransactionID),
				FinancialInvestmentTransactionID: investmentOptionalID(req.Reconciliation.FinancialInvestmentTransactionID),
				Amount:                           req.Reconciliation.Amount.orZero(),
			}
		}
		operations, err := investments.CreateOperations(r.Context(), conn, inputs, link)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		items := make([]investmentOperationDTO, 0, len(operations))
		for _, op := range operations {
			items = append(items, investmentOperationToDTO(op))
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusCreated, map[string]any{"items": items})
	}
}
