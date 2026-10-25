package integration

import (
	"testing"

	"aego/modules/animation"
	"aego/platform/input"
	"aego/scene"
)

// Create → Animation → Clip from the palette: a new clip file, an Animation Player that plays it, one Undo for the component.
func TestCreateClipFromPalette(t *testing.T) {
	s := workspaceSession(t)
	sc, h := emptyNode(t, s)
	push(t, s, key(input.KeyN, input.PrimaryMod()|input.ModShift)...)
	push(t, s, typed("clip")...)
	if i := s.App.AddPalette().Palette().Selected(); i < 0 || s.App.AddPalette().Palette().Item(i).Label != "Clip" {
		t.Fatal("\"clip\" must highlight Animation → Clip first")
	}
	push(t, s, key(input.KeyEnter, 0)...)
	p, ok := scene.Get[animation.AnimationPlayer](sc, h)
	if !ok {
		t.Fatal("the node got no Animation Player")
	}
	rel, ok := s.Host.Pipeline.PathOf(p.Clip.ID)
	if !ok || rel != "scenes/Node.aeanim" {
		t.Fatalf("clip %v at %q", p.Clip, rel)
	}
	if err := s.App.Context().Doc().History.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, ok := scene.Get[animation.AnimationPlayer](sc, h); ok {
		t.Fatal("one Undo must remove the Animation Player")
	}
}

func TestCreateControllerFromPalette(t *testing.T) {
	s := workspaceSession(t)
	sc, h := emptyNode(t, s)
	push(t, s, key(input.KeyN, input.PrimaryMod()|input.ModShift)...)
	push(t, s, typed("controller")...)
	push(t, s, key(input.KeyEnter, 0)...)
	a, ok := scene.Get[animation.Animator](sc, h)
	if !ok || a.Controller.ID.IsZero() {
		t.Fatal("the node got no Animator with the new controller")
	}
}
