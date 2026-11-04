package integration

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
)

func TestAgentDrawingIsOneReversibleAssetEdit(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	dir := "assets/pixel-test/actor"
	abs := func(ext string) string { return filepath.Join(e.root, dir+ext) }
	draw := func(tool string, args map[string]any, when func()) backend.Event {
		t.Helper()
		r := results(e.turn(t, []backend.Step{{Tool: tool, Args: args}}, when))
		if len(r) != 1 {
			t.Fatalf("%s: %v", tool, r)
		}
		return r[0]
	}
	turn := draw("asset_pixels", map[string]any{"path": dir + ".png", "grid": "one:\nab\ntwo:\nba", "palette": "a=#123456 b=#876543", "scale": 1}, e.approveAll)
	if turn.Failed {
		t.Fatal(turn.Result)
	}
	before := map[string][]byte{}
	for _, ext := range []string{".png", ".png.meta", ".aeanim", ".aeanim.meta"} {
		b, err := os.ReadFile(abs(ext))
		if err != nil {
			t.Fatal(err)
		}
		before[ext] = b
	}
	prop := e.b.Proposals()[0]
	if !e.b.CanUndo(prop.ID) {
		t.Fatal("drawing has no undo")
	}
	if err := e.b.Undo(prop.ID); err != nil {
		t.Fatal(err)
	}
	for ext := range before {
		if _, err := os.Stat(abs(ext)); !os.IsNotExist(err) {
			t.Fatalf("undo left %s: %v", ext, err)
		}
	}
	if err := e.doc.History.Redo(); err != nil {
		t.Fatal(err)
	}
	for ext, prior := range before {
		b, err := os.ReadFile(abs(ext))
		if err != nil || !bytes.Equal(prior, b) {
			t.Fatalf("redo changed %s: %v", ext, err)
		}
	}
	ops := map[string]any{"path": dir + ".png", "replace": true, "ops": []map[string]any{{"op": "pixel", "x": 1, "y": 0, "color": "#ff0000"}}}
	e.b.SetAutoApply(true)
	if r := draw("asset_paint", ops, nil); r.Failed {
		t.Fatal(r.Result)
	}
	f, err := os.Open(abs(".png"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	red, green, blue, _ := img.At(1, 0).RGBA()
	if red != 65535 || green != 0 || blue != 0 {
		t.Fatal("source did not change")
	}
	latest := e.b.Proposals()[len(e.b.Proposals())-1]
	if err := e.b.Undo(latest.ID); err != nil {
		t.Fatal(err)
	}
	for ext, prior := range before {
		b, _ := os.ReadFile(abs(ext))
		if !bytes.Equal(prior, b) {
			t.Fatal("paint undo did not restore " + ext)
		}
	}
	e.b.SetAutoApply(false)
	external := []byte("changed on disk while proposal was open")
	r := draw("asset_paint", ops, func() {
		if len(e.b.Pending()) == 0 {
			return
		}
		if err := os.WriteFile(abs(".png"), external, 0644); err != nil {
			t.Fatal(err)
		}
		e.approveAll()
	})
	if !r.Failed {
		t.Fatal("stale proposal overwrote the source")
	}
	b, _ := os.ReadFile(abs(".png"))
	if !bytes.Equal(b, external) {
		t.Fatal("conflict was overwritten")
	}
}
