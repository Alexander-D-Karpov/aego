package docstool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStudioWebPackageVersionsEveryBridgeResource(t *testing.T) {
	sharedWeb := filepath.Join("..", "..", "tools", "artstudio", "web")
	web := t.TempDir()
	for _, name := range []string{"index.html", "studio.css", "studio.js", "recovery.js", "gpu-preview.js", "aego.png"} {
		data, err := os.ReadFile(filepath.Join(sharedWeb, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(web, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	wasm := filepath.Join(dir, "artstudio.wasm")
	if err := os.WriteFile(wasm, []byte("wasm-build-one"), 0o644); err != nil {
		t.Fatal(err)
	}
	preview := filepath.Join(dir, "preview.wasm")
	if err := os.WriteFile(preview, []byte("actual-preview-runtime-build-one"), 0o644); err != nil {
		t.Fatal(err)
	}
	support := []byte("versioned go wasm support")
	check := func() string {
		t.Helper()
		if err := packageStudioWeb(web, dir, support); err != nil {
			t.Fatal(err)
		}
		index, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		js, err := os.ReadFile(filepath.Join(dir, "studio.js"))
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`studio\.css\?v=([a-f0-9]{10})`).FindSubmatch(index)
		if len(match) != 2 {
			t.Fatalf("no versioned stylesheet: %s", index)
		}
		version := string(match[1])
		for _, item := range []string{"wasm_exec.js", "recovery.js", "gpu-preview.js", "studio.js"} {
			if !strings.Contains(string(index), item+"?v="+version) {
				t.Fatalf("%s is not bound to %s", item, version)
			}
		}
		if strings.Count(string(index), "aego.png?v="+version) != 2 {
			t.Fatal("the favicon and brand mark do not use the versioned Aego icon")
		}
		if !strings.Contains(string(js), "fetch(window.AegoStudioRelease?.wasmURL || 'artstudio.wasm?v="+version+"')") {
			t.Fatal("WASM fetch lost the release override or the versioned local fallback")
		}
		gpuJS, err := os.ReadFile(filepath.Join(dir, "gpu-preview.js"))
		if err != nil || !strings.Contains(string(gpuJS), "fetch(window.AegoStudioRelease?.previewWasmURL || 'preview.wasm?v="+version+"')") {
			t.Fatal("GPU WASM fetch lost the release override or versioned fallback")
		}
		// The fallback must use the same editor and preview adapter, with only
		// its exact asset URLs versioned. It is not a reduced docs canvas.
		for _, name := range []string{"index.html", "studio.css", "studio.js", "recovery.js", "gpu-preview.js", "aego.png"} {
			source, err := os.ReadFile(filepath.Join(web, name))
			if err != nil {
				t.Fatal(err)
			}
			packaged, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil || strings.ReplaceAll(string(packaged), "?v="+version, "") != string(source) {
				t.Fatalf("local docs used a divergent %s", name)
			}
		}
		actual, err := os.ReadFile(filepath.Join(dir, "wasm_exec.js"))
		if err != nil || string(actual) != string(support) {
			t.Fatal("WASM support changed")
		}
		return version
	}
	first := check()
	if second := check(); first != second {
		t.Fatalf("same inputs are not deterministic: %s != %s", first, second)
	}
	if err := os.WriteFile(wasm, []byte("wasm-build-two"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := check()
	if second == first {
		t.Fatal("changed WASM reused the old bridge version")
	}
	if err := os.WriteFile(preview, []byte("actual-preview-runtime-build-two"), 0o644); err != nil {
		t.Fatal(err)
	}
	third := check()
	if third == second {
		t.Fatal("changed GPU WASM reused the old bridge version")
	}
	gpu, err := os.ReadFile(filepath.Join(web, "gpu-preview.js"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "gpu-preview.js"), append(gpu, []byte("\n// adapter revision\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if fourth := check(); fourth == third {
		t.Fatal("changed GPU adapter reused the old package version")
	}
}

func TestStudioWebPackageRejectsUnknownEntryPoints(t *testing.T) {
	web, dir := t.TempDir(), t.TempDir()
	for name, contents := range map[string]string{"index.html": "unexpected index", "studio.css": "css", "studio.js": "fetch('artstudio.wasm')", "recovery.js": "js", "gpu-preview.js": "gpu", "aego.png": "icon"} {
		if err := os.WriteFile(filepath.Join(web, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "artstudio.wasm"), []byte("wasm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "preview.wasm"), []byte("gpu-wasm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := packageStudioWeb(web, dir, []byte("support")); err == nil || !strings.Contains(err.Error(), "studio index") {
		t.Fatal("unknown index was silently packaged")
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("invalid index was published: %v", err)
	}
}

func TestStudioWebPackageRejectsAmbiguousWASMFetch(t *testing.T) {
	web, dir := t.TempDir(), t.TempDir()
	index := `<link href="studio.css"><link href="aego.png"><img src="aego.png"><script src="wasm_exec.js"></script><script src="recovery.js"></script><script src="gpu-preview.js"></script><script src="studio.js"></script>`
	fetch := `fetch(window.AegoStudioRelease?.wasmURL || 'artstudio.wasm')`
	for name, contents := range map[string]string{"index.html": index, "studio.css": "css", "studio.js": fetch + ";" + fetch, "recovery.js": "js", "gpu-preview.js": `fetch(window.AegoStudioRelease?.previewWasmURL || 'preview.wasm')`, "aego.png": "icon"} {
		if err := os.WriteFile(filepath.Join(web, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "artstudio.wasm"), []byte("wasm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "preview.wasm"), []byte("gpu-wasm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := packageStudioWeb(web, dir, []byte("support")); err == nil || !strings.Contains(err.Error(), "one WASM fetch") {
		t.Fatal("ambiguous WASM fetch was silently packaged")
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("ambiguous page was published: %v", err)
	}
}

func TestStudioWebPackageRejectsMissingOrAmbiguousGPU(t *testing.T) {
	for _, scenario := range []string{"missing-wasm", "missing-adapter", "ambiguous-fetch"} {
		t.Run(scenario, func(t *testing.T) {
			web, dir := t.TempDir(), t.TempDir()
			for _, name := range []string{"index.html", "studio.css", "studio.js", "recovery.js", "gpu-preview.js", "aego.png"} {
				data, err := os.ReadFile(filepath.Join("..", "..", "tools", "artstudio", "web", name))
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "missing-adapter" && name == "gpu-preview.js" {
					continue
				}
				if scenario == "ambiguous-fetch" && name == "gpu-preview.js" {
					data = append(data, []byte(`;fetch(window.AegoStudioRelease?.previewWasmURL || 'preview.wasm')`)...)
				}
				if err := os.WriteFile(filepath.Join(web, name), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "artstudio.wasm"), []byte("core"), 0o644); err != nil {
				t.Fatal(err)
			}
			if scenario != "missing-wasm" {
				if err := os.WriteFile(filepath.Join(dir, "preview.wasm"), []byte("gpu"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := packageStudioWeb(web, dir, []byte("support")); err == nil {
				t.Fatal("broken GPU preview was silently packaged")
			}
			if _, err := os.Stat(filepath.Join(dir, "index.html")); !os.IsNotExist(err) {
				t.Fatal("incomplete entry page was published")
			}
		})
	}
}
