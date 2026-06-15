// Command server is the entry point for the URL shortener API.
//
// This file is the "composition root": it reads config, builds dependencies
// (DB pool, click counter), wires them together, starts the HTTP server, and
// shuts everything down cleanly. There is no service container like Laravel's —
// we assemble everything by hand, on purpose.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/leejianyong1997/url_shortener/internal/clicks"
	"github.com/leejianyong1997/url_shortener/internal/config"
	"github.com/leejianyong1997/url_shortener/internal/handler"
	"github.com/leejianyong1997/url_shortener/internal/shortener"
	"github.com/leejianyong1997/url_shortener/internal/storage"
)

const flushInterval = 2 * time.Second

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using real environment variables")
	}
	cfg := config.Load()

	db, err := storage.Connect(cfg.DSN())
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()
	log.Printf("connected to MySQL %s:%s/%s", cfg.DBHost, cfg.DBPort, cfg.DBName)

	// ctx is cancelled when the process gets Ctrl+C (SIGINT) or SIGTERM. Every
	// long-running piece watches it to shut down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Composition root: store -> counter -> service -> handler.
	store := storage.NewLinkStore(db)
	counter := clicks.NewCounter(store)
	svc := shortener.NewService(store, counter)
	h := handler.New(svc, cfg.BaseURL)

	// Background goroutine: flush buffered clicks to MySQL every few seconds.
	go counter.Run(ctx, flushInterval)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /shorten", h.Shorten)
	mux.HandleFunc("GET /{code}", h.Redirect)

	srv := &http.Server{Addr: cfg.ServerAddr, Handler: mux}

	// Start the server in its own goroutine so main can wait for the signal.
	go func() {
		log.Printf("listening on http://localhost%s", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done() // block here until Ctrl+C
	log.Println("shutting down...")

	// Stop accepting new requests; give in-flight ones up to 5s to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}

	// Final flush so clicks buffered since the last tick are not lost. We use a
	// fresh context because ctx is already cancelled.
	if err := counter.Flush(context.Background()); err != nil {
		log.Printf("final clicks flush: %v", err)
	}
	log.Println("stopped")
}
