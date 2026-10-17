package investments_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
)

type bookFixture struct {
	t           *testing.T
	conn        *sql.DB
	sourceID    string
	rawImportID string
}

func newBookFixture(t *testing.T) *bookFixture {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	f := &bookFixture{t: t, conn: conn, sourceID: uuid.NewString(), rawImportID: uuid.NewString()}
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO data_sources (id, provider, external_item_id, label, created_at, updated_at)
		VALUES (?, 'pluggy', 'item-1', 'Corretora', ?, ?)`, f.sourceID, now, now)
	syncRunID := uuid.NewString()
	f.exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, syncRunID, f.sourceID, now, now)
	f.exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'accounts', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		f.rawImportID, syncRunID, f.sourceID, now)
	return f
}

func (f *bookFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.conn.Exec(query, args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

func (f *bookFixture) addFinancialAccount(balance string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, account_type, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, 'BANK', ?, 'BRL', ?, 'hash', ?, ?)`,
		id, f.sourceID, id, balance, f.rawImportID, now, now)
	return id
}

// addFinancialInvestment writes a provider holding. quantity/value are the
// provider's own columns; the fixture never writes a cost basis because the
// provider does not supply a trustworthy one.
func (f *bookFixture) addFinancialInvestment(balance, quantity, value string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var quantityArg, valueArg any
	if quantity != "" {
		quantityArg = quantity
	}
	if value != "" {
		valueArg = value
	}
	f.exec(`INSERT INTO financial_investments (
			id, source_id, external_id, name, investment_type, balance, quantity, value, currency_code,
			as_of_date, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, 'Tesouro Selic', 'FIXED_INCOME', ?, ?, ?, 'BRL', ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, id, balance, quantityArg, valueArg, now, f.rawImportID, now, now)
	return id
}

func (f *bookFixture) addManualAccount(name string) string {
	f.t.Helper()
	account, err := investments.CreateAccount(context.Background(), f.conn, investments.AccountInput{Name: name})
	if err != nil {
		f.t.Fatalf("CreateAccount: %v", err)
	}
	return account.ID
}

func (f *bookFixture) addOperation(accountID, positionID, kind, occurredOn, amount, quantity string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var positionArg, quantityArg any
	if positionID != "" {
		positionArg = positionID
	}
	if quantity != "" {
		quantityArg = quantity
	}
	f.exec(`INSERT INTO investment_operations (
			id, account_id, position_id, kind, occurred_on, amount, quantity, fees, taxes, source, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, '0', '0', 'manual', ?, ?)`,
		id, accountID, positionArg, kind, occurredOn, amount, quantityArg, now, now)
	return id
}

func (f *bookFixture) positions(filter investments.PositionFilter) []investments.Position {
	f.t.Helper()
	list, err := investments.ListPositions(context.Background(), f.conn, filter)
	if err != nil {
		f.t.Fatalf("ListPositions: %v", err)
	}
	return list
}

func (f *bookFixture) summary() investments.Summary {
	f.t.Helper()
	summary, err := investments.BuildSummary(context.Background(), f.conn)
	if err != nil {
		f.t.Fatalf("BuildSummary: %v", err)
	}
	return summary
}

func bookDecimalString(d *decimal.Decimal) string {
	if d == nil {
		return "<nil>"
	}
	return d.String()
}

func bookDec(value string) decimal.Decimal {
	d, err := decimal.NewFromString(value)
	if err != nil {
		panic(err)
	}
	return d
}

func bookDecPtr(value string) *decimal.Decimal {
	d := bookDec(value)
	return &d
}

func TestCreatePositionReplaysQuantityAndCost(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	created, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       accountID,
		Name:            "Ações",
		AssetType:       "Ativo",
		InitialQuantity: bookDec("10"),
		InitialUnitCost: bookDec("25"),
		OccurredOn:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	if created.Quantity.String() != "10" || bookDecimalString(created.AverageCost) != "25" || created.CurrentValue.String() != "250" {
		t.Fatalf("created = %+v", created)
	}
	// No valuation was recorded, so the holding must report cost, not a quote.
	if created.ValuationBasis != investments.ValuationBasisCostBasis || created.CurrentUnitPrice != nil || created.ValuedOn != nil {
		t.Fatalf("invented a quote: %+v", created)
	}
	if created.Source != investments.PositionSourceManual || created.LinkedInvestmentID != nil || created.Closed {
		t.Fatalf("source = %+v", created)
	}
	read, err := investments.GetPosition(ctx, f.conn, created.ID)
	if err != nil || read.CurrentValue.String() != "250" {
		t.Fatalf("GetPosition = %+v err=%v", read, err)
	}
}

func TestSameAssetCanBeHeldInDifferentAccounts(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	firstAccount, err := investments.CreateAccount(ctx, f.conn, investments.AccountInput{Name: "Corretora A"})
	if err != nil {
		t.Fatal(err)
	}
	secondAccount, err := investments.CreateAccount(ctx, f.conn, investments.AccountInput{Name: "Corretora B"})
	if err != nil {
		t.Fatal(err)
	}
	ticker := "BTC"
	first, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{AccountID: firstAccount.ID, Name: "Bitcoin", Ticker: &ticker, AssetType: "Criptoativo"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{AccountID: secondAccount.ID, Name: "Bitcoin", Ticker: &ticker, AssetType: "Criptoativo"})
	if err != nil {
		t.Fatal(err)
	}
	if first.AssetID == nil || second.AssetID == nil || *first.AssetID != *second.AssetID {
		t.Fatalf("positions should share asset: %#v %#v", first.AssetID, second.AssetID)
	}
	if _, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{AccountID: firstAccount.ID, Name: "Bitcoin", Ticker: &ticker, AssetType: "Criptoativo"}); !errors.Is(err, investments.ErrPositionAlreadyExists) {
		t.Fatalf("duplicate position error = %v", err)
	}
}

func TestCreatePositionFromValueOnlyKeepsTheAmountInvested(t *testing.T) {
	f := newBookFixture(t)
	accountID := f.addManualAccount("Custódia")
	created, err := investments.CreatePosition(context.Background(), f.conn, investments.PositionInput{
		AccountID:    accountID,
		Name:         "CDB",
		AssetType:    "Renda fixa",
		InitialValue: bookDecPtr("1500"),
		OccurredOn:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	if created.CurrentValue.String() != "1500" || created.Quantity.String() != "1" || bookDecimalString(created.AverageCost) != "1500" {
		t.Fatalf("created = %+v", created)
	}
}

func TestManualValuationMovesValueAwayFromCost(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	created, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       accountID,
		Name:            "Ações",
		AssetType:       "Ativo",
		InitialQuantity: bookDec("10"),
		InitialUnitCost: bookDec("25"),
		OccurredOn:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	f.addOperation(accountID, created.ID, "valuation", "2026-09-10", "300", "")
	valued, err := investments.GetPosition(ctx, f.conn, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if valued.CurrentValue.String() != "300" || valued.ValuationBasis != investments.ValuationBasisManualValuation {
		t.Fatalf("valued = %+v", valued)
	}
	if bookDecimalString(valued.CurrentUnitPrice) != "30" || bookDecimalString(valued.AverageCost) != "25" {
		t.Fatalf("unit price = %+v", valued)
	}
	if valued.ValuedOn == nil || valued.ValuedOn.Format("2006-01-02") != "2026-09-10" {
		t.Fatalf("valued on = %v", valued.ValuedOn)
	}
	summary := f.summary()
	if summary.UnrealizedGain.String() != "50" || summary.ManualValue.String() != "300" {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestClosedManualPositionIsHiddenUnlessRequested(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	created, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       accountID,
		Name:            "Ações",
		AssetType:       "Ativo",
		InitialQuantity: bookDec("10"),
		InitialUnitCost: bookDec("25"),
		OccurredOn:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	f.addOperation(accountID, created.ID, "sell", "2026-09-05", "300", "10")
	if open := f.positions(investments.PositionFilter{AccountID: &accountID}); len(open) != 0 {
		t.Fatalf("closed position listed: %+v", open)
	}
	closed := f.positions(investments.PositionFilter{AccountID: &accountID, IncludeClosed: true})
	if len(closed) != 1 || !closed[0].Closed || closed[0].CurrentValue.String() != "0" {
		t.Fatalf("closed = %+v", closed)
	}
	if err := investments.DeletePosition(ctx, f.conn, created.ID); !errors.Is(err, investments.ErrPositionHasOperations) {
		t.Fatalf("delete = %v", err)
	}
}

func TestSyncedPositionIsReadOnlyAndCountedOnce(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	investmentID := f.addFinancialInvestment("2000", "4", "500")
	list := f.positions(investments.PositionFilter{})
	if len(list) != 1 {
		t.Fatalf("positions = %+v", list)
	}
	synced := list[0]
	if synced.ID != investmentID || synced.Source != investments.PositionSourceSynced ||
		synced.LinkedInvestmentID == nil || *synced.LinkedInvestmentID != investmentID {
		t.Fatalf("synced = %+v", synced)
	}
	if synced.AccountID != "integrated:"+f.sourceID || synced.ValuationBasis != investments.ValuationBasisProviderBalance {
		t.Fatalf("account = %+v", synced)
	}
	if synced.CurrentValue.String() != "2000" || synced.Quantity.String() != "4" || bookDecimalString(synced.CurrentUnitPrice) != "500" {
		t.Fatalf("values = %+v", synced)
	}
	// The provider supplies no usable cost, so no gain may be reported.
	if synced.AverageCost != nil {
		t.Fatalf("invented a cost basis: %v", synced.AverageCost)
	}
	summary := f.summary()
	if summary.SyncedValue.String() != "2000" || summary.TotalValue.String() != "2000" || !summary.UnrealizedGain.IsZero() {
		t.Fatalf("summary = %+v", summary)
	}
	integrated := 0
	for _, account := range summary.Accounts {
		if account.AccountID == "integrated:"+f.sourceID {
			integrated++
			if account.CurrentValue.String() != "2000" || !account.CashBalance.IsZero() {
				t.Fatalf("integrated account = %+v", account)
			}
		}
	}
	if integrated != 1 {
		t.Fatalf("integrated accounts = %d", integrated)
	}
	_, err := investments.UpdatePosition(ctx, f.conn, investmentID, investments.PositionUpdate{
		Name: "Outro nome", AssetType: synced.AssetType,
	})
	if !errors.Is(err, investments.ErrIntegratedReadOnly) {
		t.Fatalf("rename = %v", err)
	}
	if err := investments.DeletePosition(ctx, f.conn, investmentID); !errors.Is(err, investments.ErrIntegratedReadOnly) {
		t.Fatalf("delete = %v", err)
	}
}

func TestGoalAssignmentMovesNoMoney(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	investmentID := f.addFinancialInvestment("2000", "4", "500")
	manual, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       accountID,
		Name:            "Ações",
		AssetType:       "Ativo",
		InitialQuantity: bookDec("10"),
		InitialUnitCost: bookDec("25"),
		OccurredOn:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	before := f.summary()
	goal, err := investments.CreatePortfolio(ctx, f.conn, investments.PortfolioInput{Name: "Reserva", TargetAmount: bookDecPtr("5000")})
	if err != nil {
		t.Fatalf("CreatePortfolio: %v", err)
	}
	if _, err := investments.UpdatePosition(ctx, f.conn, manual.ID, investments.PositionUpdate{
		Name: manual.Name, AssetType: manual.AssetType, PortfolioID: &goal.ID,
	}); err != nil {
		t.Fatalf("assign manual: %v", err)
	}
	synced, err := investments.GetPosition(ctx, f.conn, investmentID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := investments.UpdatePosition(ctx, f.conn, investmentID, investments.PositionUpdate{
		Name: synced.Name, AssetType: synced.AssetType, Ticker: synced.Ticker, PortfolioID: &goal.ID,
	}); err != nil {
		t.Fatalf("assign synced: %v", err)
	}
	assigned := f.positions(investments.PositionFilter{PortfolioID: &goal.ID})
	if len(assigned) != 2 {
		t.Fatalf("assigned = %+v", assigned)
	}
	after := f.summary()
	if after.TotalValue.String() != before.TotalValue.String() || after.CashBalance.String() != before.CashBalance.String() ||
		after.ManualValue.String() != before.ManualValue.String() || after.SyncedValue.String() != before.SyncedValue.String() {
		t.Fatalf("goal changed balances: before=%+v after=%+v", before, after)
	}
	reloaded, err := investments.GetPortfolio(ctx, f.conn, goal.ID)
	if err != nil || reloaded.CurrentValue.String() != "2250" {
		t.Fatalf("portfolio = %+v err=%v", reloaded, err)
	}
}

func TestDeletePositionWithoutOperations(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	created, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID: accountID, Name: "Ações", AssetType: "Ativo",
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	if created.Quantity.String() != "0" || created.AverageCost != nil || created.Closed {
		t.Fatalf("empty position = %+v", created)
	}
	if err := investments.DeletePosition(ctx, f.conn, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := investments.GetPosition(ctx, f.conn, created.ID); !errors.Is(err, investments.ErrPositionNotFound) {
		t.Fatalf("get after delete = %v", err)
	}
}

// TestGetPositionMatchesListPositions pins GetPosition to the list derivation:
// the single read replays only the holding's own account, and that must yield
// the same quantity, cost, valuation and goal as the full list does.
func TestGetPositionMatchesListPositions(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	custodyID := f.addManualAccount("Custódia")
	brokerID := f.addManualAccount("Corretora manual")
	goal, err := investments.CreatePortfolio(ctx, f.conn, investments.PortfolioInput{Name: "Reserva"})
	if err != nil {
		t.Fatalf("CreatePortfolio: %v", err)
	}
	valued, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       custodyID,
		Name:            "Ações",
		Ticker:          ptrString("ABCD3"),
		AssetType:       "Ativo",
		Notes:           ptrString("carteira principal"),
		InitialQuantity: bookDec("10"),
		InitialUnitCost: bookDec("25"),
		OccurredOn:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PortfolioID:     &goal.ID,
	})
	if err != nil {
		t.Fatalf("CreatePosition valued: %v", err)
	}
	f.addOperation(custodyID, "", "deposit", "2026-09-02", "350", "")
	f.addOperation(custodyID, valued.ID, "buy", "2026-09-03", "350", "10")
	f.addOperation(custodyID, valued.ID, "valuation", "2026-09-10", "700", "")
	closed, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       brokerID,
		Name:            "FII",
		AssetType:       "Fundo",
		InitialQuantity: bookDec("4"),
		InitialUnitCost: bookDec("100"),
		OccurredOn:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition closed: %v", err)
	}
	f.addOperation(brokerID, closed.ID, "sell", "2026-09-05", "420", "4")
	empty, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID: brokerID, Name: "Sem operações", AssetType: "Ativo",
	})
	if err != nil {
		t.Fatalf("CreatePosition empty: %v", err)
	}
	investmentID := f.addFinancialInvestment("2000", "4", "500")
	if _, err := investments.UpdatePosition(ctx, f.conn, investmentID, investments.PositionUpdate{
		Name: "Tesouro Selic", AssetType: "FIXED_INCOME", PortfolioID: &goal.ID,
	}); err != nil {
		t.Fatalf("assign synced: %v", err)
	}

	listed := f.positions(investments.PositionFilter{IncludeClosed: true})
	if len(listed) != 4 {
		t.Fatalf("positions = %+v", listed)
	}
	for _, want := range listed {
		got, err := investments.GetPosition(ctx, f.conn, want.ID)
		if err != nil {
			t.Fatalf("GetPosition(%s): %v", want.ID, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetPosition(%s) = %+v, list has %+v", want.ID, got, want)
		}
	}
	// The values themselves must be the interesting ones, or the equality
	// above would only prove that two empty derivations agree.
	byID := map[string]investments.Position{}
	for _, position := range listed {
		byID[position.ID] = position
	}
	if p := byID[valued.ID]; p.Quantity.String() != "20" || bookDecimalString(p.AverageCost) != "30" ||
		p.CurrentValue.String() != "700" || p.ValuationBasis != investments.ValuationBasisManualValuation ||
		p.PortfolioID == nil || *p.PortfolioID != goal.ID {
		t.Fatalf("valued = %+v", p)
	}
	if p := byID[closed.ID]; !p.Closed || p.Quantity.String() != "0" {
		t.Fatalf("closed = %+v", p)
	}
	if p := byID[empty.ID]; p.Closed || p.AverageCost != nil {
		t.Fatalf("empty = %+v", p)
	}
	if p := byID[investmentID]; p.Source != investments.PositionSourceSynced || p.CurrentValue.String() != "2000" ||
		p.PortfolioID == nil || *p.PortfolioID != goal.ID {
		t.Fatalf("synced = %+v", p)
	}
}

func ptrString(v string) *string {
	return &v
}

// TestPositionValuesAreExactTotals pins the ledger to totals rather than unit
// figures: 3 units bought for R$ 100 have a non-terminating unit cost, and
// multiplying it back would report 99.9999999999999999 to the API and to the
// net worth snapshot.
func TestPositionValuesAreExactTotals(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	thirds, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       accountID,
		Name:            "Fundo",
		AssetType:       "Fundo",
		InitialQuantity: bookDec("3"),
		InitialValue:    bookDecPtr("100"),
		OccurredOn:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition thirds: %v", err)
	}
	if thirds.CurrentValue.String() != "100" || bookDecimalString(thirds.TotalCost) != "100" ||
		bookDecimalString(thirds.AverageCost) != "33.3333333333333333" {
		t.Fatalf("thirds at cost = value %s cost %s avg %s", thirds.CurrentValue, bookDecimalString(thirds.TotalCost), bookDecimalString(thirds.AverageCost))
	}
	f.addOperation(accountID, thirds.ID, "valuation", "2026-09-10", "1000", "")
	valued, err := investments.GetPosition(ctx, f.conn, thirds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if valued.CurrentValue.String() != "1000" || bookDecimalString(valued.CurrentUnitPrice) != "333.3333333333333333" {
		t.Fatalf("thirds valued = value %s unit %s", valued.CurrentValue, bookDecimalString(valued.CurrentUnitPrice))
	}
	if summary := f.summary(); summary.UnrealizedGain.String() != "900" || summary.ManualValue.String() != "1000" {
		t.Fatalf("summary = gain %s manual %s", summary.UnrealizedGain, summary.ManualValue)
	}

	// A holding whose unit cost does terminate must be exact too, and the
	// quote must keep pricing the remaining units after a partial sale.
	hundreds, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID:       accountID,
		Name:            "Ações",
		AssetType:       "Ativo",
		InitialQuantity: bookDec("3"),
		InitialUnitCost: bookDec("100"),
		OccurredOn:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition hundreds: %v", err)
	}
	if hundreds.CurrentValue.String() != "300" || bookDecimalString(hundreds.AverageCost) != "100" {
		t.Fatalf("hundreds = %+v", hundreds)
	}
	f.addOperation(accountID, hundreds.ID, "valuation", "2026-09-10", "900", "")
	f.addOperation(accountID, hundreds.ID, "sell", "2026-09-12", "310", "1")
	sold, err := investments.GetPosition(ctx, f.conn, hundreds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sold.Quantity.String() != "2" || sold.CurrentValue.String() != "600" || bookDecimalString(sold.TotalCost) != "200" ||
		bookDecimalString(sold.AverageCost) != "100" || bookDecimalString(sold.CurrentUnitPrice) != "300" {
		t.Fatalf("after partial sale = %+v", sold)
	}
}

// TestUpdatePositionRefusesAnotherAssetsIdentity: the asset behind a manual
// position is shared by every holding of that instrument, so renaming it onto
// an identity the catalog already has is a conflict, not a storage failure.
func TestUpdatePositionRefusesAnotherAssetsIdentity(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	accountID := f.addManualAccount("Custódia")
	vale, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID: accountID, Name: "Vale", Ticker: ptrString("VALE3"), AssetType: "Ação",
	})
	if err != nil {
		t.Fatalf("CreatePosition VALE3: %v", err)
	}
	if _, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID: accountID, Name: "Petrobras", Ticker: ptrString("PETR4"), AssetType: "Ação",
	}); err != nil {
		t.Fatalf("CreatePosition PETR4: %v", err)
	}
	_, err = investments.UpdatePosition(ctx, f.conn, vale.ID, investments.PositionUpdate{
		Name: "Vale", Ticker: ptrString("PETR4"), AssetType: "Ação",
	})
	if !errors.Is(err, investments.ErrAssetAlreadyExists) {
		t.Fatalf("rename onto PETR4 = %v, want ErrAssetAlreadyExists", err)
	}
	kept, err := investments.GetPosition(ctx, f.conn, vale.ID)
	if err != nil || kept.Ticker == nil || *kept.Ticker != "VALE3" {
		t.Fatalf("refused update changed the asset: %+v err=%v", kept, err)
	}
	// Restating the position's own identity is not a collision.
	if _, err := investments.UpdatePosition(ctx, f.conn, vale.ID, investments.PositionUpdate{
		Name: "Vale S.A.", Ticker: ptrString("VALE3"), AssetType: "Ação",
	}); err != nil {
		t.Fatalf("rename keeping the ticker: %v", err)
	}
}

// quotedAsset registers an asset a market connector prices.
func (f *bookFixture) quotedAsset(name, ticker string) investments.Asset {
	f.t.Helper()
	source := "crypto"
	asset, err := investments.CreateAsset(context.Background(), f.conn, investments.AssetInput{
		Name: name, Ticker: &ticker, AssetType: "Criptoativo", CurrencyCode: "BRL", QuoteSource: &source,
	})
	if err != nil {
		f.t.Fatalf("CreateAsset: %v", err)
	}
	return asset
}

func (f *bookFixture) quoteOn(assetID, day, price string) {
	f.t.Helper()
	on, err := time.Parse(investments.DateLayout, day)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := investments.UpsertAssetQuote(context.Background(), f.conn, investments.AssetQuote{
		AssetID: assetID, QuotedOn: on, Price: bookDec(price), Source: "yahoo", Origin: investments.QuoteOriginMarket,
	}); err != nil {
		f.t.Fatalf("UpsertAssetQuote: %v", err)
	}
}

// openQuoted opens a position of quantity units bought on 2026-09-01.
func (f *bookFixture) openQuoted(accountID string, asset investments.Asset, quantity, unitCost string) investments.Position {
	f.t.Helper()
	position, err := investments.CreatePosition(context.Background(), f.conn, investments.PositionInput{
		AccountID: accountID, AssetID: &asset.ID, Name: asset.Name, AssetType: asset.AssetType,
		InitialQuantity: bookDec(quantity), InitialUnitCost: bookDec(unitCost),
		OccurredOn: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		f.t.Fatalf("CreatePosition: %v", err)
	}
	return position
}

func (f *bookFixture) position(id string) investments.Position {
	f.t.Helper()
	position, err := investments.GetPosition(context.Background(), f.conn, id)
	if err != nil {
		f.t.Fatalf("GetPosition: %v", err)
	}
	return position
}

// A manual holding of an asset a connector prices is worth quantity × the
// latest price, derived on every read and rounded to cents; no operation backs
// it, and the cost basis stays what was paid.
func TestManualPositionOfAQuotedAssetIsValuedAtItsLatestQuote(t *testing.T) {
	f := newBookFixture(t)
	asset := f.quotedAsset("Bitcoin", "BTC")
	position := f.openQuoted(f.addManualAccount("Corretora"), asset, "3", "30")

	if p := f.position(position.ID); p.ValuationBasis != investments.ValuationBasisCostBasis || p.CurrentValue.String() != "90" {
		t.Fatalf("without a quote = %+v, want the cost", p)
	}

	f.quoteOn(asset.ID, "2026-09-03", "30")
	f.quoteOn(asset.ID, "2026-09-10", "33.3333")
	got := f.position(position.ID)
	if got.ValuationBasis != investments.ValuationBasisMarketQuote || got.CurrentValue.String() != "100" {
		t.Fatalf("basis = %s, value = %s; want market_quote and 3 × 33.3333 rounded to 100", got.ValuationBasis, got.CurrentValue)
	}
	if bookDecimalString(got.CurrentUnitPrice) != "33.3333" || got.ValuedOn == nil || got.ValuedOn.Format(investments.DateLayout) != "2026-09-10" {
		t.Fatalf("unit price = %s, valued on = %v; want the latest quote and its day", bookDecimalString(got.CurrentUnitPrice), got.ValuedOn)
	}
	if bookDecimalString(got.TotalCost) != "90" || bookDecimalString(got.AverageCost) != "30" {
		t.Fatalf("cost = %s (average %s), want it untouched by the quote", bookDecimalString(got.TotalCost), bookDecimalString(got.AverageCost))
	}
	if ops, err := investments.ListOperations(context.Background(), f.conn, investments.OperationFilter{PositionID: &position.ID}); err != nil || len(ops) != 1 {
		t.Fatalf("operations = %+v, %v; want only the opening balance", ops, err)
	}
	if listed := f.positions(investments.PositionFilter{}); len(listed) != 1 || !reflect.DeepEqual(listed[0], got) {
		t.Fatalf("list = %+v, want the same derivation as GetPosition: %+v", listed, got)
	}
	if summary := f.summary(); summary.ManualValue.String() != "100" || summary.UnrealizedGain.String() != "10" {
		t.Fatalf("summary = %+v, want the quoted value to feed the totals", summary)
	}

	// A newer price moves the value with no write to the position.
	f.quoteOn(asset.ID, "2026-09-11", "40")
	if p := f.position(position.ID); p.CurrentValue.String() != "120" {
		t.Fatalf("after a newer quote = %s, want 120", p.CurrentValue)
	}
	// And so does a change in the units held.
	f.addOperation(position.AccountID, position.ID, "sell", "2026-09-12", "40", "1")
	if p := f.position(position.ID); p.Quantity.String() != "2" || p.CurrentValue.String() != "80" {
		t.Fatalf("after selling a unit = %+v, want 2 × 40", p)
	}
}

// The most recent statement about the price wins; on the same day the typed
// valuation does.
func TestQuotedManualPositionFollowsTheMostRecentPriceStatement(t *testing.T) {
	f := newBookFixture(t)
	asset := f.quotedAsset("Bitcoin", "BTC")
	position := f.openQuoted(f.addManualAccount("Corretora"), asset, "10", "25")
	f.quoteOn(asset.ID, "2026-09-05", "30")

	// Valued by a person after the last quote: the valuation stands.
	f.addOperation(position.AccountID, position.ID, "valuation", "2026-09-08", "400", "")
	got := f.position(position.ID)
	if got.ValuationBasis != investments.ValuationBasisManualValuation || got.CurrentValue.String() != "400" ||
		got.ValuedOn == nil || got.ValuedOn.Format(investments.DateLayout) != "2026-09-08" {
		t.Fatalf("valuation after the quote = %+v, want the typed 400", got)
	}

	// A quote the same day as the valuation does not displace it.
	f.quoteOn(asset.ID, "2026-09-08", "50")
	if p := f.position(position.ID); p.ValuationBasis != investments.ValuationBasisManualValuation || p.CurrentValue.String() != "400" {
		t.Fatalf("tie = %+v, want the typed valuation", p)
	}

	// A later quote does: the valuation is now the older statement.
	f.quoteOn(asset.ID, "2026-09-09", "60")
	got = f.position(position.ID)
	if got.ValuationBasis != investments.ValuationBasisMarketQuote || got.CurrentValue.String() != "600" ||
		got.ValuedOn == nil || got.ValuedOn.Format(investments.DateLayout) != "2026-09-09" {
		t.Fatalf("quote after the valuation = %+v, want 10 × 60 from the 9th", got)
	}

	// A valuation typed later still outranks that quote.
	f.addOperation(position.AccountID, position.ID, "valuation", "2026-09-10", "650", "")
	if p := f.position(position.ID); p.ValuationBasis != investments.ValuationBasisManualValuation || p.CurrentValue.String() != "650" {
		t.Fatalf("valuation after the quote = %+v, want the typed 650", p)
	}
}

// Only an asset with a quote connector is valued from its series: prices that
// happen to sit there for any other asset, and holdings with no units, are not
// the holder's to be priced by.
func TestPositionsOutsideAConnectorKeepTheirOwnValue(t *testing.T) {
	f := newBookFixture(t)
	accountID := f.addManualAccount("Corretora")

	plain, err := investments.CreatePosition(context.Background(), f.conn, investments.PositionInput{
		AccountID: accountID, Name: "CDB", AssetType: "Renda fixa",
		InitialQuantity: bookDec("2"), InitialUnitCost: bookDec("50"),
		OccurredOn: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	f.quoteOn(*plain.AssetID, "2026-09-05", "70")
	if p := f.position(plain.ID); p.ValuationBasis != investments.ValuationBasisCostBasis || p.CurrentValue.String() != "100" || p.ValuedOn != nil {
		t.Fatalf("asset without a connector = %+v, want the cost", p)
	}

	asset := f.quotedAsset("Bitcoin", "BTC")
	f.quoteOn(asset.ID, "2026-09-05", "70")
	soldOut := f.openQuoted(accountID, asset, "2", "50")
	f.addOperation(accountID, soldOut.ID, "sell", "2026-09-06", "140", "2")
	if p := f.position(soldOut.ID); !p.Closed || p.ValuationBasis == investments.ValuationBasisMarketQuote || !p.CurrentValue.IsZero() {
		t.Fatalf("sold out = %+v, want closed and worth nothing", p)
	}

	empty, err := investments.CreatePosition(context.Background(), f.conn, investments.PositionInput{
		AccountID: f.addManualAccount("Outra"), AssetID: &asset.ID, Name: asset.Name, AssetType: asset.AssetType,
	})
	if err != nil {
		t.Fatalf("CreatePosition (empty): %v", err)
	}
	if p := f.position(empty.ID); p.ValuationBasis == investments.ValuationBasisMarketQuote || !p.CurrentValue.IsZero() {
		t.Fatalf("empty = %+v, want no value", p)
	}
}

// Each position is valued by its own asset's quote: two assets held in one
// account, and one asset held in two accounts, do not share a price by
// accident of the account or the order they are listed in.
func TestQuotedPositionsAreValuedByTheirOwnAssetsQuote(t *testing.T) {
	f := newBookFixture(t)
	account := f.addManualAccount("Corretora")
	other := f.addManualAccount("Outra corretora")
	bitcoin := f.quotedAsset("Bitcoin", "BTC")
	ether := f.quotedAsset("Ether", "ETH")
	inBitcoin := f.openQuoted(account, bitcoin, "2", "10")
	inEther := f.openQuoted(account, ether, "3", "10")
	bitcoinElsewhere := f.openQuoted(other, bitcoin, "5", "10")
	f.quoteOn(bitcoin.ID, "2026-09-10", "100")
	f.quoteOn(ether.ID, "2026-09-10", "7")

	for _, tc := range []struct {
		position investments.Position
		want     string
	}{{inBitcoin, "200"}, {inEther, "21"}, {bitcoinElsewhere, "500"}} {
		if got := f.position(tc.position.ID); got.CurrentValue.String() != tc.want {
			t.Errorf("%s value = %s, want %s", got.Name, got.CurrentValue, tc.want)
		}
	}
	for _, listed := range f.positions(investments.PositionFilter{}) {
		want := map[string]string{inBitcoin.ID: "200", inEther.ID: "21", bitcoinElsewhere.ID: "500"}[listed.ID]
		if listed.CurrentValue.String() != want {
			t.Errorf("listed %s value = %s, want %s", listed.Name, listed.CurrentValue, want)
		}
	}
}
