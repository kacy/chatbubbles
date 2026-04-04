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
	"strings"
	"syscall"
	"time"

	"github.com/kacy/imsg-bridge/internal/api"
	"github.com/kacy/imsg-bridge/internal/auth"
	"github.com/kacy/imsg-bridge/internal/buildinfo"
	"github.com/kacy/imsg-bridge/internal/ctlsock"
	"github.com/kacy/imsg-bridge/internal/events"
	"github.com/kacy/imsg-bridge/internal/imsg"
	"github.com/kacy/imsg-bridge/internal/store"
	bridgetls "github.com/kacy/imsg-bridge/internal/tls"
	"github.com/kacy/imsg-bridge/internal/webhook"
)

func main() {
	var cfg api.Config
	var dataDir string
	var socketPath string

	flag.StringVar(&cfg.ListenAddr, "listen", ":8443", "http listen address")
	flag.StringVar(&cfg.ServerName, "server-name", hostname(), "server name")
	flag.StringVar(&cfg.TailscaleIP, "tailscale-ip", "", "override detected tailscale ip")
	flag.StringVar(&dataDir, "data-dir", defaultDataDir(), "data directory")
	flag.StringVar(&socketPath, "socket", "", "control socket path")
	flag.Parse()

	cfg.Version = buildinfo.Version
	cfg.StartedAt = time.Now().UTC()
	if cfg.TailscaleIP == "" {
		cfg.TailscaleIP = detectTailscaleIP()
	}
	pairHost := pairHost(cfg.ListenAddr, cfg.TailscaleIP)
	if socketPath == "" {
		socketPath = filepath.Join(dataDir, "imsg-bridge.sock")
	}

	material, err := bridgetls.EnsureMaterial(dataDir, cfg.ServerName, []string{cfg.TailscaleIP, hostname()})
	if err != nil {
		log.Fatal(err)
	}

	identity, err := auth.EnsureIdentity(dataDir)
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
	authService := auth.NewService(stateStore, auth.NewTokenManager(identity))
	webhookValidator := webhook.NewValidator()
	webhookService := webhook.NewService(stateStore, webhookValidator)
	webhookDispatcher := webhook.NewDispatcher(stateStore, webhookValidator)
	bootstrapCode, err := authService.EnsureBootstrap(cfg.ServerName)
	if err != nil {
		log.Fatal(err)
	}

	handler := api.NewServer(cfg, runner, hub, authService, webhookService)
	controlServer := ctlsock.New(socketPath, authService, func(ctx context.Context) (ctlsock.Status, error) {
		imsgVersion, err := runner.Version(ctx)
		if err != nil {
			return ctlsock.Status{}, err
		}

		clients, err := authService.ListClients()
		if err != nil {
			return ctlsock.Status{}, err
		}

		return ctlsock.Status{
			ServerName:     cfg.ServerName,
			Version:        buildinfo.Version,
			ImsgVersion:    imsgVersion,
			TailscaleIP:    cfg.TailscaleIP,
			PairHost:       pairHost,
			TLSFingerprint: material.Fingerprint,
			UptimeSeconds:  int64(time.Since(cfg.StartedAt).Seconds()),
			Clients:        clients,
		}, nil
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lastWatchRowID := state.LastWatchRowID
	watcher := imsg.NewWatcher(runner, lastWatchRowID)
	go func() {
		err := watcher.Run(ctx, func(event imsg.WatchEvent) {
			if rowID := event.RowID(); rowID > lastWatchRowID {
				lastWatchRowID = rowID
				if _, err := stateStore.Update(func(cfg *store.Config) error {
					cfg.LastWatchRowID = lastWatchRowID
					return nil
				}); err != nil {
					log.Printf("save watch state failed: %v", err)
				}
			}

			hub.Publish(events.Event{
				Type: event.Name(),
				Data: event.Message,
			})
			webhookDispatcher.Dispatch(ctx, events.Event{
				Type: event.Name(),
				Data: event.Message,
			})
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("watcher exited: %v", err)
		}
	}()

	go func() {
		if err := controlServer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("control socket exited: %v", err)
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
	log.Printf("control socket %s", socketPath)
	log.Printf("tls fingerprint %s", material.Fingerprint)
	logPairingWarnings(cfg.ListenAddr, cfg.TailscaleIP, pairHost)
	if bootstrapCode != "" {
		log.Printf("bootstrap pairing code %s", bootstrapCode)
	}
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

func pairHost(listenAddr string, tailscaleIP string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return ""
	}

	switch host {
	case "", "0.0.0.0", "::":
		if tailscaleIP != "" {
			host = tailscaleIP
		} else {
			host = "localhost"
		}
	}

	return net.JoinHostPort(host, port)
}

func logPairingWarnings(listenAddr string, tailscaleIP string, host string) {
	if tailscaleIP != "" {
		log.Printf("pair host %s", host)
		return
	}

	log.Printf("warning: tailscale ip was not detected automatically")

	if host == "" {
		log.Printf("warning: direct pairing is missing a reachable host; start with --tailscale-ip <ip> or bind to an explicit host")
		return
	}

	if strings.HasPrefix(host, "localhost:") || strings.HasPrefix(host, "127.0.0.1:") || strings.HasPrefix(host, "[::1]:") {
		log.Printf("warning: direct pairing will advertise %s, which only works on this machine", host)
		log.Printf("warning: if you expect phone or browser pairing over tailscale, start with --tailscale-ip <ip>")
		return
	}

	log.Printf("warning: pairing is using %s without a detected tailscale ip", host)
}
