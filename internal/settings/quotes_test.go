package settings_test

import (
	"context"
	"github.com/greg0x46/julius/internal/settings"
	"testing"
)

func TestQuoteRefreshValidationAndStorage(t *testing.T) {
	ctx := context.Background()
	conn := newTestDB(t)
	defaults, found, err := settings.GetQuoteRefresh(ctx, conn)
	if err != nil || found || defaults != settings.DefaultQuoteRefreshSettings() {
		t.Fatalf("defaults: %+v %v %v", defaults, found, err)
	}
	saved := settings.QuoteRefreshSettings{Enabled: false, Time: "08:15", Timezone: "Asia/Tokyo"}
	if err := settings.SetQuoteRefresh(ctx, conn, saved); err != nil {
		t.Fatal(err)
	}
	if got, err := settings.InitializeQuoteRefresh(ctx, conn, settings.DefaultQuoteRefreshSettings()); err != nil || got != saved {
		t.Fatalf("initializer overwrote saved settings: %+v %v", got, err)
	}
	for _, invalid := range []settings.QuoteRefreshSettings{
		{Time: "8:15", Timezone: "UTC"}, {Time: "24:00", Timezone: "UTC"},
		{Time: "19:00", Timezone: ""}, {Time: "19:00", Timezone: "Unknown/Zone"},
	} {
		if err := settings.SetQuoteRefresh(ctx, conn, invalid); err == nil {
			t.Fatalf("accepted %+v", invalid)
		}
	}
	got, found, err := settings.GetQuoteRefresh(ctx, conn)
	if err != nil || !found || got != saved {
		t.Fatalf("saved: %+v %v %v", got, found, err)
	}
	var encrypted int
	if err := conn.QueryRow(`SELECT is_encrypted FROM settings WHERE key = ?`, settings.KeyQuoteRefresh).Scan(&encrypted); err != nil || encrypted != 0 {
		t.Fatalf("encryption: %d %v", encrypted, err)
	}
	if err := settings.Set(ctx, conn, settings.KeyQuoteRefresh, `{"enabled":true,"time":"bad","timezone":"UTC"}`, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := settings.GetQuoteRefresh(ctx, conn); err == nil {
		t.Fatal("invalid saved config accepted")
	}
}
