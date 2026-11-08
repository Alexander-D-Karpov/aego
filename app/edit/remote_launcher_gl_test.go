//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"aego/core/log"
	"aego/editor/shell/launcher"
	"aego/engine"
	"aego/platform/sdl"
	"aego/project"
	"aego/project/remoteexample"
)

type trackedLauncherRemote struct {
	host  launcherRemote
	calls chan string
}

// This uses a real TLS server and the actual launcher/store/HTTP stack. Only
// the test transport accepts loopback; the production dialer rejects it.
func TestRemoteLauncherClonesOverLocalTLS(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := project.Create(t.TempDir(), "TLS example", false)
	if err != nil {
		t.Fatal(err)
	}
	projectBytes, err := os.ReadFile(p.FilePath())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"project.aego":        projectBytes,
		"scenes/demo.aescene": []byte("scene version=1 {}\n"),
	}
	var packed bytes.Buffer
	zw := zip.NewWriter(&packed)
	manifestFiles := make([]remoteexample.File, 0, len(files))
	for _, name := range []string{"project.aego", "scenes/demo.aescene"} {
		data := files[name]
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		manifestFiles = append(manifestFiles, remoteexample.File{Path: name, Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:])})
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	var manifest []byte
	var manifestRequests, archiveRequests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/example.json":
			manifestRequests.Add(1)
			_, _ = w.Write(manifest)
		case "/example.zip":
			archiveRequests.Add(1)
			_, _ = w.Write(packed.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	source := server.URL + "/example.json"
	hash := sha256.Sum256(packed.Bytes())
	manifest, err = json.Marshal(remoteexample.Manifest{
		Format: 1, ExampleID: "tls-example", Version: "1.0.0", Origin: server.URL,
		License: "MIT", Engine: remoteexample.Compatibility{Min: "0.12.1", MaxExclusive: "0.13.0"},
		EntryScene: "scenes/demo.aescene", Files: manifestFiles,
		Archive: remoteexample.Archive{URL: server.URL + "/example.zip", Size: int64(packed.Len()), SHA256: hex.EncodeToString(hash[:])},
	})
	if err != nil {
		t.Fatal(err)
	}
	host := launcherRemote{root: filepath.Join(t.TempDir(), "store"), clientFactory: func() (*remoteexample.Client, error) {
		return remoteexample.GLTestLoopbackClient(remoteexample.Policy{EngineVersion: engine.Version}, server)
	}}
	ls, err := OpenLauncher(Config{Hidden: true, Recent: filepath.Join(t.TempDir(), "recent"), Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			ls.Close()
		}
	}()
	ls.UI = launcher.New(launcher.Options{Remote: host})
	ls.UI.SetTab(launcher.TabExamples)
	for range 3 {
		if !ls.Step() {
			t.Fatal("launcher stopped")
		}
	}
	click := func(x, y float32) {
		t.Helper()
		for i := range 3 {
			if !sdl.TestPointerEvent(x, y, i) || !ls.Step() {
				t.Fatal("launcher pointer event failed")
			}
		}
	}
	click(300, 199)
	for remaining := source; len(remaining) > 0; {
		n := min(20, len(remaining))
		if !sdl.TestTextInput(remaining[:n]) || !ls.Step() {
			t.Fatal("launcher URL input failed")
		}
		remaining = remaining[n:]
	}
	click(400, 223)
	if manifestRequests.Load() != 0 || archiveRequests.Load() != 0 {
		t.Fatal("network request was sent before origin confirmation")
	}
	click(400, 271)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := ls.UI.Result(); ok {
			break
		}
		if !ls.Step() {
			break
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	root, ok := ls.UI.Result()
	if !ok {
		t.Fatalf("TLS clone did not finish (manifest=%d archive=%d)", manifestRequests.Load(), archiveRequests.Load())
	}
	entry, remote := ls.UI.ResultEntry()
	if !remote || entry != "scenes/demo.aescene" || manifestRequests.Load() != 1 || archiveRequests.Load() != 1 {
		t.Fatalf("wrong clone result: root=%q entry=%q remote=%v manifest=%d archive=%d", root, entry, remote, manifestRequests.Load(), archiveRequests.Load())
	}
	entryFile := filepath.Join(root, filepath.FromSlash(entry))
	if data, err := os.ReadFile(entryFile); err != nil || !bytes.Equal(data, files[entry]) {
		t.Fatalf("verified entry scene differs from archive: %q %v", data, err)
	}
	ls.Close()
	closed = true
	clonedProject, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	editor, err := Open(clonedProject, Config{Hidden: true, NoWatch: true, OpenPath: entry, Logger: log.Nop(), RemoteProject: remote})
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if !editor.Step() {
			editor.Close()
			t.Fatal("editor stopped after TLS clone")
		}
	}
	if doc := editor.App.Context().Docs.Active(); doc == nil || doc.Path != entry || editor.App.Play().Scene() != nil {
		editor.Close()
		t.Fatalf("TLS clone did not open its entry in the editor without Play: %+v", doc)
	}
	editor.Close()
	if err := os.WriteFile(entryFile, []byte("scene version=1 { } // locally edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := host.Open(context.Background(), source, server.URL, false)
	if err != nil || reopened.Directory != root {
		t.Fatalf("offline reopen failed: %+v %v", reopened, err)
	}
	if data, err := os.ReadFile(entryFile); err != nil || string(data) != "scene version=1 { } // locally edited\n" || manifestRequests.Load() != 1 || archiveRequests.Load() != 1 {
		t.Fatalf("offline reopen changed local edits or fetched again: %q %v", data, err)
	}
}

func (r trackedLauncherRemote) List(ctx context.Context) ([]launcher.RemoteCopy, error) {
	r.calls <- "list"
	return r.host.List(ctx)
}
func (r trackedLauncherRemote) Open(ctx context.Context, url, origin string, clone bool) (launcher.RemoteCopy, error) {
	r.calls <- "open"
	return r.host.Open(ctx, url, origin, clone)
}
func (r trackedLauncherRemote) Reopen(ctx context.Context, id string) (launcher.RemoteCopy, error) {
	r.calls <- "reopen"
	return r.host.Reopen(ctx, id)
}
func (r trackedLauncherRemote) Check(ctx context.Context, id, origin string) (launcher.RemoteUpdate, error) {
	r.calls <- "check"
	return r.host.Check(ctx, id, origin)
}

func TestRemoteLauncherNativeConfirmation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	host, _, source, entryFile := remoteFixture(t)
	tracked := trackedLauncherRemote{host: host, calls: make(chan string, 4)}
	ls, err := OpenLauncher(Config{Hidden: true, Recent: filepath.Join(t.TempDir(), "recent")})
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			ls.Close()
		}
	}()
	ls.UI = launcher.New(launcher.Options{Remote: tracked})
	ls.UI.SetTab(launcher.TabExamples)
	for range 3 {
		if !ls.Step() {
			t.Fatal("launcher stopped")
		}
	}
	capture := func(name string) {
		t.Helper()
		dir := os.Getenv("AEGO_REMOTE_LAUNCH_SHOTS")
		if dir == "" {
			return
		}
		var shot *image.RGBA
		var shotErr error
		ls.Engine.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
		ls.Step()
		if shotErr != nil || shot == nil {
			t.Fatalf("capture launcher: %v", shotErr)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, shot); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	capture("remote-launcher")
	select {
	case call := <-tracked.calls:
		t.Fatalf("opening the tab accessed the remote service: %s", call)
	default:
	}
	click := func(x, y float32) {
		t.Helper()
		for i := range 3 {
			if !sdl.TestPointerEvent(x, y, i) || !ls.Step() {
				t.Fatal("launcher pointer event failed")
			}
		}
	}
	click(300, 199)
	for len(source) > 0 {
		n := min(20, len(source))
		if !sdl.TestTextInput(source[:n]) || !ls.Step() {
			t.Fatal("launcher text input failed")
		}
		source = source[n:]
	}
	click(400, 223)
	select {
	case call := <-tracked.calls:
		t.Fatalf("service was called before the origin confirmation: %s", call)
	default:
	}
	capture("remote-confirmation")
	click(400, 271)
	select {
	case call := <-tracked.calls:
		if call != "open" {
			t.Fatalf("unexpected remote operation %s", call)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirming the origin did not start the remote operation")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := ls.UI.Result(); ok {
			break
		}
		if !ls.Step() {
			break
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	root, ok := ls.UI.Result()
	if !ok || root != filepath.Dir(filepath.Dir(entryFile)) {
		t.Fatalf("confirmed offline copy did not open: %q", root)
	}
	entry, remote := ls.UI.ResultEntry()
	if !remote || entry != "scenes/demo.aescene" {
		t.Fatalf("manifest entry scene was not selected: %q, remote=%v", entry, remote)
	}
	ls.Close()
	closed = true
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	editor, err := Open(p, Config{Hidden: true, NoWatch: true, OpenPath: entry, Logger: log.Nop(), RemoteProject: remote})
	if err != nil {
		t.Fatal(err)
	}
	defer editor.Close()
	for range 3 {
		if !editor.Step() {
			t.Fatal("editor stopped after opening remote copy")
		}
	}
	if doc := editor.App.Context().Docs.Active(); doc == nil || doc.Path != entry {
		t.Fatalf("editor did not open the remote entry scene: %+v", doc)
	}
	if editor.App.Play().Scene() != nil {
		t.Fatal("opening a remote example started the game")
	}
}
