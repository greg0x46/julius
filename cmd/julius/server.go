package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/httplog"
)

// loggedHandler puts the access log around every route served by serverHandler.
func loggedHandler(logger *slog.Logger, api, mcp http.Handler) http.Handler {
	return httplog.Middleware(logger)(serverHandler(api, mcp))
}

func serverHandler(api, mcp http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp)
	mux.Handle("/oauth/", mcp)
	mux.Handle("/.well-known/oauth-authorization-server", mcp)
	mux.Handle("/.well-known/oauth-protected-resource", mcp)
	mux.Handle("/.well-known/oauth-protected-resource/", mcp)
	mux.Handle("/mcp/", http.NotFoundHandler())
	mux.Handle("/", api)
	return mux
}

// serve drains in-flight requests before the caller closes the database.
func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	done := make(chan struct{})
	stopped := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			err := server.Shutdown(shutdown)
			if err != nil {
				_ = server.Close()
			}
			stopped <- err
		case <-done:
			stopped <- nil
		}
	}()
	err := server.Serve(listener)
	close(done)
	shutdownErr := <-stopped
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return shutdownErr
}
