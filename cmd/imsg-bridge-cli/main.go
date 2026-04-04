package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	socketPath := defaultSocketPath()

	fs := flag.NewFlagSet("imsg-bridge-cli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&socketPath, "socket", socketPath, "control socket path")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	args := fs.Args()
	if len(args) == 0 {
		return fmt.Errorf("usage: imsg-bridge-cli [--socket path] <pair|status|clients|revoke>")
	}

	switch args[0] {
	case "pair":
		return runPair(socketPath, args[1:])
	case "status":
		return runStatus(socketPath)
	case "clients":
		return runClients(socketPath)
	case "revoke":
		return runRevoke(socketPath, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runPair(socketPath string, args []string) error {
	fs := flag.NewFlagSet("pair", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	ttl := fs.Int("ttl", 120, "pairing code ttl in seconds")
	admin := fs.Bool("admin", false, "grant admin scope")
	if err := fs.Parse(args); err != nil {
		return err
	}

	scopes := []string{"read", "send", "attach"}
	if *admin {
		scopes = append(scopes, "admin")
	}

	resp, err := request(socketPath, map[string]any{
		"cmd":    "generate_pairing_code",
		"ttl":    *ttl,
		"scopes": scopes,
	})
	if err != nil {
		return err
	}

	var body struct {
		Code      string   `json:"code"`
		ExpiresAt string   `json:"expires_at"`
		Scopes    []string `json:"scopes"`
		Error     string   `json:"error"`
	}
	if err := json.Unmarshal(resp, &body); err != nil {
		return err
	}
	if body.Error != "" {
		return fmt.Errorf("%s", body.Error)
	}

	fmt.Printf("code: %s\n", body.Code)
	fmt.Printf("expires_at: %s\n", body.ExpiresAt)
	fmt.Printf("scopes: %s\n", strings.Join(body.Scopes, ","))
	return nil
}

func runStatus(socketPath string) error {
	resp, err := request(socketPath, map[string]any{"cmd": "status"})
	if err != nil {
		return err
	}

	var body struct {
		ServerName     string `json:"server_name"`
		Version        string `json:"version"`
		ImsgVersion    string `json:"imsg_version"`
		TailscaleIP    string `json:"tailscale_ip"`
		TLSFingerprint string `json:"tls_fingerprint"`
		UptimeSeconds  int64  `json:"uptime_seconds"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal(resp, &body); err != nil {
		return err
	}
	if body.Error != "" {
		return fmt.Errorf("%s", body.Error)
	}

	fmt.Printf("server_name: %s\n", body.ServerName)
	fmt.Printf("version: %s\n", body.Version)
	fmt.Printf("imsg_version: %s\n", body.ImsgVersion)
	if body.TailscaleIP != "" {
		fmt.Printf("tailscale_ip: %s\n", body.TailscaleIP)
	}
	fmt.Printf("tls_fingerprint: %s\n", body.TLSFingerprint)
	fmt.Printf("uptime_seconds: %d\n", body.UptimeSeconds)
	return nil
}

func runClients(socketPath string) error {
	resp, err := request(socketPath, map[string]any{"cmd": "list_clients"})
	if err != nil {
		return err
	}

	var body struct {
		Clients []struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			Type     string   `json:"type"`
			Scopes   []string `json:"scopes"`
			Revoked  bool     `json:"revoked"`
			LastSeen string   `json:"last_seen"`
		} `json:"clients"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp, &body); err != nil {
		return err
	}
	if body.Error != "" {
		return fmt.Errorf("%s", body.Error)
	}

	for _, client := range body.Clients {
		fmt.Printf("%s\t%s\t%s\t%s\trevoked=%t\tlast_seen=%s\n", client.ID, client.Type, client.Name, strings.Join(client.Scopes, ","), client.Revoked, client.LastSeen)
	}
	return nil
}

func runRevoke(socketPath string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: imsg-bridge-cli revoke <client-id>")
	}

	resp, err := request(socketPath, map[string]any{
		"cmd":       "revoke_client",
		"client_id": args[0],
	})
	if err != nil {
		return err
	}

	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp, &body); err != nil {
		return err
	}
	if body.Error != "" {
		return fmt.Errorf("%s", body.Error)
	}
	if !body.OK {
		return fmt.Errorf("client not found")
	}

	fmt.Println("ok")
	return nil
}

func request(socketPath string, req any) ([]byte, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}

	var resp json.RawMessage
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func defaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "imsg-bridge.sock"
	}

	return filepath.Join(home, ".local", "share", "imsg-bridge", "imsg-bridge.sock")
}
