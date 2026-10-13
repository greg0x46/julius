package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
)

// --- request DTOs, matching frontend/src/api/contracts.ts's TransactionQuery ---

type transactionFiltersRequest struct {
	Origin         *string `json:"origin"`
	SourceProvider *string `json:"source_provider"`
	CardBalance    *bool   `json:"card_balance"`
	CreditCard     *bool   `json:"credit_card"`
	DateFrom       *string `json:"date_from"`
	DateTo         *string `json:"date_to"`
	Description    *string `json:"description"`
	// The singular fields are the original contract and still work; the
	// plural ones are "any of" sets (see transactions.Filters). Both spellings
	// of one field are merged, so a client sending only account_id gets the
	// same result as before.
	AccountID        *string  `json:"account_id"`
	AccountIDs       []string `json:"account_ids"`
	Institution      *string  `json:"institution"`
	Institutions     []string `json:"institutions"`
	CategoryID       *string  `json:"category_id"`
	CategoryIDs      []string `json:"category_ids"`
	Classification   *string  `json:"classification"`
	ProviderStatus   *string  `json:"provider_status"`
	ProviderStatuses []string `json:"provider_statuses"`
	AmountMin        *string  `json:"amount_min"`
	AmountMax        *string  `json:"amount_max"`
	Uncategorized    *bool    `json:"uncategorized"`
}

// anyOfSet merges a legacy single value with its plural form into one
// de-duplicated set, dropping blanks so `""` never filters everything out.
func anyOfSet(single *string, many []string) []string {
	var set []string
	seen := make(map[string]bool)
	add := func(v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		set = append(set, v)
	}
	if single != nil {
		add(*single)
	}
	for _, v := range many {
		add(v)
	}
	return set
}

type transactionQueryRequest struct {
	Timezone string                    `json:"timezone"`
	GroupBy  string                    `json:"group_by"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Filters  transactionFiltersRequest `json:"filters"`
}

func parseDateOnly(s string) (money.Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return money.Date{}, err
	}
	y, m, d := t.Date()
	return money.Date{Year: y, Month: m, Day: d}, nil
}

// toFilters translates the wire request into transactions.Filters, reporting
// a validation problem (mirroring transaction_routes.py's checks) instead of
// running the query when the input is malformed.
func toFilters(req transactionFiltersRequest) (transactions.Filters, *Problem) {
	invalid := func(kind, title, detail string) *Problem {
		return &Problem{Type: "/problems/" + kind, Title: title, Status: 400, Detail: detail}
	}

	var f transactions.Filters
	if req.Origin != nil && *req.Origin != "manual" && *req.Origin != "synced" {
		return f, invalid("invalid-origin", "Origem inválida", "Use manual ou synced.")
	}
	f.Origin = req.Origin
	if req.SourceProvider != nil && *req.SourceProvider != "file" && *req.SourceProvider != "pluggy" {
		return f, invalid("invalid-source-provider", "Origem inválida", "Use file ou pluggy.")
	}
	f.SourceProvider = req.SourceProvider
	if req.CardBalance != nil {
		f.CardBalance = *req.CardBalance
	}
	if req.CreditCard != nil {
		f.CreditCard = *req.CreditCard
	}
	if (req.DateFrom == nil) != (req.DateTo == nil) {
		return f, invalid("invalid-date-range", "Intervalo de datas inválido",
			"Informe as datas inicial e final juntas, ou remova ambas.")
	}
	if req.DateFrom != nil && req.DateTo != nil {
		from, err1 := parseDateOnly(*req.DateFrom)
		to, err2 := parseDateOnly(*req.DateTo)
		if err1 != nil || err2 != nil {
			return f, invalid("invalid-date-range", "Intervalo de datas inválido", "Datas em formato inválido.")
		}
		if from.String() > to.String() {
			return f, invalid("invalid-date-range", "Intervalo de datas inválido",
				"A data inicial não pode ser posterior à data final.")
		}
		f.DateFrom, f.DateTo = &from, &to
	}
	f.Description = req.Description
	f.AccountIDs = anyOfSet(req.AccountID, req.AccountIDs)
	f.Institutions = anyOfSet(req.Institution, req.Institutions)
	f.CategoryIDs = anyOfSet(req.CategoryID, req.CategoryIDs)
	if req.Classification != nil {
		switch c := money.Classification(*req.Classification); c {
		case money.Inflow, money.Outflow, money.Unclassified:
			f.Classification = &c
		default:
			return f, invalid("invalid-classification", "Classificação inválida", "Use inflow, outflow ou unclassified.")
		}
	}
	f.ProviderStatuses = anyOfSet(req.ProviderStatus, req.ProviderStatuses)
	if req.AmountMin != nil {
		v, err := decimal.NewFromString(*req.AmountMin)
		if err != nil {
			return f, invalid("invalid-amount-range", "Faixa de valor inválida", "Valor mínimo inválido.")
		}
		f.AmountMin = &v
	}
	if req.AmountMax != nil {
		v, err := decimal.NewFromString(*req.AmountMax)
		if err != nil {
			return f, invalid("invalid-amount-range", "Faixa de valor inválida", "Valor máximo inválido.")
		}
		f.AmountMax = &v
	}
	if f.AmountMin != nil && f.AmountMax != nil && f.AmountMin.GreaterThan(*f.AmountMax) {
		return f, invalid("invalid-amount-range", "Faixa de valor inválida",
			"O valor mínimo não pode ser maior que o valor máximo.")
	}
	if req.Uncategorized != nil {
		f.Uncategorized = *req.Uncategorized
	}
	return f, nil
}

// --- response DTOs ---

type accountSummaryDTO struct {
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	Institution  *string `json:"institution"`
	CurrencyCode *string `json:"currency_code"`
}

type effectiveMoneyDTO struct {
	Value        string `json:"value"`
	CurrencyCode string `json:"currency_code"`
	Source       string `json:"source"`
}

type inclusionDTO struct {
	State     string     `json:"state"`
	ChangedAt *time.Time `json:"changed_at"`
	Origin    string     `json:"origin"`
	RuleName  *string    `json:"rule_name"`
}

type internalCategoryDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	IsActive  bool      `json:"is_active"`
	Icon      string    `json:"icon"`
	Color     string    `json:"color"`
	Origin    string    `json:"origin"`
	ChangedAt time.Time `json:"changed_at"`
}

type totalsEligibilityDTO struct {
	Included bool    `json:"included"`
	Reason   *string `json:"reason"`
}

type cardDTO struct {
	Number            string `json:"number"`
	InstallmentNumber *int   `json:"installment_number"`
	TotalInstallments *int   `json:"total_installments"`
}

type transactionItemDTO struct {
	ID                       string               `json:"id"`
	ExternalID               string               `json:"external_id"`
	Origin                   string               `json:"origin"`
	SourceProvider           *string              `json:"source_provider"`
	OccurredAt               *time.Time           `json:"occurred_at"`
	Description              *string              `json:"description"`
	Account                  accountSummaryDTO    `json:"account"`
	SourceCategory           *string              `json:"source_category"`
	InternalCategory         *internalCategoryDTO `json:"internal_category"`
	MovementType             *string              `json:"movement_type"`
	ProviderStatus           *string              `json:"provider_status"`
	Classification           string               `json:"classification"`
	Amount                   *string              `json:"amount"`
	CurrencyCode             *string              `json:"currency_code"`
	AmountInAccountCurrency  *string              `json:"amount_in_account_currency"`
	EffectiveMoney           *effectiveMoneyDTO   `json:"effective_money"`
	InvestmentTransferAmount string               `json:"investment_transfer_amount"`
	ReportableAmount         *string              `json:"reportable_amount"`
	Card                     *cardDTO             `json:"card"`
	Inclusion                inclusionDTO         `json:"inclusion"`
	TotalsEligibility        totalsEligibilityDTO `json:"totals_eligibility"`
	GroupKey                 string               `json:"group_key"`
}

func toItemDTO(item transactions.Item) transactionItemDTO {
	dto := transactionItemDTO{
		ID:             item.ID,
		ExternalID:     item.ExternalID,
		Origin:         item.Origin,
		SourceProvider: item.SourceProvider,
		OccurredAt:     item.OccurredAt,
		Description:    item.Description,
		SourceCategory: item.SourceCategory,
		MovementType:   item.MovementType,
		ProviderStatus: item.ProviderStatus,
		Classification: string(item.Classification),
		Amount:         item.Amount,
		CurrencyCode:   item.CurrencyCode,
		Account: accountSummaryDTO{
			ID:           item.Account.ID,
			Name:         item.Account.Name,
			Institution:  item.Account.Institution,
			CurrencyCode: item.Account.CurrencyCode,
		},
		AmountInAccountCurrency:  item.AmountInAccountCurrency,
		InvestmentTransferAmount: item.InvestmentTransferAmount,
		ReportableAmount:         item.ReportableAmount,
		Inclusion: inclusionDTO{
			State:     string(item.Inclusion.State),
			ChangedAt: item.Inclusion.ChangedAt,
			Origin:    item.Inclusion.Origin,
			RuleName:  item.Inclusion.RuleName,
		},
		TotalsEligibility: totalsEligibilityDTO{Included: item.TotalsEligibility.Included},
		GroupKey:          item.GroupKey,
	}
	if item.TotalsEligibility.Reason != nil {
		r := string(*item.TotalsEligibility.Reason)
		dto.TotalsEligibility.Reason = &r
	}
	if item.EffectiveMoney != nil {
		dto.EffectiveMoney = &effectiveMoneyDTO{
			Value:        item.EffectiveMoney.Value,
			CurrencyCode: item.EffectiveMoney.CurrencyCode,
			Source:       string(item.EffectiveMoney.Source),
		}
	}
	if item.Card != nil {
		dto.Card = &cardDTO{
			Number:            item.Card.Number,
			InstallmentNumber: item.Card.InstallmentNumber,
			TotalInstallments: item.Card.TotalInstallments,
		}
	}
	if item.InternalCategory != nil {
		dto.InternalCategory = &internalCategoryDTO{
			ID:        item.InternalCategory.ID,
			Name:      item.InternalCategory.Name,
			Kind:      string(item.InternalCategory.Kind),
			IsActive:  item.InternalCategory.IsActive,
			Icon:      item.InternalCategory.Icon,
			Color:     item.InternalCategory.Color,
			Origin:    item.InternalCategory.Origin,
			ChangedAt: item.InternalCategory.ChangedAt,
		}
	}
	return dto
}

type currencyTotalsDTO struct {
	CurrencyCode string `json:"currency_code"`
	Inflow       string `json:"inflow"`
	Outflow      string `json:"outflow"`
	Balance      string `json:"balance"`
}

func toCurrencyTotalsDTO(t transactions.CurrencyTotals) currencyTotalsDTO {
	return currencyTotalsDTO{CurrencyCode: t.CurrencyCode, Inflow: t.Inflow, Outflow: t.Outflow, Balance: t.Balance}
}

type groupDTO struct {
	Key            string              `json:"key"`
	Kind           string              `json:"kind"`
	StartDate      *string             `json:"start_date"`
	EndDate        *string             `json:"end_date"`
	ItemCount      int                 `json:"item_count"`
	PageItemCount  int                 `json:"page_item_count"`
	HasItemsBefore bool                `json:"has_items_before"`
	HasItemsAfter  bool                `json:"has_items_after"`
	Totals         []currencyTotalsDTO `json:"totals"`
}

func dateStringPtr(d *money.Date) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

func toGroupDTO(g transactions.Group) groupDTO {
	totals := make([]currencyTotalsDTO, len(g.Totals))
	for i, t := range g.Totals {
		totals[i] = toCurrencyTotalsDTO(t)
	}
	return groupDTO{
		Key:            g.Key,
		Kind:           string(g.Kind),
		StartDate:      dateStringPtr(g.StartDate),
		EndDate:        dateStringPtr(g.EndDate),
		ItemCount:      g.ItemCount,
		PageItemCount:  g.PageItemCount,
		HasItemsBefore: g.HasItemsBefore,
		HasItemsAfter:  g.HasItemsAfter,
		Totals:         totals,
	}
}

type pageDTO struct {
	Number     int `json:"number"`
	Size       int `json:"size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

type accountOptionDTO struct {
	ID          string  `json:"id"`
	Name        *string `json:"name"`
	Institution *string `json:"institution"`
}

type categoryOptionDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	IsActive bool   `json:"is_active"`
	Icon     string `json:"icon"`
	Color    string `json:"color"`
}

type availableFiltersDTO struct {
	Accounts     []accountOptionDTO  `json:"accounts"`
	Institutions []string            `json:"institutions"`
	Categories   []categoryOptionDTO `json:"categories"`
}

type transactionQueryResultDTO struct {
	ConfirmedAt      time.Time            `json:"confirmed_at"`
	StoredTotal      int                  `json:"stored_total"`
	Page             pageDTO              `json:"page"`
	Items            []transactionItemDTO `json:"items"`
	Totals           []currencyTotalsDTO  `json:"totals"`
	Groups           []groupDTO           `json:"groups"`
	AvailableFilters availableFiltersDTO  `json:"available_filters"`
}

func toResultDTO(result transactions.Result) transactionQueryResultDTO {
	items := make([]transactionItemDTO, len(result.Items))
	for i, item := range result.Items {
		items[i] = toItemDTO(item)
	}
	totals := make([]currencyTotalsDTO, len(result.Totals))
	for i, t := range result.Totals {
		totals[i] = toCurrencyTotalsDTO(t)
	}
	groups := make([]groupDTO, len(result.Groups))
	for i, g := range result.Groups {
		groups[i] = toGroupDTO(g)
	}
	accounts := make([]accountOptionDTO, len(result.AvailableFilters.Accounts))
	for i, a := range result.AvailableFilters.Accounts {
		accounts[i] = accountOptionDTO{ID: a.ID, Name: a.Name, Institution: a.Institution}
	}
	categoryOptions := make([]categoryOptionDTO, len(result.AvailableFilters.Categories))
	for i, c := range result.AvailableFilters.Categories {
		categoryOptions[i] = categoryOptionDTO{ID: c.ID, Name: c.Name, Kind: string(c.Kind), IsActive: c.IsActive, Icon: c.Icon, Color: c.Color}
	}
	return transactionQueryResultDTO{
		ConfirmedAt: result.ConfirmedAt,
		StoredTotal: result.StoredTotal,
		Page: pageDTO{
			Number: result.Page.Number, Size: result.Page.Size,
			TotalItems: result.Page.TotalItems, TotalPages: result.Page.TotalPages,
		},
		Items:  items,
		Totals: totals,
		Groups: groups,
		AvailableFilters: availableFiltersDTO{
			Accounts:     accounts,
			Institutions: result.AvailableFilters.Institutions,
			Categories:   categoryOptions,
		},
	}
}

func handleQueryTransactions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req transactionQueryRequest
		if err := decodeStrict(r, &req); err != nil {
			writeProblem(w, 422, "invalid-transaction-query", "Consulta de transações inválida", "Revise os campos enviados.")
			return
		}
		filters, problem := toFilters(req.Filters)
		if problem != nil {
			writeProblem(w, problem.Status, problem.Type[len("/problems/"):], problem.Title, problem.Detail)
			return
		}
		if _, err := time.LoadLocation(req.Timezone); err != nil || req.Timezone == "" {
			writeProblem(w, 400, "invalid-timezone", "Fuso horário inválido", "Informe um nome de fuso IANA reconhecido.")
			return
		}

		result, err := transactions.Query(r.Context(), db, transactions.QueryRequest{
			Timezone: req.Timezone,
			GroupBy:  money.GroupBy(req.GroupBy),
			Page:     req.Page,
			PageSize: req.PageSize,
			Filters:  filters,
		})
		if err != nil {
			writeProblem(w, 503, "transaction-query-unavailable", "Consulta temporariamente indisponível", "Tente novamente em instantes.")
			return
		}
		writeJSON(w, http.StatusOK, toResultDTO(result))
	}
}

type categorySpendingItemDTO struct {
	CategoryID    *string `json:"category_id"`
	CategoryName  string  `json:"category_name"`
	CategoryIcon  string  `json:"category_icon"`
	CategoryColor string  `json:"category_color"`
	Amount        string  `json:"amount"`
	Source        string  `json:"source"`
}

type spendingByCategoryDTO struct {
	FX           *exchange.Rate            `json:"fx,omitempty"`
	Month        string                    `json:"month"`
	CurrencyCode string                    `json:"currency_code"`
	Total        string                    `json:"total"`
	Items        []categorySpendingItemDTO `json:"items"`
}

func spendingByCategoryUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "spending-by-category-unavailable", "Gastos por categoria temporariamente indisponíveis", "Tente novamente em instantes.")
}

// dateWithinBounds reports whether t's calendar date (year/month/day, no
// time-of-day/timezone — matching how scenario_transactions.projected_at is
// stored) falls within [from, to] inclusive.
func dateWithinBounds(t time.Time, from, to money.Date) bool {
	d := money.Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
	notBefore := d.Year > from.Year ||
		(d.Year == from.Year && (d.Month > from.Month || (d.Month == from.Month && d.Day >= from.Day)))
	notAfter := d.Year < to.Year ||
		(d.Year == to.Year && (d.Month < to.Month || (d.Month == to.Month && d.Day <= to.Day)))
	return notBefore && notAfter
}

// projectedSpendingByCategory mirrors task 8's opt-in union: every
// scenario_transaction of scenarioID whose projected_at falls in [from, to],
// grouped by its free-text category (nil/empty groups as "Sem categoria",
// same convention as the real side) — never merged into a real category's
// total, always its own row tagged source="projetado", so the response can
// never conflate a hypothetical amount with a real one.
func projectedSpendingByCategory(ctx context.Context, db *sql.DB, scenarioID string, from, to money.Date) ([]categorySpendingItemDTO, error) {
	list, err := scenarios.ListScenarioTransactions(ctx, db, scenarioID)
	if err != nil {
		return nil, err
	}
	type accum struct {
		name   string
		amount decimal.Decimal
	}
	byCategory := make(map[string]*accum)
	var order []string
	for _, st := range list {
		if !dateWithinBounds(st.ProjectedAt, from, to) {
			continue
		}
		key, name := "", "Sem categoria"
		if st.Category != nil && *st.Category != "" {
			key, name = *st.Category, *st.Category
		}
		a, ok := byCategory[key]
		if !ok {
			a = &accum{name: name, amount: decimal.Zero}
			byCategory[key] = a
			order = append(order, key)
		}
		a.amount = a.amount.Add(st.Amount)
	}
	items := make([]categorySpendingItemDTO, len(order))
	for i, key := range order {
		a := byCategory[key]
		items[i] = categorySpendingItemDTO{CategoryName: a.name, Amount: money.CanonicalDecimal(a.amount), Source: "projetado"}
	}
	return items, nil
}

// handleSpendingByCategory sums the requesting client's current calendar
// month (in its own timezone, since "this month" depends on it) of outflow
// transactions grouped by internal category, powering the dashboard's
// spending widget. An optional scenario_id query param (task 8) unions in
// that scenario's projected installments due in the same period as
// separate, explicitly-tagged rows — never included unless asked for, and
// never merged into the real totals.
func handleSpendingByCategory(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		timezone := r.URL.Query().Get("timezone")
		loc, err := time.LoadLocation(timezone)
		if timezone == "" || err != nil {
			writeProblem(w, 400, "invalid-timezone", "Fuso horário inválido", "Informe um nome de fuso IANA reconhecido.")
			return
		}

		now := time.Now().In(loc)
		year, month, _ := now.Date()
		from := money.Date{Year: year, Month: month, Day: 1}
		lastDay := time.Date(year, month, 1, 0, 0, 0, 0, loc).AddDate(0, 1, -1)
		to := money.Date{Year: lastDay.Year(), Month: lastDay.Month(), Day: lastDay.Day()}

		items, err := transactions.SpendingByCategory(r.Context(), db, transactions.Filters{
			DateFrom: &from, DateTo: &to,
		}, timezone)
		if err != nil {
			spendingByCategoryUnavailableProblem(w)
			return
		}

		dtoItems := make([]categorySpendingItemDTO, len(items))
		for i, it := range items {
			dtoItems[i] = categorySpendingItemDTO{CategoryID: it.CategoryID, CategoryName: it.CategoryName, CategoryIcon: it.CategoryIcon, CategoryColor: it.CategoryColor, Amount: it.Amount, Source: "real"}
		}

		if scenarioID := r.URL.Query().Get("scenario_id"); scenarioID != "" {
			if _, err := scenarios.GetScenario(r.Context(), db, scenarioID); errors.Is(err, scenarios.ErrScenarioNotFound) {
				scenarioNotFoundProblem(w)
				return
			} else if err != nil {
				spendingByCategoryUnavailableProblem(w)
				return
			}
			projected, err := projectedSpendingByCategory(r.Context(), db, scenarioID, from, to)
			if err != nil {
				spendingByCategoryUnavailableProblem(w)
				return
			}
			dtoItems = append(dtoItems, projected...)
		}

		total := decimal.Zero
		for _, it := range dtoItems {
			amount, err := decimal.NewFromString(it.Amount)
			if err != nil {
				spendingByCategoryUnavailableProblem(w)
				return
			}
			total = total.Add(amount)
		}
		sort.SliceStable(dtoItems, func(i, j int) bool {
			ai, _ := decimal.NewFromString(dtoItems[i].Amount)
			aj, _ := decimal.NewFromString(dtoItems[j].Amount)
			return ai.GreaterThan(aj)
		})

		valuation, ok := reportValuation(w, r, db, time.Now().UTC())
		if !ok {
			return
		}
		for i := range dtoItems {
			valuation.fields(&dtoItems[i].Amount)
		}
		total = valuation.amount(total)
		writeJSON(w, http.StatusOK, spendingByCategoryDTO{
			FX:           &valuation.FX,
			Month:        fmt.Sprintf("%04d-%02d", year, int(month)),
			CurrencyCode: valuation.CurrencyCode,
			Total:        money.CanonicalDecimal(total),
			Items:        dtoItems,
		})
	}
}

type transactionInclusionRequest struct {
	State string `json:"state"`
}

type transactionInclusionResultDTO struct {
	TransactionID string     `json:"transaction_id"`
	State         string     `json:"state"`
	ChangedAt     *time.Time `json:"changed_at"`
}

func handleSetTransactionInclusion(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req transactionInclusionRequest
		if err := decodeStrict(r, &req); err != nil || (req.State != string(money.Considered) && req.State != string(money.Ignored)) {
			writeProblem(w, 422, "invalid-transaction-inclusion", "Decisão de inclusão inválida", "Revise o identificador e os campos enviados.")
			return
		}
		confirmation, err := transactions.SetInclusion(
			r.Context(), db, id, money.InclusionState(req.State), transactions.InclusionOriginManual, nil, nil, onIgnoredHook,
		)
		if errors.Is(err, transactions.ErrTransactionNotFound) {
			writeProblem(w, 404, "transaction-not-found", "Transação não encontrada", "")
			return
		}
		if err != nil {
			writeProblem(w, 503, "transaction-inclusion-unavailable", "Alteração temporariamente indisponível", "Tente novamente em instantes.")
			return
		}
		writeJSON(w, http.StatusOK, transactionInclusionResultDTO{
			TransactionID: confirmation.TransactionID,
			State:         string(confirmation.State),
			ChangedAt:     confirmation.ChangedAt,
		})
	}
}

type transactionCategoryRequest struct {
	CategoryID string `json:"category_id"`
}

type transactionCategoryResultDTO struct {
	TransactionID string    `json:"transaction_id"`
	CategoryID    string    `json:"category_id"`
	Origin        string    `json:"origin"`
	ChangedAt     time.Time `json:"changed_at"`
}

func handleSetTransactionCategory(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req transactionCategoryRequest
		if err := decodeStrict(r, &req); err != nil || req.CategoryID == "" {
			writeProblem(w, 422, "invalid-transaction-category", "Atribuição de categoria inválida", "Revise o identificador e os campos enviados.")
			return
		}
		decision, err := categories.AssignManual(r.Context(), db, id, req.CategoryID)
		if errors.Is(err, categories.ErrTransactionNotFound) {
			writeProblem(w, 404, "transaction-not-found", "Transação não encontrada", "")
			return
		}
		if errors.Is(err, categories.ErrCategoryInvalid) {
			writeProblem(w, 422, "invalid-category", "Categoria inválida", "A categoria informada não existe ou está inativa.")
			return
		}
		if err != nil {
			writeProblem(w, 503, "transaction-category-unavailable", "Alteração temporariamente indisponível", "Tente novamente em instantes.")
			return
		}
		writeJSON(w, http.StatusOK, transactionCategoryResultDTO{
			TransactionID: decision.TransactionID,
			CategoryID:    decision.CategoryID,
			Origin:        "manual",
			ChangedAt:     decision.ChangedAt,
		})
	}
}

// manualTransactionRequest is the request body for both creating and editing
// a lançamento manual. Amount is already signed by the caller — positive for
// money coming in, negative for money going out — the same convention
// transactions.ManualInput expects.
type manualTransactionRequest struct {
	AccountID   string          `json:"account_id"`
	Description string          `json:"description"`
	Amount      decimal.Decimal `json:"amount"`
	OccurredAt  string          `json:"occurred_at"`
	CategoryID  *string         `json:"category_id"`
}

func (req manualTransactionRequest) toManualInput() (transactions.ManualInput, bool) {
	if req.AccountID == "" || req.Description == "" || req.Amount.IsZero() {
		return transactions.ManualInput{}, false
	}
	day, err := time.Parse("2006-01-02", req.OccurredAt)
	if err != nil {
		return transactions.ManualInput{}, false
	}
	// Anchored at local noon, not UTC midnight: the user picked a calendar
	// day with no time-of-day, and occurred_at is stored and later rendered
	// as an instant (see presentation/dates.ts's formatLocalDay). A UTC
	// midnight stamp reads as the previous day everywhere west of Greenwich
	// — the same failure mode networth.creditCardBalanceAsOf already works
	// around by anchoring on time.Local noon instead.
	occurredAt := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.Local)
	return transactions.ManualInput{
		AccountID:   req.AccountID,
		Description: req.Description,
		Amount:      req.Amount,
		OccurredAt:  occurredAt,
	}, true
}

func invalidManualTransactionProblem(w http.ResponseWriter) {
	writeProblem(w, 422, "invalid-manual-transaction", "Lançamento manual inválido",
		"Informe conta, descrição, um valor diferente de zero e uma data válida.")
}

func manualTransactionUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "manual-transaction-unavailable",
		"Lançamento manual temporariamente indisponível", "Tente novamente em instantes.")
}

func invalidCategoryProblem(w http.ResponseWriter) {
	writeProblem(w, 422, "invalid-category", "Categoria inválida", "A categoria informada não existe ou está inativa.")
}

func transactionNotFoundProblem(w http.ResponseWriter) {
	writeProblem(w, 404, "transaction-not-found", "Transação não encontrada", "")
}

// writeManualTransactionError maps the errors ledger.Service can return for a
// create or edit to their Problem responses. notManualDetail and
// investmentLinkedDetail differ between edit and delete, so callers pass them.
// An invalid category is the client's fault only when the request named one:
// otherwise it comes from an automation rule pointing at a missing or inactive
// category, which is a server-side 503, not a 422 about input the client never sent.
func writeManualTransactionError(w http.ResponseWriter, err error, explicitCategory bool, investmentLinkedDetail, notManualDetail string) {
	switch {
	case errors.Is(err, transactions.ErrInvestmentLinked):
		writeProblem(w, 409, "investment-linked-transaction", "Lançamento vinculado a investimento", investmentLinkedDetail)
	case errors.Is(err, transactions.ErrTransactionNotFound):
		transactionNotFoundProblem(w)
	case errors.Is(err, transactions.ErrNotManual):
		writeProblem(w, 409, "transaction-not-manual", "Transação não é manual", notManualDetail)
	case errors.Is(err, transactions.ErrAccountArchived):
		writeProblem(w, 409, "account-archived", "Conta arquivada", "Reative a conta antes de registrar ou editar movimentações.")
	case errors.Is(err, transactions.ErrAccountNotFound):
		accountNotFoundProblem(w)
	case errors.Is(err, ledger.ErrIdempotencyConflict):
		writeProblem(w, 409, "idempotency-key-conflict", "Chave de repetição já utilizada", "Use uma nova chave para uma transação diferente.")
	case explicitCategory && errors.Is(err, categories.ErrCategoryInvalid):
		invalidCategoryProblem(w)
	default:
		manualTransactionUnavailableProblem(w)
	}
}

// handleCreateManualTransaction creates a lançamento the user authors by hand,
// on an account that already exists (see .specs/lancamentos-manuais.md). The
// category and automation ordering lives in ledger.Service.
func handleCreateManualTransaction(svc *ledger.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) > 1 || (len(keys) == 1 && !validManualIdempotencyKey(keys[0])) {
			writeProblem(w, 422, "invalid-idempotency-key", "Chave de repetição inválida", "Use de 8 a 128 caracteres ASCII sem espaços.")
			return
		}
		var req manualTransactionRequest
		if err := decodeStrict(r, &req); err != nil {
			invalidManualTransactionProblem(w)
			return
		}
		input, ok := req.toManualInput()
		if !ok {
			invalidManualTransactionProblem(w)
			return
		}
		key := ""
		if len(keys) == 1 {
			key = keys[0]
		}
		// The current auth model has exactly one owner (auth_owner.id = 1).
		item, err := svc.CreateManualIdempotent(r.Context(), input, req.CategoryID, key, 1)
		if err != nil {
			writeManualTransactionError(w, err, req.CategoryID != nil, "", "")
			return
		}
		writeJSON(w, http.StatusCreated, toItemDTO(item))
	}
}

func validManualIdempotencyKey(key string) bool {
	if len(key) < 8 || len(key) > 128 || strings.ContainsAny(key, " \t\r\n,") {
		return false
	}
	for _, b := range []byte(key) {
		if b < '!' || b > '~' {
			return false
		}
	}
	return true
}

// handleUpdateManualTransaction edits a lançamento manual's core fields and,
// when supplied, its category.
func handleUpdateManualTransaction(svc *ledger.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req manualTransactionRequest
		if err := decodeStrict(r, &req); err != nil {
			invalidManualTransactionProblem(w)
			return
		}
		input, ok := req.toManualInput()
		if !ok {
			invalidManualTransactionProblem(w)
			return
		}
		item, err := svc.UpdateManual(r.Context(), id, input, req.CategoryID)
		if err != nil {
			writeManualTransactionError(w, err, req.CategoryID != nil,
				"Desfaça os vínculos de investimento antes de editar o lançamento.",
				"Só lançamentos manuais podem ser editados.")
			return
		}
		writeJSON(w, http.StatusOK, toItemDTO(item))
	}
}

// handleDeleteManualTransaction removes a lançamento manual entirely —
// unlike a synced row, there is no upstream fetch for the ledger to
// preserve, so deleting means the row is gone, not ignored.
func handleDeleteManualTransaction(svc *ledger.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := svc.DeleteManual(r.Context(), r.PathValue("id"))
		if err != nil {
			writeManualTransactionError(w, err, false,
				"Desfaça os vínculos de investimento antes de excluir o lançamento.",
				"Só lançamentos manuais podem ser excluídos.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleCategoryBreakdown returns real category totals for a month, range, or all dates.
func handleCategoryBreakdown(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		timezone := params.Get("timezone")
		loc, err := time.LoadLocation(timezone)
		if timezone == "" || err != nil {
			writeProblem(w, 400, "invalid-timezone", "Fuso horário inválido", "Informe um nome de fuso IANA reconhecido.")
			return
		}
		var filters transactions.Filters
		var monthLabel string
		var dateFrom, dateTo *string
		invalidPeriod := func() {
			writeProblem(w, 400, "invalid-date-range", "Intervalo de datas inválido", "Informe month; ou date_from e date_to juntos; ou period=all. Não combine os modos.")
		}
		switch {
		case params.Has("month"):
			if params.Has("date_from") || params.Has("date_to") || params.Has("period") {
				invalidPeriod()
				return
			}
			month, err := time.ParseInLocation("2006-01", params.Get("month"), loc)
			if err != nil || month.Year() < 1 {
				writeProblem(w, 400, "invalid-month", "Mês inválido", "Informe o mês no formato YYYY-MM.")
				return
			}
			last := month.AddDate(0, 1, -1)
			from := money.Date{Year: month.Year(), Month: month.Month(), Day: 1}
			to := money.Date{Year: last.Year(), Month: last.Month(), Day: last.Day()}
			filters.DateFrom, filters.DateTo = &from, &to
			monthLabel = month.Format("2006-01")
		case params.Has("period"):
			if params.Get("period") != "all" || params.Has("date_from") || params.Has("date_to") {
				invalidPeriod()
				return
			}
		default:
			fromText, toText := params.Get("date_from"), params.Get("date_to")
			from, errFrom := parseDateOnly(fromText)
			to, errTo := parseDateOnly(toText)
			if errFrom != nil || errTo != nil || from.Year < 1 || to.Year < 1 || fromText > toText {
				invalidPeriod()
				return
			}
			filters.DateFrom, filters.DateTo = &from, &to
			dateFrom, dateTo = &fromText, &toText
		}
		classification := money.Classification(params.Get("classification"))
		if classification != money.Inflow && classification != money.Outflow {
			writeProblem(w, 400, "invalid-classification", "Classificação inválida", "Informe inflow ou outflow.")
			return
		}
		items, err := transactions.CategoryBreakdown(r.Context(), db, filters, timezone, classification)
		if err != nil {
			writeProblem(w, 503, "category-breakdown-unavailable", "Categorias temporariamente indisponíveis", "Tente novamente em instantes.")
			return
		}
		total := decimal.Zero
		result := make([]categorySpendingItemDTO, 0, len(items))
		for _, item := range items {
			amount, err := decimal.NewFromString(item.Amount)
			if err != nil {
				writeProblem(w, 503, "category-breakdown-unavailable", "Categorias temporariamente indisponíveis", "Tente novamente em instantes.")
				return
			}
			total = total.Add(amount)
			result = append(result, categorySpendingItemDTO{CategoryID: item.CategoryID, CategoryName: item.CategoryName, CategoryIcon: item.CategoryIcon, CategoryColor: item.CategoryColor, Amount: item.Amount, Source: "real"})
		}
		valuation, ok := reportValuation(w, r, db, time.Now().UTC())
		if !ok {
			return
		}
		for i := range result {
			valuation.fields(&result[i].Amount)
		}
		total = valuation.amount(total)
		if monthLabel != "" {
			writeJSON(w, http.StatusOK, struct {
				spendingByCategoryDTO
				Classification money.Classification `json:"classification"`
			}{spendingByCategoryDTO{FX: &valuation.FX, Month: monthLabel, CurrencyCode: valuation.CurrencyCode, Total: money.CanonicalDecimal(total), Items: result}, classification})
			return
		}
		writeJSON(w, http.StatusOK, struct {
			DateFrom       *string                   `json:"date_from"`
			DateTo         *string                   `json:"date_to"`
			CurrencyCode   string                    `json:"currency_code"`
			Total          string                    `json:"total"`
			Items          []categorySpendingItemDTO `json:"items"`
			Classification money.Classification      `json:"classification"`
			FX             *exchange.Rate            `json:"fx"`
		}{dateFrom, dateTo, valuation.CurrencyCode, money.CanonicalDecimal(total), result, classification, &valuation.FX})
	}
}
