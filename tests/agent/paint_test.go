package agent_test

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/app/agent"
	"aego/asset/pipeline"
)

func TestPaintReferenceAndRestrictedPaths(t *testing.T) {
	e := open(t)
	e.must("asset.pixels", agent.Args{"path": "assets/ref.png", "grid": "rb", "palette": "r=#de4010 b=#2058cf", "scale": 1})
	before := e.read("assets/ref.png")
	o := e.must("asset.paint", agent.Args{"path": "assets/copy.png", "source": "assets/ref.png", "ops": []map[string]any{{"op": "copy", "w": 2, "h": 1, "flipX": true}}})
	if len(o.Images) != 1 || !bytes.Equal(before, e.read("assets/ref.png")) {
		t.Fatal("reference was overwritten or missing PNG preview")
	}
	im, err := png.Decode(bytes.NewReader(e.read("assets/copy.png")))
	if err != nil {
		t.Fatal(err)
	}
	if color.NRGBAModel.Convert(im.At(0, 0)).(color.NRGBA) != (color.NRGBA{0x20, 0x58, 0xcf, 255}) {
		t.Fatal("reference pixels not used")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.dir, "assets", "linked")); err != nil {
		t.Skip(err)
	}
	if _, err := e.run("asset.paint", agent.Args{"path": "assets/linked/escape.png", "width": 2, "height": 2, "ops": "[]"}); err == nil {
		t.Fatal("followed symlink outside project")
	}
	if f, _ := os.ReadDir(outside); len(f) > 0 {
		t.Fatal("wrote outside project")
	}
}

func TestSourceImportFailureRestoresAllBytes(t *testing.T) {
	e := open(t)
	s, err := agent.Open(e.p, e.opt)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	src := "assets/atomic-art.png"
	if _, err := agent.Run(s, "asset.paint", agent.Args{"path": src, "width": 4, "height": 4, "ops": "[]"}); err != nil {
		t.Fatal(err)
	}
	prior := e.read(src)
	meta := e.read(src + ".meta")
	_, err = s.Pipeline.ApplySources([]pipeline.SourceEdit{{Path: src, Before: prior, After: []byte("invalid PNG")}, {Path: src + ".meta", Before: meta, After: meta}}, false)
	if err == nil || !bytes.Equal(e.read(src), prior) || !bytes.Equal(e.read(src+".meta"), meta) {
		t.Fatal("failed import did not restore source and sidecar", err)
	}
	id, ok := s.Pipeline.Lookup(src)
	if !ok || s.Pipeline.VerifyCurrent(id) != nil {
		t.Fatal("old import was not restored")
	}
}
