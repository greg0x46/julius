package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

func TestManualAccountAPIBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		request := func(body, id string, create bool) *httptest.ResponseRecorder {
			t.Helper()
			r := httptest.NewRequest("POST", "/api/accounts", bytes.NewBufferString(body))
			r.SetPathValue("id", id)
			w := httptest.NewRecorder()
			handleWriteManualAccount(conn, create)(w, r)
			return w
		}
		w := request(`{"name":"Manual EUR","currency_code":"EUR","opening_balance":"25"}`, "", true)
		if w.Code != 201 {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		var d accountDTO
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		if d.ExternalID != nil || d.SourceDisplayName != nil || *d.Balance != "25.00" {
			t.Fatalf("account: %+v", d)
		}
		w = request(`{"name":"Renamed USD","currency_code":"USD","opening_balance":"50"}`, d.ID, false)
		if w.Code != 200 {
			t.Fatalf("edit before movements: %d %s", w.Code, w.Body.String())
		}
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"amount":"-2","description":"Correction"}`))
		r.SetPathValue("id", d.ID)
		w = httptest.NewRecorder()
		handleAdjustManualAccount(conn)(w, r)
		if w.Code != 200 {
			t.Fatalf("adjust: %d %s", w.Code, w.Body.String())
		}
		w = request(`{"name":"Renamed","currency_code":"EUR","opening_balance":"50"}`, d.ID, false)
		if w.Code != 409 {
			t.Fatalf("unsafe edit: %d %s", w.Code, w.Body.String())
		}
		w = request(`{"name":"Archived","currency_code":"USD","opening_balance":"50","archived":true}`, d.ID, false)
		if w.Code != 200 {
			t.Fatalf("archive: %d %s", w.Code, w.Body.String())
		}
		r = httptest.NewRequest("GET", "/api/accounts", nil)
		w = httptest.NewRecorder()
		handleListAccounts(conn)(w, r)
		if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"balance":"48.00"`)) {
			t.Fatalf("read: %d %s", w.Code, w.Body.String())
		}
		for _, body := range []string{`{"name":"","currency_code":"BRL"}`, `{"name":"Invalid","currency_code":"ZZZ"}`, `{"name":"Invalid","opening_balance":"NaN"}`} {
			w = request(body, "", true)
			if w.Code != 422 {
				t.Fatalf("validation: %d %s", w.Code, w.Body.String())
			}
		}
	})
}
