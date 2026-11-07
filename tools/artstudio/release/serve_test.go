package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPRoutesAndPolicies(t *testing.T) {
	dir, m, _ := fixtureArtifact(t, "one")
	h := staticSite(dir, m, false)
	for _, v := range []struct {
		path string
		code int
	}{{"/", 200}, {"/editor/", 200}, {"/r/one/embed/", 200}, {"/r/other/editor/", 404}, {"/unknown.wasm", 404}, {"/assets/missing.png", 404}, {"/editor/missing.js", 404}, {"/%2e%2e/secret", 404}, {"/manifest.json", 200}} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", v.path, nil))
		if r.Code != v.code {
			t.Fatalf("%s: %d", v.path, r.Code)
		}
		if v.code == 404 && (r.Header().Get("Cache-Control") != "no-store" || strings.Contains(r.Header().Get("Content-Type"), "html")) {
			t.Fatal("cached HTML fallback for missing asset")
		}
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/r/one/embed/", nil))
	if r.Header().Get("X-Frame-Options") != "" || !strings.Contains(r.Header().Get("Content-Security-Policy"), m.DocsOrigins[0]) {
		t.Fatal("iframe blocked or wrong CSP")
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/editor/", nil))
	if !strings.Contains(r.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("standalone must not be embeddable")
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/"+m.WASM.Path, nil))
	if r.Header().Get("Content-Type") != "application/wasm" || !strings.Contains(r.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("WASM MIME/cache")
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("POST", "/editor/", strings.NewReader("pixels")))
	if r.Code != 405 {
		t.Fatal("write endpoint must not exist")
	}
}
func TestHTTPReferenceBytes(t *testing.T) {
	dir, m, sha := fixtureArtifact(t, "one")
	s := httptest.NewServer(staticSite(dir, m, false))
	defer s.Close()
	result, e := httpCheck(context.Background(), s.URL+"/r/one/", dir, sha)
	if e != nil {
		t.Fatal(e)
	}
	if result["status"] != "PASS" {
		t.Fatal(result)
	}
}
func TestHTTPRejectsRedirect(t *testing.T) {
	dir, _, sha := fixtureArtifact(t, "one")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://example.com", 302) }))
	defer s.Close()
	if _, e := httpCheck(context.Background(), s.URL+"/", dir, sha); e == nil {
		t.Fatal("followed redirect")
	}
}
func TestDocsPreviewDoesNotExposeEditor(t *testing.T) {
	dir, m, _ := fixtureArtifact(t, "one")
	h := staticSite(dir, m, true)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/editor/", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestGzipQuality(t *testing.T) {
	for _, v := range []string{"gzip;q=0", "gzip;q=0.000", "gzip;q=invalid", "br", "gzip;q=1.1"} {
		if acceptsGzip(v) {
			t.Fatalf("accepted %q", v)
		}
	}
	for _, v := range []string{"gzip", "br, gzip;q=0.5", "gzip;q=1.0"} {
		if !acceptsGzip(v) {
			t.Fatalf("rejected %q", v)
		}
	}
}
