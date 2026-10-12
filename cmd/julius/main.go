// Command julius is the single-binary server: it opens (and migrates)
// the database — SQLite by default, or Postgres via -db's DSN — wires the
// HTTP API, runs the background sync worker, and serves the embedded
// frontend build, with no external runtime or deployment step required for
// the SQLite (default) setup.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"
	_ "time/tzdata" // static binaries have no system zoneinfo for JULIUS_SYNC_SCHEDULE zones

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/quotes"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/syncsvc"
	"github.com/greg0x46/julius/internal/webui"
	"github.com/greg0x46/julius/internal/worker"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "seed" {
		if err := seedCommand(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "auth" {
		if err := authCommand(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	defaultDB := db.DefaultPath()
	dbPath := flag.String("db", defaultDB, "path to the SQLite database file, or a postgres://... / postgresql://... DSN to use Postgres instead (defaults to $JULIUS_DB if set)")
	addr := flag.String("addr", "localhost:4200", "address to listen on")
	flag.Parse()

	config := auth.Config{PublicURL: os.Getenv("JULIUS_PUBLIC_URL")}
	if err := config.Validate(); err != nil {
		log.Fatal(err)
	}
	master, err := auth.LoadMasterKey()
	if err != nil {
		log.Fatal(err)
	}
	schedule, scheduled, err := worker.ParseSchedule(os.Getenv("JULIUS_SYNC_SCHEDULE"))
	if err != nil {
		log.Fatal(err)
	}
	quoteProviders, err := marketdata.ParseProviders(os.Getenv("JULIUS_QUOTES_PROVIDERS"))
	if err != nil {
		log.Fatal(err)
	}
	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer conn.Close()
	if err := auth.NewStore(conn).Ready(context.Background(), master); err != nil {
		log.Fatal(err)
	}

	quotesConfig, err := quotes.InitializeSchedule(context.Background(), conn, os.Getenv("JULIUS_QUOTES_SCHEDULE"))
	if err != nil {
		log.Fatal(err)
	}

	// Same-person transfers synced before that label was mapped (see
	// categories.SourceCategoryMapping) still count as income in every total.
	// The fix travels with the binary, not with the schema, so it runs here
	// instead of as a migration: the mapping is Go code, and goose only has
	// SQL migrations to apply. BackfillAutomatic never touches a transaction
	// that already has a category, so running it on every start is a no-op
	// once the old rows are done.
	//
	// Logged rather than fatal: this pass fixes how old rows are *reported*,
	// so failing it is not a reason to refuse to start — and taking the
	// binary down would also take down the HTTP server that is the only way
	// to see what went wrong. The next start retries it.
	if applied, err := categories.BackfillAutomatic(context.Background(), conn, categories.SamePersonTransferLabel); err != nil {
		log.Printf("backfill %q categories failed, leaving those rows uncategorized: %v",
			categories.SamePersonTransferLabel, err)
	} else if applied > 0 {
		log.Printf("categorized %d %q transactions as transfers", applied, categories.SamePersonTransferLabel)
	}

	// Same reasoning as the same-person-transfer backfill above: card-bill
	// payment legs synced before this rule existed need a one-time pass, and
	// it is logged rather than fatal for the same reason.
	if applied, err := categories.BackfillAutomaticCardPayment(context.Background(), conn); err != nil {
		log.Printf("backfill card payment categories failed, leaving those rows uncategorized: %v", err)
	} else if applied > 0 {
		log.Printf("categorized %d card payment transactions as transfers", applied)
	}

	// Synced holdings had no price history before investment_asset_quotes;
	// raw_imports kept every investments payload, so the series is rebuilt
	// from them once. Logged rather than fatal for the same reason as above:
	// without it rendimento falls back to the movement history.
	if processed, err := syncsvc.BackfillAssetQuotes(context.Background(), conn); err != nil {
		log.Printf("backfill investment asset quotes failed, rendimento falls back to movement history: %v", err)
	} else if processed > 0 {
		log.Printf("rebuilt investment asset quotes from %d stored holding snapshots", processed)
	}

	frontend, err := webui.DistFS()
	if err != nil {
		log.Fatalf("load embedded frontend: %v", err)
	}

	secrets := settings.NewSecrets(master)
	marketService, err := marketdata.NewDefault(marketdata.Config{Providers: quoteProviders, BrapiToken: quotes.BrapiToken(conn, secrets)})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.Run(ctx, conn, secrets, worker.Config{
		Pluggy:                pluggy.DefaultConfig(),
		OnTransactionUpserted: automation.NewTransactionHook(payables.UnlinkIfPresent),
	})

	if scheduled {
		log.Printf("sync scheduled daily at %02d:%02d %s", schedule.Hour, schedule.Minute, schedule.Location)
		go worker.RunSchedule(ctx, conn, schedule)
	}

	log.Printf("quote refresh enabled=%t daily at %s %s", quotesConfig.Enabled, quotesConfig.Time, quotesConfig.Timezone)
	go quotes.RunSchedule(ctx, conn, marketService)

	handler := httpapi.NewServer(conn, frontend, secrets, config)
	log.Printf("listening on http://%s", *addr)
	server := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
