package integration

import (
	"slices"
	"testing"

	"aego/app/edit"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/slice"
	"aego/editor/core/workspace"
	animpanel "aego/editor/panels/animation"
	"aego/modules/render2d"
	"aego/platform"
	"aego/platform/input"
	"aego/scene"
)

func clickTile(t *testing.T, s *edit.Session, at mathx.Vec2, mods input.Mods) {
	t.Helper()
	var evs []platform.Event
	if mods != 0 {
		evs = append(evs, platform.Event{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: input.KeyLCtrl, Mods: mods}})
	}
	evs = append(evs,
		platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: at}},
		platform.Event{Kind: platform.EvMouseDown, Mouse: input.MouseEvent{Pos: at, Button: input.MouseLeft}},
		platform.Event{Kind: platform.EvMouseUp, Mouse: input.MouseEvent{Pos: at, Button: input.MouseLeft}},
	)
	if mods != 0 {
		evs = append(evs, platform.Event{Kind: platform.EvKeyUp, Key: input.KeyEvent{Scan: input.KeyLCtrl}})
	}
	push(t, s, evs...)
}

func dragTo(t *testing.T, s *edit.Session, from, to mathx.Vec2) {
	t.Helper()
	evs := []platform.Event{
		{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: from}},
		{Kind: platform.EvMouseDown, Mouse: input.MouseEvent{Pos: from, Button: input.MouseLeft}},
	}
	for k := 1; k <= 6; k++ {
		evs = append(evs, platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: from.Lerp(to, float32(k)/6)}})
	}
	evs = append(evs, platform.Event{Kind: platform.EvMouseUp, Mouse: input.MouseEvent{Pos: to, Button: input.MouseLeft}})
	push(t, s, evs...)
	s.Step()
}

func center(r mathx.Rect) mathx.Vec2 { return mathx.Vec2{X: r.X + r.W/2, Y: r.Y + r.H/2} }

// The cells of a sliced sheet open under it in Assets, select with the mouse, make a flipbook in name order and drag onto a sprite field and a sprite track.
func TestSheetCellsInAssets(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	pl := s.Host.Pipeline
	sheet := writeSheet(t, s, "sheets/runner.png", 128, 16)
	cells := slice.Plan("runner", nil, slice.Grid(128, 16, [2]int{16, 16}, [2]int{}, [2]int{}, 0))
	if err := ctx.Doc().History.Exec(&slice.Edit{Pipeline: pl, Sheet: sheet, After: cells, Apply: ctx.ApplyBatch}); err != nil {
		t.Fatal(err)
	}
	sc, h := emptyNode(t, s)
	*scene.Add[render2d.SpriteRenderer](sc, h) = render2d.DefaultSpriteRenderer()
	sc.Flush()
	ap := s.App.AssetsPanel()
	ws := s.App.Workspace()
	ws.Show(workspace.Assets)
	ap.SetFolder("assets/sheets")
	ap.SetTileSize(48)
	for range 3 {
		s.Step()
	}
	if _, _, ok := ap.TileRect(cells[0].ID); ok {
		t.Fatal("the cells show before the sheet is opened")
	}
	_, arrow, ok := ap.TileRect(sheet)
	if !ok || arrow.W == 0 {
		t.Fatal("the sliced sheet has no expand arrow")
	}
	clickTile(t, s, center(arrow), 0)
	s.Step()
	tile := func(i int) mathx.Vec2 {
		t.Helper()
		r, _, ok := ap.TileRect(cells[i].ID)
		if !ok {
			t.Fatalf("cell %d is not shown", i)
		}
		return center(r)
	}
	for i := range cells {
		tile(i)
	}
	clickTile(t, s, tile(5), 0)
	clickTile(t, s, tile(2), input.PrimaryMod())
	clickTile(t, s, tile(7), input.PrimaryMod())
	want := []ids.AssetID{cells[2].ID, cells[5].ID, cells[7].ID}
	if got := ap.Selection(); !slices.Equal(got, want) {
		t.Fatalf("selection %v, want cells 2, 5, 7 in name order", got)
	}
	ctx.Select(sc.ID(h))
	ws.Show(workspace.Inspector)
	for range 3 {
		s.Step()
	}
	push(t, s, key(input.KeyN, input.PrimaryMod()|input.ModShift)...)
	push(t, s, typed("flipbook")...)
	push(t, s, key(input.KeyEnter, 0)...)
	svc := ctx.Anim.For(ctx.Doc())
	c := svc.Clip()
	if c == nil || len(c.Tracks) != 1 || len(c.Tracks[0].Keys) != 3 {
		t.Fatalf("the flipbook clip is wrong: %+v", c)
	}
	for k, id := range want {
		if c.Tracks[0].Keys[k].V.A != id {
			t.Fatalf("frame %d is %v, want %v", k, c.Tracks[0].Keys[k].V.A, id)
		}
	}
	ctx.Select(sc.ID(h))
	for range 3 {
		s.Step()
	}
	field, ok := s.App.Inspector().AssetFieldRect("sprite")
	if !ok {
		t.Fatal("the inspector shows no sprite field")
	}
	dragTo(t, s, tile(3), center(field))
	if got, _ := scene.Get[render2d.SpriteRenderer](sc, h); got.Sprite.ID != cells[3].ID {
		t.Fatalf("the sprite field got %v, want cell 3", got.Sprite.ID)
	}
	ws.Show(animpanel.ID)
	for range 3 {
		s.Step()
	}
	track, ok := s.App.AnimationPanel().TrackRect(0)
	if !ok {
		t.Fatal("the sprite track is not shown")
	}
	before := len(svc.Clip().Tracks[0].Keys)
	dragTo(t, s, tile(1), mathx.Vec2{X: track.X + track.W - 4, Y: track.Y + track.H/2})
	keys := svc.Clip().Tracks[0].Keys
	if len(keys) != before+1 || keys[len(keys)-1].V.A != cells[1].ID {
		t.Fatalf("dropping cell 1 on the track left %d keys, last %v", len(keys), keys[len(keys)-1].V.A)
	}
}
