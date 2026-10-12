package investments

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

func assetCanonicalKey(name string, ticker *string, assetType string) string {
	clean := func(value string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToUpper(r)
			}
			return -1
		}, value)
	}
	if ticker != nil && clean(*ticker) != "" {
		return "ticker:" + clean(*ticker)
	}
	return "name:" + clean(assetType) + ":" + clean(name)
}

// assetColumns is the column order scanAsset expects. Shared by every read
// path (ListAssets, GetAsset, ensureAsset's lookup) so a new column is added
// once instead of drifting across three repeated SELECTs — mirrors
// operationColumns in operations.go.
const assetColumns = `id, canonical_key, name, ticker, asset_type, asset_class, currency_code, quote_source, quote_symbol, created_at, updated_at`

func scanAsset(row interface{ Scan(...any) error }) (Asset, error) {
	var asset Asset
	var ticker, quoteSource, quoteSymbol sql.NullString
	var created, updated string
	err := row.Scan(dbtypes.ID(&asset.ID), &asset.CanonicalKey, &asset.Name, &ticker, &asset.AssetType, &asset.AssetClass, &asset.CurrencyCode,
		&quoteSource, &quoteSymbol, dbtypes.Time(&created), dbtypes.Time(&updated))
	if err != nil {
		return Asset{}, err
	}
	if ticker.Valid {
		value := ticker.String
		asset.Ticker = &value
	}
	if quoteSource.Valid {
		value := quoteSource.String
		asset.QuoteSource = &value
	}
	if quoteSymbol.Valid {
		value := quoteSymbol.String
		asset.QuoteSymbol = &value
	}
	asset.CreatedAt, err = parseTimestamp(created)
	if err != nil {
		return Asset{}, err
	}
	asset.UpdatedAt, err = parseTimestamp(updated)
	if err != nil {
		return Asset{}, err
	}
	return asset, nil
}

func ListAssets(ctx context.Context, q Querier) ([]Asset, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+assetColumns+` FROM investment_assets ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assets := []Asset{}
	for rows.Next() {
		asset, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

func GetAsset(ctx context.Context, q Querier, id string) (Asset, error) {
	asset, err := scanAsset(q.QueryRowContext(ctx, `SELECT `+assetColumns+` FROM investment_assets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, ErrAssetNotFound
	}
	return asset, err
}

// normalizeAssetInput also enforces the shape of the quote configuration: a
// QuoteSymbol needs a QuoteSource, and a QuoteSource needs something to ask it
// for — the symbol if one was given, else the ticker. An incomplete
// instrument is rejected rather than stored half-configured. The HTTP layer
// checks that QuoteSource names a known market and canonicalizes its symbol.
func normalizeAssetInput(in AssetInput) (AssetInput, string, error) {
	name := strings.TrimSpace(in.Name)
	assetType := strings.TrimSpace(in.AssetType)
	assetClass := AssetClass(strings.TrimSpace(string(in.AssetClass)))
	if assetClass == "" {
		assetClass = InferAssetClass(assetType)
	}
	if !IsAssetClass(assetClass) || !assetTypeMatchesClass(assetClass, assetType) {
		return AssetInput{}, "", ErrInvalidInput
	}
	ticker := trimOptional(in.Ticker)
	currency := strings.ToUpper(strings.TrimSpace(in.CurrencyCode))
	if currency == "" {
		currency = "BRL"
	}
	validCurrency := len(currency) == 3
	for _, char := range currency {
		if char < 'A' || char > 'Z' {
			validCurrency = false
		}
	}
	quoteSource := trimOptional(in.QuoteSource)
	quoteSymbol := trimOptional(in.QuoteSymbol)
	// A source needs something to ask it for: the symbol when one was given
	// (an explicit override), else the ticker, which internal/quotes turns
	// into whatever the source expects. A symbol with no source is orphaned.
	if (quoteSource == nil && quoteSymbol != nil) || (quoteSource != nil && quoteSymbol == nil && ticker == nil) {
		return AssetInput{}, "", ErrInvalidInput
	}
	if name == "" || assetType == "" || !validCurrency {
		return AssetInput{}, "", ErrInvalidInput
	}
	normalized := AssetInput{
		Name: name, Ticker: ticker, AssetType: assetType, AssetClass: assetClass, CurrencyCode: currency,
		QuoteSource: quoteSource, QuoteSymbol: quoteSymbol,
	}
	return normalized, assetCanonicalKey(name, ticker, assetType), nil
}

func assetKeyExists(ctx context.Context, q Querier, canonicalKey, exceptID string) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM investment_assets WHERE canonical_key = ? AND id <> ?`,
		canonicalKey, exceptID).Scan(&count)
	return count > 0, err
}

func CreateAsset(ctx context.Context, q Querier, in AssetInput) (Asset, error) {
	normalized, key, err := normalizeAssetInput(in)
	if err != nil {
		return Asset{}, err
	}
	exists, err := assetKeyExists(ctx, q, key, "")
	if err != nil {
		return Asset{}, err
	}
	if exists {
		return Asset{}, ErrAssetAlreadyExists
	}
	now, id := db.FormatTime(time.Now()), uuid.NewString()
	if _, err := q.ExecContext(ctx, `
		INSERT INTO investment_assets
			(id, canonical_key, name, ticker, asset_type, asset_class, currency_code, quote_source, quote_symbol, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, key, normalized.Name, nullableString(normalized.Ticker), normalized.AssetType, normalized.AssetClass,
		normalized.CurrencyCode, nullableString(normalized.QuoteSource), nullableString(normalized.QuoteSymbol),
		now, now); err != nil {
		return Asset{}, err
	}
	return GetAsset(ctx, q, id)
}

func UpdateAsset(ctx context.Context, q Querier, id string, in AssetInput) (Asset, error) {
	current, err := GetAsset(ctx, q, id)
	if err != nil {
		return Asset{}, err
	}
	if in.AssetClass == "" && strings.TrimSpace(in.AssetType) == current.AssetType {
		in.AssetClass = current.AssetClass
	}
	normalized, key, err := normalizeAssetInput(in)
	if err != nil {
		return Asset{}, err
	}
	exists, err := assetKeyExists(ctx, q, key, id)
	if err != nil {
		return Asset{}, err
	}
	if exists {
		return Asset{}, ErrAssetAlreadyExists
	}
	if _, err := q.ExecContext(ctx, `
		UPDATE investment_assets
		SET canonical_key = ?, name = ?, ticker = ?, asset_type = ?, asset_class = ?, currency_code = ?,
		    quote_source = ?, quote_symbol = ?, updated_at = ?
		WHERE id = ?`,
		key, normalized.Name, nullableString(normalized.Ticker), normalized.AssetType, normalized.AssetClass,
		normalized.CurrencyCode, nullableString(normalized.QuoteSource), nullableString(normalized.QuoteSymbol),
		db.FormatTime(time.Now()), id); err != nil {
		return Asset{}, err
	}
	return GetAsset(ctx, q, id)
}

func DeleteAsset(ctx context.Context, q Querier, id string) error {
	if _, err := GetAsset(ctx, q, id); err != nil {
		return err
	}
	var positionCount int
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM investment_positions WHERE asset_id = ?`, id).Scan(&positionCount); err != nil {
		return err
	}
	if positionCount > 0 {
		return ErrAssetHasPositions
	}
	_, err := q.ExecContext(ctx, `DELETE FROM investment_assets WHERE id = ?`, id)
	return err
}

// resolveAsset reuses the canonical instrument or creates it inside the same
// transaction as its first position. An explicit asset id always wins.
func resolveAsset(ctx context.Context, q Querier, in PositionInput) (Asset, error) {
	if in.AssetID != nil && *in.AssetID != "" {
		return GetAsset(ctx, q, *in.AssetID)
	}
	return ensureAsset(ctx, q, in.Name, in.Ticker, in.AssetType, "BRL")
}

// normalizeAssetIdentity trims an instrument's name and kind and derives the
// key the catalog knows it by. Find-or-create and the read-only lookup both go
// through it, so they always agree on which asset a holding is.
func normalizeAssetIdentity(rawName string, rawTicker *string, rawKind string) (name string, ticker *string, kind, key string, err error) {
	name, kind = strings.TrimSpace(rawName), strings.TrimSpace(rawKind)
	if name == "" || kind == "" {
		return "", nil, "", "", ErrInvalidInput
	}
	ticker = trimOptional(rawTicker)
	return name, ticker, kind, assetCanonicalKey(name, ticker, kind), nil
}

// findAssetByKey is the lookup half of ensureAsset: it never writes.
func findAssetByKey(ctx context.Context, q Querier, key string) (Asset, bool, error) {
	asset, err := scanAsset(q.QueryRowContext(ctx, `SELECT `+assetColumns+` FROM investment_assets WHERE canonical_key = ?`, key))
	switch {
	case err == nil:
		return asset, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return Asset{}, false, nil
	default:
		return Asset{}, false, err
	}
}

func ensureAsset(ctx context.Context, q Querier, rawName string, rawTicker *string, rawKind, rawCurrency string) (Asset, error) {
	name, ticker, kind, key, err := normalizeAssetIdentity(rawName, rawTicker, rawKind)
	if err != nil {
		return Asset{}, err
	}
	asset, found, err := findAssetByKey(ctx, q, key)
	if err != nil {
		return Asset{}, err
	}
	if found {
		return asset, nil
	}
	currency := strings.ToUpper(strings.TrimSpace(rawCurrency))
	if currency == "" {
		currency = "BRL"
	}
	now, id := db.FormatTime(time.Now()), uuid.NewString()
	_, err = q.ExecContext(ctx, `INSERT INTO investment_assets (id, canonical_key, name, ticker, asset_type, asset_class, currency_code, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, key, name, nullableString(ticker), kind, InferAssetClass(kind), currency, now, now)
	if err != nil {
		return Asset{}, err
	}
	return GetAsset(ctx, q, id)
}
