package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

// ensureCert makes a self-signed certificate and key when the files do not
// exist yet. Clients pin it until the manager gets a certificate from a CA;
// how APs trust the manager is decided with enrollment (M4).
func ensureCert(certPath, keyPath string, hosts []string, now time.Time) error {
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	if certErr == nil && keyErr == nil {
		return nil
	}
	if certErr == nil || keyErr == nil {
		return errors.New("only one of the TLS certificate and key exists; remove it or supply both")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "aeolus"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if name, err := os.Hostname(); err == nil {
		hosts = append(hosts, name)
	}
	hosts = append(hosts, "localhost", "127.0.0.1", "::1")
	seen := map[string]bool{}
	for _, h := range hosts {
		if seen[h] {
			continue
		}
		seen[h] = true
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// fingerprint is a certificate's SHA-256, of its DER, as browsers show it:
// AB:CD:... A person checks it when joining an AP by hand (0083).
func fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	var b strings.Builder
	for i, c := range sum {
		if i > 0 {
			b.WriteByte(':')
		}
		fmt.Fprintf(&b, "%02X", c)
	}
	return b.String()
}

// runFingerprint prints the fingerprint of the manager's certificate, the
// one aeolus-enroll and LuCI's Aeolus page show on an AP (0083).
func runFingerprint(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	certPath := fs.String("cert", "/etc/aeolus/tls.crt", "TLS certificate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile(*certPath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return fmt.Errorf("%s holds no certificate", *certPath)
	}
	fmt.Fprintln(stdout, fingerprint(block.Bytes))
	return nil
}
