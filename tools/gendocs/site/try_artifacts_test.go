package site

import (
	"bytes"
	"encoding/base64"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTryArtifactsReachBrowserWithoutSourceImport(t *testing.T) {
	e := fixture()
	payload := []byte{0, 1, 2, 255, '<', '&', '"', '\n'}
	e.Entries[0].Try = "scene version=1 {}"
	e.Entries[0].TryArtifacts = payload
	dir := t.TempDir()
	if _, err := Build(e, dir, Options{Playground: true}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range []string{"welcome.html", "ru/welcome.html"} {
		page, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		body := string(page)
		m := regexp.MustCompile(`data-artifacts="([^"]+)"`).FindStringSubmatch(body)
		if m == nil {
			continue
		}
		found = true
		got, err := base64.StdEncoding.DecodeString(html.UnescapeString(m[1]))
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatal("prepared bytes changed in HTML transport", err)
		}
		if !strings.Contains(body, "Download scene only") && !strings.Contains(body, "Скачать только сцену") {
			t.Fatal("asset-dependent scene download claims to be complete")
		}
	}
	if !found {
		t.Fatal("Try artifact payload was lost by site build")
	}
	js, err := os.ReadFile(filepath.Join(dir, "assets/try.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "artifacts: b.dataset.artifacts") {
		t.Fatal("Try runtime load omits prepared resources")
	}
}
