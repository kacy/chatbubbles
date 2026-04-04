package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kacy/imsg-bridge/internal/api"
	"github.com/kacy/imsg-bridge/internal/events"
	"github.com/kacy/imsg-bridge/internal/imsg"
	"github.com/kacy/imsg-bridge/internal/store"
	bridgetls "github.com/kacy/imsg-bridge/internal/tls"
)

const version = "0.1.0"

func main() {
	var cfg api.Config
	var dataDir string

	flag.StringVar(&cfg.ListenAddr, "listen", ":8443", "http listen address")
	flag.StringVar(&cfg.ServerName, "server-name", hostname(), "server name")
	flag.StringVar(&cfg.TailscaleIP, "tailscale-ip", "", "override detected tailscale ip")
	flag.StringVar(&dataDir, "data-dir", defaultDataDir(), "data directory")
	flag.Parse()

	cfg.Version = version
	cfg.StartedAt = time.Now().UTC()
	if cfg.TailscaleIP == "" {
		cfg.TailscaleIP = detectTailscaleIP()
	}

	material, err := bridgetls.EnsureMaterial(dataDir, cfg.ServerName, []string{cfg.TailscaleIP, hostname()})
	if err != nil {
		log.Fatal(err)
	}

	runner := imsg.NewRunner("")
	hub := events.NewHub()
	stateStore := store.New(filepath.Join(dataDir, "config.json"))
	state, err := stateStore.Load()
	if err != nil {
		log.Fatal(err)
	}

	handler := api.NewServer(cfg, runner, hub)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	watcher := imsg.NewWatcher(runner, state.LastWatchRowID)
	go func() {
		err := watcher.Run(ctx, func(event imsg.WatchEvent) {
			if rowID := event.RowID(); rowID > state.LastWatchRowID {
				state.LastWatchRowID = rowID
				if err := stateStore.Save(store.Config{LastWatchRowID: state.LastWatchRowID}); err != nil {
					log.Printf("save watch state failed: %v", err)
				}
			}

			hub.Publish(events.Event{
				Type: event.Name(),
				Data: event.Message,
			})
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("watcher exited: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown failed: %v", err)
		}
	}()

	log.Printf("serving https on %s", cfg.ListenAddr)
	log.Printf("tls fingerprint %s", material.Fingerprint)
	if err := srv.ListenAndServeTLS(material.CertPath, material.KeyPath); err != nil && err != http.ErrServerClosed {
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

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "imsg-bridge-data"
	}

	return filepath.Join(home, ".local", "share", "imsg-bridge")
}

func detectTailscaleIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			if ip[0] == 100 {
				return ip.String()
			}
		}
	}

	return ""
}
