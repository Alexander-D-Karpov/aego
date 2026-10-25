package sceneio

import (
	"aego/format/aetext"
	"aego/scene"
	"aego/schema"
)

type UnknownComponent struct {
	TypeName string
	Version  int
	Fields   []aetext.Field
	Blocks   []*aetext.Node
}

type Unknown struct {
	Components []UnknownComponent `aego:"-"`
}

func RegisterUnknown(t *scene.Types) error {
	if err := scene.RegisterComponent[Unknown](t,
		schema.Name("engine.Unknown"),
		schema.Doc("Holds components whose type is not registered in this build."), schema.Clone(cloneUnknown)); err != nil {
		return err
	}
	return RegisterPrefab(t)
}

func cloneUnknown(dst, src *Unknown) {
	if src.Components == nil {
		dst.Components = nil
		return
	}
	dst.Components = make([]UnknownComponent, len(src.Components))
	for i, c := range src.Components {
		c.Fields = cloneFields(c.Fields)
		c.Blocks = cloneNodes(c.Blocks)
		dst.Components[i] = c
	}
}

func cloneFields(fs []aetext.Field) []aetext.Field {
	if fs == nil {
		return nil
	}
	out := make([]aetext.Field, len(fs))
	for i, f := range fs {
		out[i] = aetext.Field{Name: f.Name, Value: f.Value.Clone(), Pos: f.Pos}
	}
	return out
}

func cloneNodes(ns []*aetext.Node) []*aetext.Node {
	if ns == nil {
		return nil
	}
	out := make([]*aetext.Node, len(ns))
	for i, n := range ns {
		out[i] = n.Clone()
	}
	return out
}
