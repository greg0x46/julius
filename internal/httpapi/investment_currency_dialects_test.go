package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/investments"
)

func TestManualPositionInAnotherCurrencyIsRefusedBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Corretora"})
		if err != nil {
			t.Fatal(err)
		}
		// A catalog entry in dollars: a holding opened and closed, then re-quoted.
		ticker := "AAPL"
		opened, err := investments.CreatePosition(ctx, conn, investments.PositionInput{AccountID: account.ID, Name: "Apple", Ticker: &ticker, AssetType: "Ação"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`UPDATE investment_assets SET currency_code = 'USD' WHERE id = ?`, *opened.AssetID); err != nil {
			t.Fatal(err)
		}
		if err := investments.DeletePosition(ctx, conn, opened.ID); err != nil {
			t.Fatal(err)
		}
		usd, err := investments.GetAsset(ctx, conn, *opened.AssetID)
		if err != nil {
			t.Fatal(err)
		}

		body, _ := json.Marshal(map[string]any{"account_id": account.ID, "asset_id": usd.ID, "name": "Apple", "asset_type": "Ação"})
		w := httptest.NewRecorder()
		handleCreateInvestmentPosition(conn)(w, httptest.NewRequest("POST", "/api/investment-positions", bytes.NewReader(body)))
		var problem Problem
		if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
			t.Fatalf("body %q: %v", w.Body.String(), err)
		}
		if w.Code != 400 || problem.Type != "/problems/investment-unsupported-currency" || problem.Detail == "" {
			t.Fatalf("status %d, problem %+v", w.Code, problem)
		}
		positions, err := investments.ListPositions(ctx, conn, investments.PositionFilter{})
		if err != nil || len(positions) != 0 {
			t.Fatalf("positions = %+v, %v; want none", positions, err)
		}
	})
}
