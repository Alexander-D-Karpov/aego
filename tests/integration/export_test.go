package integration

import (
	"bytes"
	"debug/pe"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"aego/app/export"
	"aego/app/export/winres"
	"aego/app/projects"
	"aego/asset"
	"aego/core/ids"
	"aego/project"
	"aego/runtime/bundle"
	"aego/runtime/player"
)

var assetRef = regexp.MustCompile(`asset\("([0-9a-f]{32})"\)`)

func exportEngine(t *testing.T) string {
	t.Helper()
	root, err := projects.EngineRoot("")
	if err != nil {
		t.Skip(err)
	}
	return root
}

func sdlBuilt(root, goos, arch string) bool {
	dir := filepath.Join(root, ".cache", "native", goos+"_"+arch, "lib")
	for _, n := range []string{"libSDL3.a", "SDL3.lib"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

func exportReady(t *testing.T, root string) {
	t.Helper()
	if testing.Short() {
		t.Skip("export builds a binary")
	}
	if !sdlBuilt(root, runtime.GOOS, runtime.GOARCH) {
		t.Skip("SDL is not bootstrapped, run make bootstrap")
	}
}

func exportTemplate(t *testing.T, tmpl string) *project.Project {
	t.Helper()
	p, err := projects.Create(projects.Options{Dir: filepath.Join(t.TempDir(), "game"), Template: tmpl})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func hasIssue(rep *export.Report, code, sev string) bool {
	for _, is := range rep.Issues {
		if is.Code == code && is.Severity == sev {
			return true
		}
	}
	return false
}

func withIcon(t *testing.T, p *project.Project) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: 160, A: 255})
		}
	}
	f, err := os.Create(filepath.Join(p.Root, "icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	p.Icon = "icon.png"
	p.GameVersion = "1.2.3"
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
}

func dataOf(bin string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(filepath.Dir(bin), "..", "Resources", bundle.DataDir)
	}
	return filepath.Join(filepath.Dir(bin), bundle.DataDir)
}

func cleanEnv(home string) []string {
	if runtime.GOOS == "windows" {
		sys := os.Getenv("SystemRoot")
		return []string{"PATH=" + filepath.Join(sys, "System32"), "SystemRoot=" + sys,
			"APPDATA=" + home, "LOCALAPPDATA=" + home, "USERPROFILE=" + home}
	}
	return []string{"PATH=/usr/bin:/bin", "HOME=" + home, "XDG_CONFIG_HOME=" + home}
}

func assertNoSources(t *testing.T, dir string) {
	t.Helper()
	bad := []string{".aes", ".aescene", ".aeprefab", ".aedata", ".aetable", ".meta", ".go"}
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if slices.Contains(bad, filepath.Ext(path)) || d.Name() == project.FileName {
			t.Errorf("the export contains a source file: %s", path)
		}
		return nil
	})
}

func resolvesIn(idx *asset.Index, id ids.AssetID) bool {
	if _, ok := asset.LookupBuiltin(id); ok {
		return true
	}
	e, ok := idx.Lookup(id)
	return ok && !e.Missing
}

func unresolvedAuthoredAssets(idx *asset.Index, raw []byte) []string {
	var missing []string
	for _, m := range assetRef.FindAllSubmatch(raw, -1) {
		id, err := ids.ParseAssetID(string(m[1]))
		// A zero asset ID is an empty optional reference, not a dependency.
		if err == nil && id.IsZero() {
			continue
		}
		if err != nil || !resolvesIn(idx, id) {
			missing = append(missing, string(m[1]))
		}
	}
	return missing
}

func TestExportReferenceScanKeepsMissingAssets(t *testing.T) {
	idx := asset.NewIndex()
	present, err := ids.ParseAssetID("123456789abcdef0123456789abcdef0")
	if err != nil {
		t.Fatal(err)
	}
	idx.Put(asset.Entry{ID: present})
	const missing = "23456789abcdef0123456789abcdef01"
	raw := []byte(`asset("00000000000000000000000000000000") asset("123456789abcdef0123456789abcdef0") asset("` + missing + `")`)
	if got := unresolvedAuthoredAssets(idx, raw); !slices.Equal(got, []string{missing}) {
		t.Fatalf("expected only the real missing asset, got %v", got)
	}
	if resolvesIn(idx, ids.AssetID{}) {
		t.Fatal("an empty main-scene or manifest dependency must not resolve")
	}
}

func assertRefsResolve(t *testing.T, data string) {
	t.Helper()
	d, _, err := player.Mount(data)
	if err != nil {
		t.Fatal(err)
	}
	if !resolvesIn(d.Index, d.Game.MainScene) {
		t.Fatal("the start scene is not in the manifest")
	}
	d.Index.Each(func(e *asset.Entry) {
		for _, dep := range e.Deps {
			if !resolvesIn(d.Index, dep) {
				t.Errorf("%s depends on %s, which is not exported", e.Source, dep)
			}
		}
		if e.Kind != asset.ArtScene && e.Kind != asset.ArtPrefab {
			return
		}
		a, ok := e.Artifact(e.Kind)
		if !ok {
			t.Errorf("%s has no artifact", e.Source)
			return
		}
		raw, err := os.ReadFile(filepath.Join(data, filepath.FromSlash(strings.TrimPrefix(a.Path, bundle.Scheme+"://"))))
		if err != nil {
			t.Error(err)
			return
		}
		for _, id := range unresolvedAuthoredAssets(d.Index, raw) {
			t.Errorf("%s references %s, which does not resolve", e.Source, id)
		}
	})
}

func runExported(t *testing.T, bin string, frames string) {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command(bin, "--headless", "--frames", frames)
	cmd.Dir = t.TempDir()
	cmd.Env = cleanEnv(home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the exported game failed: %v\n%s", err, out)
	}
}

func TestExportPlatformerRunsWithoutSources(t *testing.T) {
	root := exportEngine(t)
	exportReady(t, root)
	p, err := projects.Create(projects.Options{Dir: filepath.Join(t.TempDir(), "plat"), Example: "platformer", Engine: root})
	if err != nil {
		t.Fatal(err)
	}
	withIcon(t, p)
	dev := filepath.Join(t.TempDir(), "dev")
	drep, err := export.Export(export.Options{Project: p, Engine: root, Profile: bundle.Development, Out: dev})
	if err != nil {
		t.Fatalf("%v\n%+v", err, drep.Issues)
	}
	rel := filepath.Join(t.TempDir(), "rel")
	rrep, err := export.Export(export.Options{Project: p, Engine: root, Profile: bundle.Release, Out: rel})
	if err != nil {
		t.Fatalf("%v\n%+v", err, rrep.Issues)
	}
	if rrep.BinarySize > drep.BinarySize {
		t.Errorf("release binary %d is larger than development %d", rrep.BinarySize, drep.BinarySize)
	}
	if _, err := bundle.DecodeExport(mustRead(t, filepath.Join(dev, bundle.ExportFile)), bundle.ExportFile); err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{dev, rel} {
		assertNoSources(t, out)
	}
	assertRefsResolve(t, dataOf(filepath.Join(dev, filepath.FromSlash(drep.Binary))))
	moved := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(dev, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(p.Root); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(moved, filepath.FromSlash(drep.Binary))
	runExported(t, bin, "120")
	runExported(t, filepath.Join(rel, filepath.FromSlash(rrep.Binary)), "120")
	if entries, _ := os.ReadDir(filepath.Join(moved, "profiles")); len(entries) > 0 {
		t.Error("player settings were written into the game directory")
	}
	switch runtime.GOOS {
	case "windows":
		types, err := winres.Types(mustRead(t, bin))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(types, 14) || !slices.Contains(types, 16) {
			t.Errorf("the exe has resource types %v, want an icon group and version info", types)
		}
		f, err := pe.NewFile(bytes.NewReader(mustRead(t, bin)))
		if err != nil {
			t.Fatal(err)
		}
		libs, err := f.ImportedLibraries()
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range libs {
			low := strings.ToLower(l)
			if strings.HasPrefix(low, "libgcc") || strings.HasPrefix(low, "libwinpthread") || strings.HasPrefix(low, "libstdc++") {
				t.Errorf("the exe needs %s, which a clean Windows does not have", l)
			}
		}
	case "darwin":
		app := filepath.Join(filepath.Dir(bin), "..", "..")
		if pl, err := exec.LookPath("plutil"); err == nil {
			if out, err := exec.Command(pl, "-lint", filepath.Join(app, "Contents", "Info.plist")).CombinedOutput(); err != nil {
				t.Errorf("Info.plist: %s", out)
			}
		}
		if cs, err := exec.LookPath("codesign"); err == nil {
			if out, err := exec.Command(cs, "--verify", "--deep", app).CombinedOutput(); err != nil {
				t.Errorf("codesign: %s", out)
			}
		}
	case "linux":
		if _, err := os.Stat(filepath.Join(moved, "icon.png")); err != nil {
			t.Error("the linux export has no icon.png")
		}
	}
}

// Test the canonical module name. Creating a differently named project from
// the example rewrites script/component identities and changes its replay.
func TestCanonicalPlatformerReleaseFinish(t *testing.T) {
	root := exportEngine(t)
	exportReady(t, root)
	p, err := projects.Create(projects.Options{
		Dir: filepath.Join(t.TempDir(), "Platformer"), Module: "mygame", Example: "platformer", Engine: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "release")
	rep, err := export.Export(export.Options{Project: p, Engine: root, Profile: bundle.Release, Out: out})
	if err != nil {
		t.Fatalf("%v\n%+v", err, rep.Issues)
	}
	assertNoSources(t, out)
	if err := os.RemoveAll(p.Root); err != nil {
		t.Fatal(err)
	}
	releaseBin := filepath.Join(out, filepath.FromSlash(rep.Binary))
	replay, err := filepath.Abs(lightingRouteFile)
	if err != nil {
		t.Fatal(err)
	}
	const finish = "fb3a738501b6010f2a581840c6194e41"
	const by = "c0000000000000000000000000000003"
	for _, frames := range []string{"1775", "2400"} {
		cmd := exec.Command(releaseBin, "--headless", "--frames", frames, "--seed", "42",
			"--replay", replay, "--observe-checkpoint", finish)
		cmd.Dir = t.TempDir()
		cmd.Env = cleanEnv(t.TempDir())
		out, err := cmd.Output()
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				t.Fatalf("frame %s: %v: %s", frames, err, e.Stderr)
			}
			t.Fatal(err)
		}
		var observed player.CheckpointObservation
		if err := json.Unmarshal(out, &observed); err != nil {
			t.Fatalf("frame %s: %v, output %q", frames, err, out)
		}
		want := player.CheckpointObservation{Node: finish}
		if frames == "2400" {
			want.By, want.Tick, want.Count = by, 1776, 1
		}
		if observed != want {
			t.Fatalf("frame %s: observed %+v, want %+v", frames, observed, want)
		}
	}
}

func TestTopdownReleaseRunsWithoutSources(t *testing.T) {
	root := exportEngine(t)
	exportReady(t, root)
	p, err := project.Load(copyExample(t, "topdown"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "release")
	rep, err := export.Export(export.Options{Project: p, Engine: root, Profile: bundle.Release, Out: out})
	if err != nil {
		t.Fatalf("%v\n%+v", err, rep.Issues)
	}
	assertNoSources(t, out)
	if err := os.RemoveAll(p.Root); err != nil {
		t.Fatal(err)
	}
	runExported(t, filepath.Join(out, filepath.FromSlash(rep.Binary)), "600")
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExportBlocksBrokenScripts(t *testing.T) {
	p := exportTemplate(t, projects.Template2D)
	dir := filepath.Join(p.ScriptsDir(), "game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package game\n\nscript Broken {\n\tfunc Ready() {\n\t\tmissing()\n\t}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "broken.aes"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := export.Export(export.Options{Project: p, Profile: bundle.Release, CheckOnly: true})
	if err == nil || rep == nil || !rep.Blocked || !hasIssue(rep, export.CodeScripts, "error") {
		t.Fatalf("a broken script did not block the export: %v %+v", err, rep)
	}
}

func TestExportMissingReferenceDependsOnProfile(t *testing.T) {
	p := exportTemplate(t, projects.Template2D)
	path := filepath.Join(p.ScenesDir(), "main.aescene")
	data := mustRead(t, path)
	ghost := ids.NewAssetID().String()
	next := strings.Replace(string(data), asset.BuiltinWhite.String(), ghost, 1)
	if next == string(data) {
		t.Skip("the 2d template references no builtin texture")
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := export.Export(export.Options{Project: p, Profile: bundle.Release, CheckOnly: true})
	if err == nil || !rel.Blocked || !hasIssue(rel, export.CodeMissing, "error") {
		t.Fatalf("release did not block a missing reference: %+v", rel.Issues)
	}
	dev, err := export.Export(export.Options{Project: p, Profile: bundle.Development, CheckOnly: true})
	if err != nil || dev.Blocked || !hasIssue(dev, export.CodeMissing, "warning") {
		t.Fatalf("development should only warn: %v %+v", err, dev.Issues)
	}
}

func TestExportCrossWithoutToolchain(t *testing.T) {
	root := exportEngine(t)
	if runtime.GOOS == "windows" {
		t.Skip("host is windows")
	}
	t.Setenv("CC_WINDOWS_AMD64", "")
	p := exportTemplate(t, projects.Template2D)
	_, err := export.Export(export.Options{Project: p, Engine: root, Target: "windows/amd64", Profile: bundle.Release, Out: filepath.Join(t.TempDir(), "w")})
	if err == nil || !strings.Contains(err.Error(), "C compiler") {
		t.Fatalf("want a toolchain error, got %v", err)
	}
}

func TestExportWindowsUnderWine(t *testing.T) {
	root := exportEngine(t)
	if testing.Short() || runtime.GOOS != "linux" {
		t.Skip("linux host only")
	}
	if os.Getenv("CC_WINDOWS_AMD64") == "" || !sdlBuilt(root, "windows", "amd64") {
		t.Skip("set CC_WINDOWS_AMD64 and run GOOS=windows make bootstrap")
	}
	p := exportTemplate(t, projects.Template2D)
	withIcon(t, p)
	out := filepath.Join(t.TempDir(), "win")
	rep, err := export.Export(export.Options{Project: p, Engine: root, Target: "windows/amd64", Profile: bundle.Release, Out: out})
	if err != nil {
		t.Fatalf("%v\n%+v", err, rep.Issues)
	}
	bin := filepath.Join(out, filepath.FromSlash(rep.Binary))
	types, err := winres.Types(mustRead(t, bin))
	if err != nil || !slices.Contains(types, 14) {
		t.Fatalf("resource types %v, %v", types, err)
	}
	wine, err := exec.LookPath("wine")
	if err != nil {
		t.Skip("wine is not installed, the build was checked")
	}
	cmd := exec.Command(wine, bin, "--headless", "--frames", "60")
	cmd.Env = append(os.Environ(), "WINEDEBUG=-all")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wine: %v\n%s", err, b)
	}
}
