package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NekoSuneVR/NekoDL/core/internal/api"
	"github.com/NekoSuneVR/NekoDL/core/internal/config"
	"github.com/NekoSuneVR/NekoDL/core/internal/resolver"
	"github.com/NekoSuneVR/NekoDL/core/internal/scheduler"
	"github.com/NekoSuneVR/NekoDL/core/internal/settings"
	"github.com/NekoSuneVR/NekoDL/core/internal/ytdlpengine"
)

func main() {
	configPath := flag.String("config", "nekodl.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if cfg.APIToken == "" {
		log.Println("warning: api_token is empty in config — the API is unauthenticated")
	}

	store := scheduler.NewStore(cfg.DataDir)
	sched := scheduler.New(cfg.MaxConcurrentDownloads, store)
	resolvers := resolver.NewRegistry(resolver.Dropbox{}, resolver.Pixeldrain{}, resolver.GoogleDrive{}, resolver.Mediafire{})

	settingsStore, err := settings.NewStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to load settings: %v", err)
	}

	persistCtx, stopPersisting := context.WithCancel(context.Background())
	defer stopPersisting()
	go sched.PersistPeriodically(persistCtx, 2*time.Second)

	// Independent of any task/download lifecycle by design — never
	// triggered mid-download, just a background check on its own schedule.
	updateCtx, stopUpdateChecks := context.WithCancel(context.Background())
	defer stopUpdateChecks()
	go ytdlpengine.RunPeriodicUpdateCheck(updateCtx, "", 24*time.Hour, func(output string, err error) {
		if err != nil {
			log.Printf("yt-dlp update check failed: %v", err)
			return
		}
		log.Printf("yt-dlp update check: %s", output)
	})

	srv := api.New(cfg, sched, resolvers, settingsStore)
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srv.Handler(),
	}

	go func() {
		log.Printf("NekoDL core listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
