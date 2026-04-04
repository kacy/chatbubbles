package bridgetls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureMaterialCreatesAndReusesCertificate(t *testing.T) {
	dataDir := t.TempDir()

	first, err := EnsureMaterial(dataDir, "test mac", []string{"100.64.0.3"})
	if err != nil {
		t.Fatalf("ensure material: %v", err)
	}

	if !strings.HasPrefix(first.Fingerprint, "SHA256:") {
		t.Fatalf("expected sha256 fingerprint, got %q", first.Fingerprint)
	}

	if _, err := os.Stat(first.CertPath); err != nil {
		t.Fatalf("stat cert: %v", err)
	}

	if _, err := os.Stat(first.KeyPath); err != nil {
		t.Fatalf("stat key: %v", err)
	}

	second, err := EnsureMaterial(dataDir, "test mac", []string{"100.64.0.3"})
	if err != nil {
		t.Fatalf("ensure material second time: %v", err)
	}

	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("expected stable fingerprint, got %q and %q", first.Fingerprint, second.Fingerprint)
	}
}

func TestEnsureMaterialRejectsHalfWrittenState(t *testing.T) {
	dataDir := t.TempDir()
	certPath := filepath.Join(dataDir, "tls.crt")

	if err := os.WriteFile(certPath, []byte("broken"), 0o644); err != nil {
		t.Fatalf("write fake cert: %v", err)
	}

	if _, err := EnsureMaterial(dataDir, "test mac", nil); err == nil {
		t.Fatal("expected error for incomplete tls material")
	}
}
