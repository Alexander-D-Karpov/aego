package update

import (
	"archive/tar"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	upd "aego/update"
)

// An exported game finds its new version, downloads it in Auto mode, swaps its folder and starts the new one.
func TestGameUpdatesItsFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a game folder updates through its installer on Windows")
	}
	pub, priv, _ := upd.GenerateKey()
	web := t.TempDir()
	marker := filepath.Join(t.TempDir(), "started")
	script := "#!/bin/sh\necho new > " + marker + "\n"
	f, _ := os.Create(filepath.Join(web, "game-1.1.0-linux-amd64.tar.gz"))
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "game/", Typeflag: tar.TypeDir, Mode: 0o755})
	tw.WriteHeader(&tar.Header{Name: "game/game", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(script))})
	tw.Write([]byte(script))
	tw.Close()
	gz.Close()
	f.Close()
	var release, sig []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable/release.json":
			w.Write(release)
		case "/stable/release.json.sig":
			w.Write(sig)
		default:
			http.ServeFile(w, r, filepath.Join(web, filepath.Base(r.URL.Path)))
		}
	}))
	defer srv.Close()
	sum, size, _ := upd.Sum(filepath.Join(web, "game-1.1.0-linux-amd64.tar.gz"))
	release, _ = upd.Encode(&upd.Release{Version: "1.1.0", Channel: upd.Stable, Notes: "faster",
		Assets: []upd.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: upd.KindTarball, URL: srv.URL + "/game-1.1.0-linux-amd64.tar.gz", Size: size, SHA256: sum}}})
	sig, _ = upd.Sign(release, priv)

	dir := filepath.Join(t.TempDir(), "game")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "game"), []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := &Module{sec: Section{URL: srv.URL, PublicKey: pub}, ver: "1.0.0", game: "game", in: upd.DetectGame(filepath.Join(dir, "game"))}
	m.Check()
	deadline := time.Now().Add(10 * time.Second)
	for {
		p, v, notes, msg := m.State()
		if p == Ready {
			if v != "1.1.0" || notes != "faster" {
				t.Fatalf("ready with %q %q", v, notes)
			}
			break
		}
		if p == Failed || time.Now().After(deadline) {
			t.Fatalf("the update did not download: %v %s", p, msg)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := m.Apply(); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(marker); err == nil && string(b) == "new\n" {
			if _, err := os.Stat(previousOf(dir)); err != nil {
				t.Fatal("the old folder was not kept")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the new version did not start")
}

// A release signed by another key never downloads.
func TestGameRefusesAnotherKey(t *testing.T) {
	other, _, _ := upd.GenerateKey()
	_, priv, _ := upd.GenerateKey()
	release, _ := upd.Encode(&upd.Release{Version: "9.0.0", Channel: upd.Stable})
	sig, _ := upd.Sign(release, priv)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stable/release.json" {
			w.Write(release)
			return
		}
		w.Write(sig)
	}))
	defer srv.Close()
	m := &Module{sec: Section{URL: srv.URL, PublicKey: other}, ver: "1.0.0"}
	m.Check()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p, _, _, msg := m.State(); p == Failed {
			if msg != upd.ErrUnsigned.Error() {
				t.Fatalf("failed with %q", msg)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a release signed by another key was not refused")
}

func previousOf(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".previous")
}
