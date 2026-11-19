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

	"private-drop/internal/api"
	"private-drop/internal/config"
	"private-drop/internal/db"
)

func main() {
	cfg := config.Load()
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("create data directories: %v", err)
	}
	database, err := db.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()
	if err := database.Migrate(); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	srv := api.New(database, cfg)
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Background retention / recycle cleanup.
	cleanCtx, cleanCancel := context.WithCancel(context.Background())
	defer cleanCancel()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-cleanCtx.Done():
				return
			case <-ticker.C:
				if _, err := srv.RunCleanup(cleanCtx); err != nil {
					log.Printf("cleanup: %v", err)
				}
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Private Drop listening on :%s (data dir: %s)", cfg.Port, cfg.DataDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
	case err := <-errCh:
		log.Printf("server error: %v", err)
	}
	cleanCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
