// Test-only launcher: real HTTP handlers and migrations, no external workers.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/settings"
)

func main() {
	database := flag.String("db", "", "isolated database path")
	frontend := flag.String("frontend", "", "built frontend directory")
	flag.Parse()
	if *database == "" || *frontend == "" {
		log.Fatal("db and frontend are required")
	}
	conn, err := db.Open(*database)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	master := make([]byte, 32) // Synthetic key, confined to the disposable database.
	if err := auth.NewStore(conn).Initialize(context.Background(), "owner@example.test", "synthetic-e2e-password", master, nil); err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	config := auth.Config{PublicURL: origin}
	if err := config.Validate(); err != nil {
		log.Fatal(err)
	}
	// Same header policy as the real binary, so the journeys run under the CSP.
	handler := httpapi.SecurityHeaders(config)(httpapi.NewServer(conn, os.DirFS(*frontend), settings.NewSecrets(master), config, nil))
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Println(origin)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
