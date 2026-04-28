package relay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeRelayServer builds a TLS test server that mimics the Vercel relay endpoint.
func makeRelayServer(t *testing.T, authKey string, upstreamStatus int, upstreamHeaders map[string]string, upstreamBody []byte) *httptest.Server {
	t.Helper()
	if upstreamStatus == 0 {
		upstreamStatus = 200
	}
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Key") != authKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		hdrsJSON, _ := json.Marshal(upstreamHeaders)
		w.Header().Set("X-Relay-Status", fmt.Sprintf("%d", upstreamStatus))
		w.Header().Set("X-Relay-Resp-Headers", base64.StdEncoding.EncodeToString(hdrsJSON))
		w.WriteHeader(http.StatusOK)
		w.Write(upstreamBody) //nolint:errcheck
	}))
}

func clientFor(t *testing.T, srv *httptest.Server, authKey string, paths []string) *Client {
	t.Helper()
	c := NewClient("placeholder", authKey, paths, false)
	c.httpClient = srv.Client()
	c.workerHost = srv.Listener.Addr().String()
	return c
}

func TestRelaySuccess(t *testing.T) {
	wantBody := []byte("hello upstream")
	wantHeaders := map[string]string{"Content-Type": "text/plain"}

	srv := makeRelayServer(t, "testkey", 200, wantHeaders, wantBody)
	defer srv.Close()

	c := clientFor(t, srv, "testkey", []string{"/api/api"})

	resp, err := c.Relay(context.Background(), "GET", "https://example.com/path", nil, nil)
	if err != nil {
		t.Fatalf("Relay: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode: got %d want 200", resp.StatusCode)
	}
	if string(resp.Body) != string(wantBody) {
		t.Errorf("Body: got %q want %q", resp.Body, wantBody)
	}
	if resp.Headers["Content-Type"] != "text/plain" {
		t.Errorf("Headers[Content-Type]: got %q want %q", resp.Headers["Content-Type"], "text/plain")
	}
}

func TestRelayUnauthorized(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := clientFor(t, srv, "wrongkey", []string{"/api/api"})

	_, err := c.Relay(context.Background(), "GET", "https://example.com/", nil, nil)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "auth") {
		t.Errorf("expected auth-related error, got: %v", err)
	}
}

func TestRelayNetworkError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c := clientFor(t, srv, "key", []string{"/api/api"})
	srv.Close() // shut down before calling

	_, err := c.Relay(context.Background(), "GET", "https://example.com/", nil, nil)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
}

func TestResponseToRawHTTP(t *testing.T) {
	r := &Response{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "text/html"},
		Body:       []byte("body content"),
	}
	raw := string(r.ToRawHTTP())

	if !strings.HasPrefix(raw, "HTTP/1.1 200 OK\r\n") {
		end := len(raw)
		if end > 40 {
			end = 40
		}
		t.Errorf("bad status line, got: %q", raw[:end])
	}
	if !strings.Contains(raw, "Content-Length: 12\r\n") {
		t.Errorf("missing correct Content-Length in:\n%s", raw)
	}
	if !strings.Contains(raw, "\r\n\r\nbody content") {
		t.Errorf("missing body separator or body in output")
	}
}

func TestResponseToRawHTTP_SkipsHopByHop(t *testing.T) {
	r := &Response{
		StatusCode: 200,
		Headers: map[string]string{
			"Transfer-Encoding": "chunked",
			"Connection":        "keep-alive",
			"Content-Type":      "application/json",
		},
		Body: []byte("{}"),
	}
	raw := string(r.ToRawHTTP())

	if strings.Contains(strings.ToLower(raw), "transfer-encoding") {
		t.Error("Transfer-Encoding should be stripped from output")
	}
	if strings.Contains(strings.ToLower(raw), "connection:") {
		t.Error("Connection should be stripped from output")
	}
	if !strings.Contains(raw, "Content-Type") {
		t.Error("Content-Type should be present in output")
	}
}

func TestRoundRobinPaths(t *testing.T) {
	paths := []string{"/p1", "/p2", "/p3"}
	var gotPaths []string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		hdrs, _ := json.Marshal(map[string]string{})
		w.Header().Set("X-Relay-Status", "200")
		w.Header().Set("X-Relay-Resp-Headers", base64.StdEncoding.EncodeToString(hdrs))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := clientFor(t, srv, "key", paths)

	for i := 0; i < 6; i++ {
		c.Relay(context.Background(), "GET", "https://example.com/", nil, nil) //nolint:errcheck
	}

	if len(gotPaths) != 6 {
		t.Fatalf("expected 6 calls, got %d", len(gotPaths))
	}
	for i, p := range gotPaths {
		want := paths[i%len(paths)]
		if p != want {
			t.Errorf("call %d: got path %q want %q", i, p, want)
		}
	}
}

func TestFilterRequestHeaders(t *testing.T) {
	var gotRelayHeaders map[string]string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b64 := r.Header.Get("X-Relay-Headers"); b64 != "" {
			decoded, _ := base64.StdEncoding.DecodeString(b64)
			json.Unmarshal(decoded, &gotRelayHeaders) //nolint:errcheck
		}
		hdrs, _ := json.Marshal(map[string]string{})
		w.Header().Set("X-Relay-Status", "200")
		w.Header().Set("X-Relay-Resp-Headers", base64.StdEncoding.EncodeToString(hdrs))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := clientFor(t, srv, "key", []string{"/api/api"})

	inHeaders := map[string]string{
		"Host":              "example.com",
		"Connection":        "keep-alive",
		"Transfer-Encoding": "chunked",
		"X-Custom-Header":   "preserved",
		"Authorization":     "Bearer token",
	}

	c.Relay(context.Background(), "GET", "https://example.com/", inHeaders, nil) //nolint:errcheck

	for _, shouldBeFiltered := range []string{"Host", "Connection", "Transfer-Encoding"} {
		if _, ok := gotRelayHeaders[shouldBeFiltered]; ok {
			t.Errorf("%s header should have been filtered out", shouldBeFiltered)
		}
	}
	if gotRelayHeaders["X-Custom-Header"] != "preserved" {
		t.Errorf("X-Custom-Header: got %q want %q", gotRelayHeaders["X-Custom-Header"], "preserved")
	}
	if gotRelayHeaders["Authorization"] != "Bearer token" {
		t.Errorf("Authorization: got %q want %q", gotRelayHeaders["Authorization"], "Bearer token")
	}
}
