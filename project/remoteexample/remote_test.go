package remoteexample

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func packTest(t *testing.T, files map[string][]byte, symlink string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0600)
		if name == symlink {
			h.SetMode(os.ModeSymlink | 0777)
		}
		w, e := z.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(files[name]); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func exampleManifest(origin string, packed []byte, files map[string][]byte) Manifest {
	m := Manifest{Format: 1, ExampleID: "platformer-demo", Version: "1.0.0", Engine: Compatibility{"0.12.1", "0.13.0"}, EntryScene: "scenes/main.aescene", Modules: []string{"render2d"}, License: "MIT", Origin: origin, Archive: Archive{origin + "/example.zip", int64(len(packed)), hashBytes(packed)}}
	for p, b := range files {
		m.Files = append(m.Files, File{p, int64(len(b)), hashBytes(b)})
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	return m
}

type fixture struct {
	mu     sync.Mutex
	m      Manifest
	packed []byte
	server *httptest.Server
	client *Client
	store  *Store
	hits   int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	files := map[string][]byte{"scenes/main.aescene": []byte("scene { name=\"Main\" }"), "scripts/player.aes": []byte("func Ready() {}"), "project.aego": []byte("project {}")}
	f := &fixture{packed: packTest(t, files, "")}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits++
		switch r.URL.Path {
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(f.m)
		case "/example.zip":
			_, _ = w.Write(f.packed)
		case "/redirect":
			http.Redirect(w, r, "/manifest.json", http.StatusFound)
		case "/bad-redirect":
			http.Redirect(w, r, "https://example.invalid/manifest.json", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	f.m = exampleManifest(f.server.URL, f.packed, files)
	var e error
	f.client, e = NewClient(Policy{EngineVersion: "0.12.1", InstalledModules: []string{"render2d"}})
	if e != nil {
		t.Fatal(e)
	}
	// Only this local TLS fixture replaces publicDial. Production has no
	// exported option for disabling destination checks or TLS verification.
	f.client.http = f.server.Client()
	f.store, e = NewStore(filepath.Join(t.TempDir(), "examples"), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.client.Close(); f.server.Close() })
	return f
}
func (f *fixture) url() string { return f.server.URL + "/manifest.json" }
func (f *fixture) clone(t *testing.T) Copy {
	t.Helper()
	c, e := f.store.Clone(context.Background(), f.client, f.url(), f.server.URL)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func (f *fixture) replaceArchive(t *testing.T, files map[string][]byte, link string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.packed = packTest(t, files, link)
	f.m.Archive.Size = int64(len(f.packed))
	f.m.Archive.SHA256 = hashBytes(f.packed)
}

func TestHTTPSCloneAndOfflineEditedCopy(t *testing.T) {
	f := newFixture(t)
	c := f.clone(t)
	if b, e := os.ReadFile(c.Entry); e != nil || !strings.Contains(string(b), "Main") {
		t.Fatal(string(b), e)
	}
	user := []byte("scene { name=\"User edited\" }")
	if e := os.WriteFile(c.Entry, user, 0600); e != nil {
		t.Fatal(e)
	}
	f.server.Close()
	got, e := f.store.OpenRecent(f.url())
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(got.Entry)
	if e != nil || !bytes.Equal(b, user) {
		t.Fatal("offline reopen destroyed edited source", e)
	}
	if got.Directory != c.Directory {
		t.Fatal("reopen cloned instead of using local project")
	}
}
func TestUntrustedOriginRequiresExplicitApproval(t *testing.T) {
	f := newFixture(t)
	if _, e := f.client.FetchManifest(context.Background(), f.url(), ""); e == nil {
		t.Fatal("unapproved source accepted")
	}
	if _, e := f.client.FetchManifest(context.Background(), f.url(), "https://other.invalid"); e == nil {
		t.Fatal("wrong origin approved")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hits != 0 {
		t.Fatal("unapproved request sent")
	}
}
func TestTrustedOriginsAreExact(t *testing.T) {
	for _, origin := range []string{"http://example.com", "https://example.com/catalog", "https://*.example.com"} {
		c, e := NewClient(Policy{EngineVersion: "0.12.1", TrustedOrigins: []string{origin}})
		if c != nil {
			c.Close()
		}
		if e == nil {
			t.Fatalf("invalid origin accepted: %s", origin)
		}
	}
}
func TestSameOriginRedirectAndRejectedCrossOrigin(t *testing.T) {
	f := newFixture(t)
	if _, e := f.client.FetchManifest(context.Background(), f.server.URL+"/redirect", f.server.URL); e != nil {
		t.Fatal(e)
	}
	for _, route := range []string{"/bad-redirect", "/loop"} {
		if _, e := f.client.FetchManifest(context.Background(), f.server.URL+route, f.server.URL); e == nil {
			t.Fatal("bad redirect accepted")
		}
	}
}
func TestProductionRejectsPrivateDestinations(t *testing.T) {
	c, e := NewClient(Policy{EngineVersion: "0.12.1"})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if _, e = c.FetchManifest(context.Background(), "https://127.0.0.1:1/example.json", "https://127.0.0.1:1"); e == nil || !strings.Contains(e.Error(), "non-public") {
		t.Fatalf("private destination not rejected before TLS: %v", e)
	}
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "100.64.0.1", "0.1.2.3", "192.0.2.5", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1", "2001:db8::1", "64:ff9b::7f00:1", "2002:7f00:1::"} {
		if publicIP(netip.MustParseAddr(s)) {
			t.Fatalf("non-public IP accepted %s", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(s)) {
			t.Fatalf("public IP rejected %s", s)
		}
	}
}
func TestArchiveHashFailureLeavesNoProject(t *testing.T) {
	f := newFixture(t)
	f.m.Archive.SHA256 = strings.Repeat("0", 64)
	if _, e := f.store.Clone(context.Background(), f.client, f.url(), f.server.URL); e == nil {
		t.Fatal("bad archive hash accepted")
	}
	entries, _ := os.ReadDir(filepath.Join(f.store.root, "projects"))
	if len(entries) != 0 {
		t.Fatal("partial project published")
	}
}
func TestFileHashFailureLeavesNoProject(t *testing.T) {
	f := newFixture(t)
	f.m.Files[0].SHA256 = strings.Repeat("0", 64)
	if _, e := f.store.Clone(context.Background(), f.client, f.url(), f.server.URL); e == nil {
		t.Fatal("bad file hash accepted")
	}
	entries, _ := os.ReadDir(filepath.Join(f.store.root, "projects"))
	if len(entries) != 0 {
		t.Fatal("partial extraction leaked")
	}
}
func TestManifestUnsafePaths(t *testing.T) {
	for _, p := range []string{"../escape.png", "/tmp/a.png", "C:/a.png", "a\\b.png", "a/../b.png", "a//b.png", "a/CON.png", "a/x.png:ads", "a/ending. ", "a/../.git/config", ".env", "a/NUL", "a/../", "a/a\x00.png", "a/a?.png", "a/x<y.png", "a/рисунок.png"} {
		if e := cleanPath(p); e == nil {
			t.Fatalf("unsafe path accepted %q", p)
		}
	}
	for _, p := range []string{"assets/player.png", "Scripts/Player.aes", "scenes/Main.aescene", "project.aego"} {
		if e := cleanPath(p); e != nil {
			t.Fatal(p, e)
		}
	}
}
func TestManifestDuplicatesNativeFilesAndMissingEntry(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"duplicate", func(m *Manifest) { m.Files = append(m.Files, m.Files[0]) }},
		{"case parent", func(m *Manifest) { m.Files = append(m.Files, File{"Scenes/other.aescene", 0, hashBytes(nil)}) }},
		{"native", func(m *Manifest) { m.Files = append(m.Files, File{"plugin.so", 0, hashBytes(nil)}) }},
		{"engine source", func(m *Manifest) { m.Files = append(m.Files, File{"engine.go", 0, hashBytes(nil)}) }},
		{"missing entry", func(m *Manifest) { m.EntryScene = "missing.aescene" }},
		{"license", func(m *Manifest) { m.License = "" }},
		{"provenance", func(m *Manifest) { m.Origin = "https://evil.invalid" }},
		{"archive origin", func(m *Manifest) { m.Archive.URL = "https://evil.invalid/x.zip" }},
		{"bad engine interval", func(m *Manifest) { m.Engine.MaxExclusive = m.Engine.Min }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := f.m
			m.Files = append([]File(nil), m.Files...)
			test.mutate(&m)
			if e := m.Validate(f.url(), Limits{}); e == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}
func TestDuplicateAndUnknownJSONKeys(t *testing.T) {
	f := newFixture(t)
	data, _ := json.Marshal(f.m)
	for _, bad := range [][]byte{
		append([]byte(`{"format":1,`), data[1:]...),
		append([]byte(`{"FORMAT":1,`), data[1:]...),
		append([]byte(`{"shell":"rm -rf /",`), data[1:]...),
		append(append([]byte(nil), data...), []byte(` {}`)...),
	} {
		if _, e := ParseManifest(bad, f.url(), Limits{}); e == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
}
func TestResourceLimitsAndEngineCompatibility(t *testing.T) {
	f := newFixture(t)
	if e := f.m.CheckEngine("0.12.0", []string{"render2d"}); e == nil {
		t.Fatal("old engine accepted")
	}
	if e := f.m.CheckEngine("0.13.0", []string{"render2d"}); e == nil {
		t.Fatal("exclusive version bound ignored")
	}
	if e := f.m.CheckEngine("0.12.1", nil); e == nil {
		t.Fatal("missing module accepted")
	}
	for _, limits := range []Limits{{Files: 1}, {FileBytes: 1}, {ExpandedBytes: 1}, {ArchiveBytes: 1}} {
		if e := f.m.Validate(f.url(), limits); e == nil {
			t.Fatal("limit ignored", limits)
		}
	}
}
func TestTraversalUnexpectedFileAndSymlinkArchives(t *testing.T) {
	for _, bad := range []string{"../outside.aescene", "extra.png", "assets/link.png"} {
		t.Run(bad, func(t *testing.T) {
			f := newFixture(t)
			files := map[string][]byte{"scenes/main.aescene": []byte("scene {}"), bad: []byte("../../outside")}
			f.replaceArchive(t, files, bad)
			if _, e := f.store.Clone(context.Background(), f.client, f.url(), f.server.URL); e == nil {
				t.Fatal("unsafe archive accepted")
			}
			if _, e := os.Stat(filepath.Join(f.store.root, "outside.aescene")); !os.IsNotExist(e) {
				t.Fatal("escaped extraction")
			}
			entries, _ := os.ReadDir(filepath.Join(f.store.root, "projects"))
			if len(entries) != 0 {
				t.Fatal("partial project remains")
			}
		})
	}
}
func TestDuplicateZipEntriesRejected(t *testing.T) {
	f := newFixture(t)
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for i := 0; i < 2; i++ {
		w, _ := z.Create("scenes/main.aescene")
		_, _ = w.Write([]byte("scene {}"))
	}
	_ = z.Close()
	f.packed = b.Bytes()
	f.m = exampleManifest(f.server.URL, f.packed, map[string][]byte{"scenes/main.aescene": []byte("scene {}")})
	if _, e := f.store.Clone(context.Background(), f.client, f.url(), f.server.URL); e == nil {
		t.Fatal("duplicate ZIP accepted")
	}
}
func TestZipBombAndHeaderCountPreflight(t *testing.T) {
	f := newFixture(t)
	files := map[string][]byte{"scenes/main.aescene": bytes.Repeat([]byte("a"), 1<<20)}
	f.packed = packTest(t, files, "")
	f.m = exampleManifest(f.server.URL, f.packed, files)
	if _, e := f.store.Clone(context.Background(), f.client, f.url(), f.server.URL); e == nil {
		t.Fatal("compression bomb accepted")
	}
	b := append([]byte(nil), f.packed...)
	i := bytes.LastIndex(b, []byte{'P', 'K', 5, 6})
	binary.LittleEndian.PutUint16(b[i+10:i+12], 65535)
	name := filepath.Join(t.TempDir(), "bad.zip")
	_ = os.WriteFile(name, b, 0600)
	if e := preflightZip(name, 10, 64<<20); e == nil {
		t.Fatal("oversized central count accepted")
	}
	b = append([]byte("MZ"), f.packed...)
	_ = os.WriteFile(name, b, 0600)
	if e := preflightZip(name, 10, 64<<20); e == nil {
		t.Fatal("prefixed archive accepted")
	}
}
func TestStoreAndOfflineSymlinksRejected(t *testing.T) {
	root := t.TempDir()
	if e := os.Symlink(root, filepath.Join(root, "link")); e != nil {
		t.Skip(e)
	}
	if _, e := NewStore(filepath.Join(root, "link", "store"), Limits{}); e == nil {
		t.Fatal("symlinked store accepted")
	}
	f := newFixture(t)
	c := f.clone(t)
	_ = os.Remove(c.Entry)
	_ = os.Symlink("/etc/passwd", c.Entry)
	if _, e := f.store.Reopen(c.ID); e == nil {
		t.Fatal("offline symlink entry accepted")
	}
}

func TestOfflineProjectRejectsSymlinkedProjectOrAsset(t *testing.T) {
	for _, target := range []string{"project.aego", "assets/nested-link", ".aego/cache-link"} {
		t.Run(target, func(t *testing.T) {
			f := newFixture(t)
			c := f.clone(t)
			link := filepath.Join(c.Directory, filepath.FromSlash(target))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(link); err == nil {
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := f.store.Reopen(c.ID); err == nil {
				t.Fatal("offline reopen accepted a symlink in the editable project tree")
			}
		})
	}
}
func TestUpdateCheckNeverOverwritesEditedCopy(t *testing.T) {
	f := newFixture(t)
	local := f.clone(t)
	edited := []byte("edited")
	_ = os.WriteFile(local.Entry, edited, 0600)
	f.mu.Lock()
	f.m.Version = "1.1.0"
	f.mu.Unlock()
	u, e := f.client.CheckUpdate(context.Background(), local, f.server.URL)
	if e != nil || !u.Available {
		t.Fatal(u, e)
	}
	b, _ := os.ReadFile(local.Entry)
	if !bytes.Equal(b, edited) {
		t.Fatal("update check overwrote edit")
	}
	next := f.clone(t)
	if next.Directory == local.Directory {
		t.Fatal("clone reused modified directory")
	}
	b, _ = os.ReadFile(local.Entry)
	if !bytes.Equal(b, edited) {
		t.Fatal("clone overwrote existing project")
	}
}
func TestMutatedPublishedVersionRejected(t *testing.T) {
	f := newFixture(t)
	local := f.clone(t)
	f.mu.Lock()
	f.m.Archive.SHA256 = strings.Repeat("1", 64)
	f.mu.Unlock()
	if _, e := f.client.CheckUpdate(context.Background(), local, f.server.URL); e == nil {
		t.Fatal("same version changed content silently")
	}
}
func TestCancellationAndStoreLock(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := f.store.Clone(ctx, f.client, f.url(), f.server.URL); e == nil {
		t.Fatal("canceled install succeeded")
	}
	unlock, e := f.store.lock()
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	if _, e := f.store.Clone(context.Background(), f.client, f.url(), f.server.URL); e == nil {
		t.Fatal("concurrent writer was not rejected")
	}
}
func TestDeepLinksAreDataNotCommands(t *testing.T) {
	source := "https://example.com/catalog/hero.json?v=1"
	link, e := MakeLink(source, "scene")
	if e != nil {
		t.Fatal(e)
	}
	got, e := ParseLink(link)
	if e != nil || got.ManifestURL != source || got.Mode != "scene" {
		t.Fatal(got, e)
	}
	for _, bad := range []string{
		link + "&exec=sh", link + "&path=/tmp/p", link + "&manifest=https://other.invalid",
		"aego://example?manifest=file:///etc/passwd", "aego://example?manifest=http://example.com",
		"aego://example?manifest=https://example.com&mode=play", "aego://example/path?manifest=https://example.com",
		"aego://user@example?manifest=https://example.com", "aego://example?manifest=https://user:pass@example.com",
	} {
		if _, e := ParseLink(bad); e == nil {
			t.Fatalf("unsafe deep link accepted: %s", bad)
		}
	}
}
func FuzzManifest(f *testing.F) {
	f.Add([]byte(`{"format":1}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"a":{"a":1,"a":2}}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		_, _ = ParseManifest(b, "https://example.com/m.json", Limits{})
	})
}
func FuzzDeepLink(f *testing.F) {
	f.Add("aego://example?manifest=https://example.com/m.json")
	f.Fuzz(func(t *testing.T, s string) { _, _ = ParseLink(s) })
}

func TestHTTPSizeBoundedEvenWithoutContentLength(t *testing.T) {
	f := newFixture(t)
	f.client.policy.Limits.ManifestBytes = 32
	if _, e := f.client.FetchManifest(context.Background(), f.url(), f.server.URL); e == nil {
		t.Fatal("oversized HTTP body accepted")
	}
}
func TestCopyIDCannotEscapeStore(t *testing.T) {
	f := newFixture(t)
	for _, id := range []string{"..", "../project", "/tmp", "a\\b", strings.Repeat("x", 200)} {
		if _, e := f.store.Reopen(id); e == nil {
			t.Fatal("invalid local ID", fmt.Sprint(id))
		}
	}
}

func TestManifestMustIncludeTheProjectFile(t *testing.T) {
	f := newFixture(t)
	m := f.m
	m.Files = nil
	for _, file := range f.m.Files {
		if file.Path != "project.aego" {
			m.Files = append(m.Files, file)
		}
	}
	if err := m.Validate(f.url(), Limits{}); err == nil {
		t.Fatal("manifest without project.aego was accepted")
	}
}
