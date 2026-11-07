package remoteexample

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

type Policy struct {
	TrustedOrigins   []string
	EngineVersion    string
	InstalledModules []string
	Limits           Limits
}
type Client struct {
	policy Policy
	http   *http.Client
}

func NewClient(p Policy) (*Client, error) {
	p.Limits = normalizeLimits(p.Limits)
	if _, e := version(p.EngineVersion); e != nil {
		return nil, e
	}
	for _, s := range p.TrustedOrigins {
		o, e := Origin(s)
		if e != nil || o != s {
			return nil, fmt.Errorf("trust requires an exact HTTPS origin, not a path or wildcard")
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil // Do not allow ambient proxies to bypass destination checks.
	tr.DialContext = publicDial
	tr.MaxConnsPerHost = 4
	tr.ResponseHeaderTimeout = 15 * time.Second
	c := &Client{policy: p, http: &http.Client{Transport: tr, Timeout: 90 * time.Second}}
	return c, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func publicIP(a netip.Addr) bool {
	a = a.Unmap()
	if a.Is6() && !netip.MustParsePrefix("2000::/3").Contains(a) {
		return false
	}
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "192.88.99.0/24", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64"} {
		if netip.MustParsePrefix(s).Contains(a) {
			return false
		}
	}
	return true
}
func publicDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(addr)
	if e != nil {
		return nil, e
	}
	ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if e != nil {
		return nil, e
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("host has no addresses")
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, fmt.Errorf("remote examples cannot access a non-public address")
		}
	}
	// Pin the resolved address for the connection; the TLS layer still checks
	// the original URL hostname, avoiding DNS-rebinding between check and dial.
	var last error
	dial := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	for _, ip := range ips {
		conn, e := dial.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
		last = e
	}
	return nil, last
}
func (c *Client) approve(raw, approved string) (*url.URL, error) {
	u, e := httpsURL(raw)
	if e != nil {
		return nil, e
	}
	origin := originOf(u)
	for _, trusted := range c.policy.TrustedOrigins {
		if trusted == origin {
			return u, nil
		}
	}
	if approved != origin {
		return nil, fmt.Errorf("explicit confirmation required for origin %s", origin)
	}
	return u, nil
}
func (c *Client) get(ctx context.Context, raw string, limit int64, w io.Writer) (int64, error) {
	u, e := httpsURL(raw)
	if e != nil {
		return 0, e
	}
	initial := originOf(u)
	client := *c.http
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return fmt.Errorf("too many redirects")
		}
		target, e := httpsURL(req.URL.String())
		if e != nil || originOf(target) != initial {
			return fmt.Errorf("cross-origin or unsafe redirect rejected")
		}
		return nil
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return 0, e
	}
	req.Header.Set("Accept-Encoding", "identity")
	res, e := client.Do(req)
	if e != nil {
		return 0, e
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTPS request failed with status %d", res.StatusCode)
	}
	if res.ContentLength > limit {
		return 0, fmt.Errorf("response exceeds limit")
	}
	if encoding := res.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return 0, fmt.Errorf("encoded HTTP response rejected")
	}
	n, e := io.Copy(w, io.LimitReader(res.Body, limit+1))
	if e != nil {
		return n, e
	}
	if n > limit {
		return n, fmt.Errorf("response exceeds limit")
	}
	return n, nil
}
func (c *Client) FetchManifest(ctx context.Context, raw, approved string) (Manifest, error) {
	if _, e := c.approve(raw, approved); e != nil {
		return Manifest{}, e
	}
	b := &boundedBuffer{limit: c.policy.Limits.ManifestBytes}
	if _, e := c.get(ctx, raw, c.policy.Limits.ManifestBytes, b); e != nil {
		return Manifest{}, e
	}
	m, e := ParseManifest(b.data, raw, c.policy.Limits)
	if e != nil {
		return m, e
	}
	return m, m.CheckEngine(c.policy.EngineVersion, c.policy.InstalledModules)
}

type boundedBuffer struct {
	data  []byte
	limit int64
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if int64(len(p)) > b.limit-int64(len(b.data)) {
		return 0, fmt.Errorf("response exceeds limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func hashBytes(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
