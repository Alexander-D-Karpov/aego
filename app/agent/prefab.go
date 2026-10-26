package agent

import (
	"errors"
	"path"

	"aego/project/refactor"
	"aego/scene/mutate"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

func prefabCommands() []*Command {
	owner := Arg{Name: "owner", Type: "string", Doc: "component or script owner of the override, for property and component levels"}
	field := Arg{Name: "field", Type: "string", Doc: "field name, for the property level"}
	level := Arg{Name: "level", Type: "string", Doc: "property, component, node or all (default); orphan for revert"}
	return []*Command{
		{Name: "prefab.create", Doc: "turn a node into a prefab file and link the node to it", Write: true,
			Args: []Arg{argFile, argNode, {Name: "out", Type: "string", Doc: "new prefab file, prefabs/<name>.aeprefab", Required: true, Pos: true}}, Run: (*Session).prefabCreate},
		{Name: "prefab.instantiate", Doc: "add an instance of a prefab", Write: true,
			Args: []Arg{argFile, {Name: "prefab", Type: "string", Doc: "prefab id or path", Required: true, Pos: true},
				{Name: "parent", Type: "string", Doc: "parent node, the root by default"}, {Name: "name", Type: "string", Doc: "name of the instance"}, argIndex}, Run: (*Session).prefabInstantiate},
		{Name: "prefab.apply", Doc: "write instance overrides into the prefab file", Write: true,
			Args: []Arg{argFile, argNode, level, owner, field, {Name: "nearest", Type: "boolean", Doc: "apply to the nearest nested prefab"}}, Run: (*Session).prefabApply},
		{Name: "prefab.revert", Doc: "drop instance overrides", Write: true, Destroys: true, Args: []Arg{argFile, argNode, level, owner, field}, Run: (*Session).prefabRevert},
		{Name: "prefab.unpack", Doc: "turn an instance into plain nodes", Write: true, Destroys: true,
			Args: []Arg{argFile, argNode, {Name: "deep", Type: "boolean", Doc: "unpack nested prefabs too"}}, Run: func(s *Session, a Args) (*Output, error) {
				return s.sceneTx(a, "Unpack Prefab", mutate.Unpack(a.Str("node"), a.Bool("deep")))
			}},
		{Name: "prefab.overrides", Doc: "list the overrides of the instance a node belongs to", Args: []Arg{argFile, argNode}, Run: (*Session).prefabOverrides},
	}
}

func levelOf(a Args, def string) (prefab.Level, error) {
	name := a.Str("level")
	if name == "" {
		name = def
	}
	l, ok := prefab.ParseLevel(name)
	if !ok {
		return 0, errors.New("level is property, component, node, all or orphan")
	}
	return l, nil
}

func (s *Session) prefabCreate(a Args) (*Output, error) {
	if out, ok, err := s.prefabCreateOpen(a); ok {
		return out, err
	}
	rel, before, doc, err := s.sceneFile(a)
	if err != nil {
		return nil, err
	}
	out, err := s.rel(a.Str("out"))
	if err != nil {
		return nil, err
	}
	if path.Ext(out) != PrefabExt {
		out += PrefabExt
	}
	if err := s.writable(); err != nil {
		return nil, err
	}
	if a.Bool("dry-run") {
		h, err := doc.Node(a.Str("node"))
		if err != nil {
			return nil, err
		}
		if err := refactor.CheckCreate(doc.Scene, h); err != nil {
			return nil, err
		}
		if s.exists(out) {
			return nil, errors.New(out + " already exists")
		}
		return &Output{DryRun: true, Notes: []string{"would create " + out, "would write " + rel}}, nil
	}
	id, res, err := refactor.CreatePrefab(s.Pipeline, doc, s.Library, a.Str("node"), out)
	if err != nil {
		s.reload()
		return nil, err
	}
	files, err := s.save(rel, before, doc.Bytes())
	s.reload()
	return &Output{Changes: res.Changes, Problems: res.Problems, Created: []string{id.String()}, Files: append([]string{out}, files...)}, err
}

// prefabCreateOpen makes the prefab through the editor when the scene is open there, so unsaved edits count; false when there is no such editor or scene.
func (s *Session) prefabCreateOpen(a Args) (*Output, bool, error) {
	ph, ok := s.host.(PrefabHost)
	if !ok {
		return nil, false, nil
	}
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return nil, true, err
	}
	doc, open := s.host.OpenScene(rel)
	if !open {
		return nil, false, nil
	}
	out, err := s.rel(a.Str("out"))
	if err != nil {
		return nil, true, err
	}
	if path.Ext(out) != PrefabExt {
		out += PrefabExt
	}
	if s.exists(out) {
		return nil, true, errors.New(out + " already exists")
	}
	h, err := doc.Node(a.Str("node"))
	if err != nil {
		return nil, true, err
	}
	if err := refactor.CheckCreate(doc.Scene, h); err != nil {
		return nil, true, err
	}
	if a.Bool("dry-run") {
		return &Output{DryRun: true, Notes: []string{"would create " + out, "would link the node in " + rel + " as an Undo step"}}, true, nil
	}
	id, err := ph.CreatePrefab(rel, a.Str("node"), out)
	if err != nil {
		return nil, true, err
	}
	s.reload()
	return &Output{OK: true, Created: []string{id.String()}, Files: []string{out}}, true, nil
}

func (s *Session) prefabInstantiate(a Args) (*Output, error) {
	op := mutate.Instantiate(a.Str("parent"), a.Str("prefab"))
	op.Name = a.Str("name")
	if a.Has("index") {
		i, err := a.Int("index", 0)
		if err != nil {
			return nil, err
		}
		op = op.At(i)
	}
	return s.sceneTx(a, "Instantiate", op)
}

func (s *Session) prefabApply(a Args) (*Output, error) {
	rel, before, doc, err := s.sceneFile(a)
	if err != nil {
		return nil, err
	}
	h, err := doc.Node(a.Str("node"))
	if err != nil {
		return nil, err
	}
	lvl, err := levelOf(a, "all")
	if err != nil {
		return nil, err
	}
	c := refactor.PrefabChange{Node: doc.Scene.ID(h), Level: lvl, Prop: prefab.PropKey{Owner: a.Str("owner"), Field: a.Str("field")}, Nearest: a.Bool("nearest")}
	if a.Bool("dry-run") {
		plan, err := refactor.PlanApply(doc.Scene, s.Library, c)
		if errors.Is(err, mutate.ErrNothing) {
			return &Output{DryRun: true, Notes: []string{"nothing to apply"}}, nil
		}
		if err != nil {
			return nil, err
		}
		edits, err := plan.Edits(doc.Scene.Registry(), s.Pipeline)
		if err != nil {
			return nil, err
		}
		out := &Output{DryRun: true}
		for _, e := range edits {
			out.Notes = append(out.Notes, "would write "+e.Path)
		}
		out.Notes = append(out.Notes, "would write "+rel)
		return out, nil
	}
	res, err := refactor.ApplyPrefab(doc, s.Library, s.Pipeline, c)
	if errors.Is(err, mutate.ErrNothing) {
		return &Output{Notes: []string{"nothing to apply"}}, nil
	}
	if err != nil {
		s.reload()
		return nil, err
	}
	edits := append(res.Edits, refactor.Edit{Path: rel, Before: before, After: doc.Bytes()})
	files, err := s.write(edits)
	s.reload()
	if err != nil {
		return nil, err
	}
	return &Output{Changes: res.Result.Changes, Problems: res.Result.Problems, Files: files}, nil
}

func (s *Session) prefabRevert(a Args) (*Output, error) {
	lvl, err := levelOf(a, "all")
	if err != nil {
		return nil, err
	}
	op := mutate.RevertPrefab(a.Str("node"), lvl.String())
	op.Owner, op.Field = a.Str("owner"), a.Str("field")
	out, err := s.sceneTx(a, "Revert", op)
	if errors.Is(err, mutate.ErrNothing) {
		return &Output{Notes: []string{"nothing to revert"}}, nil
	}
	return out, err
}

func (s *Session) prefabOverrides(a Args) (*Output, error) {
	_, _, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	h, err := doc.Node(a.Str("node"))
	if err != nil {
		return nil, err
	}
	root, _, ok := sceneio.OwnerRoot(doc.Scene, h)
	if !ok {
		return nil, errors.New(a.Str("node") + " is not part of a prefab instance")
	}
	inst, err := sceneio.Overrides(doc.Scene, root, s.Library)
	if err != nil {
		return nil, err
	}
	text := prefab.FormatOverrides(inst.Overrides)
	return &Output{Value: text, Text: text}, nil
}
