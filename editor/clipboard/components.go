package clipboard

import (
	"math"
	"strconv"
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/history/commands"
	"aego/editor/props"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/ui/widgets"
)

const (
	MIMEComponent = "application/x-aego-component"
	MIMEValue     = "application/x-aego-value"
	componentText = "aego-component\n"
)

type Kind uint8

const (
	KindEmpty Kind = iota
	KindNodes
	KindComponent
	KindValue
	KindText
)

type target struct {
	s *scene.Scene
}

func (t target) Scene() *scene.Scene {
	return t.s
}

func Peek(ctx *context.Editor) Kind {
	c := ctx.Clipboard
	if c == nil {
		return KindEmpty
	}
	if d, ok := c.Data(MIME); ok && len(d) > 0 {
		return KindNodes
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
	case strings.HasPrefix(t, strings.TrimSpace(componentText)):
		return KindComponent
	}
	return KindText
}

func scratch(ctx *context.Editor) *scene.Scene {
	var res asset.Resolver
	if ctx.Loader != nil {
		res = ctx.Loader
	}
	return scene.New(ctx.Types, res, scene.Options{InitialNodes: 4})
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

func encodeComponent(ctx *context.Editor, td *schema.TypeDescriptor, values []commands.FieldValue) ([]byte, error) {
	sc := scratch(ctx)
	h := sc.Create(sc.Root(), "component")
	sc.Flush()
	add := &commands.AddComponent{Node: sc.ID(h), Type: td.ID, Name: td.FullName, Values: values}
	if err := add.Do(target{sc}); err != nil {
		return nil, err
	}
	return sceneio.SaveSubtree(sc, h, true), nil
}

func decodeComponent(ctx *context.Editor, data []byte) (*schema.TypeDescriptor, []commands.FieldValue, error) {
	sc := scratch(ctx)
	c := &commands.Create{Index: -1, Fragment: data}
	if err := c.Do(target{sc}); err != nil {
		return nil, nil, err
	}
	roots := c.Roots()
	if len(roots) == 0 {
		return nil, nil, errs.New(errs.EditorCommand, "clipboard component is empty")
	}
	reg := sc.Registry()
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	var td *schema.TypeDescriptor
	sc.ComponentsOf(sc.Handle(roots[0]), func(t schema.TypeID) {
		if td != nil || (hasUnknown && t == unknown.ID) {
			return
		}
		td, _ = reg.ByID(t)
	})
	if td == nil {
		return nil, nil, errs.New(errs.EditorCommand, "clipboard component type is not loaded in this editor")
	}
	values, err := componentValues(sc, roots[0], td)
	if err != nil {
		return nil, nil, err
	}
	return td, values, nil
}

func CopyComponent(ctx *context.Editor, s *scene.Scene, node ids.NodeID, t schema.TypeID) error {
	if ctx.Clipboard == nil {
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
	data, err := encodeComponent(ctx, td, values)
	if err != nil {
		return err
	}
	ctx.Clipboard.SetData(MIMEComponent, data, componentText+string(data))
	return nil
}

func ReadComponent(ctx *context.Editor) (*schema.TypeDescriptor, []commands.FieldValue, bool) {
	c := ctx.Clipboard
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
	td, values, err := decodeComponent(ctx, data)
	if err != nil {
		return nil, nil, false
	}
	return td, values, true
}

func PasteComponent(ctx *context.Editor, d *document.SceneDocument, nodes []ids.NodeID) error {
	td, values, ok := ReadComponent(ctx)
	if !ok {
		return errs.New(errs.EditorCommand, "clipboard does not hold a component")
	}
	if len(nodes) == 0 {
		return errs.New(errs.EditorCommand, "select the nodes to paste the component onto")
	}
	s := d.Scene()
	tx := d.History.Begin("Paste " + td.FullName)
	for _, n := range nodes {
		h := s.Handle(n)
		if h.IsZero() || !s.IsAlive(h) {
			continue
		}
		if _, has := s.GetByType(h, td.ID); !has {
			if err := tx.Exec(&commands.AddComponent{Node: n, Type: td.ID, Name: td.FullName, Values: values}); err != nil {
				tx.Abort()
				return err
			}
			continue
		}
		for _, fv := range values {
			fd, ok := td.FieldByID(fv.Field)
			if !ok || fd.Flags.Has(schema.FlagReadOnly) {
				continue
			}
			if err := d.Props.Set(tx, []ids.NodeID{n}, td.ID, fv.Field, fv.Value); err != nil {
				tx.Abort()
				return err
			}
		}
	}
	tx.Commit()
	return nil
}

func CopyValue(ctx *context.Editor, s *scene.Scene, node ids.NodeID, t schema.TypeID, f schema.FieldID, text string) error {
	if ctx.Clipboard == nil {
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
	data, err := encodeComponent(ctx, td, values)
	if err != nil {
		return err
	}
	payload := append([]byte(fd.Name+"\n"), data...)
	ctx.Clipboard.SetData(MIMEValue, payload, text)
	return nil
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

func ReadValue(ctx *context.Editor, fd *schema.FieldDescriptor) (schema.Value, bool) {
	c := ctx.Clipboard
	if c == nil {
		return schema.Value{}, false
	}
	if data, ok := c.Data(MIMEValue); ok && len(data) > 0 {
		name, rest, found := strings.Cut(string(data), "\n")
		if found {
			if td, values, err := decodeComponent(ctx, []byte(rest)); err == nil {
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
