package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
)

func TestManualAccountOpeningBalanceUsesDecimalEquality(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		for _, tc := range []struct {
			name, stored, requested string
			wantStatus              int
		}{
			{"trailing zeros", "100.00", "100", 200},
			{"negative", "-100.00", "-100", 200},
			{"zero", "0.00", "0", 200},
			{"fractional precision", "9007199254740993.00100", "9007199254740993.001", 200},
			{"changed amount", "100.00", "101", 409},
			{"changed fractional amount", "9007199254740993.00100", "9007199254740993.00101", 409},
		} {
			t.Run(tc.name, func(t *testing.T) {
				id := uuid.NewString()
				now := db.FormatTime(time.Now())
				if _, err := conn.Exec(`INSERT INTO financial_accounts
					(id, origin, name, account_type, currency_code, opening_balance, created_at, updated_at)
					VALUES (?, 'manual', 'Original', 'BANK', 'BRL', ?, ?, ?)`, id, tc.stored, now, now); err != nil {
					t.Fatal(err)
				}
				if _, err := conn.Exec(`INSERT INTO account_balance_adjustments
					(id, account_id, amount, description, created_at) VALUES (?, ?, '1', 'Synthetic adjustment', ?)`, uuid.NewString(), id, now); err != nil {
					t.Fatal(err)
				}
				body, err := json.Marshal(manualAccountRequest{Name: "Renamed", CurrencyCode: "BRL", OpeningBalance: tc.requested, Archived: true})
				if err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRequest("PUT", "/api/accounts/"+id, bytes.NewReader(body))
				r.SetPathValue("id", id)
				w := httptest.NewRecorder()
				handleWriteManualAccount(conn, false)(w, r)
				if w.Code != tc.wantStatus {
					t.Fatalf("update: %d %s, want %d", w.Code, w.Body.String(), tc.wantStatus)
				}
				if tc.wantStatus == 200 {
					var got accountDTO
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					wantBalance := decimal.RequireFromString(tc.stored).Add(decimal.NewFromInt(1))
					if got.Name == nil || *got.Name != "Renamed" || got.ArchivedAt == nil || got.Balance == nil || !decimal.RequireFromString(*got.Balance).Equal(wantBalance) {
						t.Fatalf("updated account: %+v", got)
					}
				} else if !bytes.Contains(w.Body.Bytes(), []byte("account-has-movements")) {
					t.Fatalf("unexpected conflict: %s", w.Body.String())
				}
			})
		}
	})
}

func TestManualAccountRejectsInvalidStoredOpeningBalance(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	if _, err := conn.Exec(`INSERT INTO financial_accounts
		(id, origin, name, account_type, currency_code, opening_balance, created_at, updated_at)
		VALUES (?, 'manual', 'Original', 'BANK', 'BRL', 'invalid', ?, ?)`, id, now, now); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("PUT", "/api/accounts/"+id, bytes.NewBufferString(`{"name":"Renamed","opening_balance":"0"}`))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	handleWriteManualAccount(conn, false)(w, r)
	if w.Code != 503 {
		t.Fatalf("invalid stored decimal: %d %s", w.Code, w.Body.String())
	}
	var name, opening string
	if err := conn.QueryRow(`SELECT name, opening_balance FROM financial_accounts WHERE id = ?`, id).Scan(&name, &opening); err != nil {
		t.Fatal(err)
	}
	if name != "Original" || opening != "invalid" {
		t.Fatalf("invalid account changed: name=%q opening=%q", name, opening)
	}
}
