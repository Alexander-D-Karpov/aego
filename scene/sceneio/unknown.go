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
		schema.Doc("holds components whose type is not registered in this build")); err != nil {
		return err
	}
	return RegisterPrefab(t)
}
