package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/panels/projectsettings"
	"aego/engine/module"
	"aego/modules/animation"
	"aego/modules/physics"
	physset "aego/modules/physics/settings"
	"aego/platform/input"
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

func TestAnimationSettingsPageRegistered(t *testing.T) {
	t.Setenv("AEGO_LANG", "en")
	s, panel := settingsPanel(t)
	if !slices.ContainsFunc(panel.Index(), func(r projectsettings.Ref) bool {
		return r.Page == animation.ID && r.Label == "Animation Events"
	}) {
		t.Fatal("Animation Events is not available in Project Settings search")
	}
	s.App.Context().ShowSettings(animation.ID)
	if !s.Step() {
		t.Fatal("the editor quit while drawing Animation settings")
	}
}

func TestAnimationSettingsEditUndoRedoAndApply(t *testing.T) {
	t.Setenv("AEGO_LANG", "en")
	s, panel := settingsPanel(t)
	s.App.Context().ShowSettings(animation.ID)
	if !s.App.Workspace().Tree.Maximize(projectsettings.ID) || !s.Step() {
		t.Fatal("could not show the Animation settings page")
	}
	r, ok := s.App.Workspace().Tree.PanelRect(projectsettings.ID)
	if !ok || r.W < 500 {
		t.Fatalf("settings panel is not visible and wide enough: %+v", r)
	}
	th := s.App.Context().UI.Theme
	left := min(float32(180), max(float32(120), r.W*0.24))
	bodyW := max(float32(0), r.W-left-8-th.ScrollbarW-2)
	innerW := bodyW - 2*th.Padding
	x := r.X + left + 8 + th.Padding
	y := r.Y + th.Padding + th.RowHeight + th.Spacing + th.Padding
	// Three explanatory rows precede the add button.
	click(t, s, mathx.Vec2{X: x + innerW/2, Y: y + 3*(th.RowHeight+th.Spacing) + th.RowHeight/2})
	s.Step()
	if panel.Title() != "Project Settings *" || !panel.CanUndo() {
		t.Fatalf("adding an event did not create an undoable draft: %q", panel.Title())
	}
	// The separator after Add consumes two spacings; the name field is live.
	fieldX := x + mathx.Clamp(innerW*0.38, 60, 160) + 12
	fieldY := y + 4*(th.RowHeight+th.Spacing) + 2*th.Spacing + th.RowHeight/2
	click(t, s, mathx.Vec2{X: fieldX, Y: fieldY})
	push(t, s, typed("Land")...)
	s.Step()
	panel.Undo()
	if !panel.CanRedo() {
		t.Fatal("renaming an event did not produce redo history")
	}
	panel.Redo()
	// Pick float in the real payload dropdown and exercise its own history step.
	comboY := fieldY + th.RowHeight + th.Spacing
	click(t, s, mathx.Vec2{X: fieldX, Y: comboY})
	s.Step()
	floatY := comboY + th.RowHeight/2 + th.Padding + 2*(th.RowHeight+th.Spacing) + th.RowHeight/2
	click(t, s, mathx.Vec2{X: fieldX + th.Padding, Y: floatY})
	s.Step()
	panel.Undo()
	if !panel.CanRedo() {
		t.Fatal("changing the payload did not produce redo history")
	}
	panel.Redo()
	click(t, s, mathx.Vec2{X: r.X + th.Padding + 45, Y: r.Y + th.Padding + th.RowHeight/2})
	s.Step()
	got, ok := module.SectionValue[animation.Settings](s.Host.Sections, animation.ID)
	if !ok || !slices.Equal(got.Events, []animation.EventDesc{{Name: "Land", Payload: animation.PayloadFloat}}) {
		t.Fatalf("applied events = %+v, ok=%v", got, ok)
	}
	loaded, err := project.Load(s.Host.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := s.Host.Sections.Decode(animation.ID, loaded.Block(animation.ID))
	if err != nil || !slices.Equal(decoded.(*animation.Settings).Events, got.Events) {
		t.Fatalf("reopened animation settings = %+v, err=%v", decoded, err)
	}
	invalid := &animation.Settings{Events: []animation.EventDesc{{Name: "Land"}, {Name: "Land"}}}
	if _, err := s.Host.Sections.Node(animation.ID, invalid); err == nil {
		t.Fatal("duplicate event names must not be saved")
	}

	// Continue through the real controls after the first Apply. Changes remain
	// drafts until Apply, and invalid input must not overwrite the project file.
	apply := func() {
		t.Helper()
		click(t, s, mathx.Vec2{X: r.X + th.Padding + 45, Y: r.Y + th.Padding + th.RowHeight/2})
		s.Step()
	}
	name := func(value string) {
		t.Helper()
		click(t, s, mathx.Vec2{X: fieldX, Y: fieldY})
		push(t, s, key(input.KeyA, input.PrimaryMod())...)
		if value == "" {
			push(t, s, key(input.KeyBackspace, 0)...)
		} else {
			push(t, s, typed(value)...)
		}
		push(t, s, key(input.KeyEnter, 0)...)
		s.Step()
	}
	name("TouchDown")
	apply()
	file := filepath.Join(s.Host.Project.Root, "project.aego")
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = project.Load(s.Host.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = s.Host.Sections.Decode(animation.ID, loaded.Block(animation.ID))
	if err != nil || !slices.Equal(decoded.(*animation.Settings).Events, []animation.EventDesc{{Name: "TouchDown", Payload: animation.PayloadFloat}}) {
		t.Fatalf("renamed event was not saved: %+v, err=%v", decoded, err)
	}
	name("")
	apply()
	afterInvalid, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(saved, afterInvalid) || panel.Title() != "Project Settings *" {
		t.Fatalf("invalid event name changed saved project or cleared the draft: %v", err)
	}
	panel.Undo()
	s.Step()
	removeY := fieldY + 2*(th.RowHeight+th.Spacing)
	click(t, s, mathx.Vec2{X: x + innerW/2, Y: removeY})
	s.Step()
	if !panel.CanUndo() {
		t.Fatal("deleting an event did not create shared history")
	}
	panel.Undo()
	s.Step()
	if !panel.CanRedo() {
		t.Fatal("event deletion could not be redone")
	}
	panel.Redo()
	s.Step()
	apply()
	loaded, err = project.Load(s.Host.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = s.Host.Sections.Decode(animation.ID, loaded.Block(animation.ID))
	if err != nil || len(decoded.(*animation.Settings).Events) != 0 {
		t.Fatalf("event deletion was not persisted: %+v, err=%v", decoded, err)
	}
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
