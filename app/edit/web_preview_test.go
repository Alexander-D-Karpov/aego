package edit

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWebExportPreviewIsScopedAndClosable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("play"), 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(t.TempDir(), filepath.Join(dir, "outside")); err != nil {
			t.Fatal(err)
		}
	}
	var p webPreview
	ended := false
	u, err := p.serve(dir, func() { ended = true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	res, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	if runtime.GOOS != "windows" {
		res, err = http.Get(u + "outside/")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode < 400 {
			t.Fatal("external directory served")
		}
	}
	req, _ := http.NewRequest("GET", u, nil)
	req.Host = "different.example"
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatal("unexpected Host was accepted")
	}
	if _, err := p.serve(dir, nil, func() bool { return false }); err == nil {
		t.Fatal("expired conversation reopened listener")
	}
	p.close()
	if !ended {
		t.Fatal("private export still retained")
	}
	if res, err = http.Get(u); err == nil {
		res.Body.Close()
		t.Fatal("listener survived session")
	}
}
