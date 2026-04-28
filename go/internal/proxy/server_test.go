package proxy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/logicalangel/HttpOverVercel/internal/mitm"
	"github.com/logicalangel/HttpOverVercel/internal/proxy"
	"github.com/logicalangel/HttpOverVercel/internal/relay"
)

// mockRelayer records the last call and returns configured responses.
type mockRelayer struct {
	mu         sync.Mutex
	status     int
	headers    map[string]string
	body       []byte
	lastMethod string
	lastURL    string
	err        error
}

func (m *mockRelayer) Relay(ctx context.Context, method, targetURL string, headers map[string]string, body []byte) (*relay.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMethod = method
	m.lastURL = targetURL
	if m.err != nil {
		return nil, m.err
	}
	return &relay.Response{
		StatusCode: m.status,
		Headers:    m.headers,
		Body:       m.body,
	}, nil
}

// startTestProxy launches a proxy on a random free port and returns the address.
func startTestProxy(t *testing.T, ca *mitm.CAManager, r proxy.Relayer) string {
	t.Helper()

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	srv := proxy.New("127.0.0.1", port, ca, r)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go srv.ListenAndServe(ctx) //nolint:errcheck
	time.Sleep(80 * time.Millisecond)

	return addr
}

// setupCA creates a CAManager in a temp dir, restoring cwd on cleanup.
func setupCA(t *testing.T) *mitm.CAManager {
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

	ca, err := mitm.New()
	if err != nil {
		t.Fatalf("mitm.New: %v", err)
	}
	return ca
}

func TestProxyPlainHTTP(t *testing.T) {
	mock := &mockRelayer{
		status:  200,
		headers: map[string]string{"Content-Type": "text/plain"},
		body:    []byte("plain response"),
	}

	ca := setupCA(t)
	addr := startTestProxy(t, ca, mock)

	proxyURL, _ := url.Parse("http://" + addr)
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	client := &http.Client{Transport: transport}

	resp, err := client.Get("http://example.com/test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "plain response" {
		t.Errorf("body: got %q want %q", body, "plain response")
	}
}

func TestProxyConnectMITM(t *testing.T) {
	mock := &mockRelayer{
		status:  200,
		headers: map[string]string{},
		body:    []byte("hello"),
	}

	ca := setupCA(t)
	addr := startTestProxy(t, ca, mock)

	// Build pool trusting the MITM CA.
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(ca.CACertPEM()) {
		t.Fatal("failed to append CA cert")
	}

	proxyURL, _ := url.Parse("http://" + addr)
	transport := &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{RootCAs: caCertPool},
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Get("https://example.com/test")
	if err != nil {
		t.Fatalf("HTTPS GET through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("body: got %q want %q", body, "hello")
	}

	mock.mu.Lock()
	gotURL := mock.lastURL
	mock.mu.Unlock()

	if !strings.HasPrefix(gotURL, "https://example.com") {
		t.Errorf("relay URL: got %q, want prefix https://example.com", gotURL)
	}
}

func TestProxyReturns502OnRelayError(t *testing.T) {
	mock := &mockRelayer{
		err: io.ErrUnexpectedEOF,
	}

	ca := setupCA(t)
	addr := startTestProxy(t, ca, mock)

	proxyURL, _ := url.Parse("http://" + addr)
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	client := &http.Client{Transport: transport}

	resp, err := client.Get("http://example.com/fail")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status: got %d want 502", resp.StatusCode)
	}
}
