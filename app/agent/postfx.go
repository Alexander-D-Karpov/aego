package agent

import (
	"fmt"
	"slices"
	"unsafe"

	"aego/modules/render2d"
	"aego/render/postfx"
	"aego/render/shader"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

func postEffectCommands() []*Command {
	instance := Arg{Name: "instance", Type: "string", Doc: "stable ID of the effect in the camera's stack", Required: true, Pos: true}
	field := Arg{Name: "field", Type: "string", Doc: "enabled or a descriptor parameter such as Intensity", Required: true, Pos: true}
	return []*Command{
		{Name: "postfx.get", Doc: "read the authored camera stack including stable instance IDs from the live scene", Args: []Arg{argFile, argNode}, Run: (*Session).postEffectGet},
		{Name: "postfx.set", Doc: "set an effect switch or typed numeric parameter by persistent instance ID in one scene edit", Write: true, Args: []Arg{argFile, argNode, instance, field, argValue}, Run: (*Session).postEffectSet},
	}
}

func cameraStack(doc *mutate.Document, node string) (*render2d.PostProcess, error) {
	h, err := doc.Node(node)
	if err != nil {
		return nil, err
	}
	c, ok := scene.Get[render2d.PostProcess](doc.Scene, h)
	if !ok {
		return nil, fmt.Errorf("camera %q has no PostProcess component", node)
	}
	return c, nil
}

func (s *Session) postEffectGet(a Args) (*Output, error) {
	_, _, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	stack, err := cameraStack(doc, a.Str("node"))
	if err != nil {
		return nil, err
	}
	return &Output{Value: stack.Clone()}, nil
}

func (s *Session) postEffectSet(a Args) (*Output, error) {
	rel, src, doc, err := s.scene(a)
	if err != nil {
		return nil, err
	}
	c, err := cameraStack(doc, a.Str("node"))
	if err != nil {
		return nil, err
	}
	draft := c.Clone()
	id, field := a.Str("instance"), a.Str("field")
	e, ok := draft.Effect(id)
	if !ok {
		return nil, fmt.Errorf("unresolved effect instance %q", id)
	}
	if field == "enabled" {
		v, err := doc.TypedLiteral(schema.KindBool, a.Str("value"))
		if err != nil {
			return nil, err
		}
		e.Enabled = v.AsBool()
	} else {
		svc, _ := s.Modules.RegisteredService(render2d.PostEffectsService)
		reg, ok := svc.(*postfx.Registry)
		if !ok {
			return nil, fmt.Errorf("post effects registry unavailable")
		}
		d, ok := reg.Lookup(e.TypeID)
		if !ok || d.Version != e.Version {
			return nil, fmt.Errorf("unsupported %s version %d", e.TypeID, e.Version)
		}
		i := slices.IndexFunc(d.Fields, func(f postfx.Field) bool { return f.Name == field })
		if i < 0 {
			return nil, fmt.Errorf("%s has no parameter %q", e.TypeID, field)
		}
		f := &d.Fields[i]
		v, err := doc.TypedLiteral(render2d.ParameterKind(&shader.Param{Kind: f.Kind}), a.Str("value"))
		if err != nil {
			return nil, err
		}
		if err := render2d.SetEffectValue(e, f, v); err != nil {
			return nil, err
		}
	}

	td, _ := doc.Scene.Registry().ByName(render2d.NamePostProcess)
	fd, _ := td.Field("effects")
	var v schema.Value
	fd.Get(unsafe.Pointer(&draft), &v)
	op := mutate.SetValue(a.Str("node"), render2d.NamePostProcess+".effects", sceneio.WriteValue(doc.Scene.Registry(), fd, v))
	return s.applyDoc(a, rel, src, doc, mutate.Tx{Label: "Set post effect " + field, Ops: []mutate.Op{op}})
}
