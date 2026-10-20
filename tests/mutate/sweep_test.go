//go:build aego_editor

package mutate_test

import (
	"testing"
	"unsafe"

	"aego/editor/core/history/commands"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
)

func defaults(td *schema.TypeDescriptor) []commands.FieldValue {
	var p unsafe.Pointer
	switch {
	case td.Default != nil:
		p = td.Default()
	case td.New != nil:
		p = td.New()
	default:
		return nil
	}
	out := make([]commands.FieldValue, 0, len(td.Fields))
	for i := range td.Fields {
		var v schema.Value
		td.Fields[i].Get(p, &v)
		out = append(out, commands.FieldValue{Field: td.Fields[i].ID, Value: v})
	}
	return out
}

func freshNode(s *scene.Scene, td *schema.TypeDescriptor) scene.NodeHandle {
	var h scene.NodeHandle
	if td.Space.Allows(scene.Space2D.Kind()) {
		h = s.Create2D(s.Root(), "N")
	} else {
		h = s.Create3D(s.Root(), "N")
	}
	s.Flush()
	return h
}

func TestEveryComponentPassesTheEditorPaths(t *testing.T) {
	e := env(t)
	reg := e.Types.Schema()
	types, fields := 0, 0
	for _, td := range reg.Components() {
		if td.Hidden || td.FullName == "engine.Unknown" || td.ID == scriptcomp.ScriptTypeID {
			continue
		}
		types++
		list := defaults(td)
		s := scene.New(e.Types, nil, scene.Options{})
		h := freshNode(s, td)
		id := s.ID(h)
		if err := commands.NewMutation("Add", commands.EnvFor(s), commands.AddComp(s, id, td, list)).Do(target{s}); err != nil {
			t.Errorf("%s: add-comp: %v", td.FullName, err)
			continue
		}
		scratch := scene.New(e.Types, nil, scene.Options{})
		sh := freshNode(scratch, td)
		if err := commands.NewMutation("", commands.EnvFor(scratch), commands.AddRaw(scratch, scratch.ID(sh), td, list)).Do(target{scratch}); err != nil {
			t.Errorf("%s: raw add-comp: %v", td.FullName, err)
		}
		for i := range td.Fields {
			fd := &td.Fields[i]
			if !fd.Flags.Has(schema.FlagSerialize) || !fd.Flags.Has(schema.FlagEditor) || fd.Flags.Has(schema.FlagReadOnly) {
				continue
			}
			fields++
			cur, err := s.GetProperty(h, td.ID, fd.ID)
			if err != nil {
				t.Errorf("%s.%s: %v", td.FullName, fd.Name, err)
				continue
			}
			if err := commands.SetField(s, id, td, fd, cur, fd.Name).Do(target{s}); err != nil {
				t.Errorf("%s.%s: set: %v", td.FullName, fd.Name, err)
			}
		}
	}
	if types < 10 || fields < 50 {
		t.Fatalf("only %d components and %d fields were checked", types, fields)
	}
	t.Logf("%d components, %d fields", types, fields)
}
