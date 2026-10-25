package integration

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/editor/core/animedit"
	"aego/editor/core/slice"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/scene"
)

// An 8-frame sheet is sliced, Flipbook from Selection makes the clip, and Preview shows each frame at its time.
func TestFlipbookFromSlicedSheet(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	pl := s.Host.Pipeline
	img := image.NewNRGBA(image.Rect(0, 0, 128, 16))
	for x := range 128 {
		for y := range 16 {
			img.Set(x, y, color.NRGBA{R: uint8(x * 2), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pl.Abs("assets"), "runner.png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	sheet, ok := pl.Lookup("assets/runner.png")
	if !ok {
		t.Fatal("the sheet was not scanned")
	}
	b, err := pl.Import(sheet)
	if err != nil {
		t.Fatal(err)
	}
	ctx.ApplyBatch(b)
	cells := slice.Plan("runner", nil, slice.Grid(128, 16, [2]int{16, 16}, [2]int{}, [2]int{}, 0))
	if err := ctx.Doc().History.Exec(&slice.Edit{Pipeline: pl, Sheet: sheet, After: cells, Apply: ctx.ApplyBatch}); err != nil {
		t.Fatal(err)
	}
	if id, ok := pl.Lookup("assets/runner.png#runner_007"); !ok || id != cells[7].ID {
		t.Fatal("the eighth cell does not resolve")
	}
	sc, h := emptyNode(t, s)
	*scene.Add[render2d.SpriteRenderer](sc, h) = render2d.DefaultSpriteRenderer()
	sc.Flush()
	s.App.Data().Inspect(sheet)
	push(t, s, key(input.KeyN, input.PrimaryMod()|input.ModShift)...)
	push(t, s, typed("flipbook")...)
	push(t, s, key(input.KeyEnter, 0)...)
	svc := ctx.Anim.For(ctx.Doc())
	if svc.Clip() == nil || len(svc.Clip().Tracks) != 1 || len(svc.Clip().Tracks[0].Keys) != 8 {
		t.Fatalf("the flipbook clip is wrong: %+v", svc.Clip())
	}
	svc.SetMode(animedit.Preview)
	for i, c := range cells {
		svc.SetTime((float32(i) + 0.5) / animedit.FlipbookFPS)
		got, _ := scene.Get[render2d.SpriteRenderer](sc, h)
		if got.Sprite.ID != c.ID {
			t.Fatalf("at frame %d the sprite is %v, want cell %s", i, got.Sprite.ID, c.Name)
		}
	}
	svc.SetMode(animedit.Normal)
	if err := ctx.Doc().History.Undo(); err != nil {
		t.Fatal(err)
	}
}
