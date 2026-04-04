package bridgetls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type Material struct {
	CertPath    string
	KeyPath     string
	Fingerprint string
}

func EnsureMaterial(dataDir string, serverName string, hosts []string) (Material, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return Material{}, fmt.Errorf("create data dir: %w", err)
	}

	material := Material{
		CertPath: filepath.Join(dataDir, "tls.crt"),
		KeyPath:  filepath.Join(dataDir, "tls.key"),
	}

	certExists := fileExists(material.CertPath)
	keyExists := fileExists(material.KeyPath)

	switch {
	case certExists && keyExists:
		fingerprint, err := fingerprintFromCertFile(material.CertPath)
		if err != nil {
			return Material{}, err
		}

		material.Fingerprint = fingerprint
		return material, nil
	case certExists || keyExists:
		return Material{}, errors.New("tls material is incomplete")
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Material{}, fmt.Errorf("generate key: %w", err)
	}

	now := time.Now().UTC()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return Material{}, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: strings.TrimSpace(serverName),
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, host := range normalizedHosts(hosts) {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}

		template.DNSNames = append(template.DNSNames, host)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		return Material{}, fmt.Errorf("create certificate: %w", err)
	}

	if err := writePEMFile(material.CertPath, 0o644, "CERTIFICATE", certDER); err != nil {
		return Material{}, err
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return Material{}, fmt.Errorf("marshal private key: %w", err)
	}

	if err := writePEMFile(material.KeyPath, 0o600, "PRIVATE KEY", keyDER); err != nil {
		return Material{}, err
	}

	material.Fingerprint = formatFingerprint(certDER)
	return material, nil
}

func normalizedHosts(hosts []string) []string {
	values := []string{"localhost", "127.0.0.1", "::1"}

	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}

		values = append(values, host)
	}

	slices.Sort(values)
	return slices.Compact(values)
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

func fingerprintFromCertFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read certificate: %w", err)
	}

	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("certificate is not valid pem")
	}

	return formatFingerprint(block.Bytes), nil
}

func formatFingerprint(certDER []byte) string {
	sum := sha256.Sum256(certDER)
	return "SHA256:" + strings.ToUpper(hex.EncodeToString(sum[:]))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
