package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/logicalangel/HttpOverVercel/internal/mitm"
	"github.com/logicalangel/HttpOverVercel/internal/relay"
)

// Relayer is implemented by *relay.Client.
type Relayer interface {
	Relay(ctx context.Context, method, targetURL string, headers map[string]string, body []byte) (*relay.Response, error)
}

// Server is an HTTP(S) proxy server that relays traffic through a Vercel worker.
type Server struct {
	host    string
	port    int
	mitm    *mitm.CAManager
	relayer Relayer
	log     *slog.Logger
}

// New creates a new proxy Server.
func New(host string, port int, m *mitm.CAManager, r Relayer) *Server {
	return &Server{
		host:    host,
		port:    port,
		mitm:    m,
		relayer: r,
		log:     slog.Default(),
	}
}

// ListenAndServe starts accepting connections until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.host, s.port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// Close the listener when ctx is done to unblock Accept.
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	s.log.Info("proxy listening", "addr", ln.Addr().String())

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Check if we were shut down intentionally.
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		if err != io.EOF {
			s.log.Debug("reading request", "err", err)
		}
		return
	}

	if req.Method == http.MethodConnect {
		s.handleConnect(ctx, conn, req.Host)
	} else {
		s.handleHTTP(ctx, conn, req)
	}
}

func (s *Server) handleConnect(ctx context.Context, conn net.Conn, hostport string) {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if host == "" {
		host = hostport
	}

	// Acknowledge the CONNECT tunnel.
	if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		s.log.Debug("writing CONNECT response", "err", err)
		return
	}

	// MITM TLS handshake.
	tlsCfg, err := s.mitm.TLSConfig(host)
	if err != nil {
		s.log.Error("building TLS config", "host", host, "err", err)
		return
	}

	tlsConn := tls.Server(conn, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		s.log.Debug("TLS handshake", "host", host, "err", err)
		return
	}
	defer tlsConn.Close()

	reader := bufio.NewReader(tlsConn)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err != io.EOF {
				s.log.Debug("reading tunnelled request", "host", host, "err", err)
			}
			break
		}

		targetURL := "https://" + host + req.URL.RequestURI()

		raw, err := s.relayRequest(ctx, req, targetURL)
		if err != nil {
			s.log.Error("relay request", "url", targetURL, "err", err)
			errBody := []byte(err.Error())
			fmt.Fprintf(tlsConn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: %d\r\n\r\n%s", len(errBody), errBody)
			break
		}
		if _, err := tlsConn.Write(raw); err != nil {
			s.log.Debug("writing tunnelled response", "err", err)
			break
		}
	}
}

func (s *Server) handleHTTP(ctx context.Context, conn net.Conn, req *http.Request) {
	var targetURL string
	if req.URL.Scheme != "" {
		targetURL = req.URL.String()
	} else {
		scheme := "http"
		host := req.Host
		if host == "" {
			host = req.URL.Host
		}
		targetURL = scheme + "://" + host + req.URL.RequestURI()
	}

	raw, err := s.relayRequest(ctx, req, targetURL)
	if err != nil {
		s.log.Error("relay HTTP request", "url", targetURL, "err", err)
		errBody := []byte(err.Error())
		fmt.Fprintf(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: %d\r\n\r\n%s", len(errBody), errBody)
		return
	}
	conn.Write(raw) //nolint:errcheck
}

func (s *Server) relayRequest(ctx context.Context, req *http.Request, targetURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	body, err := io.ReadAll(io.LimitReader(req.Body, 64<<20)) // 64 MB limit
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}

	// Build headers map with lowercase keys.
	headers := make(map[string]string)
	for key, vals := range req.Header {
		if len(vals) > 0 {
			headers[strings.ToLower(key)] = vals[0]
		}
	}

	resp, err := s.relayer.Relay(ctx, req.Method, targetURL, headers, body)
	if err != nil {
		return nil, err
	}
	return resp.ToRawHTTP(), nil
}
