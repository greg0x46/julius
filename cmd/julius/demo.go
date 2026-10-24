package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/demo"
)

// demoCommand fills a fresh database with synthetic data for a public demo
// instance (run it with JULIUS_DEMO=true). It never touches a database that
// already has an owner, so resetting means starting from an empty file.
func demoCommand(args []string) error {
	if len(args) == 0 || args[0] != "seed" {
		return fmt.Errorf("uso: julius demo seed [-db caminho ou DSN]")
	}
	flags := flag.NewFlagSet("demo seed", flag.ContinueOnError)
	path := flags.String("db", db.DefaultPath(), "arquivo SQLite ou DSN Postgres, sem dono configurado")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("argumentos inesperados")
	}
	config := auth.Config{PublicURL: os.Getenv("JULIUS_PUBLIC_URL")}
	if err := config.Validate(); err != nil {
		return err
	}
	master, err := auth.LoadMasterKey()
	if err != nil {
		return err
	}
	conn, err := db.Open(*path)
	if err != nil {
		return err
	}
	defer conn.Close()
	summary, err := demo.Seed(context.Background(), conn, demo.Options{Master: master, PublicURL: config.PublicURL, Now: time.Now()})
	if err != nil {
		return err
	}
	fmt.Printf("Demo populada: %d contas, %d lançamentos, %d transferências, %d recorrências, %d dívidas, %d metas, %d regras, %d posições, %d operações.\n",
		summary.Accounts, summary.Transactions, summary.Transfers, summary.Recurring, summary.Payables, summary.Goals, summary.Rules, summary.Positions, summary.Operations)
	return nil
}
