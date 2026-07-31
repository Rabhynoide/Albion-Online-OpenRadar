// Command hub runs the OpenRadar Hub: a small self-hostable service that lets a group
// of radar clients pool discovered Avalonian Road edges into one shared database.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nospy/albion-openradar/internal/hub"
)

const shutdownTimeout = 10 * time.Second

func main() {
	port := envOr("PORT", "8090")
	dbPath := envOr("DB_PATH", "/data/hub.db")
	secret := os.Getenv("HUB_SECRET")
	if secret == "" {
		fmt.Println("HUB_SECRET is required (refusing to start unauthenticated)")
		os.Exit(1)
	}

	store, err := hub.OpenStore(dbPath)
	if err != nil {
		fmt.Printf("Failed to open store at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer store.Close()

	api := hub.NewAPI(store, secret)
	mux := http.NewServeMux()
	api.Register(mux)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-interrupt()
		fmt.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			fmt.Printf("Shutdown error: %v\n", err)
		}
	}()

	fmt.Printf("OpenRadar Hub listening on :%s (db: %s)\n", port, dbPath)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// interrupt returns a channel that fires once on SIGINT/SIGTERM.
func interrupt() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}
