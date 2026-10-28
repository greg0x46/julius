package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/goals"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/transactions"
)

func planningSession(t *testing.T, conn *sql.DB) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := sdk.NewServer(&sdk.Implementation{Name: "planning-test", Version: "1"}, nil)
	httpapi.RegisterMCPPlanningTools(server, conn)
	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "generic-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func planningCall(t *testing.T, s *sdk.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		content, err := json.Marshal(result.Content)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("%s: %s", name, content)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func planningExec(t *testing.T, conn *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func planningCash(t *testing.T, conn *sql.DB, currency, amount string) string {
	t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	planningExec(t, conn, `INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES(?,'manual','Synthetic cash','BANK',?,?,?,?)`, id, currency, amount, now, now)
	return id
}

func planningRate(t *testing.T, conn *sql.DB, base, quote, day, rate string) {
	t.Helper()
	planningExec(t, conn, `INSERT INTO exchange_rates(base_currency,quote_currency,as_of,effective_on,rate,source,fetched_at) VALUES(?,?,?,?,?,'synthetic',?)`, base, quote, day, day, rate, db.FormatTime(time.Now()))
}

func TestMCPPlanningMatchesApplication(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = planningOfflineTransport{local: original}
	defer func() { http.DefaultTransport = original }()
	srv, conn := newTestServer(t)
	ctx := context.Background()
	today := investments.ProviderDay(time.Now())
	date := today.Format(time.DateOnly)
	from := today.AddDate(0, 0, -5).Format(time.DateOnly)
	id := insertSyncedEquity(t, conn, "Synthetic foreign fund", "SYNTH", "10", "1100")
	quantity := "10"
	insertInvestmentMovement(t, conn, id, "BUY", "inflow", "1000", &quantity)
	planningExec(t, conn, `UPDATE financial_investments SET currency_code='USD' WHERE id=?`, id)
	asset, err := investments.ResolveSyncedAsset(ctx, conn, investments.SyncedHolding{ExternalID: id, Name: strPtr("Synthetic foreign fund"), Code: strPtr("SYNTH"), InvestmentType: strPtr("EQUITY"), CurrencyCode: strPtr("USD")})
	if err != nil {
		t.Fatal(err)
	}
	if err := investments.UpsertAssetQuote(ctx, conn, investments.AssetQuote{AssetID: asset.ID, QuotedOn: today, Price: decimal.NewFromInt(110), Source: investments.QuoteSourcePluggy, Origin: investments.QuoteOriginSync}); err != nil {
		t.Fatal(err)
	}
	planningExec(t, conn, `UPDATE financial_investment_transactions SET occurred_at=?,trade_date=? WHERE investment_id=?`, db.FormatTime(today.AddDate(0, 0, -3)), today.AddDate(0, 0, -3).Format(time.DateOnly), id)
	planningRate(t, conn, "USD", "BRL", date, "5")
	planningRate(t, conn, "BRL", "EUR", date, "0.2")
	if err := settings.Set(ctx, conn, settings.KeyReferenceCurrency, "EUR", false, nil); err != nil {
		t.Fatal(err)
	}
	account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Synthetic custody"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = investments.CreatePosition(ctx, conn, investments.PositionInput{AccountID: account.ID, Name: "Unpriced synthetic position", AssetType: "CDB", InitialQuantity: decimal.NewFromInt(1), InitialUnitCost: decimal.NewFromInt(500), OccurredOn: today.AddDate(0, 0, -10)})
	if err != nil {
		t.Fatal(err)
	}
	target := today.AddDate(1, 0, 0)
	_, err = goals.Create(ctx, conn, goals.Input{Name: "Synthetic goal", Metric: goals.MetricInvestedValue, TargetAmount: decimal.NewFromInt(20000), TargetDate: &target})
	if err != nil {
		t.Fatal(err)
	}
	session := planningSession(t, conn)
	got := planningCall(t, session, "get_investment_portfolio", map[string]any{"from": from, "to": date})
	want := investmentCall(t, srv, http.MethodGet, "/api/investment-portfolio?from="+from+"&to="+date, nil, 200)
	// Identity/current FX fetched_at differs between reads; financial fields must agree.
	for _, key := range []string{"currency_code", "from", "to", "to_clamped", "groups", "native_rates", "totals"} {
		if !reflect.DeepEqual(got[key], want[key]) {
			t.Fatalf("portfolio %s differs: MCP=%v REST=%v", key, got[key], want[key])
		}
	}
	totals := got["totals"].(map[string]any)
	if totals["partial"] != true || totals["positions_unavailable"] != float64(1) {
		t.Fatalf("partial converted portfolio: %+v", totals)
	}
	assertMoney(t, "converted portfolio", totals["end_value"], "1100")
	positions := got["groups"].([]any)[0].(map[string]any)["positions"].([]any)
	for _, raw := range positions {
		p := raw.(map[string]any)
		if p["name"] == "Unpriced synthetic position" && (p["end_value"] != nil || p["unavailable_reason"] == nil) {
			t.Fatalf("unknown silently valued: %+v", p)
		}
	}
	goalList := planningCall(t, session, "list_goals", map[string]any{})
	wantGoals, _ := json.Marshal(investmentItems(t, srv, "/api/goals"))
	gotGoals, _ := json.Marshal(goalList["items"])
	if string(gotGoals) != string(wantGoals) {
		t.Fatal("goal projections differ from application")
	}

	if goalList["currency_code"] != "BRL" {
		t.Fatal("goal currency followed display currency")
	}
	wealth := planningCall(t, session, "get_net_worth", map[string]any{"from": date, "to": date})
	live := wealth["latest"].(map[string]any)
	actual := investmentCall(t, srv, http.MethodGet, "/api/net-worth", nil, 200)["latest"].(map[string]any)
	for key, value := range live["breakdown"].(map[string]any) {
		if actual[key] != value {
			t.Fatalf("wealth %s: %v != %v", key, value, actual[key])
		}
	}
	if wealth["currency_code"] != "EUR" || live["valuation_status"] != actual["valuation_status"] {
		t.Fatal("wealth valuation differs")
	}
	clamped := planningCall(t, session, "get_investment_portfolio", map[string]any{"to": today.AddDate(0, 0, 10).Format(time.DateOnly)})
	if clamped["to"] != date || clamped["to_clamped"] != true {
		t.Fatal("future portfolio date not clamped")
	}

	cashID := planningCash(t, conn, "BRL", "100")
	if _, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: cashID, Description: "Synthetic deposit", Amount: decimal.NewFromInt(10), OccurredAt: today}); err != nil {
		t.Fatal(err)
	}
	projected := planningCall(t, session, "get_timeline", map[string]any{"from": date, "to": date, "reference_date": date})
	appTimeline := investmentCall(t, srv, http.MethodGet, "/api/timeline?from="+date+"&to="+date+"&reference_date="+date, nil, 200)
	if !reflect.DeepEqual(projected["series"], appTimeline["base"]) || !reflect.DeepEqual(projected["period_totals"], appTimeline["period_totals"]) {
		t.Fatalf("timeline differs from application: MCP series=%+v totals=%+v; REST base=%+v totals=%+v", projected["series"], projected["period_totals"], appTimeline["base"], appTimeline["period_totals"])
	}
	assertMoney(t, "reporting converted once", projected["series"].(map[string]any)["reporting"].(map[string]any)["totals"].(map[string]any)["inflow"], "2")
	planningExec(t, conn, `DELETE FROM exchange_rates WHERE base_currency='USD'`)
	missing := planningCall(t, session, "get_investment_portfolio", map[string]any{"from": from, "to": date})
	if missing["totals"].(map[string]any)["partial"] != true {
		t.Fatal("missing FX considered complete")
	}
	for _, raw := range missing["groups"].([]any)[0].(map[string]any)["positions"].([]any) {
		p := raw.(map[string]any)
		if p["source"] == "synced" && (p["unavailable_reason"] != investments.YieldReasonNoRate || p["end_value"] != nil) {
			t.Fatalf("missing investment FX hidden: %+v", p)
		}
	}

}

func TestMCPPlanningTimelineUsesUTCReferenceFXDay(t *testing.T) {
	original := time.Local
	// Pick a valid RFC3339 offset that crosses UTC midnight at the current hour.
	offset := 12 * 60 * 60
	if time.Now().UTC().Hour() < 12 {
		offset = -offset
	}
	time.Local = time.FixedZone("synthetic-other-day", offset)
	t.Cleanup(func() { time.Local = original })
	date := time.Now().UTC().Format(time.DateOnly)
	if time.Now().Format(time.DateOnly) == date {
		t.Fatal("test requires different local and UTC calendar days")
	}
	srv, conn := newTestServer(t)
	planningCash(t, conn, "BRL", "100")
	session := planningSession(t, conn)
	got := planningCall(t, session, "get_timeline", map[string]any{"from": date, "to": date, "reference_date": date, "timezone": "UTC"})
	want := investmentCall(t, srv, http.MethodGet, "/api/timeline?from="+date+"&to="+date+"&reference_date="+date+"&timezone=UTC", nil, 200)
	for _, key := range []string{"as_of", "effective_on"} {
		gotDay := got["fx"].(map[string]any)[key]
		wantDay := want["fx"].(map[string]any)[key]
		if gotDay != wantDay || gotDay != date+"T00:00:00Z" {
			t.Fatalf("reference FX %s: MCP=%v REST=%v, want UTC day %s", key, gotDay, wantDay, date)
		}
	}
	if !reflect.DeepEqual(got["series"], want["base"]) || !reflect.DeepEqual(got["period_totals"], want["period_totals"]) {
		t.Fatal("timeline differs from application with non-UTC process timezone")
	}
}

func TestMCPPlanningExplicitScenarios(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		today := investments.ProviderDay(time.Now())
		date := today.Format(time.DateOnly)
		accountID := planningCash(t, conn, "BRL", "1000.123456789012345678")
		category, err := categories.Create(ctx, conn, "Synthetic expense", money.Expense, "", "")
		if err != nil {
			t.Fatal(err)
		}
		recurring, err := recurrences.Create(ctx, conn, recurrences.Write{Name: "Synthetic recurrence", Kind: recurrences.KindExpense, Amount: decimal.NewFromInt(10), CategoryID: category.ID, Cadence: recurrences.CadenceMonthly, DayOfMonth: today.Day(), StartDate: today, IsActive: true})
		if err != nil {
			t.Fatal(err)
		}
		debt, err := payables.Create(ctx, conn, payables.KindDebt, "Synthetic debt", decimal.NewFromInt(100), decimal.NewFromInt(80))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := scenarios.CreateScenario(ctx, conn, scenarios.KindDebtPlan, "Synthetic plan", &debt.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := scenarios.CreateScenarioTransaction(ctx, conn, plan.ID, "Installment", decimal.NewFromInt(20), today, nil); err != nil {
			t.Fatal(err)
		}
		hypothetical, err := scenarios.CreateScenario(ctx, conn, scenarios.KindStandalone, "Synthetic what-if", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := scenarios.CreateScenarioTransaction(ctx, conn, hypothetical.ID, "Trip", decimal.NewFromInt(-50), today, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := scenarios.SetActive(ctx, conn, hypothetical.ID, true); err != nil {
			t.Fatal(err)
		}
		session := planningSession(t, conn)
		args := map[string]any{"from": date, "to": date, "reference_date": date}
		baseline := planningCall(t, session, "get_timeline", args)
		baseSeries := baseline["series"].(map[string]any)
		if baseSeries["starting_balance"] != "1000.123456789012345678" || baseSeries["points"].([]any)[0].(map[string]any)["balance"] != "970.123456789012345678" {
			t.Fatalf("base: %+v", baseSeries)
		}
		for _, raw := range baseSeries["entries"].([]any) {
			if raw.(map[string]any)["scenario_id"] == hypothetical.ID {
				t.Fatal("active hypothetical leaked into base")
			}
		}
		args["scenario_ids"] = []string{}
		empty := planningCall(t, session, "get_timeline", args)
		if !reflect.DeepEqual(baseSeries, empty["series"]) {
			t.Fatal("explicit empty selection differs from omitted selection")
		}
		args["scenario_ids"] = []string{hypothetical.ID, hypothetical.ID}
		selected := planningCall(t, session, "get_timeline", args)
		if selected["series"].(map[string]any)["points"].([]any)[0].(map[string]any)["balance"] != "920.123456789012345678" {
			t.Fatal("selection double counted or lost commitments")
		}
		events := planningCall(t, session, "list_projection_events", map[string]any{"from": date, "to": date, "scenario_ids": []string{hypothetical.ID}})
		if len(events["items"].([]any)) != 3 {
			t.Fatalf("events: %+v", events)
		}
		// Reconciliation is read from the owning engine and suppresses the projected occurrence.
		transactionID, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: accountID, Description: "Synthetic payment", Amount: decimal.NewFromInt(-10), OccurredAt: today})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := recurrences.PutOverride(ctx, conn, recurring.ID, today, recurrences.StateLinked, &transactionID); err != nil {
			t.Fatal(err)
		}
		events = planningCall(t, session, "list_projection_events", map[string]any{"from": date, "to": date})
		for _, raw := range events["items"].([]any) {
			e := raw.(map[string]any)
			if e["scenario_id"] == recurring.ID && e["realized"] != true {
				t.Fatalf("reconciliation missing: %+v", e)
			}
		}
		debts := planningCall(t, session, "list_payables", map[string]any{"kind": "debt"})["items"].([]any)
		if debts[0].(map[string]any)["remaining_amount"] != "80" {
			t.Fatalf("debt state: %+v", debts)
		}
		for _, name := range []string{"list_recurring_commitments", "list_scenarios"} {
			if len(planningCall(t, session, name, map[string]any{})["items"].([]any)) == 0 {
				t.Fatalf("empty %s", name)
			}
		}
		wealth := planningCall(t, session, "get_net_worth", map[string]any{"from": date, "to": date})
		if wealth["latest"].(map[string]any)["breakdown"].(map[string]any)["total"] != "990.123456789012345678" {
			t.Fatal("hypothetical affected realized wealth")
		}
	})
}

type planningOfflineTransport struct{ local http.RoundTripper }

func (p planningOfflineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme == "http" && (r.URL.Hostname() == "127.0.0.1" || r.URL.Hostname() == "localhost") {
		return p.local.RoundTrip(r)
	}
	return nil, errors.New("synthetic provider outage")
}

func TestMCPPlanningUnavailableAndValidation(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = planningOfflineTransport{local: original}
	defer func() { http.DefaultTransport = original }()
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		session := planningSession(t, conn)
		today := investments.ProviderDay(time.Now()).Format(time.DateOnly)
		for _, tc := range []struct {
			name string
			args map[string]any
		}{
			{"get_timeline", map[string]any{"from": "2026-02-30", "to": today, "reference_date": today}},
			{"get_timeline", map[string]any{"from": today, "to": "2020-01-01", "reference_date": today}},
			{"get_timeline", map[string]any{"from": "2020-01-01", "to": today, "reference_date": today}},
			{"get_timeline", map[string]any{"from": today, "to": today, "reference_date": today, "timezone": "invalid"}},
			{"get_timeline", map[string]any{"from": today, "to": today, "reference_date": today, "scenario_ids": []string{uuid.NewString()}}},
			{"get_net_worth", map[string]any{"from": today, "to": "2099-01-01"}},
			{"get_investment_portfolio", map[string]any{"to": today, "group_by": "unsupported"}},
			{"get_investment_portfolio", map[string]any{"to": today, "source": "unsupported"}},
			{"list_payables", map[string]any{"kind": "unsupported"}},
			{"list_goals", map[string]any{"limit": 101}},
			{"list_scenarios", map[string]any{"offset": -1}},
		} {
			result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil || !result.IsError {
				t.Fatalf("accepted %s %+v: %+v %v", tc.name, tc.args, result, err)
			}
		}
		accountID := planningCash(t, conn, "USD", "123.45")
		for _, name := range []string{"get_net_worth", "get_timeline"} {
			args := map[string]any{"from": today, "to": today}
			if name == "get_timeline" {
				args["reference_date"] = today
			}
			result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
			if err != nil || !result.IsError || result.StructuredContent != nil {
				t.Fatalf("unavailable became money: %+v %v", result, err)
			}
			data, _ := json.Marshal(result.Content)
			if !strings.Contains(string(data), "unavailable") {
				t.Fatalf("missing valuation state: %s", data)
			}
			if name == "get_timeline" && (!strings.Contains(string(data), "USD") || !strings.Contains(string(data), "provider_failure")) {
				t.Fatalf("missing coverage: %s", data)
			}
		}
		planningRate(t, conn, "USD", "BRL", today, "5")
		old := investments.ProviderDay(time.Now()).AddDate(0, 0, -2)
		if _, err := transactions.CreateManual(context.Background(), conn, transactions.ManualInput{AccountID: accountID, Description: "Synthetic foreign deposit", Amount: decimal.NewFromInt(1), OccurredAt: old}); err != nil {
			t.Fatal(err)
		}
		wealth := planningCall(t, session, "get_net_worth", map[string]any{"from": old.Format(time.DateOnly), "to": today})
		series := wealth["series"].([]any)
		if len(series) != 3 {
			t.Fatalf("history length: %d", len(series))
		}
		for _, raw := range series[:2] {
			p := raw.(map[string]any)
			if p["valuation_status"] != "unavailable" || p["breakdown"] != nil {
				t.Fatalf("missing historical FX became zero: %+v", p)
			}
		}
		assertMoney(t, "live foreign cash", wealth["latest"].(map[string]any)["breakdown"].(map[string]any)["total"], "622.25")
	})
}

func TestMCPPlanningPaginationAndOutputBound(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	for _, name := range []string{"First", "Second", "Third"} {
		if _, err := scenarios.CreateScenario(ctx, conn, scenarios.KindStandalone, name, nil); err != nil {
			t.Fatal(err)
		}
	}
	session := planningSession(t, conn)
	first := planningCall(t, session, "list_scenarios", map[string]any{"limit": 2})
	second := planningCall(t, session, "list_scenarios", map[string]any{"limit": 2, "offset": 2})
	if len(first["items"].([]any)) != 2 || first["next_offset"] != float64(2) || len(second["items"].([]any)) != 1 || second["next_offset"] != nil {
		t.Fatal("invalid pagination")
	}
	longName := strings.Repeat("x", 300000)
	if _, err := scenarios.CreateScenario(ctx, conn, scenarios.KindStandalone, longName, nil); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "list_scenarios", Arguments: map[string]any{}})
	if err != nil || !result.IsError {
		t.Fatalf("unbounded output: %+v %v", result, err)
	}
	data, _ := json.Marshal(result)
	if len(data) > 2048 || !strings.Contains(string(data), "256 KiB") {
		t.Fatal("oversized error payload")
	}
	// Database details never cross the tool boundary.
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	unavailable, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "list_payables", Arguments: map[string]any{}})
	if err != nil || !unavailable.IsError || unavailable.StructuredContent != nil {
		t.Fatalf("database failure: %+v %v", unavailable, err)
	}
	safe, _ := json.Marshal(unavailable.Content)
	if strings.Contains(string(safe), "database") || strings.Contains(string(safe), "sql") {
		t.Fatalf("internal error leaked: %s", safe)
	}
	// No client can choose an HTTP endpoint or invoke a write through the schema.
	if result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "list_scenarios", Arguments: map[string]any{"path": "/api/settings"}}); err == nil && !result.IsError {
		t.Fatal("accepted an unknown argument")
	}
}
