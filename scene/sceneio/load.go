package sceneio

import (
	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
)

const FormatVersion = 1

type LoadOptions struct {
	Strict  bool
	KeepIDs bool
	Prefabs *Library
}

type Diag struct {
	Path string
	Pos  aetext.Pos
	Msg  string
}

type Unresolved struct {
	Node   ids.NodeID
	Asset  ids.AssetID
	Target ids.NodeID
	Type   string
	Field  string
}

type Report struct {
	Warnings   []Diag
	Unknown    []string
	Unresolved []Unresolved
	Missing    []MissingPrefab
}

func (r *Report) warn(path string, pos aetext.Pos, msg string) {
	r.Warnings = append(r.Warnings, Diag{Path: path, Pos: pos, Msg: msg})
}

type loader struct {
	scene    *scene.Scene
	reg      *schema.Registry
	res      asset.Resolver
	lib      *Library
	opt      LoadOptions
	report   *Report
	name     string
	fileIDs  map[ids.NodeID]scene.NodeHandle
	deferred []deferredRef
	created  []scene.NodeHandle
}

type deferredRef struct {
	node   scene.NodeHandle
	typeID schema.TypeID
	field  schema.FieldID
	script string
	value  schema.Value
	name   string
	fname  string
}

func Load(src []byte, name string, types *scene.Types, res asset.Resolver, opt LoadOptions) (*scene.Scene, *Report, error) {
	s := scene.New(types, res, scene.Options{})
	_, report, err := loadInto(s, s.Root(), src, name, opt, true)
	if err != nil {
		return nil, report, err
	}
	return s, report, nil
}

func LoadInto(s *scene.Scene, parent scene.NodeHandle, src []byte, name string, opt LoadOptions) ([]scene.NodeHandle, *Report, error) {
	return loadInto(s, parent, src, name, opt, false)
}

func loadInto(s *scene.Scene, parent scene.NodeHandle, src []byte, name string, opt LoadOptions, whole bool) ([]scene.NodeHandle, *Report, error) {
	report := &Report{}
	lib := opt.Prefabs
	if lib == nil {
		lib = NewLibrary(s.Registry(), s.Resolver())
	}
	l := &loader{
		scene:   s,
		reg:     s.Registry(),
		res:     s.Resolver(),
		lib:     lib,
		opt:     opt,
		report:  report,
		name:    name,
		fileIDs: map[ids.NodeID]scene.NodeHandle{},
	}
	doc, err := parseDocument(src, name, report)
	if err != nil {
		return nil, report, err
	}
	if whole {
		if v, ok := doc.Attr("space"); ok {
			sp, known := scene.ParseSpace(v.Str)
			if !known {
				report.warn(name, doc.Pos, "unknown scene space "+v.Str)
			}
			s.SetDefaultSpace(sp)
		}
	}
	for _, block := range doc.Blocks {
		if block.Head != "node" {
			continue
		}
		if err := l.loadNode(block, parent); err != nil {
			return nil, report, err
		}
	}
	if err := l.resolveDeferred(); err != nil {
		return nil, report, err
	}
	s.Flush()
	s.UpdateTransforms()
	return l.created, report, nil
}

func parseDocument(src []byte, name string, report *Report) (*aetext.Node, error) {
	doc, err := aetext.Parse(src, name)
	if err != nil {
		return nil, errs.Wrap(err, errs.SceneIOParse, "cannot parse the document")
	}
	if doc.Head != "scene" && doc.Head != "prefab" {
		return nil, errs.New(errs.SceneIOParse, "unexpected document root",
			log.F("path", name), log.F("head", doc.Head))
	}
	version := FormatVersion
	if v, ok := doc.Attr("version"); ok {
		version = int(v.AsInt())
	}
	if version > FormatVersion {
		return nil, errs.New(errs.SceneIOVersion, "document was written by a newer engine",
			log.F("path", name), log.F("version", version), log.F("supported", FormatVersion))
	}
	if version < FormatVersion {
		migrated, err := Migrate(doc, version, FormatVersion)
		if err != nil {
			return nil, err
		}
		report.warn(name, doc.Pos, "document was migrated from version "+itoa(version))
		doc = migrated
	}
	return doc, nil
}

func (l *loader) loadNode(n *aetext.Node, parent scene.NodeHandle) error {
	if _, ok := n.Attr("prefab"); ok {
		return l.loadInstance(n, parent)
	}
	var id, fileID ids.NodeID
	if n.Label != "" {
		parsed, err := ids.ParseNodeID(n.Label)
		switch {
		case err == nil:
			fileID = parsed
			if l.opt.KeepIDs {
				id = parsed
			}
		case l.opt.KeepIDs && l.opt.Strict:
			return errs.Wrap(err, errs.SceneIOParse, "malformed node id").With(log.F("path", l.name))
		case l.opt.KeepIDs:
			l.report.warn(l.name, n.Pos, "malformed node id, generating a new one")
		}
	}
	name := "Node"
	if v, ok := n.Attr("name"); ok {
		name = v.Str
	}
	sp := scene.Space2D
	if v, ok := n.Attr("space"); ok {
		parsed, known := scene.ParseSpace(v.Str)
		if !known {
			if l.opt.Strict {
				return errs.New(errs.SceneIOParse, "unknown node space", log.F("path", l.name), log.F("space", v.Str))
			}
			l.report.warn(l.name, n.Pos, "unknown node space "+v.Str+", loading as 2d")
		}
		sp = parsed
	}
	h, err := l.scene.CreateIn(parent, name, id, sp)
	if err != nil {
		return errs.Wrap(err, errs.SceneIOParse, "node cannot be placed under its parent").
			With(log.F("path", l.name), log.F("node", name), log.F("line", n.Pos.Line))
	}
	if h.IsZero() {
		return errs.New(errs.SceneIOParse, "cannot create a node", log.F("path", l.name))
	}
	l.created = append(l.created, h)
	if !fileID.IsZero() {
		l.fileIDs[fileID] = h
	}

	if v, ok := n.Attr("enabled"); ok {
		l.scene.SetEnabled(h, v.Bool)
	}
	if v, ok := n.Attr("layers"); ok {
		l.scene.SetLayers(h, uint64(v.AsInt()))
	}
	if v, ok := n.Attr("instanceOf"); ok {
		if aid, err := ids.ParseAssetID(v.Str); err == nil {
			if pi, ok := addInstance(l.scene, h); ok {
				pi.Prefab, pi.Nested = aid, true
			}
		}
	}

	for _, block := range n.Blocks {
		switch block.Head {
		case "transform":
			if sp == scene.Space2D {
				l.scene.SetLocal(h, readTransform(block))
			} else {
				l.report.warn(l.name, block.Pos, "2d transform on a 3d node is ignored")
			}
		case "transform3d":
			if sp == scene.Space3D {
				l.scene.SetLocal3D(h, readTransform3D(block))
			} else {
				l.report.warn(l.name, block.Pos, "3d transform on a 2d node is ignored")
			}
		case "component":
			if err := l.loadComponent(h, block); err != nil {
				return err
			}
		case "node":
			if err := l.loadNode(block, h); err != nil {
				return err
			}
		default:
			l.report.warn(l.name, block.Pos, "unknown block "+block.Head)
		}
	}
	return nil
}

func readTransform(n *aetext.Node) scene.Transform2D {
	t := scene.IdentityTransform()
	if v, ok := n.Field("position"); ok {
		t.Position = readVec2(v, t.Position)
	}
	if v, ok := n.Field("rotation"); ok {
		t.Rotation = float32(v.AsFloat())
	}
	if v, ok := n.Field("scale"); ok {
		t.Scale = readVec2(v, t.Scale)
	}
	return t
}

func readTransform3D(n *aetext.Node) scene.Transform3D {
	t := scene.IdentityTransform3D()
	if v, ok := n.Field("position"); ok {
		t.Position = readVec3(v, t.Position)
	}
	if v, ok := n.Field("rotation"); ok {
		q := readVec4(v, mathx.Vec4{W: 1})
		t.Rotation = mathx.Quat(q)
	}
	if v, ok := n.Field("scale"); ok {
		t.Scale = readVec3(v, t.Scale)
	}
	return t
}

func readVec2(v aetext.Value, def mathx.Vec2) mathx.Vec2 {
	if v.Kind != aetext.VList || len(v.List) != 2 {
		return def
	}
	return mathx.Vec2{X: float32(v.List[0].AsFloat()), Y: float32(v.List[1].AsFloat())}
}

func readVec3(v aetext.Value, def mathx.Vec3) mathx.Vec3 {
	if v.Kind != aetext.VList || len(v.List) != 3 {
		return def
	}
	return mathx.Vec3{X: float32(v.List[0].AsFloat()), Y: float32(v.List[1].AsFloat()), Z: float32(v.List[2].AsFloat())}
}

func readVec4(v aetext.Value, def mathx.Vec4) mathx.Vec4 {
	if v.Kind != aetext.VList || len(v.List) != 4 {
		return def
	}
	return mathx.Vec4{
		X: float32(v.List[0].AsFloat()),
		Y: float32(v.List[1].AsFloat()),
		Z: float32(v.List[2].AsFloat()),
		W: float32(v.List[3].AsFloat()),
	}
}

func (l *loader) loadComponent(h scene.NodeHandle, n *aetext.Node) error {
	typeName := n.Label
	td, ok := l.reg.ByName(typeName)
	if !ok {
		if l.opt.Strict {
			return errs.New(errs.SceneIOStrict, "unknown component type",
				log.F("path", l.name), log.F("type", typeName))
		}
		l.storeUnknown(h, n)
		l.report.Unknown = append(l.report.Unknown, typeName)
		return nil
	}
	if td.FullName == scriptcomp.ScriptName {
		return l.loadScripts(h, n)
	}
	if !td.Space.Allows(l.scene.Space(h).Kind()) {
		l.report.warn(l.name, n.Pos, "component "+td.FullName+" expects a "+td.Space.String()+" node")
	}
	p, err := l.scene.AddByType(h, td.ID)
	if err != nil {
		return err
	}
	if td.Default != nil && td.Copy != nil {
		td.Copy(p, td.Default())
	}
	for _, f := range n.Fields {
		fd, ok := td.Field(f.Name)
		if !ok {
			l.report.warn(l.name, f.Pos, "unknown field "+typeName+"."+f.Name)
			continue
		}
		value, err := l.convert(fd, f.Value)
		if err != nil {
			if l.opt.Strict {
				return err
			}
			l.report.warn(l.name, f.Pos, err.Error())
			continue
		}
		if hasNodes(value) {
			l.deferred = append(l.deferred, deferredRef{
				node: h, typeID: td.ID, field: fd.ID, value: value, name: typeName, fname: fd.Name,
			})
			continue
		}
		if err := fd.Set(p, value); err != nil {
			if l.opt.Strict {
				return err
			}
			l.report.warn(l.name, f.Pos, err.Error())
		}
	}
	return nil
}

func (l *loader) storeUnknown(h scene.NodeHandle, n *aetext.Node) {
	u := scene.Add[Unknown](l.scene, h)
	if u == nil {
		return
	}
	version := 1
	if v, ok := n.Attr("version"); ok {
		version = int(v.AsInt())
	}
	u.Components = append(u.Components, UnknownComponent{
		TypeName: n.Label,
		Version:  version,
		Fields:   n.Fields,
		Blocks:   n.Blocks,
	})
}

func (l *loader) convert(fd *schema.FieldDescriptor, v aetext.Value) (schema.Value, error) {
	switch fd.Kind {
	case schema.KindBool:
		return schema.Bool(v.Bool), nil
	case schema.KindInt32:
		return schema.Int32(int32(v.AsInt())), nil
	case schema.KindInt64:
		return schema.Int64(v.AsInt()), nil
	case schema.KindFloat32:
		return schema.Float32(float32(v.AsFloat())), nil
	case schema.KindFloat64:
		return schema.Float64(v.AsFloat()), nil
	case schema.KindString:
		return schema.String(v.Str), nil
	case schema.KindVec2:
		return schema.Vec2(readVec2(v, mathx.Vec2{})), nil
	case schema.KindVec3:
		return schema.Vec3(readVec3(v, mathx.Vec3{})), nil
	case schema.KindVec4:
		return schema.Vec4(readVec4(v, mathx.Vec4{})), nil
	case schema.KindQuat:
		q := readVec4(v, mathx.Vec4{W: 1})
		return schema.Quat(mathx.Quat(q)), nil
	case schema.KindColor:
		return schema.Color(unpackColor(v.Color)), nil
	case schema.KindRect:
		return schema.Rect(readRect(v)), nil
	case schema.KindCurve:
		c, err := ReadCurve(v)
		if err != nil {
			return schema.Value{}, err
		}
		return schema.Value{Kind: schema.KindCurve, Cv: c}, nil
	case schema.KindEnum:
		if v.Kind == aetext.VIdent {
			if n, ok := fd.EnumValueOf(v.Ident); ok {
				return schema.Enum(n), nil
			}
			return schema.Value{}, errs.New(errs.SceneIOParse,
				"unknown enum value "+v.Ident+" for field "+fd.Name)
		}
		return schema.Enum(v.AsInt()), nil
	case schema.KindAssetRef:
		if v.Str == "" {
			return schema.Asset(ids.AssetID{}), nil
		}
		id, err := ids.ParseAssetID(v.Str)
		if err != nil {
			return schema.Value{}, err
		}
		if !id.IsZero() && l.res != nil && !l.resolves(fd, id) {
			l.report.Unresolved = append(l.report.Unresolved, Unresolved{
				Asset: id, Type: fd.AssetKind, Field: fd.Name,
			})
		}
		return schema.Asset(id), nil
	case schema.KindNodeRef:
		if v.Str == "" {
			return schema.Node(ids.NodeID{}), nil
		}
		id, err := ids.ParseNodeID(v.Str)
		if err != nil {
			return schema.Value{}, err
		}
		return schema.Node(id), nil
	case schema.KindList:
		out := schema.Value{Kind: schema.KindList}
		elem := schema.FieldDescriptor{Kind: fd.Elem, Struct: fd.Struct, Enum: fd.Enum, AssetKind: fd.AssetKind, Name: fd.Name}
		for _, item := range v.List {
			ev, err := l.convert(&elem, item)
			if err != nil {
				return schema.Value{}, err
			}
			out.List = append(out.List, ev)
		}
		return out, nil
	case schema.KindStruct:
		td, ok := l.reg.ByID(fd.Struct)
		if !ok {
			return schema.Value{}, errs.New(errs.SceneIOParse, "unregistered nested struct")
		}
		obj := &schema.Object{Type: td.ID, Fields: make([]schema.Value, len(td.Fields))}
		for i := range td.Fields {
			for _, f := range v.Obj {
				if f.Name != td.Fields[i].Name {
					continue
				}
				ev, err := l.convert(&td.Fields[i], f.Value)
				if err != nil {
					return schema.Value{}, err
				}
				obj.Fields[i] = ev
			}
		}
		return schema.Value{Kind: schema.KindStruct, Obj: obj}, nil
	}
	return schema.Value{}, errs.New(errs.SceneIOParse, "field kind cannot be read from text",
		log.F("field", fd.Name))
}

func hasNodes(v schema.Value) bool {
	switch v.Kind {
	case schema.KindNodeRef:
		return !v.Node.IsZero()
	case schema.KindList:
		for _, e := range v.List {
			if hasNodes(e) {
				return true
			}
		}
	case schema.KindStruct:
		if v.Obj != nil {
			for _, e := range v.Obj.Fields {
				if hasNodes(e) {
					return true
				}
			}
		}
	}
	return false
}

func (l *loader) remap(v *schema.Value, typ, field string) error {
	switch v.Kind {
	case schema.KindNodeRef:
		if v.Node.IsZero() {
			return nil
		}
		if h, ok := l.fileIDs[v.Node]; ok {
			v.Node = l.scene.ID(h)
			return nil
		}
		if h := l.scene.Handle(v.Node); !h.IsZero() {
			return nil
		}
		if l.opt.Strict {
			return errs.New(errs.SceneIOStrict, "node reference does not resolve",
				log.F("path", l.name), log.F("field", typ+"."+field), log.F("target", v.Node.String()))
		}
		l.report.Unresolved = append(l.report.Unresolved, Unresolved{Target: v.Node, Type: typ, Field: field})
	case schema.KindList:
		for i := range v.List {
			if err := l.remap(&v.List[i], typ, field); err != nil {
				return err
			}
		}
	case schema.KindStruct:
		if v.Obj == nil {
			return nil
		}
		for i := range v.Obj.Fields {
			if err := l.remap(&v.Obj.Fields[i], typ, field); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *loader) resolves(fd *schema.FieldDescriptor, id ids.AssetID) bool {
	switch fd.AssetKind {
	case "scene":
		_, ok := l.res.Scene(id)
		return ok
	case "prefab":
		_, ok := l.res.Prefab(id)
		return ok
	case "mesh":
		_, ok := l.res.Mesh(id)
		return ok
	case string(asset.ArtData), string(asset.ArtTable):
		ds, ok := l.res.(asset.DataSource)
		if !ok {
			return true
		}
		if fd.AssetKind == string(asset.ArtData) {
			_, ok = ds.Data(id)
		} else {
			_, ok = ds.Table(id)
		}
		return ok
	default:
		_, ok := l.res.Texture(id)
		return ok
	}
}

func (l *loader) resolveDeferred() error {
	for _, d := range l.deferred {
		v := d.value
		if err := l.remap(&v, d.name, d.fname); err != nil {
			return err
		}
		if d.script != "" {
			if sc, ok := scriptcomp.ScriptsOf(l.scene, d.node); ok {
				if i := sc.Index(d.script); i >= 0 {
					sc.Attachments[i].Props.Set(d.fname, v)
				}
			}
			continue
		}
		if err := l.scene.SetProperty(d.node, d.typeID, d.field, v); err != nil {
			l.report.warn(l.name, aetext.Pos{}, err.Error())
		}
	}
	l.deferred = nil
	return nil
}

func unpackColor(v uint32) mathx.Color {
	return mathx.ColorRGBA8(
		uint8(v>>24),
		uint8(v>>16),
		uint8(v>>8),
		uint8(v),
	)
}

func readRect(v aetext.Value) mathx.Rect {
	if v.Kind != aetext.VList || len(v.List) != 4 {
		return mathx.Rect{}
	}
	return mathx.Rect{
		X: float32(v.List[0].AsFloat()),
		Y: float32(v.List[1].AsFloat()),
		W: float32(v.List[2].AsFloat()),
		H: float32(v.List[3].AsFloat()),
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
