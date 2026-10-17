package types_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db/dbtest"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

func TestDriverValues(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		query := `SELECT '2026-10-11T09:30:00.123456789-03:00', '2026-10-11', 'b438a2c8-6c7d-4e94-8e2f-5c230900671a', TRUE, '12345678901234567890.123456789', NULL`
		if dbtest.IsPostgres(conn) {
			query = `SELECT TIMESTAMPTZ '2026-10-11T09:30:00.123456-03:00', DATE '2026-10-11', UUID 'b438a2c8-6c7d-4e94-8e2f-5c230900671a', TRUE, NUMERIC '12345678901234567890.123456789', NULL`
		}
		var stamp, date time.Time
		var id string
		var active bool
		var amount decimal.Decimal
		optional := new(time.Time)
		if err := conn.QueryRow(query).Scan(dbtypes.Time(&stamp), dbtypes.Date(&date), dbtypes.ID(&id), dbtypes.Bool(&active), dbtypes.Decimal(&amount), dbtypes.Time(&optional)); err != nil {
			t.Fatal(err)
		}
		if stamp.Hour() != 12 || stamp.Minute() != 30 || stamp.Location() != time.UTC {
			t.Fatal(stamp)
		}
		if date.Format(dbtypes.DateLayout) != "2026-10-11" || date.Hour() != 0 {
			t.Fatal(date)
		}
		if id != "b438a2c8-6c7d-4e94-8e2f-5c230900671a" {
			t.Fatal(id)
		}
		wantPrecision := 123456789
		if dbtest.IsPostgres(conn) {
			wantPrecision = 123456000
		}
		if stamp.Nanosecond() != wantPrecision {
			t.Fatal(stamp)
		}
		if !active || amount.String() != "12345678901234567890.123456789" || optional != nil {
			t.Fatal(active, amount, optional)
		}
		var encodedTime, encodedDate, encodedID, encodedAmount, absent sql.NullString
		var activeInt int
		if err := conn.QueryRow(query).Scan(dbtypes.Time(&encodedTime), dbtypes.Date(&encodedDate), dbtypes.ID(&encodedID), dbtypes.Bool(&activeInt), dbtypes.Decimal(&encodedAmount), dbtypes.Time(&absent)); err != nil {
			t.Fatal(err)
		}
		if !encodedTime.Valid || encodedTime.String != stamp.Format(dbtypes.TimeLayout) || encodedDate.String != "2026-10-11" || encodedID.String != id || activeInt != 1 || encodedAmount.String != amount.String() || absent.Valid {
			t.Fatal(encodedTime, encodedDate, encodedID, activeInt, encodedAmount, absent)
		}
		if err := conn.QueryRow(`SELECT 'invalid'`).Scan(dbtypes.Date(&date)); err == nil {
			t.Fatal("accepted malformed date")
		}
		if err := conn.QueryRow(`SELECT NULL`).Scan(dbtypes.Bool(&active)); err == nil {
			t.Fatal("accepted null required boolean")
		}
	})
}
