package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"sync"
	"time"
)

const (
	caDir      = "ca"
	caCertFile = "ca/ca.crt"
	caKeyFile  = "ca/ca.key"
	subjectCN  = "MasterHttpRelayVPN"
)

// CAManager generates and caches TLS certificates for MITM interception.
type CAManager struct {
	caCert    *x509.Certificate
	caKey     *rsa.PrivateKey
	caCertPEM []byte
	mu        sync.RWMutex
	cache     map[string]*tls.Config
}

// New loads or creates a CA from ca/ca.crt and ca/ca.key relative to the working directory.
func New() (*CAManager, error) {
	m := &CAManager{cache: make(map[string]*tls.Config)}

	certPEM, err := os.ReadFile(caCertFile)
	if err == nil {
		keyPEM, err := os.ReadFile(caKeyFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA key: %w", err)
		}
		if err := m.loadCA(certPEM, keyPEM); err != nil {
			return nil, fmt.Errorf("loading CA: %w", err)
		}
		return m, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading CA cert: %w", err)
	}

	// Generate a new CA
	if err := os.MkdirAll(caDir, 0755); err != nil {
		return nil, fmt.Errorf("creating ca dir: %w", err)
	}

	certPEM, keyPEM, err := generateCA()
	if err != nil {
		return nil, fmt.Errorf("generating CA: %w", err)
	}

	if err := os.WriteFile(caCertFile, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("writing CA cert: %w", err)
	}
	if err := os.WriteFile(caKeyFile, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("writing CA key: %w", err)
	}

	if err := m.loadCA(certPEM, keyPEM); err != nil {
		return nil, fmt.Errorf("loading generated CA: %w", err)
	}
	return m, nil
}

func (m *CAManager) loadCA(certPEM, keyPEM []byte) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to decode CA cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parsing CA cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("failed to decode CA key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parsing CA key: %w", err)
	}

	m.caCert = cert
	m.caKey = key
	m.caCertPEM = certPEM
	return nil
}

func generateCA() (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: subjectCN,
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, nil
}

// TLSConfig returns a *tls.Config for the given host, with a freshly signed leaf cert (cached).
func (m *CAManager) TLSConfig(host string) (*tls.Config, error) {
	m.mu.RLock()
	if cfg, ok := m.cache[host]; ok {
		m.mu.RUnlock()
		return cfg, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cfg, ok := m.cache[host]; ok {
		return cfg, nil
	}

	leafCert, err := m.generateLeafCert(host)
	if err != nil {
		return nil, fmt.Errorf("generating leaf cert for %q: %w", host, err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{*leafCert},
	}
	m.cache[host] = tlsCfg
	return tlsCfg, nil
}

func (m *CAManager) generateLeafCert(host string) (*tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: host,
		},
		Issuer:    m.caCert.Subject,
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	// Use IP SAN if host is an IP, otherwise DNS SAN
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, m.caCert, &key.PublicKey, m.caKey)
	if err != nil {
		return nil, err
	}

	// Concatenate leaf PEM + CA PEM for the chain
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	chainPEM := append(leafPEM, m.caCertPEM...)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	tlsCert, err := tls.X509KeyPair(chainPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tlsCert, nil
}

// CACertFile returns the path to the CA certificate file.
func (m *CAManager) CACertFile() string { return caCertFile }

// CACertPEM returns the CA certificate in PEM format.
func (m *CAManager) CACertPEM() []byte { return m.caCertPEM }
