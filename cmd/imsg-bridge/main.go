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

	"github.com/kacy/imsg-bridge/internal/api"
	"github.com/kacy/imsg-bridge/internal/imsg"
)

const version = "0.1.0"

func main() {
	var cfg api.Config

	flag.StringVar(&cfg.ListenAddr, "listen", ":8443", "http listen address")
	flag.StringVar(&cfg.ServerName, "server-name", hostname(), "server name")
	flag.StringVar(&cfg.TailscaleIP, "tailscale-ip", "", "override detected tailscale ip")
	flag.Parse()

	cfg.Version = version
	cfg.StartedAt = time.Now().UTC()

	runner := imsg.NewRunner("")
	handler := api.NewServer(cfg, runner)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown failed: %v", err)
		}
	}()

	log.Printf("serving on %s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "imsg-bridge"
	}

	return name
}
