package transactions_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestPendingCardInstallmentsCountOnceInTheirPeriods(t *testing.T) {
	for _, basis := range []string{settings.PeriodBasisOccurredAt, settings.PeriodBasisPaidAt} {
		t.Run(basis, func(t *testing.T) {
			f := newFixture(t)
			f.setPreference(settings.KeyTransactionsPeriodBasis, basis)
			card := f.addAccount(account{CurrencyCode: strp("BRL"), AccountType: strp("CREDIT")})
			var ids []string
			for i := 0; i < 3; i++ {
				occurred := time.Date(2026, time.March+time.Month(i), 15, 12, 0, 0, 0, time.UTC)
				billID := fmt.Sprintf("bill-%d", i+1)
				due := time.Date(2026, time.April+time.Month(i), 10, 0, 0, 0, 0, time.UTC)
				meta := fmt.Sprintf(`{"installmentNumber":%d,"totalInstallments":3,"billId":%q}`, i+1, billID)
				if i < 2 {
					f.addBill(bill{AccountID: card, ExternalID: billID, DueDate: due})
				} else {
					meta = `{"installmentNumber":3,"totalInstallments":3,"billForecastDate":"2026-06"}`
				}
				id := f.addTransaction(txn{
					AccountID: card, Description: strp("Compra Nubank sintética"),
					Amount: strp("-100.00"), AmountInAccountCurrency: strp("-100.00"), CurrencyCode: strp("BRL"),
					OccurredAt: &occurred, ProviderStatus: strp("PENDING"), MovementType: strp("DEBIT"),
					CreditCardMetadata: &meta,
				})
				f.setCategory(id, categorySupermercado)
				ids = append(ids, id)
				dates, err := transactions.FetchCardDueDates(context.Background(), f.conn)
				if err != nil {
					t.Fatal(err)
				}
				if got := dates.ProjectedEntryDate(card, occurred, &meta, false); !got.Equal(due) {
					t.Fatalf("installment %d projected on %s, want %s", i+1, got, due)
				}
			}
			for month := time.February; month <= time.July; month++ {
				filters := transactions.Filters{
					DateFrom: &money.Date{Year: 2026, Month: month, Day: 1},
					DateTo:   &money.Date{Year: 2026, Month: month, Day: 31},
				}
				// Use the actual last day for short months.
				filters.DateTo.Day = time.Date(2026, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
				result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
					Timezone: "UTC", GroupBy: money.GroupMonth, Page: 1, PageSize: 50, Filters: filters,
				})
				if err != nil {
					t.Fatal(err)
				}
				spending, err := transactions.SpendingByCategory(context.Background(), f.conn, filters, "UTC")
				if err != nil {
					t.Fatal(err)
				}
				firstMonth := time.March
				if basis == settings.PeriodBasisPaidAt {
					firstMonth = time.April
				}
				index := int(month - firstMonth)
				if index < 0 || index >= len(ids) {
					if len(result.Items) != 0 || len(result.Totals) != 0 || len(spending) != 0 {
						t.Fatalf("%s: unexpected expense: %+v / %+v", month, result, spending)
					}
					continue
				}
				if len(result.Items) != 1 || result.Items[0].ID != ids[index] || !result.Items[0].TotalsEligibility.Included ||
					len(result.Totals) != 1 || result.Totals[0].Outflow != "100.00" || result.Totals[0].Inflow != "0" {
					t.Fatalf("%s: want exactly one installment expense: %+v", month, result)
				}
				if len(spending) != 1 || spending[0].Amount != "100.00" {
					t.Fatalf("%s category spending = %+v, want 100.00", month, spending)
				}
			}
		})
	}
}

func TestPendingCardPurchaseAndBillSettlementHaveSeparateEffects(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	card := f.addAccount(account{CurrencyCode: strp("BRL"), AccountType: strp("CREDIT"), Balance: strp("-200.00")})
	bank := f.addAccount(account{CurrencyCode: strp("BRL"), AccountType: strp("BANK"), Balance: strp("1000.00")})
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	f.addBill(bill{AccountID: card, ExternalID: "previous-bill",
		ClosingDate: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
		DueDate:     time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)})
	f.addBill(bill{AccountID: card, ExternalID: "open-bill",
		ClosingDate: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
		DueDate:     time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)})
	f.addTransaction(txn{AccountID: card, Amount: strp("-200.00"), AmountInAccountCurrency: strp("-200.00"),
		CurrencyCode: strp("BRL"), OccurredAt: &now, ProviderStatus: strp("PENDING"),
		MovementType: strp("DEBIT"), CreditCardMetadata: strp(`{"billId":"open-bill"}`)})

	assertEffects := func(wantCash, wantDebt string) {
		t.Helper()
		result, err := transactions.Query(ctx, f.conn, transactions.QueryRequest{
			Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Totals) != 1 || result.Totals[0].Outflow != "200.00" || result.Totals[0].Inflow != "0" {
			t.Fatalf("expense counted again: %+v", result.Totals)
		}
		cash, err := transactions.CashOnHand(ctx, f.conn, nil)
		if err != nil || cash.String() != wantCash {
			t.Fatalf("cash = %s, %v; want %s", cash, err, wantCash)
		}
		debt, err := transactions.CreditCardTransactionTotalAt(ctx, f.conn, "BRL", now, time.UTC)
		if err != nil || debt.String() != wantDebt {
			t.Fatalf("card debt = %s, %v; want %s", debt, err, wantDebt)
		}
	}
	assertEffects("1000", "200")
	for _, leg := range []struct{ account, amount, movement string }{
		{bank, "-200.00", "DEBIT"}, {card, "200.00", "CREDIT"},
	} {
		id := f.addTransaction(txn{AccountID: leg.account, Amount: strp(leg.amount),
			AmountInAccountCurrency: strp(leg.amount), CurrencyCode: strp("BRL"), OccurredAt: &now,
			ProviderStatus: strp("POSTED"), MovementType: strp(leg.movement),
			CreditCardMetadata: strp(`{"billId":"open-bill"}`)})
		f.setCategory(id, categories.CardPaymentCategoryID)
		item, found, err := transactions.GetItem(ctx, f.conn, id)
		if err != nil || !found || item.TotalsEligibility.Included || !item.TotalsEligibility.MovesCash() {
			t.Fatalf("payment leg = %+v, found=%v, err=%v", item, found, err)
		}
	}
	// Synced cash changes when the provider reports the payment's new balance.
	assertEffects("1000", "0")
	f.exec("UPDATE financial_accounts SET balance = '800.00' WHERE id = ?", bank)
	assertEffects("800", "0")
}
