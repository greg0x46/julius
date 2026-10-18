package transactions_test

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/transactions"
)

// queryMonthIn is queryMonthOf in an explicit request timezone.
func queryMonthIn(t *testing.T, f *fixture, tz string, year int, month time.Month) []transactions.Item {
	t.Helper()
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: tz, GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		Filters: transactions.Filters{
			DateFrom: &money.Date{Year: year, Month: month, Day: 1},
			DateTo:   &money.Date{Year: year, Month: month, Day: last},
		},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	return result.Items
}

// A bill due on the 1st is a calendar day, not a UTC instant: west of UTC it
// must still count toward the month it falls due in.
func TestPeriodBasisPaidAtBillDueDateIsACalendarDayInRequestZone(t *testing.T) {
	for _, tz := range []string{"UTC", "America/Sao_Paulo"} {
		t.Run(tz, func(t *testing.T) {
			f := newFixture(t)
			f.setPreference(settings.KeyTransactionsPeriodBasis, settings.PeriodBasisPaidAt)
			acc := f.addAccount(account{CurrencyCode: strp("BRL"), AccountType: strp("CREDIT")})
			f.addBill(bill{AccountID: acc, ExternalID: "bill-1", DueDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)})
			occurred := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
			id := f.addTransaction(txn{
				AccountID: acc, Amount: strp("-10.00"), AmountInAccountCurrency: strp("-10.00"),
				CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"),
				MovementType: strp("DEBIT"), CreditCardMetadata: strp(`{"billId":"bill-1"}`),
			})

			if got := queryMonthIn(t, f, tz, 2026, time.April); len(got) != 1 || got[0].ID != id {
				t.Fatalf("April items = %+v, want the transaction", got)
			}
			if got := queryMonthIn(t, f, tz, 2026, time.March); len(got) != 0 {
				t.Fatalf("March items = %+v, want none", got)
			}
		})
	}
}

// Estimating "one month after the purchase" must clamp to the end of a
// shorter month instead of overflowing into the next one.
func TestPeriodBasisPaidAtEstimateClampsToEndOfMonth(t *testing.T) {
	for _, tz := range []string{"UTC", "America/Sao_Paulo"} {
		t.Run(tz, func(t *testing.T) {
			f := newFixture(t)
			f.setPreference(settings.KeyTransactionsPeriodBasis, settings.PeriodBasisPaidAt)
			acc := f.addAccount(account{CurrencyCode: strp("BRL"), AccountType: strp("CREDIT")})
			occurred := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
			id := f.addTransaction(txn{
				AccountID: acc, Amount: strp("-10.00"), AmountInAccountCurrency: strp("-10.00"),
				CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"),
				MovementType: strp("DEBIT"),
			})

			if got := queryMonthIn(t, f, tz, 2026, time.February); len(got) != 1 || got[0].ID != id {
				t.Fatalf("February items = %+v, want the transaction", got)
			}
			if got := queryMonthIn(t, f, tz, 2026, time.March); len(got) != 0 {
				t.Fatalf("March items = %+v, want none", got)
			}
		})
	}
}
