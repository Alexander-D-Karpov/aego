package integration

import (
	"testing"

	"aego/app/edit"
	"aego/editor/core/command"
	"aego/modules/physics/components"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/scene"
)

func push(t *testing.T, s *edit.Session, evs ...platform.Event) {
	t.Helper()
	plat, ok := s.Host.Engine.Platform().(*headless.Platform)
	if !ok {
		t.Skip("needs the headless platform")
	}
	for _, ev := range evs {
		plat.Push(ev)
		s.Step()
	}
}

func key(k input.Scancode, mods input.Mods) []platform.Event {
	return []platform.Event{
		{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: k, Mods: mods}},
		{Kind: platform.EvKeyUp, Key: input.KeyEvent{Scan: k}},
	}
}

func typed(text string) []platform.Event {
	var out []platform.Event
	for _, r := range text {
		ev := platform.Event{Kind: platform.EvTextInput}
		ev.TextN = uint8(copy(ev.Text[:], string(r)))
		out = append(out, ev)
	}
	return out
}

func emptyNode(t *testing.T, s *edit.Session) (*scene.Scene, scene.NodeHandle) {
	t.Helper()
	ctx := s.App.Context()
	if err := ctx.Commands.Run(command.SceneCreateNode); err != nil {
		t.Fatal(err)
	}
	s.Step()
	sel := ctx.Selected()
	if len(sel) != 1 {
		t.Fatalf("Create Empty selected %d nodes", len(sel))
	}
	sc := ctx.Scene()
	return sc, sc.Handle(sel[0])
}

// Ctrl+Shift+A, "mot", Enter adds Motion 2D with a kinematic Rigid Body 2D, and one Undo takes both back.
func TestPaletteAddsWithoutMouse(t *testing.T) {
	s := workspaceSession(t)
	sc, h := emptyNode(t, s)
	push(t, s, key(input.KeyA, input.PrimaryMod()|input.ModShift)...)
	pal := s.App.AddPalette()
	if !pal.Active() {
		t.Fatal("Ctrl+Shift+A did not open the palette")
	}
	push(t, s, typed("mot")...)
	if got := pal.Palette().Query(); got != "mot" {
		t.Fatalf("the search field got %q; it must take the keys at once", got)
	}
	push(t, s, key(input.KeyEnter, 0)...)
	if pal.Active() {
		t.Fatal("Enter must close the palette")
	}
	reg := sc.Registry()
	motion, _ := reg.ByName("gameplay.Motion2D")
	body, _ := reg.ByName(components.NameBody)
	if _, ok := sc.GetByType(h, motion.ID); !ok {
		t.Fatal("no Motion 2D after Enter")
	}
	b, ok := scene.Get[components.Body2D](sc, h)
	if !ok {
		t.Fatal("Motion 2D came without Rigid Body 2D")
	}
	if b.Type != components.Kinematic {
		t.Fatalf("body type %v, want kinematic", b.Type)
	}
	if err := s.App.Context().Doc().History.Undo(); err != nil {
		t.Fatal(err)
	}
	_, hasM := sc.GetByType(h, motion.ID)
	_, hasB := sc.GetByType(h, body.ID)
	if hasM || hasB {
		t.Fatal("one Undo must take back Motion 2D and Rigid Body 2D")
	}
}

// Shift+Enter adds and keeps the palette open with the search field focused; Esc closes it.
func TestPaletteShiftEnterKeepsOpen(t *testing.T) {
	s := workspaceSession(t)
	sc, h := emptyNode(t, s)
	push(t, s, key(input.KeyA, input.PrimaryMod()|input.ModShift)...)
	push(t, s, typed("health")...)
	push(t, s, key(input.KeyEnter, input.ModShift)...)
	pal := s.App.AddPalette()
	if !pal.Active() {
		t.Fatal("Shift+Enter must keep the palette open")
	}
	health, _ := sc.Registry().ByName("gameplay.Health")
	if _, ok := sc.GetByType(h, health.ID); !ok {
		t.Fatal("Shift+Enter did not add Health")
	}
	push(t, s, typed("x")...)
	if got := pal.Palette().Query(); got != "healthx" {
		t.Fatalf("after Shift+Enter the field got %q", got)
	}
	push(t, s, key(input.KeyEscape, 0)...)
	if pal.Active() {
		t.Fatal("Esc must close the palette")
	}
}

// Ctrl+Shift+N opens Create; Enter on the search result creates the node.
func TestPaletteCreates(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	before := len(ctx.Scene().PreOrder())
	push(t, s, key(input.KeyN, input.PrimaryMod()|input.ModShift)...)
	if !s.App.AddPalette().Active() {
		t.Fatal("Ctrl+Shift+N did not open Create")
	}
	push(t, s, typed("empty")...)
	push(t, s, key(input.KeyEnter, 0)...)
	if got := len(ctx.Scene().PreOrder()); got != before+1 {
		t.Fatalf("nodes %d → %d, want one more", before, got)
	}
}

// The open palette without typing allocates nothing more per frame than the editor without it.
func TestPaletteFrameAllocs(t *testing.T) {
	skipUnderRace(t)
	s := workspaceSession(t)
	emptyNode(t, s)
	for range 30 {
		s.Step()
	}
	closed := testing.AllocsPerRun(30, func() { s.Step() })
	push(t, s, key(input.KeyA, input.PrimaryMod()|input.ModShift)...)
	for range 30 {
		s.Step()
	}
	open := testing.AllocsPerRun(30, func() { s.Step() })
	push(t, s, typed("box")...)
	for range 10 {
		s.Step()
	}
	query := testing.AllocsPerRun(30, func() { s.Step() })
	if open > closed || query > closed {
		t.Fatalf("frame allocations: closed %v, open %v, with a query %v", closed, open, query)
	}
}
