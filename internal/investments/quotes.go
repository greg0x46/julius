package investments

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

// Quote sources written by investments: who wrote a price, kept for display
// and audit. Market data provider names are also stored as sources; those
// names are defined in internal/marketdata. Which price may replace which is
// not decided by the source but by the origin.
const (
	QuoteSourcePluggy = "pluggy"
	// QuoteSourceIssue is the unit price a fixed income title was bought at
	// (amountOriginal ÷ quantity), dated on its purchase. It is what lets a
	// holding bought before the connection existed report rendimento since
	// inception without any movement history.
	QuoteSourceIssue = "issue"
)

// QuoteOrigin is the kind of statement a price makes, which is what decides
// precedence between two writers of the same day: a market data provider's
// price never replaces one the Open Finance sync observed or one derived from
// a title's purchase.
type QuoteOrigin string

const (
	// QuoteOriginSync is a price the provider reported on a sync.
	QuoteOriginSync QuoteOrigin = "sync"
	// QuoteOriginIssue is the PU a fixed income title was bought at, derived
	// from the holding rather than observed.
	QuoteOriginIssue QuoteOrigin = "issue"
	// QuoteOriginMarket is a price from a market data provider (spot or close).
	QuoteOriginMarket QuoteOrigin = "market"
)

// Valid reports whether o is one of the known origins.
func (o QuoteOrigin) Valid() bool {
	switch o {
	case QuoteOriginSync, QuoteOriginIssue, QuoteOriginMarket:
		return true
	}
	return false
}

// providerZone is the wall clock provider instants are read in. Pluggy dates
// Nubank holdings as Brazilian midnights (issueDate 2024-09-06T03:00Z) and
// as-of stamps late in the Brazilian evening (2026-09-28T00:03Z is the 27th),
// so taking the UTC date would shift both by a day.
var providerZone = time.FixedZone("BRT", -3*3600)

// ProviderDay is the calendar day a provider instant belongs to.
func ProviderDay(t time.Time) time.Time { return Day(t.In(providerZone)) }

// AssetQuote is one dated unit price of an asset.
type AssetQuote struct {
	AssetID  string
	QuotedOn time.Time
	Price    decimal.Decimal
	// Source is who wrote the price (pluggy, issue, a market data provider).
	Source string
	// Origin is the kind of statement it makes; writers must set it.
	Origin      QuoteOrigin
	RawImportID *string
}

// UpsertAssetQuote stores the asset's price for the day, replacing whatever
// another writer stored that day: one price per asset per day, last writer
// wins, with source recording who that was and origin what kind of price it
// is. It is the Open Finance sync's writer; market data providers go through
// UpsertConnectorQuotes, which spares what the sync observed.
func UpsertAssetQuote(ctx context.Context, q Querier, quote AssetQuote) error {
	if !quote.Origin.Valid() {
		return ErrInvalidInput
	}
	now := db.FormatTime(time.Now())
	_, err := q.ExecContext(ctx, `
		INSERT INTO investment_asset_quotes (asset_id, quoted_on, price, source, origin, raw_import_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (asset_id, quoted_on) DO UPDATE SET
			price = excluded.price, source = excluded.source, origin = excluded.origin,
			raw_import_id = excluded.raw_import_id, updated_at = excluded.updated_at`,
		quote.AssetID, formatDate(quote.QuotedOn), money.CanonicalDecimal(quote.Price), quote.Source, string(quote.Origin),
		nullableString(quote.RawImportID), now, now)
	return err
}

// insertAssetQuoteIfAbsent stores a derived price only where nothing observed
// exists for the day: an issue PU must never replace a quote the provider or
// a market data provider actually reported.
func insertAssetQuoteIfAbsent(ctx context.Context, q Querier, quote AssetQuote) error {
	if !quote.Origin.Valid() {
		return ErrInvalidInput
	}
	now := db.FormatTime(time.Now())
	_, err := q.ExecContext(ctx, `
		INSERT INTO investment_asset_quotes (asset_id, quoted_on, price, source, origin, raw_import_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (asset_id, quoted_on) DO NOTHING`,
		quote.AssetID, formatDate(quote.QuotedOn), money.CanonicalDecimal(quote.Price), quote.Source, string(quote.Origin),
		nullableString(quote.RawImportID), now, now)
	return err
}

// ListAssetQuotes returns the asset's quotes up to and including the given
// day (nil for all), oldest first.
func ListAssetQuotes(ctx context.Context, q Querier, assetID string, until *time.Time) ([]AssetQuote, error) {
	query := assetQuoteSelect + ` WHERE asset_id = ?`
	args := []any{assetID}
	if until != nil {
		query += ` AND quoted_on <= ?`
		args = append(args, formatDate(*until))
	}
	rows, err := q.QueryContext(ctx, query+` ORDER BY quoted_on`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	quotes := []AssetQuote{}
	for rows.Next() {
		quote, err := scanAssetQuote(rows)
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
	}
	return quotes, rows.Err()
}

// LatestAssetQuote is the asset's most recent quote, nil while its series is
// empty. It carries no age limit: the caller decides how stale a price may be.
func LatestAssetQuote(ctx context.Context, q Querier, assetID string) (*AssetQuote, error) {
	quote, err := scanAssetQuote(q.QueryRowContext(ctx,
		assetQuoteSelect+` WHERE asset_id = ? ORDER BY quoted_on DESC LIMIT 1`, assetID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &quote, nil
}

const assetQuoteSelect = `SELECT asset_id, quoted_on, price, source, origin, raw_import_id FROM investment_asset_quotes`

func scanAssetQuote(row interface{ Scan(...any) error }) (AssetQuote, error) {
	var (
		quote           AssetQuote
		quotedOn, price string
		origin          string
		rawImportID     sql.NullString
	)
	if err := row.Scan(dbtypes.ID(&quote.AssetID), dbtypes.Date(&quotedOn), dbtypes.Decimal(&price), &quote.Source, &origin, dbtypes.ID(&rawImportID)); err != nil {
		return AssetQuote{}, err
	}
	quote.Origin = QuoteOrigin(origin)
	var err error
	if quote.QuotedOn, err = parseDate(quotedOn); err != nil {
		return AssetQuote{}, err
	}
	if quote.Price, err = decimal.NewFromString(price); err != nil {
		return AssetQuote{}, err
	}
	if rawImportID.Valid {
		v := rawImportID.String
		quote.RawImportID = &v
	}
	return quote, nil
}

// SyncedHolding is what a provider holding says about its own identity and
// price: the fields asset resolution and quote recording need, whether they
// come from the current financial_investments row or a past raw payload.
type SyncedHolding struct {
	ExternalID     string
	Name           *string
	Code           *string
	ISIN           *string
	InvestmentType *string
	Subtype        *string
	CurrencyCode   *string
	IssuerCNPJ     *string
	Rate           *decimal.Decimal
	RateType       *string
	IssueDate      *time.Time
	PurchaseDate   *time.Time
	DueDate        *time.Time
	Quantity       *decimal.Decimal
	Value          *decimal.Decimal
	Amount         *decimal.Decimal
	AmountOriginal *decimal.Decimal
	AsOfDate       *time.Time
}

func present(s *string) bool { return s != nil && strings.TrimSpace(*s) != "" }

// syncedHoldingColumns is the column order scanSyncedHolding expects, read
// from financial_investments aliased as fi.
const syncedHoldingColumns = `fi.external_id, fi.name, fi.code, fi.isin, fi.investment_type, fi.subtype,
	fi.currency_code, fi.issuer_cnpj, fi.rate, fi.rate_type, fi.issue_date, fi.purchase_date, fi.due_date,
	fi.quantity, fi.value, fi.amount, fi.amount_original, fi.as_of_date`

// syncedHoldingRow receives syncedHoldingColumns; holding() parses it.
type syncedHoldingRow struct {
	externalID                                                string
	name, code, isin, investmentType, subtype, currency, cnpj sql.NullString
	rate, rateType, issueDate, purchaseDate, dueDate          sql.NullString
	quantity, value, amount, amountOriginal, asOfDate         sql.NullString
}

func (r *syncedHoldingRow) dests() []any {
	return []any{dbtypes.ID(&r.externalID), &r.name, &r.code, &r.isin, &r.investmentType, &r.subtype,
		&r.currency, &r.cnpj, dbtypes.Decimal(&r.rate), &r.rateType, dbtypes.Date(&r.issueDate), dbtypes.Date(&r.purchaseDate), dbtypes.Date(&r.dueDate),
		dbtypes.Decimal(&r.quantity), dbtypes.Decimal(&r.value), dbtypes.Decimal(&r.amount), dbtypes.Decimal(&r.amountOriginal), dbtypes.Time(&r.asOfDate)}
}

func (r *syncedHoldingRow) holding() (SyncedHolding, error) {
	str := func(ns sql.NullString) *string {
		if !ns.Valid || ns.String == "" {
			return nil
		}
		v := ns.String
		return &v
	}
	h := SyncedHolding{
		ExternalID: r.externalID, Name: str(r.name), Code: str(r.code), ISIN: str(r.isin),
		InvestmentType: str(r.investmentType), Subtype: str(r.subtype), CurrencyCode: str(r.currency),
		IssuerCNPJ: str(r.cnpj), RateType: str(r.rateType),
	}
	for _, item := range []struct {
		raw sql.NullString
		dst **decimal.Decimal
	}{
		{r.rate, &h.Rate}, {r.quantity, &h.Quantity}, {r.value, &h.Value},
		{r.amount, &h.Amount}, {r.amountOriginal, &h.AmountOriginal},
	} {
		if !item.raw.Valid || item.raw.String == "" {
			continue
		}
		v, err := decimal.NewFromString(item.raw.String)
		if err != nil {
			return SyncedHolding{}, err
		}
		*item.dst = &v
	}
	for _, item := range []struct {
		raw sql.NullString
		dst **time.Time
	}{
		{r.issueDate, &h.IssueDate}, {r.purchaseDate, &h.PurchaseDate},
		{r.dueDate, &h.DueDate}, {r.asOfDate, &h.AsOfDate},
	} {
		var v *time.Time
		if item.dst == &h.AsOfDate {
			var err error
			v, err = dbtypes.ParseNullTime(item.raw)
			if err != nil {
				return SyncedHolding{}, err
			}
		} else if item.raw.Valid {
			// Provider terms are calendar days anchored at BRT midnight.
			day, err := dbtypes.ParseDate(item.raw)
			if err != nil {
				return SyncedHolding{}, err
			}
			anchored := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, providerZone)
			v = &anchored
		}
		*item.dst = v
	}
	return h, nil
}

func (h SyncedHolding) isFixedIncome() bool {
	return h.InvestmentType != nil && strings.EqualFold(*h.InvestmentType, "FIXED_INCOME")
}

// DisplayName is the holding's name, falling back to its identifiers.
func (h SyncedHolding) DisplayName() string {
	for _, candidate := range []*string{h.Name, h.Code, h.ISIN} {
		if present(candidate) {
			return *candidate
		}
	}
	return h.ExternalID
}

// AssetType is the provider's type, else subtype, else a generic label.
func (h SyncedHolding) AssetType() string {
	for _, candidate := range []*string{h.InvestmentType, h.Subtype} {
		if present(candidate) {
			return *candidate
		}
	}
	return "Investimento"
}

// Ticker is the holding's instrument code: the provider's code or ISIN, and
// for fixed income without either, the synthetic FixedIncomeCode. Without
// that, every CDB of an issuer shares the name-based asset and one price
// series, although each title has its own PU.
func (h SyncedHolding) Ticker() *string {
	for _, candidate := range []*string{h.Code, h.ISIN} {
		if present(candidate) {
			return candidate
		}
	}
	if h.isFixedIncome() {
		return FixedIncomeCode(h.Subtype, h.IssuerCNPJ, h.Rate, h.RateType, h.IssueDate, h.DueDate)
	}
	return nil
}

// FixedIncomeCode names a fixed income title by the terms that fix its unit
// price: issuer, indexer and rate, issue and due date, e.g.
// CDB-30680829-120CDI-20260913-20280912. Two holdings with the same terms
// accrue the same PU, so sharing one asset and one quote series is correct.
// It returns nil when a term is missing: a code built from partial terms
// would merge titles that differ in the missing one.
func FixedIncomeCode(subtype, issuerCNPJ *string, rate *decimal.Decimal, rateType *string, issueDate, dueDate *time.Time) *string {
	if !present(issuerCNPJ) || rate == nil || issueDate == nil || dueDate == nil {
		return nil
	}
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, *issuerCNPJ)
	// The first eight digits are the CNPJ root: the same issuer whatever
	// branch the provider happens to report.
	if len(digits) > 8 {
		digits = digits[:8]
	}
	if digits == "" {
		return nil
	}
	kind := "RF"
	if present(subtype) {
		kind = strings.ToUpper(strings.TrimSpace(*subtype))
	}
	indexer := ""
	if present(rateType) {
		indexer = strings.ToUpper(strings.TrimSpace(*rateType))
	}
	code := kind + "-" + digits + "-" + rate.String() + indexer + "-" +
		ProviderDay(*issueDate).Format("20060102") + "-" + ProviderDay(*dueDate).Format("20060102")
	return &code
}

// ResolveSyncedAsset finds or creates the catalog asset a provider holding
// belongs to — the same derivation listSyncedPositions uses, so a quote is
// always stored against the asset the position reports.
func ResolveSyncedAsset(ctx context.Context, q Querier, h SyncedHolding) (Asset, error) {
	currency := "BRL"
	if present(h.CurrencyCode) {
		currency = *h.CurrencyCode
	}
	name, ticker, kind := h.assetIdentity()
	return ensureAsset(ctx, q, name, ticker, kind, currency)
}

// unitPrice is the holding's price per unit on its as-of day. amount ÷
// quantity is preferred over the provider's value: value is rounded to six
// places (0.010061), which on 163004 units of a CDB is seven cents away from
// the amount the provider states for the same holding.
func (h SyncedHolding) unitPrice() (decimal.Decimal, bool) {
	if h.Quantity == nil || !h.Quantity.IsPositive() {
		return decimal.Zero, false
	}
	if h.Amount != nil && h.Amount.IsPositive() {
		return h.Amount.DivRound(*h.Quantity, 12), true
	}
	if h.Value != nil && h.Value.IsPositive() {
		return *h.Value, true
	}
	return decimal.Zero, false
}

// issuePrice is the PU the title was bought at, and the day it was bought.
func (h SyncedHolding) issuePrice() (decimal.Decimal, time.Time, bool) {
	if !h.isFixedIncome() || h.AmountOriginal == nil || !h.AmountOriginal.IsPositive() ||
		h.Quantity == nil || !h.Quantity.IsPositive() {
		return decimal.Zero, time.Time{}, false
	}
	bought := h.PurchaseDate
	if bought == nil {
		bought = h.IssueDate
	}
	if bought == nil {
		return decimal.Zero, time.Time{}, false
	}
	return h.AmountOriginal.DivRound(*h.Quantity, 12), ProviderDay(*bought), true
}

// RecordSyncedQuotes stores what one provider snapshot says about its
// asset's price: the unit price on its as-of day and, for fixed income, the
// PU it was bought at. A redeemed holding (no quantity) says nothing: Pluggy
// keeps a placeholder value of 0.01 on it.
func RecordSyncedQuotes(ctx context.Context, q Querier, h SyncedHolding, rawImportID *string) error {
	price, priced := h.unitPrice()
	issue, boughtOn, issued := h.issuePrice()
	if (!priced || h.AsOfDate == nil) && !issued {
		return nil
	}
	asset, err := ResolveSyncedAsset(ctx, q, h)
	if err != nil {
		return err
	}
	if priced && h.AsOfDate != nil {
		if err := UpsertAssetQuote(ctx, q, AssetQuote{
			AssetID: asset.ID, QuotedOn: ProviderDay(*h.AsOfDate), Price: price,
			Source: QuoteSourcePluggy, Origin: QuoteOriginSync, RawImportID: rawImportID,
		}); err != nil {
			return err
		}
	}
	if issued {
		if err := insertAssetQuoteIfAbsent(ctx, q, AssetQuote{
			AssetID: asset.ID, QuotedOn: boughtOn, Price: issue,
			Source: QuoteSourceIssue, Origin: QuoteOriginIssue,
		}); err != nil {
			return err
		}
	}
	return nil
}

// PruneUnusedSyncedAssets removes the name-keyed assets fixed income
// holdings resolved to before they had a per-title code: every CDB of an
// issuer used to share one, which nothing resolves to any more. Only an
// asset nothing references — no manual position, no quote, no quote market — is
// removed, so one the user adopted for a manual holding stays.
func PruneUnusedSyncedAssets(ctx context.Context, q Querier) (int, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+syncedHoldingColumns+` FROM financial_investments fi`)
	if err != nil {
		return 0, err
	}
	holdings := []SyncedHolding{}
	for rows.Next() {
		var row syncedHoldingRow
		if err := rows.Scan(row.dests()...); err != nil {
			rows.Close()
			return 0, err
		}
		holding, err := row.holding()
		if err != nil {
			rows.Close()
			return 0, err
		}
		holdings = append(holdings, holding)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	current, superseded := map[string]bool{}, map[string]bool{}
	for _, holding := range holdings {
		current[assetCanonicalKey(holding.DisplayName(), holding.Ticker(), holding.AssetType())] = true
		if !present(holding.Code) && !present(holding.ISIN) {
			superseded[assetCanonicalKey(holding.DisplayName(), nil, holding.AssetType())] = true
		}
	}
	removed := 0
	for key := range superseded {
		if current[key] {
			continue
		}
		result, err := q.ExecContext(ctx, `
			DELETE FROM investment_assets
			WHERE canonical_key = ? AND quote_source IS NULL
			  AND NOT EXISTS (SELECT 1 FROM investment_positions p WHERE p.asset_id = investment_assets.id)
			  AND NOT EXISTS (SELECT 1 FROM investment_asset_quotes q WHERE q.asset_id = investment_assets.id)`, key)
		if err != nil {
			return 0, err
		}
		if n, err := result.RowsAffected(); err == nil {
			removed += int(n)
		}
	}
	return removed, nil
}

// FormatProviderDatePtr stores the calendar day on the provider's wall clock.
func FormatProviderDatePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ProviderDay(*t).Format(dbtypes.DateLayout)
}
