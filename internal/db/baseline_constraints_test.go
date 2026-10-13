package db_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

func TestBaselineExactDecimalChecks(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		now := db.FormatTime(time.Now())
		for _, amount := range []string{"0." + strings.Repeat("0", 400) + "1", strings.Repeat("9", 100) + ".123456789012345678901"} {
			id := uuid.NewString()
			if _, err := conn.Exec(`INSERT INTO payables(id,kind,name,total_amount,created_at,updated_at) VALUES(?,'debt','Synthetic debt',?,?,?)`, id, amount, now, now); err != nil {
				t.Fatalf("exact positive %s rejected: %v", amount, err)
			}
			var stored string
			if err := conn.QueryRow(`SELECT total_amount FROM payables WHERE id=?`, id).Scan(dbtypes.Decimal(&stored)); err != nil {
				t.Fatal(err)
			}
			if stored != amount {
				t.Fatalf("amount changed from %s to %s", amount, stored)
			}
		}
		for _, amount := range []string{"0", "-1", "invalid"} {
			if _, err := conn.Exec(`INSERT INTO payables(id,kind,name,total_amount,created_at,updated_at) VALUES(?,'debt','Invalid debt',?,?,?)`, uuid.NewString(), amount, now, now); err == nil {
				t.Fatalf("accepted amount %s", amount)
			}
		}
	})
}

func TestBaselineCurrencyAndEnumChecks(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		now := db.FormatTime(time.Now())
		for _, currency := range []string{"brl", "BR", "BRLL", "123"} {
			if _, err := conn.Exec(`INSERT INTO financial_accounts(id,origin,account_type,currency_code,opening_balance,created_at,updated_at) VALUES(?,'manual','BANK',?,'0',?,?)`, uuid.NewString(), currency, now, now); err == nil {
				t.Fatalf("accepted currency %q", currency)
			}
		}
		if _, err := conn.Exec(`INSERT INTO categories(id,name,kind,created_at,updated_at) VALUES(?,'Invalid category','unknown',?,?)`, uuid.NewString(), now, now); err == nil {
			t.Fatal("accepted unknown category kind")
		}
		if _, err := conn.Exec(`INSERT INTO categories(id,name,kind,is_active,created_at,updated_at) VALUES(?,'Invalid flag','expense',2,?,?)`, uuid.NewString(), now, now); err == nil {
			t.Fatal("accepted a flag outside 0/1")
		}
	})
}
