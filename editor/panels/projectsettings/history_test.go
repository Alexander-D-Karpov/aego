package projectsettings_test

import (
	"reflect"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/panels/projectsettings"
	"aego/engine/i18n"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/project"
	"aego/schema"
)

const section = "engine"

type rig struct {
	p        *projectsettings.Panel
	sec      *module.Section
	sections *module.Sections
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
	return rig{p: pn.(*projectsettings.Panel), sec: sec, sections: s.Host.Sections}
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

// Exercise the registered section codec without introducing an editor-panel
// dependency on the concrete animation module.
func TestAnimationEventDraftUndoRedoAndValidation(t *testing.T) {
	r := newRig(t)
	const sectionID = "animation"
	block := &aetext.Node{Head: sectionID, Blocks: []*aetext.Node{{Head: "event", Label: "Land", Attrs: []aetext.Field{{Name: "payload", Value: aetext.Ident("float")}}}}}
	decoded, err := r.sections.Decode(sectionID, block)
	if err != nil {
		t.Fatal(err)
	}
	r.p.Edit(sectionID, func(d any) {
		destination := reflect.ValueOf(d).Elem().FieldByName("Events")
		destination.Set(reflect.AppendSlice(destination, reflect.ValueOf(decoded).Elem().FieldByName("Events")))
	})
	r.p.Edit(sectionID, func(d any) {
		reflect.ValueOf(d).Elem().FieldByName("Events").Index(0).FieldByName("Name").SetString("TouchDown")
	})
	check := func(name string) {
		t.Helper()
		node, err := r.sections.Encode(sectionID, r.p.Draft(sectionID))
		if err != nil {
			t.Fatal(err)
		}
		if len(node.Blocks) != 1 || node.Blocks[0].Label != name {
			t.Fatalf("section history did not preserve its event list: %s", aetext.Write(node))
		}
		payload, ok := node.Blocks[0].Attr("payload")
		if !ok || payload.Ident != "float" {
			t.Fatalf("section history lost the registered payload codec: %+v", payload)
		}
	}
	r.p.Undo()
	check("Land")
	r.p.Redo()
	check("TouchDown")
	block.Blocks[0].Attrs[0].Value = aetext.Ident("not-a-payload")
	if _, err := r.sections.Decode(sectionID, block); err == nil {
		t.Fatal("section codec accepted an unknown payload")
	}
}

func TestAnimationSettingsSearchUsesBothLanguages(t *testing.T) {
	r := newRig(t)
	bundle := i18n.Default()
	before := bundle.Language()
	defer bundle.SetLanguage(before)
	for _, lang := range []string{i18n.English, i18n.Russian} {
		bundle.SetLanguage(lang)
		seen := make(map[string]bool)
		for _, item := range r.p.Index() {
			if item.Page == "animation" {
				seen[item.Label] = true
			}
		}
		for _, label := range []string{"animation", "Animation Events", "События анимации", "Payload", "Данные"} {
			if !seen[label] {
				t.Errorf("language %s: animation settings search is missing %q", lang, label)
			}
		}
		if bundle.Language() != lang {
			t.Fatalf("index changed the active UI language: %s", bundle.Language())
		}
	}
}
