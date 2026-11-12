package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"aego/art"
)

type buildOptions struct {
	Root, Out, ID, Origin, DocsOrigin, AegoVersion string
	Release                                        bool
	Epoch                                          int64
}

func command(root string, env []string, args ...string) ([]byte, error) {
	c := exec.Command(args[0], args[1:]...)
	c.Dir = root
	if env != nil {
		c.Env = append(os.Environ(), env...)
	}
	b, e := c.CombinedOutput()
	if e != nil {
		return nil, fmt.Errorf("%s: %w\n%s", args[0], e, b)
	}
	return b, nil
}
func sourceHash(root string) (string, error) {
	var paths []string
	// Include renderer/module dependencies and their embedded resources too:
	// the lazy preview belongs to this exact artifact, not to a separately
	// versioned or silently changing runtime.
	for _, dir := range []string{"art", "tools/artstudio", "anim", "asset", "contracts", "core", "engine", "format", "gpu", "modules/render2d", "modules/particles", "platform", "render", "save", "scene", "schema", "script", "vfs"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("source symlink: %s", p)
			}
			if d.IsDir() {
				if d.Name() == "__pycache__" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	for _, p := range []string{"go.mod", "go.sum"} {
		if _, e := os.Stat(filepath.Join(root, p)); e == nil {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		b, e := readBounded(filepath.Join(root, p), maxFileBytes)
		if e != nil {
			return "", e
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func asset(out, name string, b []byte) (string, error) {
	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext) + "." + digest(b)[:16] + ext
	p := "assets/" + name
	return p, writeNew(filepath.Join(out, filepath.FromSlash(p)), b)
}

func standaloneDependencyAllowed(p string) bool {
	switch p {
	case "aego/art", "aego/art/internal/aseprite", "aego/tools/artstudio/host", "aego/tools/artstudio/wasm":
		return true
	default:
		return !strings.HasPrefix(p, "aego/")
	}
}

func build(o buildOptions) (buildManifest, error) {
	var m buildManifest
	if !identifier.MatchString(o.ID) || !validOrigin(o.Origin, o.Release) || !validOrigin(o.DocsOrigin, o.Release) {
		return m, fmt.Errorf("invalid build ID/origins")
	}
	root, e := filepath.Abs(o.Root)
	if e != nil {
		return m, e
	}
	engineVersion, e := sourceEngineVersion(root)
	if e != nil {
		return m, e
	}
	out, e := filepath.Abs(o.Out)
	if e != nil {
		return m, e
	}
	if _, e := os.Lstat(out); !os.IsNotExist(e) {
		return m, fmt.Errorf("output must not exist: %s", out)
	}
	for _, d := range []string{"art", "tools", ".git"} {
		if out == filepath.Join(root, d) || strings.HasPrefix(out, filepath.Join(root, d)+string(filepath.Separator)) {
			return m, fmt.Errorf("output inside source tree")
		}
	}
	source, e := sourceHash(root)
	if e != nil {
		return m, e
	}
	commit := ""
	if b, e := command(root, nil, "git", "rev-parse", "HEAD"); e == nil {
		commit = strings.TrimSpace(string(b))
	}
	if o.Release {
		if !commitPattern.MatchString(commit) {
			return m, fmt.Errorf("release requires Git checkout")
		}
		status, e := command(root, nil, "git", "status", "--porcelain=v1", "--untracked-files=all")
		if e != nil {
			return m, e
		}
		if len(bytes.TrimSpace(status)) != 0 {
			return m, fmt.Errorf("release build requires an exact clean tree; use snapshot build for uncommitted work")
		}
	}
	if o.Epoch <= 0 {
		if o.Release {
			b, e := command(root, nil, "git", "show", "-s", "--format=%ct", "HEAD")
			if e != nil {
				return m, e
			}
			if _, e = fmt.Sscan(string(b), &o.Epoch); e != nil {
				return m, e
			}
		} else {
			o.Epoch = time.Now().Unix()
		}
	}
	deps, e := command(root, []string{"GOOS=js", "GOARCH=wasm"}, "go", "list", "-buildvcs=false", "-deps", "./tools/artstudio/wasm")
	if e != nil {
		return m, e
	}
	for _, p := range strings.Fields(string(deps)) {
		if !standaloneDependencyAllowed(p) {
			return m, fmt.Errorf("unexpected engine dependency in standalone WASM: %s", p)
		}
	}
	previewDeps, e := command(root, []string{"GOOS=js", "GOARCH=wasm"}, "go", "list", "-buildvcs=false", "-deps", "./tools/artstudio/previewwasm")
	if e != nil {
		return m, e
	}
	for _, p := range strings.Fields(string(previewDeps)) {
		for _, forbidden := range []string{"aego/app/", "aego/editor/", "aego/project/", "aego/asset/pipeline", "aego/asset/importers/"} {
			if strings.HasPrefix(p, forbidden) {
				return m, fmt.Errorf("desktop/import dependency in preview WASM: %s", p)
			}
		}
		if strings.HasPrefix(p, "aego/modules/") && p != "aego/modules/render2d" && p != "aego/modules/particles" {
			return m, fmt.Errorf("unneeded runtime module in preview WASM: %s", p)
		}
	}
	if e = os.MkdirAll(filepath.Dir(out), 0755); e != nil {
		return m, e
	}
	tmp, e := os.MkdirTemp(filepath.Dir(out), ".artstudio-build-")
	if e != nil {
		return m, e
	}
	defer os.RemoveAll(tmp)
	wasmFile := filepath.Join(tmp, "core.wasm")
	if _, e = command(root, []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}, "go", "build", "-buildvcs=false", "-trimpath", "-o", wasmFile, "./tools/artstudio/wasm"); e != nil {
		return m, e
	}
	wasm, e := os.ReadFile(wasmFile)
	if e != nil {
		return m, e
	}
	if e = os.Remove(wasmFile); e != nil {
		return m, e
	}
	if len(wasm) > 8<<20 {
		return m, fmt.Errorf("standalone WASM exceeds 8 MiB budget")
	}
	wasmName, e := asset(tmp, "artstudio.wasm", wasm)
	if e != nil {
		return m, e
	}
	previewFile := filepath.Join(tmp, "preview.wasm")
	if _, e = command(root, []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}, "go", "build", "-buildvcs=false", "-trimpath", "-o", previewFile, "./tools/artstudio/previewwasm"); e != nil {
		return m, e
	}
	preview, e := os.ReadFile(previewFile)
	if e != nil {
		return m, e
	}
	if e = os.Remove(previewFile); e != nil {
		return m, e
	}
	if len(preview) > 32<<20 {
		return m, fmt.Errorf("lazy GPU preview WASM exceeds 32 MiB budget")
	}
	previewName, e := asset(tmp, "preview.wasm", preview)
	if e != nil {
		return m, e
	}
	goroot, e := command(root, nil, "go", "env", "GOROOT")
	if e != nil {
		return m, e
	}
	groot := strings.TrimSpace(string(goroot))
	runtimeJS, e := os.ReadFile(filepath.Join(groot, "lib/wasm/wasm_exec.js"))
	if os.IsNotExist(e) {
		runtimeJS, e = os.ReadFile(filepath.Join(groot, "misc/wasm/wasm_exec.js"))
	}
	if e != nil {
		return m, e
	}
	names := map[string]string{}
	names["wasm_exec.js"], e = asset(tmp, "wasm_exec.js", runtimeJS)
	if e != nil {
		return m, e
	}
	for _, name := range []string{"studio.js", "studio.css", "recovery.js", "gpu-preview.js", "aego.png"} {
		b, e := os.ReadFile(filepath.Join(root, "tools/artstudio/web", name))
		if e != nil {
			return m, e
		}
		names[name], e = asset(tmp, name, b)
		if e != nil {
			return m, e
		}
	}
	for _, name := range []string{"protocol.js", "adapter.js", "surface.js", "surface.css", "docs-embed.js", "docs.css"} {
		b, e := os.ReadFile(filepath.Join(root, "tools/artstudio/release/web", name))
		if e != nil {
			return m, e
		}
		names[name], e = asset(tmp, name, b)
		if e != nil {
			return m, e
		}
	}
	examples, e := writeExamples(tmp, filepath.Join(root, "tools/artstudio/release/examples"))
	if e != nil {
		return m, e
	}
	templates, e := writeEffectTemplates(tmp, filepath.Join(root, "art/res/effects"))
	if e != nil {
		return m, e
	}
	for _, pair := range [][2]string{{"upstream/point4frames.aseprite", "point4frames.aseprite"}, {"LICENSE.txt", "LICENSE.txt"}, {"README.md", "PROVENANCE.md"}} {
		b, err := os.ReadFile(filepath.Join(root, "art/testdata/aseprite", pair[0]))
		if err != nil {
			return m, err
		}
		if err = writeNew(filepath.Join(tmp, "examples/aseprite", pair[1]), b); err != nil {
			return m, err
		}
	}
	config := map[string]any{"protocol": embedProtocolVersion, "documentVersion": art.FormatVersion, "buildID": o.ID, "commit": commit, "snapshot": !o.Release, "sourceSHA256": source, "appOrigin": o.Origin, "docsOrigins": []string{o.DocsOrigin, o.Origin}, "wasmPath": wasmName, "previewWasmPath": previewName, "examples": examples, "maxDocumentBytes": 64 << 20}
	cb := jsonBytes(config)
	configJS := append([]byte("'use strict';\nwindow.AegoStudioRelease = "), cb...)
	configJS = append(configJS, []byte(";\nwindow.AegoStudioRelease.baseURL = new URL('../', document.currentScript.src).href;\nwindow.AegoStudioRelease.wasmURL = new URL(window.AegoStudioRelease.wasmPath, window.AegoStudioRelease.baseURL).href;\nwindow.AegoStudioRelease.previewWasmURL = new URL(window.AegoStudioRelease.previewWasmPath, window.AegoStudioRelease.baseURL).href;\nObject.freeze(window.AegoStudioRelease);\n")...)
	names["config.js"], e = asset(tmp, "config.js", configJS)
	if e != nil {
		return m, e
	}
	original, e := os.ReadFile(filepath.Join(root, "tools/artstudio/web/index.html"))
	if e != nil {
		return m, e
	}
	if !bytes.Contains(original, []byte(`<script src="studio.js"></script>`)) {
		return m, fmt.Errorf("web shell changed; update release adapter before building")
	}
	for _, route := range []string{"editor", "embed"} {
		if e = writeEditorPage(tmp, route, string(original), names, o.ID); e != nil {
			return m, e
		}
	}
	for _, ex := range examples {
		if e = writeEditorPage(tmp, "examples/"+ex.ID, string(original), names, o.ID); e != nil {
			return m, e
		}
	}
	if e = writeLanding(tmp, names, examples, templates, o.ID, !o.Release); e != nil {
		return m, e
	}
	if e = writeGuides(tmp, names, examples, o); e != nil {
		return m, e
	}
	if e = writeNew(filepath.Join(tmp, "dependency-audit.txt"), deps); e != nil {
		return m, e
	}
	if e = writeNew(filepath.Join(tmp, "preview-dependency-audit.txt"), previewDeps); e != nil {
		return m, e
	}
	// Precompression is deterministic; both encoded and decoded bytes are inventoried.
	files, e := inventory(tmp)
	if e != nil {
		return m, e
	}
	for _, f := range files {
		ext := filepath.Ext(f.Path)
		if ext != ".wasm" && ext != ".js" && ext != ".css" && ext != ".html" && ext != ".json" {
			continue
		}
		b, e := os.ReadFile(filepath.Join(tmp, filepath.FromSlash(f.Path)))
		if e != nil {
			return m, e
		}
		var gz bytes.Buffer
		z, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		if _, e = z.Write(b); e != nil {
			return m, e
		}
		if e = z.Close(); e != nil {
			return m, e
		}
		if f.Path == wasmName && gz.Len() > 3<<20 {
			return m, fmt.Errorf("compressed WASM exceeds 3 MiB budget")
		}
		if f.Path == previewName && gz.Len() > 10<<20 {
			return m, fmt.Errorf("compressed lazy GPU preview WASM exceeds 10 MiB budget")
		}
		if e = writeNew(filepath.Join(tmp, filepath.FromSlash(f.Path+".gz")), gz.Bytes()); e != nil {
			return m, e
		}
	}
	files, e = inventory(tmp)
	if e != nil {
		return m, e
	}
	m = buildManifest{Schema: manifestVersion, AegoVersion: o.AegoVersion, StudioVersion: "12.5-integration", DocumentVersion: art.FormatVersion, ProtocolVersion: embedProtocolVersion, BuildID: o.ID, Commit: commit, SourceSHA256: source, BuiltAt: time.Unix(o.Epoch, 0).UTC().Format(time.RFC3339), Toolchain: runtime.Version(), Release: o.Release, AppOrigin: o.Origin, DocsOrigins: []string{o.DocsOrigin, o.Origin}, WASM: resource{wasmName, int64(len(wasm)), digest(wasm), mimeFor(wasmName)}, Examples: examples, Files: files}
	m.PreviewWASM = &resource{previewName, int64(len(preview)), digest(preview), mimeFor(previewName)}
	m.EngineSourceVersion = engineVersion
	m.Templates = templates
	if e = writeNew(filepath.Join(tmp, "manifest.json"), jsonBytes(m)); e != nil {
		return m, e
	}
	if _, _, e = verify(tmp, ""); e != nil {
		return m, e
	}
	after, e := sourceHash(root)
	if e != nil {
		return m, e
	}
	if source != after {
		return m, fmt.Errorf("sources changed during build")
	}
	if o.Release {
		b, e := command(root, nil, "git", "status", "--porcelain=v1", "--untracked-files=all")
		if e != nil || len(bytes.TrimSpace(b)) != 0 {
			return m, fmt.Errorf("tree changed during release build")
		}
	}
	if e = os.Rename(tmp, out); e != nil {
		return m, e
	}
	return m, nil
}
