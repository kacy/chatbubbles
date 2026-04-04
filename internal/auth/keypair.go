package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Identity struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

func EnsureIdentity(dataDir string) (Identity, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return Identity{}, fmt.Errorf("create data dir: %w", err)
	}

	keyPath := filepath.Join(dataDir, "identity.key")
	pubPath := filepath.Join(dataDir, "identity.pub")

	keyExists := fileExists(keyPath)
	pubExists := fileExists(pubPath)

	switch {
	case keyExists && pubExists:
		return loadIdentity(keyPath, pubPath)
	case keyExists || pubExists:
		return Identity{}, errors.New("identity material is incomplete")
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, fmt.Errorf("generate identity: %w", err)
	}

	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return Identity{}, fmt.Errorf("marshal private identity: %w", err)
	}

	if err := writePEMFile(keyPath, 0o600, "PRIVATE KEY", privateDER); err != nil {
		return Identity{}, err
	}

	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return Identity{}, fmt.Errorf("marshal public identity: %w", err)
	}

	if err := writePEMFile(pubPath, 0o644, "PUBLIC KEY", publicDER); err != nil {
		return Identity{}, err
	}

	return Identity{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}, nil
}

func loadIdentity(keyPath string, pubPath string) (Identity, error) {
	privateRaw, err := os.ReadFile(keyPath)
	if err != nil {
		return Identity{}, fmt.Errorf("read identity key: %w", err)
	}

	privateBlock, _ := pem.Decode(privateRaw)
	if privateBlock == nil || privateBlock.Type != "PRIVATE KEY" {
		return Identity{}, errors.New("identity key is not valid pem")
	}

	privateKeyAny, err := x509.ParsePKCS8PrivateKey(privateBlock.Bytes)
	if err != nil {
		return Identity{}, fmt.Errorf("parse identity key: %w", err)
	}

	privateKey, ok := privateKeyAny.(ed25519.PrivateKey)
	if !ok {
		return Identity{}, errors.New("identity key is not ed25519")
	}

	publicRaw, err := os.ReadFile(pubPath)
	if err != nil {
		return Identity{}, fmt.Errorf("read identity public key: %w", err)
	}

	publicBlock, _ := pem.Decode(publicRaw)
	if publicBlock == nil || publicBlock.Type != "PUBLIC KEY" {
		return Identity{}, errors.New("identity public key is not valid pem")
	}

	publicKeyAny, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		return Identity{}, fmt.Errorf("parse identity public key: %w", err)
	}

	publicKey, ok := publicKeyAny.(ed25519.PublicKey)
	if !ok {
		return Identity{}, errors.New("identity public key is not ed25519")
	}

	return Identity{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}, nil
}

func writePEMFile(path string, mode os.FileMode, blockType string, bytes []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer file.Close()

	if err := pem.Encode(file, &pem.Block{Type: blockType, Bytes: bytes}); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
