package mitm

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"os"
	"testing"
)

func setupTempDir(t *testing.T) (origDir string) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	return origDir
}

func TestNewCreatesFiles(t *testing.T) {
	setupTempDir(t)

	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m == nil {
		t.Fatal("CAManager is nil")
	}

	if _, err := os.Stat(caCertFile); os.IsNotExist(err) {
		t.Errorf("ca.crt not created")
	}
	if _, err := os.Stat(caKeyFile); os.IsNotExist(err) {
		t.Errorf("ca.key not created")
	}
}

func TestNewLoadsExisting(t *testing.T) {
	setupTempDir(t)

	m1, err := New()
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	pem1 := m1.CACertPEM()

	m2, err := New()
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	pem2 := m2.CACertPEM()

	if !bytes.Equal(pem1, pem2) {
		t.Error("second New() returned different cert PEM — should reuse existing files")
	}
}

func TestTLSConfigForHost(t *testing.T) {
	setupTempDir(t)

	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cfg, err := m.TLSConfig("example.com")
	if err != nil {
		t.Fatalf("TLSConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("TLSConfig returned nil")
	}
	if len(cfg.Certificates) == 0 {
		t.Error("TLSConfig has no certificates")
	}
}

func TestTLSConfigCached(t *testing.T) {
	setupTempDir(t)

	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cfg1, err := m.TLSConfig("cached.example.com")
	if err != nil {
		t.Fatalf("TLSConfig 1: %v", err)
	}
	cfg2, err := m.TLSConfig("cached.example.com")
	if err != nil {
		t.Fatalf("TLSConfig 2: %v", err)
	}

	if cfg1 != cfg2 {
		t.Error("expected same *tls.Config pointer on second call (cache miss)")
	}
}

func TestSignedCertVerifiesAgainstCA(t *testing.T) {
	setupTempDir(t)

	m, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tlsCfg, err := m.TLSConfig("verify.example.com")
	if err != nil {
		t.Fatalf("TLSConfig: %v", err)
	}

	// Parse the leaf certificate from the TLS config
	tlsCert := tlsCfg.Certificates[0]
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	// Build CA pool from manager
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(m.CACertPEM()) {
		t.Fatal("failed to append CA cert to pool")
	}

	// Verify leaf against CA pool
	opts := x509.VerifyOptions{
		DNSName: "verify.example.com",
		Roots:   pool,
	}
	// Need to parse intermediates too (chain[1:])
	intermediates := x509.NewCertPool()
	for _, certDER := range tlsCert.Certificate[1:] {
		c, err := x509.ParseCertificate(certDER)
		if err == nil {
			intermediates.AddCert(c)
		}
	}
	opts.Intermediates = intermediates

	if _, err := leaf.Verify(opts); err != nil {
		t.Errorf("leaf cert verification failed: %v", err)
	}

	_ = tls.Certificate{} // ensure tls import is used
}
