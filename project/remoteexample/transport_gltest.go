//go:build aego_gltest && linux && cgo && !aego_nonative

package remoteexample

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
)

// GLTestLoopbackClient keeps all manifest, origin, redirect and archive checks
// but replaces publicDial with httptest's certificate-trusting TLS transport.
// This test-only adapter cannot be built into the normal editor. Production
// NewClient rejects loopback hosts and never uses this transport.
func GLTestLoopbackClient(p Policy, server *httptest.Server) (*Client, error) {
	if server == nil || server.TLS == nil {
		return nil, fmt.Errorf("TLS test server required")
	}
	u, err := url.Parse(server.URL)
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("HTTPS test server required")
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("loopback test server required")
	}
	c, err := NewClient(p)
	if err != nil {
		return nil, err
	}
	testHTTP := server.Client()
	if testHTTP == nil || testHTTP.Transport == nil {
		return nil, fmt.Errorf("test TLS transport required")
	}
	c.http = &http.Client{Transport: testHTTP.Transport, Timeout: c.http.Timeout}
	return c, nil
}
