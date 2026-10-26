//go:build aego_update

// Package update runs a real update: two builds of the aego CLI, a release server on loopback, the install.sh layout and an AppImage (D362).
package update

import (
	"archive/tar"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	upd "aego/update"
)

type kit struct {
	t        *testing.T
	pub      string
	priv     string
	v1, v2   string
	web      string
	cache    string
	config   string
	srv      *httptest.Server
	release  []byte
	sig      []byte
	tampered bool
}

// build compiles the aego CLI at a version, trusting the kit's key.
func (k *kit) build(version string) string {
	k.t.Helper()
	out := filepath.Join(k.t.TempDir(), "aego")
	cmd := exec.Command("go", "build", "-o", out, "-ldflags", "-X aego/engine.Version="+version+" -X aego/update.PublicKey="+k.pub, "aego/cmd/aego")
	cmd.Dir = filepath.Join("..", "..")
	if b, err := cmd.CombinedOutput(); err != nil {
		k.t.Fatalf("build %s: %v\n%s", version, err, b)
	}
	return out
}

func setup(t *testing.T) *kit {
	t.Helper()
	pub, priv, err := upd.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	k := &kit{t: t, pub: pub, priv: priv, web: t.TempDir(), cache: t.TempDir(), config: t.TempDir()}
	k.v1, k.v2 = k.build("0.99.1"), k.build("0.99.2")
	k.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable/release.json":
			data := k.release
			if k.tampered {
				data = []byte(strings.Replace(string(data), "0.99.2", "0.99.3", 1))
			}
			w.Write(data)
		case "/stable/release.json.sig":
			w.Write(k.sig)
		default:
			http.ServeFile(w, r, filepath.Join(k.web, filepath.Base(r.URL.Path)))
		}
	}))
	t.Cleanup(k.srv.Close)
	tarball(t, filepath.Join(k.web, "aego-v0.99.2-linux-amd64.tar.gz"), "aego-v0.99.2-linux-amd64", k.v2)
	copyFile(t, k.v2, filepath.Join(k.web, "Aego-0.99.2-x86_64.AppImage"))
	r := &upd.Release{Version: "0.99.2", Channel: upd.Stable, Published: time.Now().UTC()}
	for _, a := range []struct {
		file string
		kind upd.Kind
	}{{"aego-v0.99.2-linux-amd64.tar.gz", upd.KindTarball}, {"Aego-0.99.2-x86_64.AppImage", upd.KindAppImage}} {
		sum, size, err := upd.Sum(filepath.Join(k.web, a.file))
		if err != nil {
			t.Fatal(err)
		}
		r.Assets = append(r.Assets, upd.Asset{OS: "linux", Arch: "amd64", Kind: a.kind, URL: k.srv.URL + "/" + a.file, Size: size, SHA256: sum})
	}
	if k.release, err = upd.Encode(r); err != nil {
		t.Fatal(err)
	}
	if k.sig, err = upd.Sign(k.release, priv); err != nil {
		t.Fatal(err)
	}
	return k
}

// run starts an installed aego with the kit's server and a clean user cache and config.
func (k *kit) run(exe string, env []string, args ...string) (string, error) {
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "AEGO_UPDATE_URL="+k.srv.URL, "XDG_CACHE_HOME="+k.cache, "XDG_CONFIG_HOME="+k.config, "SDL_AUDIO_DRIVER=dummy")
	cmd.Env = append(cmd.Env, env...)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func (k *kit) version(exe string, env ...string) string {
	k.t.Helper()
	out, err := k.run(exe, env, "version")
	if err != nil {
		k.t.Fatalf("aego version: %v\n%s", err, out)
	}
	return strings.TrimSpace(out)
}

// install lays v1 out the way install.sh does: share/aego/<version>, current, bin/aego.
func (k *kit) install() string {
	prefix := k.t.TempDir()
	root := filepath.Join(prefix, "share", "aego")
	os.MkdirAll(filepath.Join(root, "0.99.1", "bin"), 0o755)
	copyFile(k.t, k.v1, filepath.Join(root, "0.99.1", "bin", "aego"))
	os.Symlink("0.99.1", filepath.Join(root, "current"))
	os.MkdirAll(filepath.Join(prefix, "bin"), 0o755)
	os.Symlink(filepath.Join(root, "current", "bin", "aego"), filepath.Join(prefix, "bin", "aego"))
	return filepath.Join(prefix, "bin", "aego")
}

func TestTarballUpdateRestoreAndRollback(t *testing.T) {
	k := setup(t)
	aego := k.install()
	if v := k.version(aego); !strings.Contains(v, "0.99.1") {
		t.Fatalf("installed %q", v)
	}
	game := filepath.Join(t.TempDir(), "game")
	if out, err := k.run(aego, nil, "new", game); err != nil {
		t.Fatalf("aego new: %v\n%s", err, out)
	}
	k.tampered = true
	if out, err := k.run(aego, nil, "update", "apply"); err == nil || !strings.Contains(out, "not signed") {
		t.Fatalf("a changed release.json was installed: %v\n%s", err, out)
	}
	k.tampered = false
	if out, err := k.run(aego, nil, "update", "apply"); err != nil {
		t.Fatalf("aego update apply: %v\n%s", err, out)
	}
	if v := k.version(aego); !strings.Contains(v, "0.99.2") {
		t.Fatalf("after the update aego is %q", v)
	}
	if out, err := k.run(aego, nil, "update", "restore"); err != nil {
		t.Fatalf("aego update restore: %v\n%s", err, out)
	}
	if v := k.version(aego); !strings.Contains(v, "0.99.1") {
		t.Fatalf("after restore aego is %q", v)
	}
	if out, err := k.run(aego, nil, "update", "apply"); err != nil {
		t.Fatalf("second apply: %v\n%s", err, out)
	}
	// the new version's first run dies before its trial ends, so the next start goes back to 0.99.1
	cmd := exec.Command(aego, "mcp")
	cmd.Env = append(os.Environ(), "AEGO_UPDATE_URL="+k.srv.URL, "XDG_CACHE_HOME="+k.cache, "XDG_CONFIG_HOME="+k.config)
	cmd.Dir = game
	stdin, _ := cmd.StdinPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	cmd.Process.Signal(syscall.SIGKILL)
	cmd.Wait()
	stdin.Close()
	if v := k.version(aego); !strings.Contains(v, "0.99.1") {
		t.Fatalf("a version that crashed in its trial was kept: %q", v)
	}
	if v := k.version(aego); !strings.Contains(v, "0.99.1") {
		t.Fatalf("the rollback did not stick: %q", v)
	}
}

func TestAppImageUpdate(t *testing.T) {
	k := setup(t)
	img := filepath.Join(t.TempDir(), "Aego-x86_64.AppImage")
	copyFile(t, k.v1, img)
	env := []string{"APPIMAGE=" + img}
	if out, err := k.run(img, env, "update", "apply"); err != nil {
		t.Fatalf("aego update apply: %v\n%s", err, out)
	}
	if v := k.version(img, env...); !strings.Contains(v, "0.99.2") {
		t.Fatalf("after the update the AppImage is %q", v)
	}
	if out, err := k.run(img, env, "update", "restore"); err != nil {
		t.Fatalf("restore: %v\n%s", err, out)
	}
	if v := k.version(img, env...); !strings.Contains(v, "0.99.1") {
		t.Fatalf("after restore the AppImage is %q", v)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

// tarball packs a binary as release.sh does: one folder with bin/aego.
func tarball(t *testing.T, path, top, bin string) {
	t.Helper()
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: top + "/", Typeflag: tar.TypeDir, Mode: 0o755})
	tw.WriteHeader(&tar.Header{Name: top + "/bin/", Typeflag: tar.TypeDir, Mode: 0o755})
	tw.WriteHeader(&tar.Header{Name: top + "/bin/aego", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(data))})
	tw.Write(data)
	tw.Close()
	gz.Close()
	f.Close()
}
