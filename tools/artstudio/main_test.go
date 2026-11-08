package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticHostServesBuildAndRejectsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "<title>Studio</title>", "artstudio.wasm": "\x00asm", "private.txt": "not public"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := staticHandler(dir)
	for _, test := range []struct {
		method, path, host string
		want               int
	}{
		{"GET", "/", "127.0.0.1:8097", 200},
		{"HEAD", "/", "localhost:8097", 200},
		{"GET", "/artstudio.wasm", "[::1]:8097", 200},
		{"GET", "/private.txt", "127.0.0.1:8097", 404},
		{"GET", "/../private.txt", "127.0.0.1:8097", 404},
		{"GET", "/.git/config", "127.0.0.1:8097", 404},
		{"POST", "/", "127.0.0.1:8097", 405},
		{"GET", "/", "attacker.example:8097", 403},
	} {
		t.Run(test.method+test.path+test.host, func(t *testing.T) {
			r := httptest.NewRequest(test.method, test.path, nil)
			r.Host = test.host
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("got %d; want %d", w.Code, test.want)
			}
			if test.want == http.StatusOK {
				if w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "wasm-unsafe-eval") {
					t.Fatal("security headers missing")
				}
				if test.method == http.MethodHead && w.Body.Len() != 0 {
					t.Fatal("HEAD returned a body")
				}
			}
		})
	}
}
