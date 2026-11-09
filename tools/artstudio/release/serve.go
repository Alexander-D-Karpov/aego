package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func csp(m buildManifest, p string, docs bool) string {
	common := "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; "
	if docs || strings.HasPrefix(p, "docs/") {
		// Firefox checks a sandboxed iframe's blob download against the
		// parent's frame-src. Permit local blob URLs without admitting another
		// network origin; form submission and script policies stay restricted.
		return common + "frame-src " + m.AppOrigin + " blob:; frame-ancestors 'none'"
	}
	if strings.HasPrefix(p, "embed/") {
		return common + "frame-ancestors " + strings.Join(m.DocsOrigins, " ")
	}
	return common + "frame-ancestors 'none'"
}
func staticSite(dir string, m buildManifest, docs bool) http.Handler {
	files := map[string]resource{}
	for _, f := range m.Files {
		files[f.Path] = f
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if strings.Contains(r.URL.EscapedPath(), "%") || strings.Contains(r.URL.Path, "\\") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasPrefix(p, "r/") {
			prefix := "r/" + m.BuildID + "/"
			if !strings.HasPrefix(p, prefix) {
				http.NotFound(w, r)
				return
			}
			p = strings.TrimPrefix(p, prefix)
		}
		if p == "" {
			if docs {
				http.Redirect(w, r, "/docs/ru/first-sprite/", http.StatusFound)
				return
			}
			p = "index.html"
		}
		if strings.HasSuffix(p, "/") {
			p += "index.html"
		}
		if !validPath(p) {
			http.NotFound(w, r)
			return
		}
		if docs && !strings.HasPrefix(p, "docs/") && !strings.HasPrefix(p, "assets/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", csp(m, p, docs))
		if !strings.HasPrefix(p, "embed/") {
			w.Header().Set("X-Frame-Options", "DENY")
		}
		f, ok := files[p]
		if p == "manifest.json" && !docs {
			b, e := os.ReadFile(filepath.Join(dir, p))
			if e != nil {
				http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", mimeFor(p))
			w.Header().Set("ETag", `"`+digest(b)+`"`)
			http.ServeContent(w, r, p, time.Time{}, bytes.NewReader(b))
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		original := f
		if !strings.HasSuffix(p, ".gz") {
			w.Header().Set("Vary", "Accept-Encoding")
			if acceptsGzip(r.Header.Get("Accept-Encoding")) {
				if gz, exists := files[p+".gz"]; exists {
					f = gz
					w.Header().Set("Content-Encoding", "gzip")
				}
			}
		}
		if hashedFile.MatchString(original.Path) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("Content-Type", original.MIME)
		w.Header().Set("ETag", `"`+f.SHA256+`"`)
		b, e := readBounded(filepath.Join(dir, filepath.FromSlash(f.Path)), maxFileBytes)
		if e != nil {
			http.Error(w, "resource unavailable", http.StatusServiceUnavailable)
			return
		}
		http.ServeContent(w, r, original.Path, time.Time{}, bytes.NewReader(b))
	})
}
func acceptsGzip(s string) bool {
	for _, token := range strings.Split(s, ",") {
		parts := strings.Split(strings.TrimSpace(token), ";")
		if parts[0] != "gzip" {
			continue
		}
		for _, p := range parts[1:] {
			key, value, found := strings.Cut(strings.TrimSpace(p), "=")
			if found && key == "q" {
				quality, err := strconv.ParseFloat(value, 64)
				if err != nil || quality <= 0 || quality > 1 {
					return false
				}
			}
		}
		return true
	}
	return false
}
func listenLocal(addr string) (net.Listener, error) {
	host, _, e := net.SplitHostPort(addr)
	if e != nil {
		return nil, e
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("development host must bind to an explicit loopback address")
	}
	return net.Listen("tcp", addr)
}
func serve(dir, addr, docsAddr string) error {
	m, _, e := verify(dir, "")
	if e != nil {
		return e
	}
	app, e := listenLocal(addr)
	if e != nil {
		return e
	}
	defer app.Close()
	errc := make(chan error, 2)
	if docsAddr != "" {
		l, e := listenLocal(docsAddr)
		if e != nil {
			return e
		}
		defer l.Close()
		go func() {
			s := &http.Server{Handler: staticSite(dir, m, true), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
			errc <- s.Serve(l)
		}()
		fmt.Printf("Docs preview: http://%s/docs/ru/first-sprite/\n", l.Addr())
	}
	fmt.Printf("Asset Studio: http://%s/r/%s/\n", app.Addr(), m.BuildID)
	go func() {
		s := &http.Server{Handler: staticSite(dir, m, false), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
		errc <- s.Serve(app)
	}()
	return <-errc
}

// httpCheck verifies bytes actually served over HTTP. There are no upload,
// telemetry or authentication endpoints. Redirects are always rejected.
func httpCheck(ctx context.Context, base, dir, expected string) (map[string]any, error) {
	m, hash, e := verify(dir, expected)
	if e != nil {
		return nil, e
	}
	u, e := url.Parse(base)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(u.Path, "/") {
		return nil, fmt.Errorf("invalid HTTP base")
	}
	if !validOrigin(u.Scheme+"://"+u.Host, m.Release) {
		return nil, fmt.Errorf("invalid HTTP origin")
	}
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("redirect refused") }}
	checked := 0
	get := func(p string, want resource) error {
		target := u.ResolveReference(&url.URL{Path: p})
		req, e := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
		if e != nil {
			return e
		}
		req.Header.Set("Accept-Encoding", "identity")
		res, e := client.Do(req)
		if e != nil {
			return e
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, p)
		}
		if ct := strings.Split(res.Header.Get("Content-Type"), ";")[0]; !compatibleMIME(p, ct, want.MIME) {
			return fmt.Errorf("wrong MIME for %s: %s", p, ct)
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, want.Bytes+1))
		if e != nil {
			return e
		}
		if int64(len(b)) != want.Bytes || digest(b) != want.SHA256 {
			return fmt.Errorf("HTTP bytes mismatch: %s", p)
		}
		if strings.HasSuffix(p, ".html") {
			policy := res.Header.Get("Content-Security-Policy")
			if !strings.Contains(policy, "frame-ancestors") || !strings.Contains(policy, "object-src 'none'") {
				return fmt.Errorf("missing CSP: %s", p)
			}
			if strings.HasPrefix(p, "embed/") {
				for _, origin := range m.DocsOrigins {
					if !strings.Contains(policy, origin) {
						return fmt.Errorf("embed CSP origin missing")
					}
				}
			}
		}
		checked++
		return nil
	}
	b, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e = get("manifest.json", resource{"manifest.json", int64(len(b)), hash, mimeFor("manifest.json")}); e != nil {
		return nil, e
	}
	for _, f := range m.Files {
		if e = get(f.Path, f); e != nil {
			return nil, e
		}
	}
	return map[string]any{"buildID": m.BuildID, "manifestSHA256": hash, "filesVerified": checked, "baseURL": base, "status": "PASS"}, nil
}

func compatibleMIME(path, got, want string) bool {
	if got == strings.Split(want, ";")[0] {
		return true
	}
	if strings.HasSuffix(path, ".js") && got == "application/javascript" {
		return true
	}
	if strings.HasSuffix(path, ".gz") && (got == "application/gzip" || got == "application/x-gzip") {
		return true
	}
	return false
}
