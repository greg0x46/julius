package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

func serverHandler(api, mcp http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp)
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
