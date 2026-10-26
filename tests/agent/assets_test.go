package agent_test

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/agent"
	"aego/asset/pipeline"
	"aego/project"
)

const hero = "idle:\n.rr.\nrssr\n.bb.\nrun1:\nr..r\nrssr\nb..b\n"

func TestPixelsMakeAnAtlasAndFlipbook(t *testing.T) {
	e := open(t)
	out := e.must("asset.pixels", agent.Args{"path": "assets/sprites/hero.png", "grid": hero, "scale": 2,
		"palette": `{"r": "#d0312d", "s": "#f4c7a1", "b": "#2b4fd8"}`})
	if len(out.Images) != 1 || len(out.Files) != 2 || out.Files[1] != "assets/sprites/hero.aeanim" {
		t.Fatalf("images %d files %v", len(out.Images), out.Files)
	}
	img, err := png.Decode(bytes.NewReader(e.read("assets/sprites/hero.png")))
	if err != nil || img.Bounds().Dx() != 16 || img.Bounds().Dy() != 6 {
		t.Fatalf("atlas %v %v", img.Bounds(), err)
	}
	meta, err := pipeline.ParseMeta(e.read("assets/sprites/hero.png.meta"), "assets/sprites/hero.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Subs) != 2 || meta.Subs[0].Name != "hero_idle" || meta.Subs[1].Rect != [4]int{8, 0, 8, 6} {
		t.Fatalf("cells %+v", meta.Subs)
	}
	clip := string(e.read("assets/sprites/hero.aeanim"))
	if !strings.Contains(clip, meta.Subs[0].ID.String()) || !strings.Contains(clip, meta.Subs[1].ID.String()) {
		t.Fatalf("the flipbook does not show the frames:\n%s", clip)
	}

	if _, err := e.run("asset.pixels", agent.Args{"path": "assets/sprites/hero.png", "grid": "rr", "palette": "r=#f00"}); err == nil || !strings.Contains(err.Error(), "replace") {
		t.Fatalf("an existing sprite was drawn over without replace: %v", err)
	}
	e.must("asset.pixels", agent.Args{"path": "assets/sprites/hero.png", "grid": strings.Replace(hero, ".bb.", "bbbb", 1), "scale": 2, "replace": true,
		"palette": `{"r": "#d0312d", "s": "#f4c7a1", "b": "#2b4fd8"}`})
	again, _ := pipeline.ParseMeta(e.read("assets/sprites/hero.png.meta"), "assets/sprites/hero.png.meta")
	if again.ID != meta.ID || again.Subs[0].ID != meta.Subs[0].ID {
		t.Fatal("drawing over a sprite changed its ids")
	}
}

func TestPixelsUseTheArtSection(t *testing.T) {
	e := open(t)
	text := string(e.read("project.aego"))
	text = strings.TrimRight(text, "\n")
	text = text[:strings.LastIndex(text, "}")] + "  art {\n    palette=[\"#000000\", \"#ffffff\"]\n    pixelsPerUnit=16\n  }\n}\n"
	e.write("project.aego", []byte(text))
	p, err := project.Load(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	e.p = p
	out := e.must("asset.pixels", agent.Args{"path": "assets/coin.png", "grid": "01\n10"})
	img, err := png.Decode(bytes.NewReader(e.read("assets/coin.png")))
	if err != nil || img.Bounds().Dx() != 8 {
		t.Fatalf("pixelsPerUnit 16 at the default 64 scales by 4: %v %v", img.Bounds(), err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("a single frame makes a flipbook: %v", out.Files)
	}
	out = e.must("asset.pixels", agent.Args{"path": "assets/off.png", "grid": "r", "palette": "r=#ff0000"})
	if len(out.Notes) == 0 || !strings.Contains(out.Notes[0], "outside the art palette") {
		t.Fatalf("notes %v", out.Notes)
	}
	if _, err := e.run("asset.pixels", agent.Args{"path": "assets/x.png", "grid": "2"}); err == nil {
		t.Fatal("a key past the palette was drawn")
	}
}

func TestSVGDrawsATexture(t *testing.T) {
	e := open(t)
	out := e.must("asset.svg", agent.Args{"path": "assets/ui/gem.png", "width": 32,
		"svg": `<svg viewBox="0 0 16 16"><path d="M8 1 L15 8 L8 15 L1 8 Z" fill="#3cf" stroke="#036" stroke-width="1"/><filter/></svg>`})
	if len(out.Images) != 1 || len(out.Notes) != 1 {
		t.Fatalf("images %d notes %v", len(out.Images), out.Notes)
	}
	img, err := png.Decode(bytes.NewReader(e.read("assets/ui/gem.png")))
	if err != nil || img.Bounds().Dx() != 32 {
		t.Fatal(err)
	}
	if _, _, _, a := img.At(16, 16).RGBA(); a == 0 {
		t.Fatal("the gem is not drawn")
	}
	meta, _ := pipeline.ParseMeta(e.read("assets/ui/gem.png.meta"), "assets/ui/gem.png.meta")
	if v, ok := meta.Settings["filter"]; !ok || v.Ident != "linear" {
		t.Fatalf("an SVG texture is not filtered linearly: %v", meta.Settings)
	}
	dry := e.must("asset.svg", agent.Args{"path": "assets/ui/dry.png", "svg": `<svg><rect width="4" height="4"/></svg>`, "dry-run": true})
	if !dry.DryRun || len(dry.Images) != 1 {
		t.Fatal("a dry run has no preview")
	}
	if _, err := os.Stat(filepath.Join(e.dir, "assets", "ui", "dry.png")); err == nil {
		t.Fatal("a dry run wrote the file")
	}
}
