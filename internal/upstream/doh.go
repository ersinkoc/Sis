package upstream

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	mdns "github.com/miekg/dns"
)

const maxDNSMessageSize = 65535

const bootstrapRetryDelay = 100 * time.Millisecond

// DoHClient forwards DNS messages to a single DNS-over-HTTPS upstream.
type DoHClient struct {
	id        string
	url       string
	client    *http.Client
	timeout   time.Duration
	bootstrap []string
}

// NewDoHClient creates a DNS-over-HTTPS client from upstream config.
func NewDoHClient(c config.Upstream) *DoHClient {
	timeout := c.Timeout.Duration
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	client := &DoHClient{id: c.ID, url: c.URL, timeout: timeout, bootstrap: append([]string(nil), c.Bootstrap...)}
	client.client = &http.Client{Timeout: timeout, Transport: client.transport()}
	return client
}

// ID returns the configured upstream identifier.
func (c *DoHClient) ID() string {
	if c == nil {
		return ""
	}
	return c.id
}

func (c *DoHClient) transport() http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.ForceAttemptHTTP2 = true
	host := dohHost(c.url)
	if host == "" || len(c.bootstrap) == 0 {
		return transport
	}
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
	dialer := &net.Dialer{Timeout: c.timeout, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		targetHost, port, err := net.SplitHostPort(address)
		if err != nil || !strings.EqualFold(targetHost, host) {
			return dialer.DialContext(ctx, network, address)
		}
		var lastErr error
		for i, ip := range c.bootstrap {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
			if i < len(c.bootstrap)-1 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(bootstrapRetryDelay):
				}
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no bootstrap IPs configured for %s", host)
		}
		return nil, lastErr
	}
	return transport
}

func dohHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if host == "" {
		return ""
	}
	return host
}

// Forward sends msg to the DoH endpoint and returns the decoded DNS response.
func (c *DoHClient) Forward(ctx context.Context, msg *mdns.Msg) (*mdns.Msg, error) {
	if c == nil {
		return nil, fmt.Errorf("doh client is not configured")
	}
	if msg == nil {
		return nil, fmt.Errorf("dns message is required")
	}
	if c.client == nil {
		c.client = &http.Client{Timeout: c.timeout, Transport: c.transport()}
	}
	wire, err := msg.Pack()
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.url, bytes.NewReader(wire))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream %s: HTTP %d", c.id, resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" && !isDNSMessageContent(contentType) {
		return nil, fmt.Errorf("upstream %s: unexpected content type %q", c.id, contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDNSMessageSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxDNSMessageSize {
		return nil, fmt.Errorf("upstream %s: response too large", c.id)
	}
	var out mdns.Msg
	if err := out.Unpack(body); err != nil {
		return nil, err
	}
	if out.Id != msg.Id {
		return nil, fmt.Errorf("upstream %s: mismatched response id", c.id)
	}
	return &out, nil
}

func isDNSMessageContent(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return contentType == "application/dns-message" || strings.HasPrefix(contentType, "application/dns-message;")
}
