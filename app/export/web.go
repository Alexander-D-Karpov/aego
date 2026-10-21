package export

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"aego/app/export/icon"
	"aego/asset"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine/problems"
	"aego/runtime/bundle"
)

//go:embed web
var webFS embed.FS

const (
	CodeWebAsset  = "EXP010"
	CodeWebNative = "EXP011"
	CodeWebMP3    = "EXP012"
	WebWasm       = "game.wasm"
	WebFiles      = "files.json"
	WebBudget     = 8 << 20
	webExec       = "wasm_exec.js"
	webLoader     = "loader.js"
	webIndex      = "index.html"
	webIcon       = "favicon.png"
	webIconSize   = 64
	buildKeyLen   = 12
)

var webIndexTmpl = template.Must(template.ParseFS(webFS, "web/"+webIndex))

var webCompressed = map[string]bool{".wasm": true, ".js": true, ".html": true, ".json": true, ".aetext": true}

type webFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type webList struct {
	Wasm     int64     `json:"wasm"`
	Manifest webFile   `json:"manifest"`
	Files    []webFile `json:"files"`
}

type webPage struct {
	Title string
	Build string
	Icon  bool
}

func webUnsupported(asset.ArtifactKind) bool {
	return false
}

func CheckWeb(s *Snapshot, native bool) []problems.Diagnostic {
	var out []problems.Diagnostic
	if native {
		out = append(out, diag(CodeWebNative, problems.Warning,
			"the native Go module is compiled to WebAssembly: it must build without cgo and must not use files, processes or the network", "native/"))
	}
	_, ffmpegErr := exec.LookPath("ffmpeg")
	s.idx.Each(func(e *asset.Entry) {
		if ffmpegErr != nil && strings.EqualFold(filepath.Ext(e.Source), ".mp3") {
			out = append(out, diag(CodeWebMP3, problems.Warning, e.Source+" is an mp3; it is converted to ogg on import, which needs ffmpeg in PATH", e.Source))
		}
		if e.Missing || exportSkip(e.Kind) || !webUnsupported(e.Kind) {
			return
		}
		out = append(out, diag(CodeWebAsset, problems.Warning, e.Source+" is a "+string(e.Kind)+" asset, which the web runtime cannot load", e.Source))
	})
	return out
}

func goRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return "", errs.Wrap(err, errs.BuildFailed, "cannot ask the Go toolchain for GOROOT")
	}
	return strings.TrimSpace(string(out)), nil
}

func wasmExecPath(root string) (string, error) {
	for _, p := range []string{filepath.Join(root, "lib", "wasm", webExec), filepath.Join(root, "misc", "wasm", webExec)} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errs.New(errs.BuildFailed, "the Go toolchain has no "+webExec, log.F("goroot", root))
}

func writeFile(path string, data []byte, what string) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write "+what).With(log.F("path", path))
	}
	return nil
}

func gzipped(raw []byte) []byte {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	zw.Write(raw)
	zw.Close()
	return buf.Bytes()
}

func precompress(path, brotli string) (int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, errs.Wrap(err, errs.VFSIO, "cannot read a file to compress").With(log.F("path", path))
	}
	gz := gzipped(raw)
	if len(gz) >= len(raw)*9/10 {
		return int64(len(raw)), nil
	}
	if err := writeFile(path+".gz", gz, "a compressed copy"); err != nil {
		return 0, err
	}
	if brotli != "" {
		if out, err := exec.Command(brotli, "-f", "-k", "-q", "11", path).CombinedOutput(); err != nil {
			return 0, errs.New(errs.BuildFailed, "brotli failed: "+strings.TrimSpace(string(out)), log.F("path", path))
		}
	}
	return int64(len(gz)), nil
}

func (j *job) webTitle() string {
	if t := strings.TrimSpace(j.p.Window.Title); t != "" {
		return t
	}
	return AppName(j.p)
}

func (j *job) packWeb() error {
	src, err := wasmExecPath(j.goroot)
	if err != nil {
		return err
	}
	exec, err := os.ReadFile(src)
	if err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot read "+webExec)
	}
	if err := writeFile(filepath.Join(j.out, webExec), exec, webExec); err != nil {
		return err
	}
	loader, err := fs.ReadFile(webFS, "web/"+webLoader)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(j.out, webLoader), loader, webLoader); err != nil {
		return err
	}
	if j.icon != nil {
		data, err := icon.PNG(icon.Resize(j.icon, webIconSize, webIconSize))
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(j.out, webIcon), data, "the icon"); err != nil {
			return err
		}
	}
	list, err := json.Marshal(webList{Wasm: j.rep.BinarySize, Manifest: webFile{Path: bundle.ManifestFile, Size: j.manifestSize}, Files: j.files})
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(j.l.data, WebFiles), list, WebFiles); err != nil {
		return err
	}
	build := j.rep.Hash
	if len(build) > buildKeyLen {
		build = build[:buildKeyLen]
	}
	var page bytes.Buffer
	if err := webIndexTmpl.Execute(&page, webPage{Title: j.webTitle(), Build: build, Icon: j.icon != nil}); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(j.out, webIndex), page.Bytes(), webIndex); err != nil {
		return err
	}
	return j.compressWeb()
}

func (j *job) compressWeb() error {
	brotli, _ := exec.LookPath("brotli")
	art := filepath.Join(j.l.data, bundle.ArtifactsDir)
	wasm := j.l.binary()
	return filepath.WalkDir(j.out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !webCompressed[filepath.Ext(p)] && filepath.Dir(p) != art {
			return nil
		}
		n, err := precompress(p, brotli)
		if err != nil {
			return err
		}
		if p == wasm {
			j.rep.BinaryGzip = n
		}
		return nil
	})
}
