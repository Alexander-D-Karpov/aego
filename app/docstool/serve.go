package docstool

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// siteHandler serves the generator's clean URLs, never a directory inventory.
// A guide can have both physics.html and a physics/ directory of child pages.
func siteHandler(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		local := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
		if info, err := os.Stat(local + ".html"); err == nil && !info.IsDir() {
			if strings.HasSuffix(r.URL.Path, "/") {
				target := (&url.URL{Path: clean, RawQuery: r.URL.RawQuery}).String()
				http.Redirect(w, r, target, http.StatusPermanentRedirect)
				return
			}
			copy := r.Clone(r.Context())
			copy.URL.Path, copy.URL.RawPath = clean+".html", ""
			files.ServeHTTP(w, copy)
			return
		}
		if info, err := os.Stat(local); err == nil && info.IsDir() {
			if index, err := os.Stat(filepath.Join(local, "index.html")); err != nil || index.IsDir() {
				http.NotFound(w, r)
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}
