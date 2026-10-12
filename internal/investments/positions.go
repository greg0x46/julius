package investments

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
)

type PositionFilter struct {
	AccountID     *string
	PortfolioID   *string
	IncludeClosed bool
	Source        *string
	SourceID      *string
}

const manualPositionColumn = "manual_position_id"
const syncedPositionColumn = "financial_investment_id"

// ListPositions answers with manual holdings and provider holdings in one
// list. Manual numbers are always replayed from investment_operations, so a
// corrected operation cannot leave a stale quantity or cost behind.
func ListPositions(ctx context.Context, q Querier, filter PositionFilter) ([]Position, error) {
	if err := EnsureIntegratedAccounts(ctx, q); err != nil {
		return nil, err
	}
	manual, err := listManualPositions(ctx, q, positionScope{})
	if err != nil {
		return nil, err
	}
	synced, err := listSyncedPositions(ctx, q, positionScope{})
	if err != nil {
		return nil, err
	}
	positions := append(manual, synced...)
	sort.SliceStable(positions, func(i, j int) bool {
		if positions[i].AccountID != positions[j].AccountID {
			return positions[i].AccountID < positions[j].AccountID
		}
		if positions[i].Name != positions[j].Name {
			return positions[i].Name < positions[j].Name
		}
		return positions[i].ID < positions[j].ID
	})

	result := []Position{}
	for _, position := range positions {
		if filter.Source != nil && string(position.Source) != *filter.Source {
			continue
		}
		if filter.SourceID != nil && position.AccountID != "integrated:"+*filter.SourceID {
			continue
		}
		if filter.AccountID != nil && position.AccountID != *filter.AccountID {
			continue
		}
		if filter.PortfolioID != nil && (position.PortfolioID == nil || *position.PortfolioID != *filter.PortfolioID) {
			continue
		}
		if position.Closed && !filter.IncludeClosed {
			continue
		}
		result = append(result, position)
	}
	return result, nil
}

// GetPosition runs the same manual and synced derivations as ListPositions,
// scoped to the one row: a single read must not grow a second way of computing
// quantity, cost and valuation that could disagree with the list. Scoping the
// manual query to the id means only that holding's account is replayed, so a
// read costs one account's operations rather than every account's.
func GetPosition(ctx context.Context, q Querier, id string) (Position, error) {
	if err := EnsureIntegratedAccounts(ctx, q); err != nil {
		return Position{}, err
	}
	scope := positionScope{ID: id}
	manual, err := listManualPositions(ctx, q, scope)
	if err != nil {
		return Position{}, err
	}
	if len(manual) > 0 {
		return manual[0], nil
	}
	synced, err := listSyncedPositions(ctx, q, scope)
	if err != nil {
		return Position{}, err
	}
	if len(synced) > 0 {
		return synced[0], nil
	}
	return Position{}, ErrPositionNotFound
}

// positionScope narrows the position queries to one holding. The zero value
// reads the whole book for ListPositions; GetPosition sets ID so the same
// scan and replay run for a single row.
type positionScope struct {
	ID string
}

// where renders the optional filter for the column that holds the position id
// in the query at hand (p.id for manual rows, fi.id for provider rows).
func (s positionScope) where(column string) (string, []any) {
	if s.ID == "" {
		return "", nil
	}
	return "WHERE " + column + " = ?", []any{s.ID}
}

func listManualPositions(ctx context.Context, q Querier, scope positionScope) ([]Position, error) {
	where, args := scope.where("p.id")
	rows, err := q.QueryContext(ctx, `
		SELECT p.id, p.account_id, p.asset_id, a.name, a.ticker, a.asset_type, a.quote_source, p.notes,
		       p.created_at, p.updated_at, pp.portfolio_id
		FROM investment_positions p
		JOIN investment_assets a ON a.id = p.asset_id
		LEFT JOIN investment_position_portfolios pp ON pp.manual_position_id = p.id
		`+where+`
		ORDER BY p.account_id, a.name, p.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	positions := []Position{}
	// quoted marks the assets whose price series values their holdings.
	quoted := map[string]bool{}
	for rows.Next() {
		var (
			position                                Position
			ticker, quoteSource, notes, portfolioID sql.NullString
			createdAtRaw, updatedAtRaw              string
		)
		var assetID string
		if err := rows.Scan(&position.ID, &position.AccountID, &assetID, &position.Name, &ticker, &position.AssetType,
			&quoteSource, &notes, &createdAtRaw, &updatedAtRaw, &portfolioID); err != nil {
			return nil, err
		}
		quoted[assetID] = quoteSource.Valid && quoteSource.String != ""
		position.Source = PositionSourceManual
		position.AssetID = &assetID
		position.ValuationBasis = ValuationBasisCostBasis
		position.CurrencyCode = brl()
		if ticker.Valid {
			v := ticker.String
			position.Ticker = &v
		}
		if notes.Valid {
			v := notes.String
			position.Notes = &v
		}
		if portfolioID.Valid {
			v := portfolioID.String
			position.PortfolioID = &v
		}
		createdAt, err := parseTimestamp(createdAtRaw)
		if err != nil {
			return nil, err
		}
		updatedAt, err := parseTimestamp(updatedAtRaw)
		if err != nil {
			return nil, err
		}
		position.CreatedAt, position.UpdatedAt = &createdAt, &updatedAt
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ledgers := map[string]accountLedger{}
	latest := map[string]*AssetQuote{} // per asset: one lookup however many accounts hold it
	for i := range positions {
		ledger, ok := ledgers[positions[i].AccountID]
		if !ok {
			if ledger, err = replayAccount(ctx, q, positions[i].AccountID); err != nil {
				return nil, err
			}
			ledgers[positions[i].AccountID] = ledger
		}
		var quote *AssetQuote
		if assetID := *positions[i].AssetID; quoted[assetID] {
			if quote, ok = latest[assetID]; !ok {
				if quote, err = LatestAssetQuote(ctx, q, assetID); err != nil {
					return nil, err
				}
				latest[assetID] = quote
			}
		}
		if err := applyLedgerState(&positions[i], ledger.positions[positions[i].ID], quote); err != nil {
			return nil, err
		}
	}
	return positions, nil
}

// applyLedgerState copies the replayed holding onto the position. Without a
// dated valuation or a market quote the holding is reported at cost and
// identified as such, rather than carrying an invented quote. Values come from
// the ledger's exact totals; the unit figures are derived for display and never
// multiplied back.
//
// quote is the latest price in the series of an asset that has a quote
// market, nil otherwise. The most recent statement about the price wins: a
// quote dated after the last typed valuation values the holding at quantity ×
// price, derived on every read, and on the same day the typed valuation wins.
// An old quote keeps valuing the holding, and ValuedOn says how old it is.
func applyLedgerState(position *Position, state *positionLedger, quote *AssetQuote) error {
	if state == nil {
		return nil
	}
	quantity, averageCost, totalCost := state.quantity, state.averageCost(), state.totalCost
	position.Quantity = quantity
	position.AverageCost = &averageCost
	position.TotalCost = &totalCost
	position.Closed = quantity.IsZero()
	valuedDay, err := state.valuedDay()
	if err != nil {
		return err
	}
	if quote != nil && quantity.IsPositive() && (valuedDay == nil || valuedDay.Before(quote.QuotedOn)) {
		unitPrice, quotedOn := quote.Price, quote.QuotedOn
		position.ValuationBasis = ValuationBasisMarketQuote
		position.CurrentValue = quantity.Mul(unitPrice).Round(2)
		position.CurrentUnitPrice = &unitPrice
		position.ValuedOn = &quotedOn
		return nil
	}
	if state.valuationTotal == nil {
		position.CurrentValue = totalCost
		return nil
	}
	position.ValuationBasis = ValuationBasisManualValuation
	position.CurrentValue = state.valuedValue()
	if quantity.IsPositive() {
		unitPrice := state.valuationUnitPrice()
		position.CurrentUnitPrice = &unitPrice
	}
	position.ValuedOn = valuedDay
	return nil
}

// listSyncedPositions projects provider holdings as read-only positions. No
// average cost is derived: financial_investments.amount is quantity × value
// for fixed income holdings, so using it as an invested amount would report a
// fabricated gain.
func listSyncedPositions(ctx context.Context, q Querier, scope positionScope) ([]Position, error) {
	where, args := scope.where("fi.id")
	rows, err := q.QueryContext(ctx, `
		SELECT fi.id, ia.id, fi.balance, fi.created_at, fi.updated_at, pp.portfolio_id,
		       `+syncedHoldingColumns+`
		FROM financial_investments fi
		JOIN investment_accounts ia ON ia.source_id = fi.source_id
		LEFT JOIN investment_position_portfolios pp ON pp.financial_investment_id = fi.id
		`+where+`
		ORDER BY ia.id, fi.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	positions := []Position{}
	holdings := []SyncedHolding{}
	for rows.Next() {
		var (
			position                   Position
			balance, portfolioID       sql.NullString
			createdAtRaw, updatedAtRaw string
			row                        syncedHoldingRow
		)
		if err := rows.Scan(append([]any{&position.ID, &position.AccountID, &balance, &createdAtRaw, &updatedAtRaw,
			&portfolioID}, row.dests()...)...); err != nil {
			return nil, err
		}
		holding, err := row.holding()
		if err != nil {
			return nil, err
		}
		linked := position.ID
		position.Source = PositionSourceSynced
		position.ValuationBasis = ValuationBasisProviderBalance
		position.LinkedInvestmentID = &linked
		position.Name = holding.DisplayName()
		position.Ticker = holding.Ticker()
		position.AssetType = holding.AssetType()
		position.CurrencyCode = holding.CurrencyCode
		if portfolioID.Valid {
			v := portfolioID.String
			position.PortfolioID = &v
		}
		if balance.Valid {
			value, err := decimal.NewFromString(balance.String)
			if err != nil {
				return nil, err
			}
			position.CurrentValue = value
		}
		if holding.Quantity != nil {
			position.Quantity = *holding.Quantity
		}
		position.CurrentUnitPrice = holding.Value
		if holding.AsOfDate != nil {
			day := Day(*holding.AsOfDate)
			position.ValuedOn = &day
		}
		createdAt, err := parseTimestamp(createdAtRaw)
		if err != nil {
			return nil, err
		}
		updatedAt, err := parseTimestamp(updatedAtRaw)
		if err != nil {
			return nil, err
		}
		position.CreatedAt, position.UpdatedAt = &createdAt, &updatedAt
		// A redeemed provider holding keeps reporting a zero balance; it is
		// closed for the same reason a manual holding with no quantity is.
		position.Closed = position.CurrentValue.IsZero()
		positions = append(positions, position)
		holdings = append(holdings, holding)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// Provider holdings join the same canonical catalog as manual positions.
	// This lets a BTC imported from one institution and a BTC held manually in
	// another account share identity while provider balances stay read-only.
	for i := range positions {
		asset, err := ResolveSyncedAsset(ctx, q, holdings[i])
		if err != nil {
			return nil, err
		}
		positions[i].AssetID = &asset.ID
	}
	return positions, nil
}

func brl() *string {
	v := "BRL"
	return &v
}

// CreatePosition opens a manual holding. An opening amount becomes a single
// initial_balance operation so the replay, not a stored column, owns the
// resulting quantity and cost.
func CreatePosition(ctx context.Context, conn *sql.DB, in PositionInput) (Position, error) {
	if in.InitialQuantity.IsNegative() || in.InitialUnitCost.IsNegative() ||
		(in.InitialValue != nil && in.InitialValue.IsNegative()) {
		return Position{}, ErrInvalidInput
	}
	quantity, total, unitPrice := openingHolding(in)
	if (in.InitialQuantity.IsPositive() || in.InitialUnitCost.IsPositive()) && !quantity.IsPositive() {
		return Position{}, ErrInvalidInput
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Position{}, err
	}
	defer tx.Rollback()

	// Integrated custodies accept manual holdings too; the replay keeps them
	// apart from the provider's balances.
	if _, err := getRawAccount(ctx, tx, in.AccountID); err != nil {
		return Position{}, err
	}
	asset, err := resolveAsset(ctx, tx, in)
	if err != nil {
		return Position{}, err
	}
	var alreadyExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM investment_positions WHERE account_id = ? AND asset_id = ?)`, in.AccountID, asset.ID).Scan(&alreadyExists); err != nil {
		return Position{}, err
	}
	if alreadyExists {
		return Position{}, ErrPositionAlreadyExists
	}
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_positions (id, account_id, asset_id, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, in.AccountID, asset.ID, nullableString(trimOptional(in.Notes)), now, now); err != nil {
		return Position{}, err
	}
	if err := setPortfolioAssignment(ctx, tx, manualPositionColumn, id, in.PortfolioID); err != nil {
		return Position{}, err
	}
	if quantity.IsPositive() && total.IsPositive() {
		occurredOn := in.OccurredOn
		if occurredOn.IsZero() {
			occurredOn = time.Now()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO investment_operations (
				id, account_id, position_id, kind, occurred_on, amount, quantity, unit_price,
				fees, taxes, source, created_at, updated_at
			) VALUES (?, ?, ?, 'initial_balance', ?, ?, ?, ?, '0', '0', 'manual', ?, ?)`,
			uuid.NewString(), in.AccountID, id, formatDate(occurredOn), money.CanonicalDecimal(total),
			money.CanonicalDecimal(quantity), money.CanonicalDecimal(unitPrice), now, now); err != nil {
			return Position{}, err
		}
		if _, err := replayAccount(ctx, tx, in.AccountID); err != nil {
			return Position{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Position{}, err
	}
	return GetPosition(ctx, conn, id)
}

// openingHolding turns the opening fields into one quantity/cost pair. A
// caller that only knows the amount invested gets a single unit at that cost:
// the replay needs a quantity, and inventing a unit count would report a unit
// price the user never saw.
func openingHolding(in PositionInput) (quantity, total, unitPrice decimal.Decimal) {
	quantity = in.InitialQuantity
	switch {
	case quantity.IsPositive() && in.InitialUnitCost.IsPositive():
		return quantity, quantity.Mul(in.InitialUnitCost), in.InitialUnitCost
	case in.InitialValue == nil || !in.InitialValue.IsPositive():
		return decimal.Zero, decimal.Zero, decimal.Zero
	case quantity.IsPositive():
		return quantity, *in.InitialValue, in.InitialValue.Div(quantity)
	default:
		return decimal.NewFromInt(1), *in.InitialValue, *in.InitialValue
	}
}

// UpdatePosition never touches money: it edits the local description and the
// goal a holding belongs to. A provider holding only accepts the goal.
//
// It reads the holding twice on purpose: once to learn the source and the
// provider fields the caller may not change, and once after commit so the
// caller sees exactly what a subsequent read would. The first read goes
// through GetPosition rather than a lighter query because the provider name,
// ticker and asset type are derived there, and a second derivation could
// reject or accept an update the list would disagree with.
func UpdatePosition(ctx context.Context, conn *sql.DB, id string, in PositionUpdate) (Position, error) {
	current, err := GetPosition(ctx, conn, id)
	if err != nil {
		return Position{}, err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Position{}, err
	}
	defer tx.Rollback()

	if current.Source == PositionSourceSynced {
		if changesProviderFields(current, in) {
			return Position{}, ErrIntegratedReadOnly
		}
		if err := setPortfolioAssignment(ctx, tx, syncedPositionColumn, id, in.PortfolioID); err != nil {
			return Position{}, err
		}
		if err := tx.Commit(); err != nil {
			return Position{}, err
		}
		return GetPosition(ctx, conn, id)
	}

	name, assetType := strings.TrimSpace(in.Name), strings.TrimSpace(in.AssetType)
	if name == "" || assetType == "" || current.AssetID == nil {
		return Position{}, ErrInvalidInput
	}
	asset, err := GetAsset(ctx, tx, *current.AssetID)
	if err != nil {
		return Position{}, err
	}
	assetClass := asset.AssetClass
	if assetType != asset.AssetType {
		assetClass = InferAssetClass(assetType)
	}
	// The asset is shared by every position of the same instrument, so the
	// new identity must not collide with another catalog entry: the UNIQUE
	// index would refuse it anyway, but as a storage error, not a conflict.
	key := assetCanonicalKey(name, in.Ticker, assetType)
	if exists, err := assetKeyExists(ctx, tx, key, *current.AssetID); err != nil {
		return Position{}, err
	} else if exists {
		return Position{}, ErrAssetAlreadyExists
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE investment_assets SET canonical_key = ?, name = ?, ticker = ?, asset_type = ?, asset_class = ?, updated_at = ?
		WHERE id = ?`,
		key, name, nullableString(trimOptional(in.Ticker)), assetType, assetClass,
		db.FormatTime(time.Now()), *current.AssetID); err != nil {
		return Position{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE investment_positions SET notes = ?, updated_at = ?
		WHERE id = ?`,
		nullableString(trimOptional(in.Notes)), db.FormatTime(time.Now()), id); err != nil {
		return Position{}, err
	}
	if err := setPortfolioAssignment(ctx, tx, manualPositionColumn, id, in.PortfolioID); err != nil {
		return Position{}, err
	}
	if err := tx.Commit(); err != nil {
		return Position{}, err
	}
	return GetPosition(ctx, conn, id)
}

func changesProviderFields(current Position, in PositionUpdate) bool {
	same := func(a *string, b string) bool {
		trimmed := strings.TrimSpace(b)
		if a == nil {
			return trimmed == ""
		}
		return *a == trimmed
	}
	return strings.TrimSpace(in.Name) != current.Name ||
		strings.TrimSpace(in.AssetType) != current.AssetType ||
		!same(current.Ticker, derefOrEmpty(in.Ticker)) ||
		!same(current.Notes, derefOrEmpty(in.Notes))
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func DeletePosition(ctx context.Context, conn *sql.DB, id string) error {
	position, err := GetPosition(ctx, conn, id)
	if err != nil {
		return err
	}
	if position.Source != PositionSourceManual {
		return ErrIntegratedReadOnly
	}
	var operations int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM investment_operations WHERE position_id = ?`, id).
		Scan(&operations); err != nil {
		return err
	}
	if operations > 0 {
		return ErrPositionHasOperations
	}
	_, err = conn.ExecContext(ctx, `DELETE FROM investment_positions WHERE id = ?`, id)
	return err
}

// setPortfolioAssignment rewrites the local goal annotation. It is a local
// choice about grouping, so it never writes to the holding itself and never
// records an operation.
func setPortfolioAssignment(ctx context.Context, q Querier, column, positionID string, portfolioID *string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM investment_position_portfolios WHERE `+column+` = ?`, positionID); err != nil {
		return err
	}
	if portfolioID == nil || *portfolioID == "" {
		return nil
	}
	if _, err := getRawPortfolio(ctx, q, *portfolioID); err != nil {
		return err
	}
	now := db.FormatTime(time.Now())
	_, err := q.ExecContext(ctx, `
		INSERT INTO investment_position_portfolios (`+column+`, portfolio_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, positionID, *portfolioID, now, now)
	return err
}
