package clipboard

import (
	"math"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/props"
	"aego/engine/i18n"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/ui/widgets"
)

const (
	MIMEComponent  = "application/x-aego-component"
	MIMEComponents = "application/x-aego-components"
	MIMEValue      = "application/x-aego-value"
	componentText  = "aego-component\n"
	componentsText = "aego-components\n"
)

type Kind uint8

const (
	KindEmpty Kind = iota
	KindNodes
	KindComponent
	KindValue
	KindText
	KindComponents
)

type Env struct {
	Clip   context.DataClipboard
	Types  *scene.Types
	Res    asset.Resolver
	OnAdd  func(*schema.TypeDescriptor) func(*scene.Scene, scene.NodeHandle)
	Notify func(string)
}

func EnvOf(ctx *context.Editor) Env {
	e := Env{Clip: ctx.Clipboard, Types: ctx.Types, OnAdd: ctx.OnAdd}
	e.Notify = func(text string) { ctx.Notify(context.LevelWarn, text) }
	if ctx.Loader != nil {
		e.Res = ctx.Loader
	}
	return e
}

type target struct {
	s *scene.Scene
}

func (t target) Scene() *scene.Scene {
	return t.s
}

func (e Env) Peek() Kind {
	c := e.Clip
	if c == nil {
		return KindEmpty
	}
	if d, ok := c.Data(MIME); ok && len(d) > 0 {
		return KindNodes
	}
	if d, ok := c.Data(MIMEComponents); ok && len(d) > 0 {
		return KindComponents
	}
	if d, ok := c.Data(MIMEComponent); ok && len(d) > 0 {
		return KindComponent
	}
	if d, ok := c.Data(MIMEValue); ok && len(d) > 0 {
		return KindValue
	}
	t := strings.TrimSpace(c.Text())
	switch {
	case t == "":
		return KindEmpty
	case strings.HasPrefix(t, "prefab"):
		return KindNodes
	case strings.HasPrefix(t, strings.TrimSpace(componentsText)):
		return KindComponents
	case strings.HasPrefix(t, strings.TrimSpace(componentText)):
		return KindComponent
	}
	return KindText
}

func (e Env) scratch() *scene.Scene {
	return scene.New(e.Types, e.Res, scene.Options{InitialNodes: 4})
}

func componentValues(s *scene.Scene, node ids.NodeID, td *schema.TypeDescriptor) ([]commands.FieldValue, error) {
	a := props.New(s, nil)
	out := make([]commands.FieldValue, 0, len(td.Fields))
	for i := range td.Fields {
		v, err := a.Get(node, td.ID, td.Fields[i].ID)
		if err != nil {
			return nil, err
		}
		out = append(out, commands.FieldValue{Field: td.Fields[i].ID, Value: v})
	}
	return out, nil
}

func (e Env) encodeComponent(td *schema.TypeDescriptor, values []commands.FieldValue) ([]byte, error) {
	sc := e.scratch()
	h := sc.Create(sc.Root(), "component")
	sc.Flush()
	if err := putComponent(sc, h, td, values); err != nil {
		return nil, err
	}
	return sceneio.SaveSubtree(sc, h, true), nil
}

func (e Env) decodeComponent(data []byte) (*schema.TypeDescriptor, []commands.FieldValue, error) {
	sc := e.scratch()
	root, err := loadScratch(sc, data)
	if err != nil {
		return nil, nil, err
	}
	reg := sc.Registry()
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	var td *schema.TypeDescriptor
	sc.ComponentsOf(sc.Handle(root), func(t schema.TypeID) {
		if td != nil || (hasUnknown && t == unknown.ID) {
			return
		}
		td, _ = reg.ByID(t)
	})
	if td == nil {
		return nil, nil, errs.New(errs.EditorCommand, "clipboard component type is not loaded in this editor")
	}
	values, err := componentValues(sc, root, td)
	if err != nil {
		return nil, nil, err
	}
	return td, values, nil
}

func (e Env) CopyComponent(s *scene.Scene, node ids.NodeID, t schema.TypeID) error {
	if e.Clip == nil {
		return errs.New(errs.EditorCommand, "clipboard is unavailable")
	}
	td, ok := s.Registry().ByID(t)
	if !ok {
		return errs.New(errs.SchemaUnknownType, "component type is not registered")
	}
	values, err := componentValues(s, node, td)
	if err != nil {
		return err
	}
	data, err := e.encodeComponent(td, values)
	if err != nil {
		return err
	}
	e.Clip.SetData(MIMEComponent, data, componentText+string(data))
	return nil
}

func (e Env) ReadComponent() (*schema.TypeDescriptor, []commands.FieldValue, bool) {
	c := e.Clip
	if c == nil {
		return nil, nil, false
	}
	data, ok := c.Data(MIMEComponent)
	if !ok || len(data) == 0 {
		t := c.Text()
		if !strings.HasPrefix(t, componentText) {
			return nil, nil, false
		}
		data = []byte(strings.TrimPrefix(t, componentText))
	}
	td, values, err := e.decodeComponent(data)
	if err != nil {
		return nil, nil, false
	}
	return td, values, true
}

func (e Env) PasteComponent(d *document.SceneDocument, nodes []ids.NodeID) error {
	td, values, ok := e.ReadComponent()
	if !ok {
		return errs.New(errs.EditorCommand, "clipboard does not hold a component")
	}
	if len(nodes) == 0 {
		return errs.New(errs.EditorCommand, "select the nodes to paste the component onto")
	}
	tx := d.History.Begin(i18n.Tr("editor.history.paste_component", "Paste {name}", i18n.Str("name", schema.TypeLabel(td))))
	if err := e.pasteInto(tx, d, nodes, td, values); err != nil {
		tx.Abort()
		return err
	}
	tx.Commit()
	return nil
}

func (e Env) CopyValue(s *scene.Scene, node ids.NodeID, t schema.TypeID, f schema.FieldID, text string) error {
	if e.Clip == nil {
		return errs.New(errs.EditorCommand, "clipboard is unavailable")
	}
	td, ok := s.Registry().ByID(t)
	if !ok {
		return errs.New(errs.SchemaUnknownType, "component type is not registered")
	}
	fd, ok := td.FieldByID(f)
	if !ok {
		return errs.New(errs.SchemaBadField, "unknown field")
	}
	values, err := componentValues(s, node, td)
	if err != nil {
		return err
	}
	data, err := e.encodeComponent(td, values)
	if err != nil {
		return err
	}
	payload := append([]byte(fd.Name+"\n"), data...)
	e.Clip.SetData(MIMEValue, payload, text)
	return nil
}

func (e Env) ReadValue(fd *schema.FieldDescriptor) (schema.Value, bool) {
	c := e.Clip
	if c == nil {
		return schema.Value{}, false
	}
	if data, ok := c.Data(MIMEValue); ok && len(data) > 0 {
		name, rest, found := strings.Cut(string(data), "\n")
		if found {
			if td, values, err := e.decodeComponent([]byte(rest)); err == nil {
				for i := range td.Fields {
					src := &td.Fields[i]
					if src.Name != name || !compatible(fd, src) {
						continue
					}
					for _, fv := range values {
						if fv.Field == src.ID {
							return fv.Value, true
						}
					}
				}
			}
		}
	}
	return parseText(fd, strings.TrimSpace(c.Text()))
}

func Peek(ctx *context.Editor) Kind {
	return EnvOf(ctx).Peek()
}

func CopyComponent(ctx *context.Editor, s *scene.Scene, node ids.NodeID, t schema.TypeID) error {
	return EnvOf(ctx).CopyComponent(s, node, t)
}

func ReadComponent(ctx *context.Editor) (*schema.TypeDescriptor, []commands.FieldValue, bool) {
	return EnvOf(ctx).ReadComponent()
}

func PasteComponent(ctx *context.Editor, d *document.SceneDocument, nodes []ids.NodeID) error {
	return EnvOf(ctx).PasteComponent(d, nodes)
}

func CopyValue(ctx *context.Editor, s *scene.Scene, node ids.NodeID, t schema.TypeID, f schema.FieldID, text string) error {
	return EnvOf(ctx).CopyValue(s, node, t, f, text)
}

func ReadValue(ctx *context.Editor, fd *schema.FieldDescriptor) (schema.Value, bool) {
	return EnvOf(ctx).ReadValue(fd)
}

func compatible(a, b *schema.FieldDescriptor) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case schema.KindEnum:
		return a.EnumName == b.EnumName
	case schema.KindStruct:
		return a.Struct == b.Struct
	case schema.KindList:
		return a.Elem == b.Elem && a.Struct == b.Struct
	case schema.KindAssetRef:
		return a.AssetKind == b.AssetKind || a.AssetKind == "" || b.AssetKind == ""
	}
	return true
}

func parseFloat(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
	return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
}

func parseText(fd *schema.FieldDescriptor, s string) (schema.Value, bool) {
	if s == "" {
		return schema.Value{}, false
	}
	switch fd.Kind {
	case schema.KindBool:
		b, err := strconv.ParseBool(strings.ToLower(s))
		return schema.Bool(b), err == nil
	case schema.KindInt32:
		f, ok := parseFloat(s)
		return schema.Int32(int32(math.Round(f))), ok
	case schema.KindInt64:
		f, ok := parseFloat(s)
		return schema.Int64(int64(math.Round(f))), ok
	case schema.KindFloat32:
		f, ok := parseFloat(s)
		return schema.Float32(float32(f)), ok
	case schema.KindFloat64:
		f, ok := parseFloat(s)
		return schema.Float64(f), ok
	case schema.KindString:
		return schema.String(s), true
	case schema.KindVec2:
		parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\t' })
		if len(parts) != 2 {
			return schema.Value{}, false
		}
		x, okx := parseFloat(parts[0])
		y, oky := parseFloat(parts[1])
		return schema.Vec2(mathx.Vec2{X: float32(x), Y: float32(y)}), okx && oky
	case schema.KindColor:
		c, ok := widgets.ParseHexColor(s)
		return schema.Color(c), ok
	case schema.KindEnum:
		for _, ev := range fd.Enum {
			if strings.EqualFold(ev.Name, s) {
				return schema.Enum(ev.Value), true
			}
		}
	}
	return schema.Value{}, false
}
