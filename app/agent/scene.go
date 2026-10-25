package agent

import (
	"errors"
	"maps"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"aego/project/refs"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
)

var (
	argFile  = Arg{Name: "file", Type: "string", Doc: "scene or prefab file, relative to the project", Required: true, Pos: true}
	argNode  = Arg{Name: "node", Type: "string", Doc: "node path from the root (Player/Sprite), Root, or #id", Required: true, Pos: true}
	argPath  = Arg{Name: "path", Type: "string", Doc: "Comp.field, Comp.field.x, Comp.list[2], transform.position, enabled", Required: true, Pos: true}
	argValue = Arg{Name: "value", Type: "string", Doc: `value in scene syntax: 2.5, [1, 2], #ff8800ff, Kinematic, "text", asset("<id>") or a project path`, Required: true, Pos: true}
	argIndex = Arg{Name: "index", Type: "integer", Doc: "position among the siblings"}
)

type NodeView struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Path       string            `json:"path"`
	Enabled    bool              `json:"enabled"`
	Prefab     string            `json:"prefab,omitempty"`
	Transform  map[string]string `json:"transform,omitempty"`
	Components []CompView        `json:"components,omitempty"`
	Scripts    []string          `json:"scripts,omitempty"`
	ScriptData []CompView        `json:"scriptFields,omitempty"`
	Children   []NodeView        `json:"children,omitempty"`
}

type CompView struct {
	Type   string            `json:"type"`
	Fields map[string]string `json:"fields,omitempty"`
}

func sceneCommands() []*Command {
	optPath := argPath
	optPath.Required = false
	optParent := Arg{Name: "parent", Type: "string", Doc: "parent node, the root by default"}
	return []*Command{
		{Name: "scene.tree", Doc: "node tree with IDs, components, scripts and prefab links", Args: []Arg{argFile}, Run: (*Session).sceneTree},
		{Name: "scene.get", Doc: "a node with its transform and every saved field, or one field", Args: []Arg{argFile, argNode, optPath}, Run: (*Session).sceneGet},
		{Name: "scene.set", Doc: "set a field", Write: true, Args: []Arg{argFile, argNode, argPath, argValue}, Run: func(s *Session, a Args) (*Output, error) {
			return s.sceneTx(a, "Set", mutate.Set(a.Str("node"), a.Str("path"), a.Str("value")))
		}},
		{Name: "scene.reset", Doc: "reset a field to its default, a transform part to identity", Write: true, Args: []Arg{argFile, argNode, argPath}, Run: func(s *Session, a Args) (*Output, error) {
			return s.sceneTx(a, "Reset", mutate.Reset(a.Str("node"), a.Str("path")))
		}},
		{Name: "scene.add-node", Doc: "add an empty node", Write: true, Args: []Arg{argFile, {Name: "name", Type: "string", Doc: "node name", Required: true, Pos: true}, optParent,
			{Name: "space", Type: "string", Doc: "2d or 3d, the parent's space by default"}, argIndex}, Run: (*Session).sceneAddNode},
		{Name: "scene.remove-node", Doc: "remove a node with its children", Write: true, Destroys: true, Args: []Arg{argFile, argNode}, Run: func(s *Session, a Args) (*Output, error) {
			return s.sceneTx(a, "Remove Node", mutate.RemoveNode(a.Str("node")))
		}},
		{Name: "scene.move", Doc: "move a node under another parent", Write: true, Args: []Arg{argFile, argNode, {Name: "parent", Type: "string", Doc: "new parent node", Required: true, Pos: true}, argIndex,
			{Name: "keep-world", Type: "boolean", Doc: "keep the world transform instead of the local one"}}, Run: (*Session).sceneMove},
		{Name: "scene.rename", Doc: "rename a node", Write: true, Args: []Arg{argFile, argNode, {Name: "name", Type: "string", Doc: "new name", Required: true, Pos: true}}, Run: func(s *Session, a Args) (*Output, error) {
			return s.sceneTx(a, "Rename", mutate.Rename(a.Str("node"), a.Str("name")))
		}},
		{Name: "scene.add-comp", Doc: "add a component with its defaults and the components it requires, or attach a script by its type name", Write: true, Args: []Arg{argFile, argNode, {Name: "comp", Type: "string", Doc: "component or script name, full or a unique suffix (Body2D, EnemyAI)", Required: true, Pos: true},
			{Name: "no-required", Type: "boolean", Doc: "add only this component; missing requirements stay as problems"}}, Run: (*Session).sceneAddComp},
		{Name: "scene.remove-comp", Doc: "remove a component", Write: true, Destroys: true, Args: []Arg{argFile, argNode, {Name: "comp", Type: "string", Doc: "component or script name", Required: true, Pos: true}}, Run: func(s *Session, a Args) (*Output, error) {
			return s.sceneTx(a, "Remove Component", mutate.RemoveComp(a.Str("node"), a.Str("comp")))
		}},
		{Name: "scene.apply", Doc: "apply a transaction (tx version=1 { op … }) in one step", Write: true, Args: []Arg{argFile,
			{Name: "tx", Type: "string", Doc: "transaction text"}, {Name: "tx-file", Type: "string", Doc: "transaction file"}}, Run: (*Session).sceneApply},
		{Name: "scene.validate", Doc: "load problems and references to nodes the scene does not have", Args: []Arg{argFile}, Run: (*Session).sceneValidate},
		{Name: "scene.new", Doc: "create an empty scene", Write: true, Args: []Arg{{Name: "file", Type: "string", Doc: "new scene file, scenes/<name>.aescene", Required: true, Pos: true}}, Run: (*Session).sceneNew},
		{Name: "scene.run", Doc: "run a scene headless and report the log, the state hash and where the watched nodes went", Args: []Arg{{Name: "file", Type: "string", Doc: "scene file, the main scene by default", Pos: true},
			{Name: "frames", Type: "integer", Doc: "frames to run, 120 by default"}, {Name: "seed", Type: "integer", Doc: "random seed, 1 by default"},
			{Name: "watch", Type: "string", Doc: "comma-separated node paths whose world positions the report samples: Player,MovingPlatform"}}, Run: (*Session).sceneRun},
	}
}

// scene is the scene of a command; in the editor an open scene is its live document, src nil.
func (s *Session) scene(a Args) (string, []byte, *mutate.Document, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return "", nil, nil, err
	}
	if s.host != nil {
		if doc, ok := s.host.OpenScene(rel); ok {
			return rel, nil, doc, nil
		}
	}
	return s.sceneDisk(rel)
}

func (s *Session) sceneDisk(rel string) (string, []byte, *mutate.Document, error) {
	src, err := s.read(rel)
	if err != nil {
		return "", nil, nil, err
	}
	doc, err := mutate.Load(src, rel, s.env())
	return rel, src, doc, err
}

// sceneFile is the scene on disk, for commands that write other files with it; an open scene with unsaved edits is refused.
func (s *Session) sceneFile(a Args) (string, []byte, *mutate.Document, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return "", nil, nil, err
	}
	if s.host != nil && s.host.SceneDirty(rel) {
		return "", nil, nil, errors.New(rel + " has unsaved changes in the editor; save it first")
	}
	return s.sceneDisk(rel)
}

// sceneAddComp adds a component and, unless no-required is set, the components it requires first, in one transaction (D144).
func (s *Session) sceneAddComp(a Args) (*Output, error) {
	rel, src, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	node, comp := a.Str("node"), a.Str("comp")
	ops := []mutate.Op{mutate.AddComp(node, comp)}
	var notes []string
	if !a.Bool("no-required") {
		td, err := doc.Component(comp)
		if err != nil {
			return nil, err
		}
		h, err := doc.Node(node)
		if err != nil {
			return nil, err
		}
		req, err := mutate.Required(doc.Scene, td, h)
		if err != nil {
			return nil, err
		}
		ops = ops[:0]
		for _, r := range req {
			ops = append(ops, mutate.AddComp(node, r.FullName))
			notes = append(notes, "added "+label(r)+" (required by "+label(td)+")")
		}
		ops = append(ops, mutate.AddComp(node, td.FullName))
	}
	out, err := s.applyDoc(a, rel, src, doc, mutate.Tx{Label: "Add Component", Ops: ops})
	if out != nil {
		out.Notes = append(notes, out.Notes...)
	}
	return out, err
}

func label(td *schema.TypeDescriptor) string {
	if td.DisplayName != "" {
		return td.DisplayName
	}
	return td.FullName
}

func (s *Session) sceneTx(a Args, label string, ops ...mutate.Op) (*Output, error) {
	rel, src, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	return s.applyDoc(a, rel, src, doc, mutate.Tx{Label: label, Ops: ops})
}

func (s *Session) applyDoc(a Args, rel string, src []byte, doc *mutate.Document, tx mutate.Tx) (*Output, error) {
	live := src == nil && s.host != nil
	var res *mutate.Result
	var err error
	switch {
	case a.Bool("dry-run"):
		res, err = doc.DryRun(tx)
	case live:
		res, err = s.host.ApplyScene(rel, tx.Label, tx)
	default:
		res, err = doc.Apply(tx)
	}
	if err != nil {
		return nil, err
	}
	out := &Output{DryRun: res.DryRun, Changes: res.Changes, Problems: append(slices.Clone(doc.Problems), res.Problems...), Created: res.Created}
	switch {
	case res.DryRun:
		return out, nil
	case live:
		out.Notes = append(out.Notes, "applied to "+rel+" in the editor as one Undo step; save the scene to write it")
		return out, nil
	}
	out.Files, err = s.save(rel, src, doc.Bytes())
	return out, err
}

func (s *Session) sceneAddNode(a Args) (*Output, error) {
	op := mutate.AddNode(a.Str("parent"), a.Str("name")).In(a.Str("space"))
	if a.Has("index") {
		i, err := a.Int("index", 0)
		if err != nil {
			return nil, err
		}
		op = op.At(i)
	}
	return s.sceneTx(a, "Add Node", op)
}

func (s *Session) sceneMove(a Args) (*Output, error) {
	op := mutate.Move(a.Str("node"), a.Str("parent"))
	if a.Has("index") {
		i, err := a.Int("index", 0)
		if err != nil {
			return nil, err
		}
		op = op.At(i)
	}
	if a.Bool("keep-world") {
		op = op.KeepingWorld()
	}
	return s.sceneTx(a, "Move", op)
}

func (s *Session) sceneApply(a Args) (*Output, error) {
	text, name := []byte(a.Str("tx")), "tx"
	if a.Has("tx-file") {
		b, err := os.ReadFile(a.Str("tx-file"))
		if err != nil {
			return nil, err
		}
		text, name = b, a.Str("tx-file")
	}
	if len(text) == 0 {
		return nil, errors.New("scene.apply needs tx or tx-file")
	}
	tx, err := mutate.ParseTx(text, name)
	if err != nil {
		return nil, err
	}
	if tx.Label == "" {
		tx.Label = "Apply Transaction"
	}
	rel, src, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	return s.applyDoc(a, rel, src, doc, tx)
}

func (s *Session) sceneValidate(a Args) (*Output, error) {
	rel, _, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	out := &Output{Problems: slices.Clone(doc.Problems)}
	for _, m := range refs.FileMissingNodes(s.Pipeline, rel) {
		out.Problems = append(out.Problems, mutate.Problem{Error: true, Node: m.Path, Message: m.Owner + "." + m.Field + " references node #" + m.Target.String() + ", which is not in the scene"})
	}
	if len(out.Problems) == 0 {
		out.Notes = []string{rel + ": ok"}
	}
	return out, nil
}

func (s *Session) sceneNew(a Args) (*Output, error) {
	rel, err := s.rel(a.Str("file"))
	if err != nil {
		return nil, err
	}
	if path.Ext(rel) != SceneExt {
		rel += SceneExt
	}
	if a.Bool("dry-run") {
		if err := s.writable(); err != nil {
			return nil, err
		}
		if s.exists(rel) {
			return nil, errors.New(rel + " already exists")
		}
		return &Output{DryRun: true, Notes: []string{"would create " + rel}}, nil
	}
	sc := scene.New(s.Types, sources{s.Pipeline}, scene.Options{})
	id, err := s.create(rel, sceneio.Save(sc))
	if err != nil {
		return nil, err
	}
	return &Output{Created: []string{id.String()}, Files: []string{rel}}, nil
}

func (s *Session) sceneRun(a Args) (*Output, error) {
	if s.opt.Run == nil {
		return nil, errors.New("headless runs are not available in this build")
	}
	frames, err := a.Int("frames", 120)
	if err != nil {
		return nil, err
	}
	seed, err := a.Int("seed", 1)
	if err != nil {
		return nil, err
	}
	req := RunRequest{Frames: frames, Seed: uint64(seed), Watch: WatchList(a.Str("watch"))}
	if a.Has("file") {
		rel, err := s.rel(a.Str("file"))
		if err != nil {
			return nil, err
		}
		req.Scene = s.Pipeline.Abs(rel)
	}
	var rep RunReport
	run := func() error {
		var rerr error
		rep, rerr = s.opt.Run(s.Project, req)
		return rerr
	}
	if s.borrowed {
		err = run()
	} else {
		err = s.release(run)
	}
	if err != nil {
		return nil, err
	}
	return RunOutput(rep), nil
}

// RunOutput is the answer to scene.run: the report, its text and the run's error as a problem.
func RunOutput(rep RunReport) *Output {
	out := &Output{Value: rep}
	if rep.Error != "" {
		out.Problems = append(out.Problems, mutate.Problem{Error: true, Message: rep.Error})
	}
	var b strings.Builder
	b.WriteString("frames: " + strconv.Itoa(rep.Frames) + "\nhash: " + rep.Hash + "\n")
	for _, k := range slices.Sorted(maps.Keys(rep.Counters)) {
		b.WriteString(k + ": " + strconv.FormatUint(rep.Counters[k], 10) + "\n")
	}
	for _, t := range rep.Tracks {
		b.WriteString(t.Node + ":")
		if t.Error != "" {
			b.WriteString(" " + t.Error)
		}
		for i, p := range t.At {
			b.WriteString(" f" + strconv.Itoa(t.Frames[i]) + " [" + strconv.FormatFloat(float64(p[0]), 'g', 4, 32) + ", " + strconv.FormatFloat(float64(p[1]), 'g', 4, 32) + "]")
		}
		b.WriteString("\n")
	}
	for _, l := range rep.Log {
		b.WriteString(l + "\n")
	}
	out.Text = b.String()
	return out
}

// WatchList splits scene.run's watch argument into node paths.
func WatchList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Session) sceneTree(a Args) (*Output, error) {
	_, _, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	v := s.view(doc.Scene, doc.Scene.Root(), false, true)
	var b strings.Builder
	treeText(&b, v, 0)
	return &Output{Value: v, Text: b.String(), Problems: doc.Problems}, nil
}

func (s *Session) sceneGet(a Args) (*Output, error) {
	_, _, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	h, err := doc.Node(a.Str("node"))
	if err != nil {
		return nil, err
	}
	v := s.view(doc.Scene, h, true, false)
	if !a.Has("path") {
		return &Output{Value: v}, nil
	}
	text, ok := pick(v, a.Str("path"))
	if !ok {
		return nil, errors.New(v.Path + " has no field " + a.Str("path"))
	}
	return &Output{Value: text, Text: text}, nil
}

func pick(v NodeView, p string) (string, bool) {
	i := strings.LastIndexByte(p, '.')
	if i < 0 {
		return "", false
	}
	comp, field := p[:i], p[i+1:]
	if c, f, _ := strings.Cut(p, "."); strings.EqualFold(c, "transform") {
		t, ok := v.Transform[strings.ToLower(f)]
		return t, ok
	}
	for _, list := range [][]CompView{v.Components, v.ScriptData} {
		for _, c := range list {
			if c.Type == comp || strings.HasSuffix(c.Type, "."+comp) {
				t, ok := c.Fields[field]
				return t, ok
			}
		}
	}
	return "", false
}

func (s *Session) view(sc *scene.Scene, h scene.NodeHandle, fields, deep bool) NodeView {
	reg := sc.Registry()
	v := NodeView{Name: sc.Name(h), Path: mutate.RootName + sc.Path(h), Enabled: sc.Enabled(h)}
	if h == sc.Root() {
		v.ID, v.Path = mutate.RootName, mutate.RootName
	} else {
		v.ID = "#" + sc.ID(h).String()
		if fields {
			v.Transform = pose(reg, sc, h)
		}
	}
	sc.ComponentsOf(h, func(t schema.TypeID) {
		if t == scriptcomp.ScriptTypeID {
			return
		}
		td, ok := reg.ByID(t)
		if !ok || (td.Hidden && td.FullName != "engine.Unknown") {
			return
		}
		c := CompView{Type: td.FullName}
		if fields {
			c.Fields = fieldValues(sc, h, td)
		}
		v.Components = append(v.Components, c)
	})
	sort.Slice(v.Components, func(i, j int) bool { return v.Components[i].Type < v.Components[j].Type })
	if scs, ok := scriptcomp.ScriptsOf(sc, h); ok {
		for _, at := range scs.Attachments {
			v.Scripts = append(v.Scripts, at.TypeName)
			if td, ok := reg.ByName(at.TypeName); ok && fields {
				v.ScriptData = append(v.ScriptData, CompView{Type: td.FullName, Fields: fieldValues(sc, h, td)})
			}
		}
	}
	if pi, ok := sceneio.InstanceOf(sc, h); ok && pi.Owner {
		v.Prefab = s.label(pi.Prefab)
	}
	if deep {
		sc.Children(h, func(c scene.NodeHandle) bool {
			if sc.IsAlive(c) {
				v.Children = append(v.Children, s.view(sc, c, fields, true))
			}
			return true
		})
	}
	return v
}

func fieldValues(sc *scene.Scene, h scene.NodeHandle, td *schema.TypeDescriptor) map[string]string {
	out := map[string]string{}
	reg := sc.Registry()
	td.Serializable(func(fd *schema.FieldDescriptor) {
		if v, err := sc.GetProperty(h, td.ID, fd.ID); err == nil {
			out[fd.Name] = mutate.FormatValue(sceneio.WriteValue(reg, fd, v))
		}
	})
	return out
}

func pose(reg *schema.Registry, sc *scene.Scene, h scene.NodeHandle) map[string]string {
	f := func(k schema.Kind, v schema.Value) string {
		return mutate.FormatValue(sceneio.WriteValue(reg, &schema.FieldDescriptor{Kind: k, Flags: schema.FlagSerialize}, v))
	}
	p := sc.Pose(h)
	if p.Space == scene.Space3D {
		return map[string]string{"space": "3d", "position": f(schema.KindVec3, schema.Vec3(p.T3.Position)),
			"rotation": f(schema.KindQuat, schema.Quat(p.T3.Rotation)), "scale": f(schema.KindVec3, schema.Vec3(p.T3.Scale))}
	}
	return map[string]string{"space": "2d", "position": f(schema.KindVec2, schema.Vec2(p.T2.Position)),
		"rotation": f(schema.KindFloat32, schema.Float32(p.T2.Rotation)), "scale": f(schema.KindVec2, schema.Vec2(p.T2.Scale))}
}

func treeText(b *strings.Builder, v NodeView, depth int) {
	b.WriteString(strings.Repeat("  ", depth) + v.Name + "  " + v.ID)
	var tags []string
	for _, c := range v.Components {
		tags = append(tags, c.Type[strings.LastIndexByte(c.Type, '.')+1:])
	}
	tags = append(tags, v.Scripts...)
	if len(tags) > 0 {
		b.WriteString("  [" + strings.Join(tags, ", ") + "]")
	}
	if v.Prefab != "" {
		b.WriteString("  ← " + v.Prefab)
	}
	if !v.Enabled {
		b.WriteString("  (disabled)")
	}
	b.WriteString("\n")
	for _, c := range v.Children {
		treeText(b, c, depth+1)
	}
}
