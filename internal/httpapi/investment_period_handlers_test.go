package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
)

func periodField(t *testing.T, node map[string]any, key string) any {
	t.Helper()
	value, present := node[key]
	if !present {
		t.Fatalf("response has no %q: %+v", key, node)
	}
	return value
}

func TestInvestmentPeriodReportsPositionsGroupsAndTotals(t *testing.T) {
	srv, conn := newTestServer(t)
	ctx := context.Background()
	today := investments.ProviderDay(time.Now())
	from := today.AddDate(0, 0, -5).Format("2006-01-02")

	// A synced holding bought three days ago and priced above what it cost, and a
	// manual one that was never valued: only the first has a stated yield.
	id := insertSyncedEquity(t, conn, "Fundo Sintetico", "FSNT11", "10", "1100")
	quantity := "10"
	insertInvestmentMovement(t, conn, id, "BUY", "inflow", "1000", &quantity)
	boughtOn := today.AddDate(0, 0, -3)
	if _, err := conn.Exec(`UPDATE financial_investment_transactions SET occurred_at = ?, trade_date = ? WHERE investment_id = ?`,
		db.FormatTime(boughtOn), boughtOn.Format("2006-01-02"), id); err != nil {
		t.Fatal(err)
	}
	quoteToday(t, conn, "Fundo Sintetico", "FSNT11", "110")
	account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Corretora"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := investments.CreatePosition(ctx, conn, investments.PositionInput{
		AccountID: account.ID, Name: "Manual Sintetico", AssetType: "CDB", InitialQuantity: decimal.NewFromInt(1), InitialUnitCost: decimal.NewFromInt(500),
		OccurredOn: today.AddDate(0, 0, -10),
	}); err != nil {
		t.Fatal(err)
	}

	got := investmentCall(t, srv, http.MethodGet, "/api/investment-portfolio?from="+from, nil, http.StatusOK)
	if got["from"] != from || got["to"] != today.Format("2006-01-02") || got["group_by"] != "none" || got["source"] != nil || got["currency_code"] != "BRL" {
		t.Fatalf("echo = %+v", got)
	}
	totals := got["totals"].(map[string]any)
	assertMoney(t, "end value", totals["end_value"], "1100")
	assertMoney(t, "contributions", totals["contributions"], "1000")
	assertMoney(t, "yield", totals["yield"], "100")
	if periodField(t, totals, "partial") != true || totals["positions_total"] != float64(2) || totals["positions_unavailable"] != float64(1) {
		t.Fatalf("totals = %+v", totals)
	}
	groups := got["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	group := groups[0].(map[string]any)
	if group["key"] != "all" || group["account_id"] != nil || group["asset_class"] != nil {
		t.Fatalf("group = %+v", group)
	}
	assertMoney(t, "share", group["share"], "1")
	byName := map[string]map[string]any{}
	for _, raw := range group["positions"].([]any) {
		position := raw.(map[string]any)
		byName[position["name"].(string)] = position
	}
	synced, manual := byName["Fundo Sintetico"], byName["Manual Sintetico"]
	assertMoney(t, "synced share", synced["share"], "1")
	if manual["share"] != nil {
		t.Fatalf("an unavailable position has a share: %+v", manual)
	}
	held := totals["held"].(map[string]any)
	assertMoney(t, "held yield", held["yield"], "100")
	assertMoney(t, "closed yield", totals["closed"].(map[string]any)["yield"], "0")
	assertMoney(t, "synced yield", synced["yield"], "100")
	assertMoney(t, "synced invested", synced["invested"], "1000")
	assertMoney(t, "synced native yield", synced["native"].(map[string]any)["yield"], "100")
	if synced["unavailable_reason"] != nil || synced["asset_class"] != "variable_income" || synced["source"] != "synced" {
		t.Fatalf("synced = %+v", synced)
	}
	// What cannot be stated is null, never zero.
	if manual["unavailable_reason"] != investments.YieldReasonNoPrice || manual["yield"] != nil || manual["end_value"] != nil || manual["native"] != nil {
		t.Fatalf("manual = %+v", manual)
	}
	assertMoney(t, "manual cost basis", manual["invested"], "500")

	byClass := investmentCall(t, srv, http.MethodGet, "/api/investment-portfolio?group_by=class&source=synced", nil, http.StatusOK)
	if byClass["source"] != "synced" || byClass["group_by"] != "class" || byClass["from"] != nil {
		t.Fatalf("echo = %+v", byClass)
	}
	classGroups := byClass["groups"].([]any)
	if len(classGroups) != 1 || classGroups[0].(map[string]any)["asset_class"] != "variable_income" || classGroups[0].(map[string]any)["label"] != "Renda variável" {
		t.Fatalf("class groups = %+v", classGroups)
	}
	assertMoney(t, "since inception yield", byClass["totals"].(map[string]any)["yield"], "100")

	byAccount := investmentCall(t, srv, http.MethodGet, "/api/investment-portfolio?group_by=account&account_id="+account.ID, nil, http.StatusOK)
	accountGroups := byAccount["groups"].([]any)
	if len(accountGroups) != 1 || accountGroups[0].(map[string]any)["account_id"] != account.ID {
		t.Fatalf("account groups = %+v", accountGroups)
	}
	// The totals stay the portfolio's whatever is listed.
	assertMoney(t, "totals independent of the account filter", byAccount["totals"].(map[string]any)["end_value"], "1100")
}

func TestInvestmentPeriodRejectsBadParametersAndIsPrivate(t *testing.T) {
	srv, _ := newTestServer(t)
	tomorrow := investments.ProviderDay(time.Now()).AddDate(0, 0, 1).Format("2006-01-02")
	for _, query := range []string{
		"from=2026-13-01", "to=ontem", "from=2026-09-10&to=2026-09-01", "from=" + tomorrow,
		"group_by=institution", "source=pluggy",
	} {
		if kind := investmentProblem(t, srv, http.MethodGet, "/api/investment-portfolio?"+query, nil, http.StatusBadRequest); kind != "invalid-investment-input" {
			t.Errorf("%s: problem = %q", query, kind)
		}
	}
	if kind := investmentProblem(t, srv, http.MethodGet, "/api/investment-portfolio?account_id=missing", nil, http.StatusNotFound); kind != "investment-account-not-found" {
		t.Errorf("unknown account: problem = %q", kind)
	}
	// A future `to` is clamped to today and the response says so.
	clamped := investmentCall(t, srv, http.MethodGet, "/api/investment-portfolio?to="+tomorrow, nil, http.StatusOK)
	if clamped["to"] != investments.ProviderDay(time.Now()).Format("2006-01-02") || clamped["to_clamped"] != true {
		t.Fatalf("clamped = to %v, to_clamped %v", clamped["to"], clamped["to_clamped"])
	}
	if exact := investmentCall(t, srv, http.MethodGet, "/api/investment-portfolio", nil, http.StatusOK); exact["to_clamped"] != false {
		t.Fatalf("to_clamped = %v without a future to", exact["to_clamped"])
	}
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/investment-portfolio", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d, cache-control %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
}

func TestInvestmentPeriodRequiresAuthentication(t *testing.T) {
	srv, _ := newLockedTestServer(t)
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/investment-portfolio", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
