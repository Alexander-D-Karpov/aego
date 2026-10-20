//go:build aego_editor

package mutate_test

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"

	"aego/core/ids"
	"aego/editor/core/clipboard"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
)

const steps = 50

type target struct {
	s *scene.Scene
}

func (t target) Scene() *scene.Scene {
	return t.s
}

func idOf(s *scene.Scene, h scene.NodeHandle) ids.NodeID {
	if h == s.Root() {
		return ids.NodeID{}
	}
	return s.ID(h)
}

func below(s *scene.Scene) []scene.NodeHandle {
	var out []scene.NodeHandle
	for _, h := range s.PreOrder() {
		if h != s.Root() {
			out = append(out, h)
		}
	}
	return out
}

func inside(s *scene.Scene, h, anc scene.NodeHandle) bool {
	for c := h; !c.IsZero(); c = s.Parent(c) {
		if c == anc {
			return true
		}
		if c == s.Root() {
			return false
		}
	}
	return false
}

type fieldRef struct {
	td *schema.TypeDescriptor
	fd *schema.FieldDescriptor
}

func floats(s *scene.Scene, h scene.NodeHandle) []fieldRef {
	var out []fieldRef
	s.ComponentsOf(h, func(t schema.TypeID) {
		td, ok := s.Registry().ByID(t)
		if !ok {
			return
		}
		for i := range td.Fields {
			fd := &td.Fields[i]
			if fd.Kind == schema.KindFloat32 && fd.Flags.Has(schema.FlagSerialize) && fd.Flags.Has(schema.FlagEditor) && !fd.Flags.Has(schema.FlagReadOnly) {
				out = append(out, fieldRef{td, fd})
			}
		}
	})
	return out
}

func comps(s *scene.Scene, h scene.NodeHandle) []*schema.TypeDescriptor {
	var out []*schema.TypeDescriptor
	s.ComponentsOf(h, func(t schema.TypeID) {
		if td, ok := s.Registry().ByID(t); ok && t != scriptcomp.ScriptTypeID {
			out = append(out, td)
		}
	})
	return out
}

func pick[T any](r *rand.Rand, list []T) (T, bool) {
	var zero T
	if len(list) == 0 {
		return zero, false
	}
	return list[r.IntN(len(list))], true
}

func commandPair(t *testing.T, r *rand.Rand, a, b *scene.Scene) (history.Command, history.Command, string) {
	t.Helper()
	nodes := below(a)
	h, ok := pick(r, nodes)
	if !ok {
		return nil, nil, ""
	}
	id := a.ID(h)
	env := commands.EnvFor(b)
	switch r.IntN(7) {
	case 0:
		f, ok := pick(r, floats(a, h))
		if !ok {
			return nil, nil, ""
		}
		lo, hi := 0.0, 10.0
		if f.fd.HasMin {
			lo = f.fd.Min
		}
		if f.fd.HasMax {
			hi = f.fd.Max
		}
		hi = min(hi, lo+1000)
		v := schema.Float32(float32(lo + r.Float64()*(hi-lo)))
		before, err := a.GetProperty(h, f.td.ID, f.fd.ID)
		if err != nil {
			t.Fatal(err)
		}
		return &legacySetProperty{Node: id, Type: f.td.ID, Field: f.fd.ID, Before: before, After: v, Name: f.fd.Name},
			commands.SetField(b, id, f.td, f.fd, v, f.fd.Name), "set " + f.td.FullName + "." + f.fd.Name
	case 1:
		name := fmt.Sprintf("N%d", r.IntN(1000))
		return &legacyRename{Node: id, Before: a.Name(h), After: name},
			commands.NewMutation("Rename", env, mutate.Rename(commands.Ref(id), name)), "rename"
	case 2:
		frag, err := clipboard.Remap(sceneio.SaveSubtree(a, h, false))
		if err != nil {
			t.Fatal(err)
		}
		p, _ := pick(r, append(nodes, a.Root()))
		return &legacyCreate{Parent: idOf(a, p), Index: -1, Fragment: frag},
			commands.NewMutation("Create", env, mutate.Insert(commands.Ref(idOf(a, p)), string(frag))), "create"
	case 3:
		return &legacyDelete{Node: id}, commands.NewMutation("Delete", env, mutate.RemoveNode(commands.Ref(id))), "delete"
	case 4:
		var parents []scene.NodeHandle
		for _, p := range append(nodes, a.Root()) {
			if !inside(a, p, h) {
				parents = append(parents, p)
			}
		}
		p, ok := pick(r, parents)
		if !ok {
			return nil, nil, ""
		}
		keep := r.IntN(2) == 0
		op := mutate.Move(commands.Ref(id), commands.Ref(idOf(a, p)))
		if keep {
			op = op.KeepingWorld()
		}
		return &legacyReparent{Node: id, NewParent: idOf(a, p), NewIndex: -1, KeepWorld: keep},
			commands.NewMutation("Reparent", env, op), "reparent"
	case 5:
		td, ok := a.Registry().ByName("engine.Tags")
		if !ok {
			return nil, nil, ""
		}
		if _, has := a.GetByType(h, td.ID); has {
			return nil, nil, ""
		}
		return &legacyAddComponent{Node: id, Type: td.ID, Name: td.FullName},
			commands.NewMutation("Add", env, mutate.AddComp(commands.Ref(id), td.FullName)), "add tags"
	default:
		td, ok := pick(r, comps(a, h))
		if !ok {
			return nil, nil, ""
		}
		return &legacyRemoveComponent{Node: id, Type: td.ID, Name: td.FullName},
			commands.NewMutation("Remove", env, mutate.RemoveComp(commands.Ref(id), td.FullName)), "remove " + td.FullName
	}
}

func TestHistoryAdapterMatchesCommands(t *testing.T) {
	e := env(t)
	src, err := os.ReadFile(sceneFile)
	if err != nil {
		t.Fatal(err)
	}
	for seed := uint64(1); seed <= 5; seed++ {
		r := rand.New(rand.NewPCG(seed, 96))
		a, _, err := sceneio.Load(src, "a", e.Types, nil, sceneio.LoadOptions{KeepIDs: true})
		if err != nil {
			t.Fatal(err)
		}
		b, _, err := sceneio.Load(src, "b", e.Types, nil, sceneio.LoadOptions{KeepIDs: true})
		if err != nil {
			t.Fatal(err)
		}
		ha, hb := history.New(target{a}), history.New(target{b})
		origin := sceneio.Save(a)
		done := 0
		for i := 0; done < steps && i < steps*10; i++ {
			old, mut, what := commandPair(t, r, a, b)
			if old == nil {
				continue
			}
			ea, eb := ha.Exec(old), hb.Exec(mut)
			if (ea == nil) != (eb == nil) {
				t.Fatalf("seed %d step %d %s: commands %v, mutate %v", seed, done, what, ea, eb)
			}
			if ea != nil {
				continue
			}
			done++
			if sa, sb := sceneio.Save(a), sceneio.Save(b); !bytes.Equal(sa, sb) {
				t.Fatalf("seed %d step %d %s: scenes differ\ncommands:\n%s\nmutate:\n%s", seed, done, what, sa, sb)
			}
		}
		final := sceneio.Save(a)
		if err := ha.Seek(0); err != nil {
			t.Fatal(err)
		}
		if err := hb.Seek(0); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(sceneio.Save(b), origin) || !bytes.Equal(sceneio.Save(a), origin) {
			t.Fatalf("seed %d: undo to the start differs\norigin:\n%s\ngot b:\n%s", seed, origin, sceneio.Save(b))
		}
		if err := hb.Seek(done); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(sceneio.Save(b), final) {
			t.Fatalf("seed %d: redo to the end differs", seed)
		}
	}
}
