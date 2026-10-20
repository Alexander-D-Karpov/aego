package projectsettings_test

import (
	"reflect"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/panels/projectsettings"
	"aego/engine/module"
	"aego/project"
	"aego/schema"
)

const section = "engine"

type rig struct {
	p   *projectsettings.Panel
	sec *module.Section
}

func newRig(t *testing.T) rig {
	t.Helper()
	p, err := project.Create(t.TempDir(), "SettingsUndo", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	pn, ok := s.App.Workspace().Panel(projectsettings.ID)
	if !ok {
		t.Fatal("no project settings panel")
	}
	sec, ok := s.Host.Sections.Desc(section)
	if !ok || sec.Type == nil {
		t.Fatal("no engine section")
	}
	return rig{p: pn.(*projectsettings.Panel), sec: sec}
}

func (r rig) field() *schema.FieldDescriptor {
	for i := range r.sec.Type.Fields {
		if r.sec.Type.Fields[i].Name == "fixedHz" {
			return &r.sec.Type.Fields[i]
		}
	}
	panic("no fixedHz field")
}

func (r rig) hz() float64 {
	var v schema.Value
	r.field().Get(reflect.ValueOf(r.p.Draft(section)).UnsafePointer(), &v)
	return v.F
}

func (r rig) setHz(hz float64) {
	r.p.Edit(section, func(d any) { r.field().Set(reflect.ValueOf(d).UnsafePointer(), schema.Float64(hz)) })
}

func TestUnsavedOnlyWhenValuesDiffer(t *testing.T) {
	r := newRig(t)
	r.setHz(30)
	if r.p.Title() != "Project Settings *" {
		t.Fatal("a real change is not marked unsaved")
	}
	r.setHz(0)
	if r.p.Title() != "Project Settings" {
		t.Fatal("changing a value back still shows unsaved changes")
	}
}

func TestUndoRedoSettings(t *testing.T) {
	r := newRig(t)
	r.setHz(30)
	r.setHz(120)
	if !r.p.CanUndo() {
		t.Fatal("nothing to undo")
	}
	r.p.Undo()
	if r.hz() != 30 {
		t.Fatalf("undo gave %v, want 30", r.hz())
	}
	r.p.Undo()
	if r.hz() != 0 || r.p.Title() != "Project Settings" {
		t.Fatalf("second undo gave %v, title %q", r.hz(), r.p.Title())
	}
	r.p.Redo()
	if r.hz() != 30 {
		t.Fatalf("redo gave %v, want 30", r.hz())
	}
	r.setHz(60)
	if r.p.CanRedo() {
		t.Fatal("a new change must clear redo")
	}
}
