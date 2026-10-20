package integration

import (
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/panels/projectsettings"
	"aego/engine/module"
	"aego/modules/physics"
	physset "aego/modules/physics/settings"
	"aego/project"
)

func settingsPanel(t *testing.T) (*edit.Session, *projectsettings.Panel) {
	t.Helper()
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	s.App.Context().ShowSettings("project")
	s.Step()
	pn, ok := s.App.Workspace().Panel(projectsettings.ID)
	if !ok {
		t.Fatal("no project settings panel")
	}
	return s, pn.(*projectsettings.Panel)
}

func TestSettingsSearchIndex(t *testing.T) {
	_, panel := settingsPanel(t)
	want := map[string]string{"Max Fixed Steps": "engine", "Music Volume": "defaults/audio", "Gravity": "physics", "Reduce Motion": "defaults/accessibility"}
	for _, r := range panel.Index() {
		if page, ok := want[r.Label]; ok && r.Page == page && r.Field != "" {
			delete(want, r.Label)
		}
	}
	if len(want) > 0 {
		t.Fatalf("not indexed: %v", want)
	}
}

func TestPhysicsSettingsApplyDuringPlay(t *testing.T) {
	s, _ := settingsPanel(t)
	pc := s.App.Play()
	if err := pc.StartProject(); err != nil {
		t.Fatal(err)
	}
	pc.Tick(1.0 / 60)
	sim := physics.SimOf(pc.World())
	if sim == nil {
		t.Fatal("no physics in the play world")
	}
	cur, _ := module.SectionValue[physset.Project](s.Host.Sections, physset.ID)
	next := *cur
	next.Gravity, next.HasGravity = mathx.Vec2{X: 1, Y: -2}, true
	next.SetMatrix([physset.MaxLayers]uint32{})
	if err := s.Host.Sections.Set(physset.ID, &next); err != nil {
		t.Fatal(err)
	}
	ctx := &module.AttachContext{World: pc.World(), Scene: pc.Scene(), Editor: true, Play: true}
	if err := s.Host.Modules.Reconfigure(ctx); err != nil {
		t.Fatal(err)
	}
	if g := sim.World().Gravity(); g != (mathx.Vec2{X: 1, Y: -2}) {
		t.Fatalf("gravity %v after a live change", g)
	}
	pc.Tick(1.0 / 60)
}
