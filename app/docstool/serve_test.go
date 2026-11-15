package docstool

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsServerCleanURLsDoNotExposeDirectoryListings(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"physics.html": "<h1>Physics</h1>", "physics/units.html": "<h1>Units</h1>", "play/index.html": "<h1>Playground</h1>", "hidden/file.txt": "private inventory"} {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct {
		path           string
		status         int
		text, location string
	}{
		{"/physics", 200, "<h1>Physics</h1>", ""},
		{"/physics/", 308, "", "/physics"},
		{"/physics/?test=1", 308, "", "/physics?test=1"},
		{"/physics/units", 200, "<h1>Units</h1>", ""},
		{"/play/", 200, "<h1>Playground</h1>", ""},
		{"/hidden/", 404, "", ""},
	} {
		r := httptest.NewRecorder()
		siteHandler(root).ServeHTTP(r, httptest.NewRequest(http.MethodGet, item.path, nil))
		if r.Code != item.status || !strings.Contains(r.Body.String(), item.text) || r.Header().Get("Location") != item.location || strings.Contains(r.Body.String(), "Directory listing") {
			t.Errorf("%s: status=%d location=%q body=%s", item.path, r.Code, r.Header().Get("Location"), r.Body.String())
		}
	}
}
