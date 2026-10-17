package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/investments"
)

func TestInvestmentMonthlyMovementsBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Corretora"})
		if err != nil {
			t.Fatal(err)
		}
		position, err := investments.CreatePosition(ctx, conn, investments.PositionInput{AccountID: account.ID, Name: "Ações", AssetType: "Ação"})
		if err != nil {
			t.Fatal(err)
		}
		on := func(month time.Month, day int) time.Time { return time.Date(2026, month, day, 0, 0, 0, 0, time.UTC) }
		qty := decimal.RequireFromString
		for _, in := range []investments.OperationInput{
			{Kind: investments.OperationDeposit, OccurredOn: on(1, 10), Amount: qty("1000")},
			{Kind: investments.OperationBuy, OccurredOn: on(1, 11), Amount: qty("600"), Fees: qty("12"), PositionID: &position.ID, Quantity: ptrDecimal(qty("6"))},
			{Kind: investments.OperationSell, OccurredOn: on(2, 5), Amount: qty("260"), Fees: qty("3"), Taxes: qty("1"), PositionID: &position.ID, Quantity: ptrDecimal(qty("2"))},
			{Kind: investments.OperationWithdrawal, OccurredOn: on(2, 6), Amount: qty("200")},
		} {
			in.AccountID = account.ID
			if _, err := investments.CreateOperation(ctx, conn, in); err != nil {
				t.Fatalf("CreateOperation(%s): %v", in.Kind, err)
			}
		}

		w := httptest.NewRecorder()
		handleListInvestmentMonthlyMovements(conn)(w, httptest.NewRequest("GET", "/api/investment-monthly-movements?account_id="+account.ID, nil))
		if w.Code != 200 {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var out struct {
			Items []investmentMonthlyMovementDTO `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Items) != 2 {
			t.Fatalf("items = %+v", out.Items)
		}
		jan, feb := out.Items[0], out.Items[1]
		// The 2 units sold wrote off 204 of the 612 paid and netted 256.
		if jan.Month != "2026-01" || jan.Contributions != "1000" || jan.Fees != "12" || !sameDecimal(jan.RealizedGain, "0") {
			t.Errorf("january = %+v", jan)
		}
		if feb.Month != "2026-02" || feb.Withdrawals != "200" || feb.Fees != "3" || feb.Taxes != "1" || feb.Income != "0" || !sameDecimal(feb.RealizedGain, "52") {
			t.Errorf("february = %+v", feb)
		}
	})
}

func sameDecimal(got, want string) bool {
	g, err := decimal.NewFromString(got)
	return err == nil && g.Equal(decimal.RequireFromString(want))
}

func ptrDecimal(d decimal.Decimal) *decimal.Decimal { return &d }
