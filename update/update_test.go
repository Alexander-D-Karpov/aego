package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func signed(t *testing.T, r *Release) (data, sig []byte, pub string) {
	t.Helper()
	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	data, err = Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	sig, err = Sign(data, priv)
	if err != nil {
		t.Fatal(err)
	}
	return data, sig, pub
}

func TestVerifyRefusesWhatIsNotSigned(t *testing.T) {
	data, sig, pub := signed(t, &Release{Version: "0.12.0", Channel: Stable})
	key, _ := ParseKey(pub)
	if r, err := Verify(data, sig, key); err != nil || r.Version != "0.12.0" {
		t.Fatalf("a signed release did not verify: %v", err)
	}
	forged := []byte(strings.Replace(string(data), "0.12.0", "0.13.0", 1))
	if _, err := Verify(forged, sig, key); err != ErrUnsigned {
		t.Fatalf("a changed release.json was accepted: %v", err)
	}
	if _, err := Verify(data, nil, key); err != ErrUnsigned {
		t.Fatalf("a release without a signature was accepted: %v", err)
	}
	other, _, _ := GenerateKey()
	okey, _ := ParseKey(other)
	if _, err := Verify(data, sig, okey); err != ErrUnsigned {
		t.Fatalf("a release signed by another key was accepted: %v", err)
	}
	if _, err := Verify(data, sig, nil); err != ErrNoKey {
		t.Fatalf("a build without a key accepted a release: %v", err)
	}
}

func TestCompareAndNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"0.12.0", "0.11.9", 1}, {"0.11.0", "0.11.0-dev", 1}, {"v1.0.0", "1.0.0", 0}, {"0.10.0", "0.11.0-dev", -1}, {"0.11.0-dev+abc", "0.11.0-dev", 0}} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	built := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	night := &Release{Version: "0.11.0-dev", Published: built.Add(time.Hour)}
	if !night.Newer(Build{Version: "0.11.0-dev", Time: built}) {
		t.Fatal("a later nightly of the same version is not newer")
	}
	if night.Newer(Build{Version: "0.11.0-dev", Time: built.Add(2 * time.Hour)}) {
		t.Fatal("an earlier nightly is newer")
	}
	if (&Release{Version: "0.11.0"}).Newer(Build{Version: "0.11.0"}) {
		t.Fatal("the same release is newer")
	}
	if (&Release{Version: "0.12.0", MinVersion: "0.11.0"}).Allowed(Build{Version: "0.10.0"}) {
		t.Fatal("a version below minVersion may update")
	}
}

func TestPolicy(t *testing.T) {
	now := time.Now()
	p := Policy{Mode: Auto, LastCheck: now.Add(-time.Hour)}
	if !p.Due(now, false) || p.Due(now, true) || !p.Due(now.Add(Every), true) {
		t.Fatal("checks are due at start and every six hours")
	}
	r := &Release{Version: "0.12.0"}
	b := Build{Version: "0.11.0"}
	if !p.Wants(r, b) {
		t.Fatal("a newer release is not offered")
	}
	p.Skip("0.12.0")
	if p.Wants(r, b) || !p.Wants(&Release{Version: "0.12.1"}, b) {
		t.Fatal("skipping a version should skip only that one")
	}
	if (&Policy{Mode: Off}).Due(now, false) {
		t.Fatal("Off checks on its own")
	}
}

// server publishes a signed channel and counts the requests that sent a matching ETag.
func server(t *testing.T, r *Release) (*httptest.Server, string, *atomic.Int32) {
	t.Helper()
	data, sig, pub := signed(t, r)
	var notModified atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/stable/release.json":
			if req.Header.Get("If-None-Match") == `"v1"` {
				notModified.Add(1)
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"v1"`)
			w.Write(data)
		case "/stable/release.json.sig":
			w.Write(sig)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, pub, &notModified
}

func TestCheckUsesTheETag(t *testing.T) {
	srv, pub, notModified := server(t, &Release{Version: "0.12.0", Channel: Stable})
	s := Source{BaseURL: srv.URL, Channel: Stable, Key: pub, Cache: t.TempDir()}
	for range 2 {
		r, err := s.Check(context.Background())
		if err != nil || r.Version != "0.12.0" {
			t.Fatalf("check: %v", err)
		}
	}
	if notModified.Load() != 1 {
		t.Fatalf("the second check sent no ETag (%d)", notModified.Load())
	}
	if _, err := (&Source{BaseURL: srv.URL, Channel: Nightly, Key: pub}).Check(context.Background()); err == nil {
		t.Fatal("a missing channel checked fine")
	}
}

func TestCheckRetriesTransientFailures(t *testing.T) {
	data, sig, pub := signed(t, &Release{Version: "0.12.0", Channel: Stable})
	var manifests, signatures atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/stable/release.json":
			if manifests.Add(1) < 3 {
				http.Error(w, "temporary", http.StatusServiceUnavailable)
				return
			}
			w.Write(data)
		case "/stable/release.json.sig":
			if signatures.Add(1) == 1 {
				http.Error(w, "temporary", http.StatusTooManyRequests)
				return
			}
			w.Write(sig)
		default:
			http.NotFound(w, req)
		}
	}))
	defer srv.Close()
	r, err := (&Source{BaseURL: srv.URL, Key: pub}).Check(context.Background())
	if err != nil || r.Version != "0.12.0" || manifests.Load() != 3 || signatures.Load() != 2 {
		t.Fatalf("retry result: release=%+v err=%v manifest requests=%d signature requests=%d", r, err, manifests.Load(), signatures.Load())
	}
}

type retryTransport struct{ calls atomic.Int32 }

func (t *retryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.calls.Add(1) == 1 {
		return nil, context.DeadlineExceeded
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
}

func TestCheckRetriesRequestDeadlineButRespectsCallerCancellation(t *testing.T) {
	transport := &retryTransport{}
	s := &Source{Client: &http.Client{Transport: transport}}
	if data, _, err := s.get(context.Background(), "https://example.invalid/release.json", ""); err != nil || string(data) != "ok" || transport.calls.Load() != 2 {
		t.Fatalf("deadline retry: %q, %v, calls=%d", data, err, transport.calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := transport.calls.Load()
	if _, _, err := s.get(ctx, "https://example.invalid/release.json", ""); !errors.Is(err, context.Canceled) || transport.calls.Load() != before {
		t.Fatalf("cancelled request: %v, calls=%d", err, transport.calls.Load()-before)
	}
}

func TestCheckDoesNotRetryMissingOrUnsignedRelease(t *testing.T) {
	data, _, pub := signed(t, &Release{Version: "0.12.0", Channel: Stable})
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		count.Add(1)
		if req.URL.Path == "/stable/release.json" {
			w.Write(data)
			return
		}
		if req.URL.Path == "/stable/release.json.sig" {
			w.Write([]byte("not a signature"))
			return
		}
		http.NotFound(w, req)
	}))
	defer srv.Close()
	if _, err := (&Source{BaseURL: srv.URL, Key: pub}).Check(context.Background()); err == nil || count.Load() != 2 {
		t.Fatalf("unsigned release: err=%v requests=%d", err, count.Load())
	}
	count.Store(0)
	if _, err := (&Source{BaseURL: srv.URL, Channel: Nightly, Key: pub}).Check(context.Background()); err == nil || count.Load() != 1 {
		t.Fatalf("missing release: err=%v requests=%d", err, count.Load())
	}
}

func TestDownloadResumesAndChecks(t *testing.T) {
	payload := []byte(strings.Repeat("aego update payload ", 4000))
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	os.WriteFile(src, payload, 0o644)
	sum, size, _ := Sum(src)
	var cut atomic.Bool
	cut.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := 0
		if rg := req.Header.Get("Range"); rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(len(payload)-1)+"/"+strconv.Itoa(len(payload)))
			w.WriteHeader(http.StatusPartialContent)
		}
		body := payload[start:]
		if cut.Swap(false) {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Write(body[:len(body)/3])
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				c.Close()
			}
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	a := &Asset{URL: srv.URL + "/aego.tar.gz", SHA256: sum, Size: size}
	out := filepath.Join(dir, "updates")
	if _, err := Download(context.Background(), nil, a, out, nil); err == nil {
		t.Fatal("a cut download finished")
	}
	part, err := os.Stat(filepath.Join(out, "aego.tar.gz.part"))
	if err != nil || part.Size() == 0 {
		t.Fatal("the cut download left nothing to resume")
	}
	var last int64
	got, err := Download(context.Background(), nil, a, out, func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(got); string(b) != string(payload) || last != size {
		t.Fatalf("the resumed download is wrong (%d of %d)", last, size)
	}
	bad := &Asset{URL: srv.URL + "/other.bin", SHA256: strings.Repeat("0", 64)}
	if _, err := Download(context.Background(), nil, bad, out, nil); err != ErrChecksum {
		t.Fatalf("a wrong checksum was accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "other.bin")); err == nil {
		t.Fatal("the file with the wrong checksum was kept")
	}
	if _, err := Download(context.Background(), nil, &Asset{URL: a.URL}, out, nil); err != ErrChecksum {
		t.Fatal("an asset without a checksum was downloaded")
	}
}

// tarball packs one top folder with bin/aego, as release.sh does.
func tarball(t *testing.T, path, top, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: top + "/", Typeflag: tar.TypeDir, Mode: 0o755})
	tw.WriteHeader(&tar.Header{Name: top + "/bin/", Typeflag: tar.TypeDir, Mode: 0o755})
	tw.WriteHeader(&tar.Header{Name: top + "/bin/aego", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(content))})
	tw.Write([]byte(content))
	tw.Close()
	gz.Close()
	f.Close()
}

func TestTarballUpdateAndRestore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share", "aego")
	os.MkdirAll(filepath.Join(root, "0.11.0", "bin"), 0o755)
	os.WriteFile(filepath.Join(root, "0.11.0", "bin", "aego"), []byte("v1"), 0o755)
	os.Symlink("0.11.0", filepath.Join(root, "current"))
	in, err := Detect(filepath.Join(root, "current", "bin", "aego"))
	if err != nil || in.Kind != KindTarball || in.Root != root {
		t.Fatalf("detected %+v: %v", in, err)
	}
	file := filepath.Join(t.TempDir(), "aego-v0.12.0-linux-amd64.tar.gz")
	tarball(t, file, "aego-v0.12.0-linux-amd64", "v2")
	a, err := in.Apply(file, "0.12.0")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(in.Launch); string(b) != "v2" || a.Previous != "0.11.0" {
		t.Fatalf("after the update bin/aego is %q, previous %q", b, a.Previous)
	}
	if !in.HasPrevious() {
		t.Fatal("the update kept no previous version")
	}
	if _, err := in.Restore(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(in.Launch); string(b) != "v1" {
		t.Fatalf("after Restore bin/aego is %q", b)
	}
	bad := filepath.Join(t.TempDir(), "evil.tar.gz")
	tarball(t, bad, "../../outside", "x")
	if _, err := in.Apply(bad, "0.13.0"); err == nil {
		t.Fatal("an archive with a path outside it was unpacked")
	}
}

func TestAppImageUpdateAndRestore(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "Aego.AppImage")
	os.WriteFile(img, []byte("v1"), 0o755)
	t.Setenv("APPIMAGE", img)
	in, err := Detect("/tmp/.mount_aego/usr/bin/aego")
	if err != nil || in.Kind != KindAppImage {
		t.Fatalf("detected %+v: %v", in, err)
	}
	next := filepath.Join(t.TempDir(), "Aego-v2.AppImage")
	os.WriteFile(next, []byte("v2"), 0o644)
	if _, err := in.Apply(next, "0.12.0"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(img); string(b) != "v2" {
		t.Fatal("the AppImage was not replaced")
	}
	if fi, _ := os.Stat(img); fi.Mode()&0o111 == 0 {
		t.Fatal("the new AppImage is not executable")
	}
	if _, err := in.Restore(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(img); string(b) != "v1" {
		t.Fatal("Restore did not bring the old AppImage back")
	}
}

// A new version that crashes before it has run TrialTime is rolled back on its next start.
func TestTrialRollsBackACrashedVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "share", "aego")
	for _, v := range []string{"0.11.0", "0.12.0"} {
		os.MkdirAll(filepath.Join(root, v, "bin"), 0o755)
		os.WriteFile(filepath.Join(root, v, "bin", "aego"), []byte(v), 0o755)
	}
	os.Symlink("0.12.0", filepath.Join(root, "current"))
	os.Symlink("0.11.0", filepath.Join(root, "previous"))
	in, _ := Detect(filepath.Join(root, "current", "bin", "aego"))
	state := t.TempDir()
	BeginTrial(state, Applied{Version: "0.12.0", Previous: "0.11.0"})
	now := time.Now()
	if back, _, err := Start(state, in, "0.12.0", now); err != nil || back != "" {
		t.Fatalf("the first run rolled back: %q %v", back, err)
	}
	back, _, err := Start(state, in, "0.12.0", now.Add(3*time.Second))
	if err != nil || back != "0.12.0" {
		t.Fatalf("a run after an unconfirmed one did not roll back: %q %v", back, err)
	}
	if b, _ := os.ReadFile(in.Launch); string(b) != "0.11.0" {
		t.Fatalf("after the rollback bin/aego is %q", b)
	}
	BeginTrial(state, Applied{Version: "0.12.0"})
	Start(state, in, "0.12.0", now)
	Confirm(state, "0.11.0")
	if back, _, _ := Start(state, in, "0.12.0", now.Add(time.Second)); back == "" {
		t.Fatal("another version's exit confirmed the trial")
	}
	BeginTrial(state, Applied{Version: "0.12.0"})
	Start(state, in, "0.12.0", now)
	Confirm(state, "0.12.0")
	if back, _, _ := Start(state, in, "0.12.0", now.Add(time.Minute)); back != "" {
		t.Fatal("a confirmed update was rolled back")
	}
}

// fakeSetup is a stand-in installer that records how it was run, so the Windows update and restore run end to end on any system.
func fakeSetup(t *testing.T, path, version, log string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	script := "#!/bin/sh\necho " + version + " \"$@\" >> " + log + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// An installer update keeps the installed version's setup.exe in updates/previous; Restore runs it with /S /UPDATE, and a crashed trial does too.
func TestInstallerKeepsAndRestoresPreviousSetup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in installer is a shell script")
	}
	dir, state := t.TempDir(), t.TempDir()
	log := filepath.Join(dir, "runs.txt")
	root := filepath.Join(dir, "Aego")
	fakeSetup(t, filepath.Join(root, SetupFile), "0.11.0", log)
	os.WriteFile(filepath.Join(root, "VERSION"), []byte("0.11.0\n"), 0o644)
	in := Install{Kind: KindInstaller, Root: root, Launch: filepath.Join(root, "aego-editor.exe"), Keep: filepath.Join(state, "previous")}
	if in.HasPrevious() {
		t.Fatal("a fresh install has a previous version")
	}
	next := filepath.Join(state, "0.12.0", "aego-0.12.0-setup.exe")
	fakeSetup(t, next, "0.12.0", log)
	a, err := in.Apply(next, "0.12.0")
	if err != nil || a.Exit == nil || a.Previous != "0.11.0" {
		t.Fatalf("apply: %+v %v", a, err)
	}
	if err := a.Exit.Run(); err != nil {
		t.Fatal(err)
	}
	// the new installer copies itself over setup.exe, as aego.nsi does
	fakeSetup(t, filepath.Join(root, SetupFile), "0.12.0", log)
	if !in.HasPrevious() {
		t.Fatal("the update kept no previous installer")
	}
	r, err := in.Restore()
	if err != nil || r.Exit == nil {
		t.Fatalf("restore: %+v %v", r, err)
	}
	if err := r.Exit.Run(); err != nil {
		t.Fatal(err)
	}
	fakeSetup(t, filepath.Join(root, SetupFile), "0.11.0", log)
	if a, err = in.Apply(next, "0.12.0"); err != nil {
		t.Fatal(err)
	}
	if err := a.Exit.Run(); err != nil {
		t.Fatal(err)
	}
	fakeSetup(t, filepath.Join(root, SetupFile), "0.12.0", log)
	BeginTrial(state, a)
	now := time.Now()
	Start(state, in, "0.12.0", now)
	back, exit, err := Start(state, in, "0.12.0", now.Add(time.Second))
	if err != nil || back != "0.12.0" || exit == nil {
		t.Fatalf("a crashed trial did not run the kept installer: %q %v %v", back, exit, err)
	}
	if err := exit.Run(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	want := "0.12.0 /S /UPDATE\n0.11.0 /S /UPDATE\n0.12.0 /S /UPDATE\n0.11.0 /S /UPDATE\n"
	if string(b) != want {
		t.Fatalf("installers ran as\n%s\nwant\n%s", b, want)
	}
}

// Clean keeps the one previous installer, the release cache and newer downloads; older downloads and a restore's copy go.
func TestCleanKeepsOnePrevious(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"0.10.0", "0.11.0", "0.12.0", "previous", "cache", "restore"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	Clean(dir, "0.11.0")
	entries, _ := os.ReadDir(dir)
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, " ") != "0.12.0 cache previous" {
		t.Fatalf("after Clean: %v", got)
	}
}
