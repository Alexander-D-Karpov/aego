package docstool

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aego/app/assembly/builtin"
	"aego/app/export/webaudio"
	"aego/engine"
	"aego/engine/module"
	"aego/project"
	"aego/project/remoteexample"
)

type Env struct {
	Project *project.Project
	Engine  string
	Modules []module.Module
	Native  func(args ...string) error
	Stdout  io.Writer
	Stderr  io.Writer
	Repo    string
	Demo    bool
	// ExamplesLicense is the reviewed license of packaged example sources.
	// An empty local value leaves gendocs' honest NOASSERTION default intact.
	ExamplesLicense     string
	ExamplesLicenseFile string
	ExamplesProduction  bool
	// Nil preserves gendocs' canonical default; a pointer to an empty string
	// deliberately disables source example packages for a non-production build.
	ExamplesURL     *string
	ExamplesVersion string
	// Lang also checks the translations into this language (D348).
	Lang string
}

func (e *Env) export(path string) error {
	if e.Native != nil {
		args := []string{"docs", "export"}
		if path != "" {
			args = append(args, "--out", path)
		}
		return e.Native(args...)
	}
	reg, err := ExportRegistry(e.Project, e.Modules)
	if err != nil {
		return err
	}
	return WriteExport(reg, Title(e.Project), path, e.Stdout)
}

func (e *Env) Export(path string) int {
	if err := e.export(path); err != nil {
		fmt.Fprintln(e.Stderr, "aego:", err)
		return 1
	}
	return 0
}

func (e *Env) Check(strict bool) int {
	if e.Native != nil {
		args := []string{"docs", "check"}
		if strict {
			args = append(args, "--strict")
		}
		if e.Lang != "" {
			args = append(args, "--lang", e.Lang)
		}
		if err := e.Native(args...); err != nil {
			return 1
		}
		return 0
	}
	reg, err := Registry(e.Project, e.Modules)
	if err != nil {
		fmt.Fprintln(e.Stderr, "aego:", err)
		return 1
	}
	extra := append(DiagnosticProblems(), GoProblems(reg, e.Engine, strict)...)
	if e.Lang != "" {
		extra = append(extra, reg.LangProblems(e.Lang, strict)...)
	}
	code := Check(reg, strict, e.Stdout, extra...)
	if CheckTry(reg, e.Stdout) > 0 && code == 0 {
		code = 1
	}
	if CheckExamples(reg, e.Stdout) > 0 && code == 0 {
		code = 1
	}
	if CheckSteps(reg, e.Stdout) > 0 && code == 0 {
		code = 1
	}
	return code
}

func (e *Env) Report(asJSON bool) int {
	if e.Native != nil {
		args := []string{"docs", "report"}
		if asJSON {
			args = append(args, "--json")
		}
		if err := e.Native(args...); err != nil {
			return 1
		}
		return 0
	}
	reg, err := Registry(e.Project, e.Modules)
	if err != nil {
		fmt.Fprintln(e.Stderr, "aego:", err)
		return 1
	}
	return Report(reg, e.Engine, e.Stdout, asJSON)
}

func (e *Env) goCmd(dir string, env []string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = e.Stdout
	cmd.Stderr = e.Stderr
	return cmd.Run()
}

const defaultBranch = "master"

func (e *Env) git(args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", e.Engine}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (e *Env) commit() string {
	if c := e.git("rev-parse", "--short", "HEAD"); c != "" {
		return c
	}
	return os.Getenv("AEGO_COMMIT")
}

func (e *Env) branch() string {
	if b := e.git("rev-parse", "--abbrev-ref", "HEAD"); b != "" && b != "HEAD" {
		return b
	}
	return defaultBranch
}

func (e *Env) Build(out string, play bool) error {
	if e.Engine == "" {
		return errors.New("cannot locate the aego source tree; pass --engine <dir> or set AEGO_ROOT")
	}
	license := strings.TrimSpace(e.ExamplesLicense)
	if e.ExamplesProduction && (license == "" || strings.EqualFold(license, "NOASSERTION")) {
		return errors.New("official example publication requires an explicit reviewed --examples-license")
	}
	if e.ExamplesURL != nil {
		raw := *e.ExamplesURL
		if raw == "" {
			if e.ExamplesProduction {
				return errors.New("official example publication cannot disable --examples-url")
			}
		} else {
			u, err := url.Parse(raw)
			if err != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(u.Path, "/") {
				return errors.New("--examples-url must be an HTTPS site root ending in / without a query or fragment")
			}
			if _, err := remoteexample.Origin(raw); err != nil {
				return fmt.Errorf("invalid --examples-url: %w", err)
			}
		}
	}
	licenseFile := e.ExamplesLicenseFile
	if licenseFile == "" && e.ExamplesProduction {
		return errors.New("official example publication requires --examples-license-file")
	}
	if licenseFile != "" {
		var err error
		licenseFile, err = filepath.Abs(licenseFile)
		if err != nil {
			return fmt.Errorf("example license file: %w", err)
		}
		data, err := os.ReadFile(licenseFile)
		if err != nil || len(data) == 0 || len(data) > 64<<10 || !utf8.Valid(data) || strings.TrimSpace(string(data)) == "" {
			return errors.New("example license file is unreadable, empty, invalid UTF-8, or larger than 64 KiB")
		}
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	var studioURL string
	if artifact := os.Getenv("ARTSTUDIO_ARTIFACT"); artifact != "" {
		var err error
		studioURL, err = e.integrateStudioGuides(artifact, abs)
		if err != nil {
			return err
		}
	}
	var playV, tryV string
	if play {
		if playV, err = e.playground(filepath.Join(abs, "play")); err != nil {
			return err
		}
		if tryV, err = e.tryApp(filepath.Join(abs, "try")); err != nil {
			return err
		}
		if studioURL == "" {
			if err = e.artStudio(filepath.Join(abs, "studio")); err != nil {
				return err
			}
		}
	}
	tmp, err := os.CreateTemp("", "aego-docs-*.json")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := e.export(tmp.Name()); err != nil {
		return err
	}
	args := []string{"run", ".", "-in", tmp.Name(), "-out", abs, "-version", engine.Version}
	if e.ExamplesURL != nil {
		args = append(args, "-examples-url", *e.ExamplesURL)
	}
	if e.ExamplesVersion != "" {
		args = append(args, "-examples-version", e.ExamplesVersion)
	}
	if license != "" {
		args = append(args, "-examples-license", license)
	}
	if licenseFile != "" {
		args = append(args, "-examples-license-file", licenseFile)
	}
	if e.ExamplesProduction {
		args = append(args, "-examples-production")
	}
	if play {
		args = append(args, "-playground", "-play-version", playV, "-try-version", tryV)
		if studioURL == "" {
			args = append(args, "-studio")
		}
	}
	if studioURL != "" {
		args = append(args, "-studio-url", studioURL, "-studio-guides")
	}
	if e.Demo {
		args = append(args, "-demo")
	}
	if e.Repo != "" {
		args = append(args, "-repo", e.Repo, "-edit", strings.TrimRight(e.Repo, "/")+"/src/branch/"+e.branch())
	}
	if c := e.commit(); c != "" {
		args = append(args, "-commit", c)
	}
	var icons []string
	for _, d := range builtin.IconSources() {
		icons = append(icons, filepath.Join(e.Engine, filepath.FromSlash(d)))
	}
	args = append(args, "-icons", strings.Join(icons, ","))
	if shots := filepath.Join(e.Engine, "docs", "shots"); fileExists(filepath.Join(shots, "demo.png")) {
		args = append(args, "-shots", shots)
	}
	if poster := demoPoster(e.Engine); poster != "" {
		args = append(args, "-poster", poster)
	}
	if hero := filepath.Join(e.Engine, "docs", "hero.png"); fileExists(hero) {
		args = append(args, "-hero", hero)
	}
	if err := e.goCmd(filepath.Join(e.Engine, "tools", "gendocs"), nil, args...); err != nil {
		return fmt.Errorf("gendocs failed: %w", err)
	}
	return nil
}

// integrateStudioGuides consumes one verified release artifact. It copies only
// its pinned lesson pages and host scripts, never another copy of the WASM.
func (e *Env) integrateStudioGuides(artifact, out string) (string, error) {
	args := []string{"run", "./tools/artstudio/release", "integrate-docs", "-dir", artifact, "-out", out}
	if hash := os.Getenv("ARTSTUDIO_MANIFEST_SHA256"); hash != "" {
		args = append(args, "-sha256", hash)
	}
	cmd := exec.Command("go", args...)
	cmd.Dir, cmd.Stderr = e.Engine, e.Stderr
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("integrate pinned Art Studio guides: %w", err)
	}
	var result struct {
		Status, EditorURL, ManifestSHA256 string
	}
	if err := json.Unmarshal(data, &result); err != nil || result.Status != "PASS" || result.EditorURL == "" || result.ManifestSHA256 == "" {
		return "", fmt.Errorf("invalid Art Studio guide integration response")
	}
	if e.Stdout != nil {
		fmt.Fprintf(e.Stdout, "Art Studio guides: %s (manifest %s)\n", result.EditorURL, result.ManifestSHA256)
	}
	return result.EditorURL, nil
}

// artStudio packages the existing UI/core and its optional real GPU preview for
// local docs without ARTSTUDIO_ARTIFACT. Production uses integrateStudioGuides
// above and pins the already-verified standalone artifact instead of rebuilding.
// Neither WASM is loaded by ordinary documentation pages.
func (e *Env) artStudio(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, entry := range []struct{ name, pkg string }{
		{"artstudio.wasm", "./tools/artstudio/wasm"},
		{"preview.wasm", "./tools/artstudio/previewwasm"},
	} {
		wasm := filepath.Join(dir, entry.name)
		if err := e.goCmd(e.Engine, []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}, "build", "-buildvcs=false", "-trimpath", "-ldflags", "-s -w", "-o", wasm, entry.pkg); err != nil {
			return fmt.Errorf("build Art Studio %s: %w", entry.name, err)
		}
		if _, err := gzipCopy(wasm); err != nil {
			return err
		}
	}
	root, err := goRoot()
	if err != nil {
		return err
	}
	support, err := os.ReadFile(filepath.Join(root, "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	return packageStudioWeb(filepath.Join(e.Engine, "tools", "artstudio", "web"), dir, support)
}

// Each entry carries the same digest, so static caches cannot mix a new WASM
// with an old JavaScript bridge. The source UI also works unmodified on the
// loopback development host; only these exact relative URLs are versioned.
func packageStudioWeb(web, dir string, support []byte) error {
	wasm, err := os.ReadFile(filepath.Join(dir, "artstudio.wasm"))
	if err != nil {
		return err
	}
	preview, err := os.ReadFile(filepath.Join(dir, "preview.wasm"))
	if err != nil {
		return err
	}
	names := []string{"index.html", "studio.css", "studio.js", "recovery.js", "gpu-preview.js", "aego.png"}
	data := make(map[string][]byte, len(names))
	parts := [][]byte{wasm, preview, support}
	for _, name := range names {
		data[name], err = os.ReadFile(filepath.Join(web, name))
		if err != nil {
			return err
		}
		parts = append(parts, data[name])
	}
	version := versionOf(parts...)
	index := data["index.html"]
	for _, entry := range []struct{ attr, name string }{{"href", "studio.css"}, {"href", "aego.png"}, {"src", "aego.png"}, {"src", "wasm_exec.js"}, {"src", "recovery.js"}, {"src", "gpu-preview.js"}, {"src", "studio.js"}} {
		old := []byte(entry.attr + `="` + entry.name + `"`)
		if bytes.Count(index, old) != 1 {
			return fmt.Errorf("studio index needs exactly one %s", old)
		}
		index = bytes.Replace(index, old, []byte(entry.attr+`="`+entry.name+"?v="+version+`"`), 1)
	}
	js := data["studio.js"]
	old := []byte(`fetch(window.AegoStudioRelease?.wasmURL || 'artstudio.wasm')`)
	if bytes.Count(js, old) != 1 {
		return errors.New("studio needs exactly one WASM fetch")
	}
	data["studio.js"] = bytes.Replace(js, old, []byte(`fetch(window.AegoStudioRelease?.wasmURL || 'artstudio.wasm?v=`+version+`')`), 1)
	previewJS := data["gpu-preview.js"]
	previewFetch := []byte(`fetch(window.AegoStudioRelease?.previewWasmURL || 'preview.wasm')`)
	if bytes.Count(previewJS, previewFetch) != 1 {
		return errors.New("studio needs exactly one GPU preview WASM fetch")
	}
	data["gpu-preview.js"] = bytes.Replace(previewJS, previewFetch, []byte(`fetch(window.AegoStudioRelease?.previewWasmURL || 'preview.wasm?v=`+version+`')`), 1)
	data["index.html"], data["wasm_exec.js"] = index, support
	for _, name := range append(names, "wasm_exec.js") {
		if err := os.WriteFile(filepath.Join(dir, name), data[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// The landing page poster represents the current game; editor screenshot
// fixtures intentionally use a historical scene for reproducible docs shots.
func demoPoster(root string) string {
	for _, name := range []string{"docs/demo-poster.png", "docs/shots/demo.png"} {
		if p := filepath.Join(root, filepath.FromSlash(name)); fileExists(p) {
			return p
		}
	}
	return ""
}

func (e *Env) tryApp(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	wasm := filepath.Join(dir, "aego-docs.wasm")
	if err := e.goCmd(e.Engine, []string{"GOOS=js", "GOARCH=wasm"}, "build", "-tags", "aego_editor", "-trimpath", "-ldflags", "-s -w", "-o", wasm, "./cmd/aego-try"); err != nil {
		return "", fmt.Errorf("docs runtime build failed: %w", err)
	}
	if _, err := gzipCopy(wasm); err != nil {
		return "", err
	}
	root, err := goRoot()
	if err != nil {
		return "", err
	}
	support, err := os.ReadFile(filepath.Join(root, "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), support, 0o644); err != nil {
		return "", err
	}
	blob, err := os.ReadFile(wasm)
	if err != nil {
		return "", err
	}
	return versionOf(blob, support), nil
}

// gzipCopy writes path.gz next to path, the copy the site serves, and returns its size.
func gzipCopy(path string) (int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	zw.Write(raw)
	if err := zw.Close(); err != nil {
		return 0, err
	}
	return int64(buf.Len()), os.WriteFile(path+".gz", buf.Bytes(), 0o644)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func goRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func versionOf(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil)[:5])
}

func (e *Env) playground(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	wasm := filepath.Join(dir, "aego.wasm")
	if err := e.goCmd(e.Engine, []string{"GOOS=js", "GOARCH=wasm"}, "build", "-trimpath", "-ldflags", "-s -w", "-o", wasm, "./cmd/aego-playground"); err != nil {
		return "", fmt.Errorf("playground build failed: %w", err)
	}
	gz, err := gzipCopy(wasm)
	if err != nil {
		return "", err
	}
	root, err := goRoot()
	if err != nil {
		return "", err
	}
	if err := copyFile(filepath.Join(root, "lib", "wasm", "wasm_exec.js"), filepath.Join(dir, "wasm_exec.js")); err != nil {
		return "", err
	}
	if err := webaudio.Build(e.Engine, dir, true); err != nil {
		return "", fmt.Errorf("playground audio worker build failed: %w", err)
	}
	host, worker, err := webaudio.Scripts(e.Engine, dir)
	if err != nil {
		return "", err
	}
	reactor, err := os.ReadFile(filepath.Join(dir, webaudio.Reactor))
	if err != nil {
		return "", err
	}
	if _, err := gzipCopy(filepath.Join(dir, webaudio.Reactor)); err != nil {
		return "", err
	}
	web := filepath.Join(e.Engine, "cmd", "aego-playground", "web")
	files, err := os.ReadDir(web)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(wasm)
	if err != nil {
		return "", err
	}
	blob, err := os.ReadFile(wasm)
	if err != nil {
		return "", err
	}
	parts := [][]byte{blob, host, worker, reactor}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(web, f.Name()))
		if err != nil {
			return "", err
		}
		parts = append(parts, data)
	}
	version := versionOf(parts...)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		src, dst := filepath.Join(web, f.Name()), filepath.Join(dir, f.Name())
		if f.Name() != "index.html" {
			if err := copyFile(src, dst); err != nil {
				return "", err
			}
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return "", err
		}
		data = bytes.ReplaceAll(data, []byte(`data-wasm-size="0"`), []byte(`data-wasm-size="`+strconv.FormatInt(info.Size(), 10)+`"`))
		data = bytes.ReplaceAll(data, []byte(`data-wasm-gzip="0"`), []byte(`data-wasm-gzip="`+strconv.FormatInt(gz, 10)+`"`))
		data = bytes.ReplaceAll(data, []byte(`data-version="dev"`), []byte(`data-version="`+version+`"`))
		data = bytes.ReplaceAll(data, []byte(`?v=dev"`), []byte(`?v=`+version+`"`))
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return "", err
		}
	}
	return version, nil
}

func (e *Env) watched() []string {
	var roots []string
	if e.Engine != "" {
		roots = append(roots, e.Engine)
	}
	if e.Project != nil && e.Project.Root != e.Engine {
		roots = append(roots, e.Project.Root)
	}
	return roots
}

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "site" || name == "node_modules" || name == "third_party"
}

func (e *Env) stamp() string {
	var n, size int64
	var latest time.Time
	for _, root := range e.watched() {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != root && skipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			slash := filepath.ToSlash(p)
			doc := strings.HasSuffix(slash, ".md") && strings.Contains(slash, "/res/docs/")
			if !doc && filepath.Ext(p) != ".aes" {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			n++
			size += info.Size()
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
			return nil
		})
	}
	return strconv.FormatInt(n, 10) + ":" + strconv.FormatInt(size, 10) + ":" + latest.String()
}

func (e *Env) watch(out string) {
	last := e.stamp()
	for range time.Tick(time.Second) {
		s := e.stamp()
		if s == last {
			continue
		}
		last = s
		start := time.Now()
		if err := e.Build(out, false); err != nil {
			fmt.Fprintln(e.Stderr, "docs:", err)
			continue
		}
		fmt.Fprintf(e.Stdout, "docs: rebuilt in %s\n", time.Since(start).Round(time.Millisecond))
	}
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (e *Env) Serve(addr, out string, play bool) error {
	if err := e.Build(out, play); err != nil {
		return err
	}
	go e.watch(out)
	fmt.Fprintf(e.Stdout, "docs: serving %s on http://%s\n", out, addr)
	return http.ListenAndServe(addr, noCache(siteHandler(out)))
}
