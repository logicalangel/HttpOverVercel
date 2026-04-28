package relay

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

var hopByHop = map[string]bool{
	"host":                true,
	"connection":          true,
	"proxy-connection":    true,
	"proxy-authorization": true,
	"transfer-encoding":   true,
	"content-length":      true,
}

// Response holds the upstream response decoded from the relay.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// Client sends requests through the Vercel relay worker.
type Client struct {
	workerHost string
	authKey    string
	paths      []string
	counter    atomic.Uint64
	httpClient *http.Client
}

// NewClient creates a relay client. verifySSL=false skips TLS verification.
func NewClient(workerHost, authKey string, paths []string, verifySSL bool) *Client {
	if len(paths) == 0 {
		paths = []string{"/api/api"}
	}

	transport := &http.Transport{
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 50,
	}
	if !verifySSL {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	return &Client{
		workerHost: workerHost,
		authKey:    authKey,
		paths:      paths,
		httpClient: &http.Client{Transport: transport},
	}
}

// nextPath picks the next relay path via round-robin.
func (c *Client) nextPath() string {
	idx := c.counter.Add(1) - 1
	return c.paths[idx%uint64(len(c.paths))]
}

// Relay sends the request through the Vercel relay and returns the upstream response.
func (c *Client) Relay(ctx context.Context, method, targetURL string, headers map[string]string, body []byte) (*Response, error) {
	path := c.nextPath()
	reqURL := "https://" + c.workerHost + path

	urlB64 := base64.StdEncoding.EncodeToString([]byte(targetURL))

	// Filter hop-by-hop headers
	filtered := make(map[string]string)
	for k, v := range headers {
		if !hopByHop[strings.ToLower(k)] {
			filtered[k] = v
		}
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating relay request: %w", err)
	}

	req.Header.Set("X-Auth-Key", c.authKey)
	req.Header.Set("X-Relay-Method", method)
	req.Header.Set("X-Relay-URL", urlB64)
	if len(filtered) > 0 {
		hdrsJSON, err := json.Marshal(filtered)
		if err == nil {
			req.Header.Set("X-Relay-Headers", base64.StdEncoding.EncodeToString(hdrsJSON))
		}
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("relay request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("relay auth failed: check auth_key in config")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("relay returned unexpected status %d", resp.StatusCode)
	}

	statusCode, _ := strconv.Atoi(resp.Header.Get("X-Relay-Status"))
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	var respHeaders map[string]string
	if b64 := resp.Header.Get("X-Relay-Resp-Headers"); b64 != "" {
		if decoded, err := base64.StdEncoding.DecodeString(b64); err == nil {
			json.Unmarshal(decoded, &respHeaders) //nolint:errcheck
		}
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading relay response body: %w", err)
	}

	return &Response{
		StatusCode: statusCode,
		Headers:    respHeaders,
		Body:       respBody,
	}, nil
}

// ToRawHTTP converts a Response to raw HTTP/1.1 wire bytes.
func (r *Response) ToRawHTTP() []byte {
	var buf bytes.Buffer

	statusText := http.StatusText(r.StatusCode)
	if statusText == "" {
		statusText = "Unknown"
	}
	fmt.Fprintf(&buf, "HTTP/1.1 %d %s\r\n", r.StatusCode, statusText)

	for k, v := range r.Headers {
		kl := strings.ToLower(k)
		if kl == "transfer-encoding" || kl == "connection" {
			continue
		}
		fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
	}

	fmt.Fprintf(&buf, "Content-Length: %d\r\n", len(r.Body))
	buf.WriteString("\r\n")
	buf.Write(r.Body)

	return buf.Bytes()
}
