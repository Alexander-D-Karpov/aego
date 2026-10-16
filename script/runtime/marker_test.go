package runtime

import (
	"aego/engine/module"
	"aego/scene"
	"aego/schema"
)

type Marker struct {
	Order int32 `aego:"order"`
	FlipX bool  `aego:"flipX"`
}

type markerModule struct{}

func (markerModule) Info() module.Info {
	return module.Info{ID: "test"}
}

func (markerModule) Register(r *module.Registry) error {
	return module.Component[Marker](r, schema.Name("test.Marker")).Err()
}

func registerMarker(types *scene.Types) error {
	_, err := module.Load(types, nil, markerModule{})
	return err
}
