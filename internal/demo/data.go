package demo

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/investments"
)

// monthsBack is how much history the demo shows.
const monthsBack = 6

// run builds the whole sample: a household with a salary, fixed bills,
// day-to-day spending, savings, a debt, goals and a small portfolio.
func (s *seeder) run() error {
	cats := s.categories()

	checking := id(s.call("POST", "/api/accounts", map[string]any{"name": "Conta Corrente", "currency_code": "BRL", "opening_balance": "3200.00"}))
	savings := id(s.call("POST", "/api/accounts", map[string]any{"name": "Reserva", "currency_code": "BRL", "opening_balance": "18000.00"}))
	wallet := id(s.call("POST", "/api/accounts", map[string]any{"name": "Carteira", "currency_code": "BRL", "opening_balance": "600.00"}))
	s.summary.Accounts = 3
	if s.err != nil {
		return s.err
	}

	s.history(cats, checking, savings, wallet)
	s.planning(cats, checking)
	s.rules(cats)
	s.goals()
	s.portfolio()
	return s.err
}

// categories maps the default category names to ids.
func (s *seeder) categories() map[string]string {
	out := map[string]string{}
	for _, c := range s.list("/api/categories") {
		name, _ := c["name"].(string)
		out[name] = id(c)
	}
	return out
}

func (s *seeder) expense(account, cat, description string, when time.Time, amount string) {
	if when.After(s.today) {
		return
	}
	s.call("POST", "/api/transactions", map[string]any{
		"account_id": account, "category_id": cat, "description": description,
		"amount": "-" + amount, "occurred_at": date(when),
	})
	s.summary.Transactions++
}

func (s *seeder) income(account, cat, description string, when time.Time, amount string) {
	if when.After(s.today) {
		return
	}
	s.call("POST", "/api/transactions", map[string]any{
		"account_id": account, "category_id": cat, "description": description,
		"amount": amount, "occurred_at": date(when),
	})
	s.summary.Transactions++
}

func (s *seeder) transfer(from, to, description string, when time.Time, amount string) {
	if when.After(s.today) {
		return
	}
	s.call("POST", "/api/transfers", map[string]any{
		"source":      map[string]any{"account_id": from, "description": description, "amount": "-" + amount, "occurred_at": date(when)},
		"destination": map[string]any{"account_id": to, "description": description, "amount": amount, "occurred_at": date(when)},
	})
	s.summary.Transfers++
}

// history writes monthsBack months of income, bills and spending up to today.
func (s *seeder) history(cats map[string]string, checking, savings, wallet string) {
	first := time.Date(s.today.Year(), s.today.Month()-monthsBack+1, 1, 0, 0, 0, 0, time.UTC)
	for m := 0; m < monthsBack; m++ {
		month := first.AddDate(0, m, 0)
		at := func(d int) time.Time { return month.AddDate(0, 0, d-1) }

		s.income(checking, cats["Salário"], "Salário Empresa Exemplo", at(5), "8500.00")
		s.expense(checking, cats["Moradia"], "Aluguel", at(6), "2200.00")
		s.transfer(checking, savings, "Aporte na reserva", at(7), "1500.00")
		s.expense(checking, cats["Moradia"], "Conta de luz", at(10), s.rng.between(140, 230))
		s.expense(checking, cats["Serviços digitais"], "Internet fibra", at(10), "119.90")
		s.expense(checking, cats["Serviços digitais"], "Netflix", at(12), "55.90")
		s.expense(checking, cats["Serviços digitais"], "Spotify", at(12), "21.90")
		s.expense(checking, cats["Educação"], "Curso de inglês", at(15), "199.00")
		s.expense(checking, cats["Saúde"], "Academia", at(15), "119.90")
		s.income(savings, cats["Rendimentos"], "Rendimento da reserva", at(28), s.rng.between(130, 190))

		for _, d := range []int{3, 9, 16, 23} {
			s.expense(checking, cats["Supermercado"], "Supermercado Bom Preço", at(d), s.rng.between(210, 520))
		}
		for _, d := range []int{4, 8, 13, 19, 25} {
			s.expense(checking, cats["Alimentação"], "Restaurante e delivery", at(d), s.rng.between(32, 135))
		}
		for _, d := range []int{2, 11, 18, 26} {
			s.expense(checking, cats["Transporte"], "Corridas de aplicativo", at(d), s.rng.between(16, 52))
		}
		s.expense(checking, cats["Transporte"], "Combustível", at(14), s.rng.between(170, 260))
		s.expense(wallet, cats["Lazer"], "Cinema e passeios", at(20), s.rng.between(25, 60))
		s.expense(checking, cats["Saúde"], "Farmácia", at(21), s.rng.between(40, 160))
		if m%2 == 1 {
			s.income(checking, cats["Trabalho autônomo"], "Projeto freelance", at(18), s.rng.between(900, 1900))
			s.expense(checking, cats["Compras"], "Compras online", at(22), s.rng.between(120, 480))
		}
	}
}

// planning registers what is still to come: recurring commitments and debts.
func (s *seeder) planning(cats map[string]string, checking string) {
	start := date(time.Date(s.today.Year(), s.today.Month()-monthsBack+1, 1, 0, 0, 0, 0, time.UTC))
	recurring := []map[string]any{
		{"name": "Salário", "kind": "income", "amount": "8500.00", "category_id": cats["Salário"], "day_of_month": 5},
		{"name": "Aluguel", "kind": "expense", "amount": "2200.00", "category_id": cats["Moradia"], "day_of_month": 6},
		{"name": "Internet fibra", "kind": "expense", "amount": "119.90", "category_id": cats["Serviços digitais"], "day_of_month": 10},
		{"name": "Streaming", "kind": "expense", "amount": "77.80", "category_id": cats["Serviços digitais"], "day_of_month": 12},
		{"name": "Academia", "kind": "expense", "amount": "119.90", "category_id": cats["Saúde"], "day_of_month": 15},
	}
	for _, r := range recurring {
		r["account_id"], r["cadence"], r["start_date"], r["is_active"] = checking, "monthly", start, true
		s.call("POST", "/api/recurring-scenarios", r)
		s.summary.Recurring++
	}
	s.call("POST", "/api/recurring-scenarios", map[string]any{
		"name": "Seguro do carro", "kind": "expense", "amount": "1850.00", "category_id": cats["Seguros"], "account_id": checking,
		"cadence": "annual", "day_of_month": 20, "month_of_year": int(s.today.AddDate(0, 3, 0).Month()), "start_date": start, "is_active": true,
	})
	s.summary.Recurring++

	s.call("POST", "/api/payables", map[string]any{"kind": "debt", "name": "Financiamento do notebook", "total_amount": "6000.00", "initial_remaining_amount": "3600.00"})
	s.call("POST", "/api/payables", map[string]any{"kind": "receivable", "name": "Empréstimo ao João", "total_amount": "1200.00"})
	s.summary.Payables = 2
}

// rules adds an automation that categorizes a recurring merchant.
func (s *seeder) rules(cats map[string]string) {
	s.call("POST", "/api/automation-rules", map[string]any{
		"name": "Streaming", "is_active": true, "logic_operator": "and", "apply_retroactively": false,
		"conditions": []map[string]any{{"field": "description", "operator": "contains", "value": "Netflix"}},
		"actions":    []map[string]any{{"type": "set_category", "category_id": cats["Serviços digitais"]}},
	})
	s.summary.Rules = 1
}

func (s *seeder) goals() {
	s.call("POST", "/api/goals", map[string]any{
		"name": "Reserva de emergência", "metric": "invested_value", "target_amount": "80000.00",
		"target_date": date(s.today.AddDate(1, 6, 0)), "notes": "Seis meses de despesas.",
	})
	s.call("POST", "/api/goals", map[string]any{
		"name": "Viagem para o Japão", "metric": "invested_value", "target_amount": "62000.00",
		"target_date": date(s.today.AddDate(0, 11, 0)),
	})
	s.summary.Goals = 2
}

// portfolio opens a broker account with a few holdings and later purchases.
func (s *seeder) portfolio() {
	account := id(s.call("POST", "/api/investment-accounts", map[string]any{"name": "Corretora Exemplo", "kind": "manual", "currency_code": "BRL"}))
	opened := date(s.today.AddDate(0, -monthsBack, 0))
	// Purchases draw on the account's cash, so fund it first.
	s.call("POST", "/api/investment-operations", map[string]any{"account_id": account, "kind": "deposit", "occurred_on": opened, "amount": "9000.00"})
	s.summary.Operations++
	holdings := []struct {
		name, ticker, kind, quantity, cost string
		baseCents, monthlyGain             int // fixed income only: opening value and growth per month
		startCents, endCents               int // variable income only: price path since opening, in cents
	}{
		{"Tesouro Selic 2029", "", "Tesouro Direto", "1.5", "14500.00", 2175000, 20660, 0, 0},
		{"CDB Banco Exemplo 110% CDI", "", "CDB", "1", "10000.00", 1000000, 9700, 0, 0},
		{"Itaú Unibanco PN", "ITUB4", "Ação", "120", "31.50", 0, 0, 3150, 3780},
		{"HGLG Logística", "HGLG11", "FII", "12", "158.00", 0, 0, 15800, 16640},
	}
	// Listed assets come from the catalog, which carries their quote market.
	catalog := map[string]string{}
	for _, a := range s.list("/api/investment-assets") {
		if ticker, _ := a["ticker"].(string); ticker != "" {
			catalog[ticker] = id(a)
		}
	}
	for _, h := range holdings {
		body := map[string]any{
			"account_id": account, "name": h.name, "asset_type": h.kind,
			"initial_quantity": h.quantity, "initial_unit_cost": h.cost, "occurred_on": opened,
		}
		if h.ticker != "" {
			body["ticker"] = h.ticker
			body["asset_id"] = catalog[h.ticker]
		}
		created := s.call("POST", "/api/investment-positions", body)
		position := id(created)
		assetID, _ := created["asset_id"].(string)
		s.summary.Positions++
		if h.ticker != "" {
			for i := 1; i <= 3; i++ {
				lo, hi, lot := h.startCents/100, h.endCents/100, "10"
				if h.ticker == "HGLG11" {
					lot = "3"
				}
				s.call("POST", "/api/investment-operations", map[string]any{
					"account_id": account, "position_id": position, "kind": "buy",
					"occurred_on": date(s.today.AddDate(0, -i*2+1, 0)), "quantity": lot, "unit_price": s.rng.between(lo, hi), "fees": "0",
				})
				s.summary.Operations++
			}
			s.quotes(assetID, h.ticker, h.startCents, h.endCents)
		}
		if h.ticker == "HGLG11" {
			for i := 1; i < monthsBack; i++ {
				s.call("POST", "/api/investment-operations", map[string]any{
					"account_id": account, "kind": "income", "notes": "Rendimentos do fundo imobiliário",
					"occurred_on": date(s.today.AddDate(0, -i, 0)), "amount": s.rng.between(38, 52),
				})
				s.summary.Operations++
			}
		}
		if h.monthlyGain != 0 {
			// A month-by-month valuation shows the fixed income growing.
			for k := 1; k <= monthsBack; k++ {
				cents := h.baseCents + k*h.monthlyGain
				s.call("POST", "/api/investment-operations", map[string]any{
					"account_id": account, "position_id": position, "kind": "valuation",
					"occurred_on": date(s.today.AddDate(0, k-monthsBack, 0)), "amount": fmt.Sprintf("%d.%02d", cents/100, cents%100),
				})
				s.summary.Operations++
			}
		}
	}
}

// quotes writes a daily price path from the opening day to today and marks the
// range as fetched. The demo never calls a market data provider, so listed
// assets would otherwise be worth nothing (and flagged as partial history).
func (s *seeder) quotes(assetID, symbol string, startCents, endCents int) {
	if s.err != nil || assetID == "" {
		return
	}
	from := s.today.AddDate(0, -monthsBack, 0)
	days := int(s.today.Sub(from).Hours() / 24)
	for i := 0; i <= days; i++ {
		cents := startCents + (endCents-startCents)*i/days
		wiggle := cents / 150
		cents += int(s.rng.next()%uint64(2*wiggle+1)) - wiggle
		if i == days {
			cents = endCents
		}
		price := decimal.New(int64(cents), -2)
		err := investments.UpsertAssetQuote(s.ctx, s.conn, investments.AssetQuote{
			AssetID: assetID, QuotedOn: from.AddDate(0, 0, i), Price: price, Source: "demo", Origin: investments.QuoteOriginMarket,
		})
		if err != nil {
			s.err = fmt.Errorf("demo: quote: %w", err)
			return
		}
	}
	coverage := investments.QuoteCoverage{AssetID: assetID, Source: "demo", Symbol: symbol, From: from, To: s.today}
	if err := investments.SaveQuoteCoverage(s.ctx, s.conn, coverage); err != nil {
		s.err = fmt.Errorf("demo: quote coverage: %w", err)
	}
}
