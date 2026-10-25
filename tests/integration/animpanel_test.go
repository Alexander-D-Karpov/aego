package integration

import (
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/animedit"
	"aego/editor/panels/animation"
	"aego/project"
)

func TestAnimationPanelFrameAllocs(t *testing.T) {
	skipUnderRace(t)
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ctx := s.App.Context()
	d, err := ctx.Docs.Open("scenes/main.aescene")
	if err != nil {
		t.Fatal(err)
	}
	sc := d.Scene()
	s.App.Workspace().Show(animation.ID)
	measure := func(node string) float64 {
		ctx.Select(sc.ID(sc.Find(sc.Root(), node)))
		for range 60 {
			s.Step()
		}
		return testing.AllocsPerRun(30, func() { s.Step() })
	}
	empty := measure("Camera")
	timeline := measure("Player")
	if ctx.Anim.For(d).Clip() == nil {
		t.Fatal("the panel did not open a clip of the Player")
	}
	ctx.Anim.For(d).SetMode(animedit.Preview)
	preview := testing.AllocsPerRun(30, func() { s.Step() })
	ctx.Anim.For(d).SetMode(animedit.Normal)
	s.App.AnimationPanel().ShowCurves(true, -1, 0)
	for range 10 {
		s.Step()
	}
	curves := testing.AllocsPerRun(30, func() { s.Step() })
	s.App.AnimationPanel().ShowCurves(false, -1, 0)
	if timeline > empty || preview > empty || curves > empty {
		t.Fatalf("frame allocations: empty panel %v, timeline %v, preview %v, curves %v; the timeline must not allocate", empty, timeline, preview, curves)
	}
}
