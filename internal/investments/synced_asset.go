package investments

import (
	"context"
	"errors"
)

// assetIdentity is what the catalog keys a provider holding's asset by: its
// display name, instrument code and type. ResolveSyncedAsset and
// FindSyncedAsset both derive the asset from it.
func (h SyncedHolding) assetIdentity() (name string, ticker *string, kind string) {
	return h.DisplayName(), h.Ticker(), h.AssetType()
}

// FindSyncedAsset is ResolveSyncedAsset without the create: the catalog asset
// a provider holding belongs to, if there is one yet. Read paths use it, so a
// GET never writes and two concurrent ones cannot race on the asset's unique
// key. An asset kept in another currency is not the holding's.
func FindSyncedAsset(ctx context.Context, q Querier, h SyncedHolding) (Asset, bool, error) {
	_, _, _, key, err := normalizeAssetIdentity(h.assetIdentity())
	if errors.Is(err, ErrInvalidInput) {
		// ResolveSyncedAsset refuses such a holding, so no asset can exist.
		return Asset{}, false, nil
	}
	if err != nil {
		return Asset{}, false, err
	}
	asset, found, err := findAssetByKey(ctx, q, key)
	if err != nil || !found || asset.CurrencyCode != h.currency() {
		return Asset{}, false, err
	}
	return asset, true, nil
}
