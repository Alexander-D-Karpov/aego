package docstool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/engine/docs"
	"aego/project"
	"aego/script/langsvc"
)

const blink = `package demo

// Blink flashes the sprite of its node.
script Blink {
	// Times is how often it flashes.
	@export var Times int = 3

	// Flashed fires after every flash.
	signal Flashed()

	func hidden() {
	}
}
`

func TestProjectScripts(t *testing.T) {
	p, err := project.Create(filepath.Join(t.TempDir(), "game"), "game", false)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(p.ScriptsDir(), "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blink.aes"), []byte(blink), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := Registry(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := docs.ScriptsPrefix + p.Module + "/demo.Blink"
	e, ok := reg.Get(base)
	if !ok || !strings.Contains(e.Body, "flashes the sprite") {
		t.Fatalf("%s = %+v %v", base, e, ok)
	}
	for _, id := range []string{base + ".Times", base + ".Flashed"} {
		if e, ok := reg.Get(id); !ok || e.Summary == "" {
			t.Errorf("%s is missing or has no doc: %+v", id, e)
		}
	}
	if _, ok := reg.Get(base + ".hidden"); ok {
		t.Error("unexported method is documented")
	}
	if _, ok := reg.Get(docs.ProjectID); !ok {
		t.Error("no project page")
	}
}

func TestExportRoundTrip(t *testing.T) {
	reg, err := Registry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteExport(reg, "Aego", "", &buf); err != nil {
		t.Fatal(err)
	}
	var exp docs.Export
	if err := json.Unmarshal(buf.Bytes(), &exp); err != nil {
		t.Fatal(err)
	}
	if len(exp.Entries) != reg.Len() || len(exp.Nav) == 0 {
		t.Fatalf("export has %d entries and %d roots", len(exp.Entries), len(exp.Nav))
	}
	clamp := -1
	for i, e := range exp.Entries {
		if e.ID == "aego/math.Clamp" {
			clamp = i
		}
	}
	if clamp < 0 || exp.Entries[clamp].Kind != docs.Function {
		t.Fatal("aego/math.Clamp did not survive the round trip as a function")
	}
	if !strings.Contains(buf.String(), `"Kind":"function"`) {
		t.Error("kinds are not exported by name")
	}
}

func TestCheckBuiltin(t *testing.T) {
	reg, err := Registry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := Check(reg, false, &out); code != 0 {
		t.Fatalf("builtin documentation has errors:\n%s", out.String())
	}
}

const probe = `package demo

import "aego/math"

script Probe {
	var blink Coroutine

	func Ready() {
		x := math.Clamp(2.0, 0.0, 1.0)
		sr := node.Get<SpriteRenderer>()
		if sr != nil {
			sr.Tint = Color{1, 1, 1, x}
		}
		blink = start Run()
	}

	func Run() coroutine {
		yield wait(0.1)
	}
}
`

func TestF1Refs(t *testing.T) {
	p, err := project.Create(filepath.Join(t.TempDir(), "game"), "game", false)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(p.ScriptsDir(), "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "probe.aes")
	if err := os.WriteFile(path, []byte(probe), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, ws, err := build(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	file, ok := ws.FileID(path)
	if !ok {
		t.Fatal("probe.aes is not in the workspace")
	}
	snap := ws.Snapshot()
	svc := langsvc.New(ws, nil)
	for _, tc := range []struct{ at, want string }{
		{"Clamp", "aego/math.Clamp"},
		{"SpriteRenderer", "engine.SpriteRenderer"},
		{"Tint", "engine.SpriteRenderer.Tint"},
		{"yield", "keyword/yield"},
		{"wait", "keyword/yield-wait"},
		{"Coroutine", "keyword/coroutine-type"},
		{"Ready", "keyword/ready"},
	} {
		off := strings.Index(probe, tc.at)
		h, ok := svc.HoverIn(snap, file, uint32(off+1))
		if !ok || h.Ref == "" {
			t.Errorf("%s: no hover reference", tc.at)
			continue
		}
		e, ok := reg.Resolve(h.Ref)
		if !ok || e.ID != tc.want {
			t.Errorf("%s: ref %q resolves to %q, want %q", tc.at, h.Ref, e.ID, tc.want)
		}
	}
}
