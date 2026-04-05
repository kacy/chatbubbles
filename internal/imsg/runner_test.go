package imsg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinaryPrefersExplicitPath(t *testing.T) {
	t.Setenv("CHATBUBBLES_IMSG_BIN", "/tmp/from-env")

	if got := ResolveBinary("/tmp/from-flag"); got != "/tmp/from-flag" {
		t.Fatalf("expected explicit path, got %q", got)
	}
}

func TestResolveBinaryPrefersEnvVar(t *testing.T) {
	t.Setenv("CHATBUBBLES_IMSG_BIN", "/tmp/from-env")

	if got := ResolveBinary(""); got != "/tmp/from-env" {
		t.Fatalf("expected env path, got %q", got)
	}
}

func TestResolveBinaryFindsSiblingCheckoutFromWorkingDir(t *testing.T) {
	root := t.TempDir()
	bridgeDir := filepath.Join(root, "chatbubbles")
	imsgDir := filepath.Join(root, "imsg", "bin")

	if err := os.MkdirAll(bridgeDir, 0o755); err != nil {
		t.Fatalf("mkdir bridge dir: %v", err)
	}
	if err := os.MkdirAll(imsgDir, 0o755); err != nil {
		t.Fatalf("mkdir imsg dir: %v", err)
	}

	imsgPath := filepath.Join(imsgDir, "imsg")
	if err := os.WriteFile(imsgPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write imsg binary: %v", err)
	}

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(prevWD); chdirErr != nil {
			t.Fatalf("restore wd: %v", chdirErr)
		}
	})

	if err := os.Chdir(bridgeDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Setenv("CHATBUBBLES_IMSG_BIN", "")

	got := ResolveBinary("")
	resolvedGot, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("resolve got path: %v", err)
	}
	resolvedWant, err := filepath.EvalSymlinks(imsgPath)
	if err != nil {
		t.Fatalf("resolve want path: %v", err)
	}

	if resolvedGot != resolvedWant {
		t.Fatalf("expected sibling checkout path %q, got %q", resolvedWant, resolvedGot)
	}
}

func TestResolveBinaryFallsBackToPathLookup(t *testing.T) {
	t.Setenv("CHATBUBBLES_IMSG_BIN", "")

	if got := ResolveBinary(""); got != "imsg" {
		t.Fatalf("expected default binary name, got %q", got)
	}
}
