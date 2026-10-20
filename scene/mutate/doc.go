package mutate

import (
	"bytes"

	"aego/asset"
	"aego/core/ids"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

const RootName = "Root"

type Env struct {
	Types    *scene.Types
	Resolver asset.Resolver
	Library  *sceneio.Library
	Assets   func(ids.AssetID) bool
	Lookup   func(path string) (ids.AssetID, bool)
	OnAdd    func(td *schema.TypeDescriptor, s *scene.Scene, h scene.NodeHandle)
	OnRemove func(td *schema.TypeDescriptor, s *scene.Scene, h scene.NodeHandle)
}

type Problem struct {
	Error   bool   `json:"error"`
	Node    string `json:"node,omitempty"`
	Message string `json:"message"`
}

type Document struct {
	Name     string
	Prefab   bool
	Scene    *scene.Scene
	Problems []Problem
	env      Env
	reg      *schema.Registry
}

func isPrefab(src []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(src), []byte("prefab"))
}

func Load(src []byte, name string, env Env) (*Document, error) {
	s, rep, err := sceneio.Load(src, name, env.Types, env.Resolver, sceneio.LoadOptions{KeepIDs: true, Prefabs: env.Library})
	if err != nil {
		return nil, err
	}
	d := &Document{Name: name, Prefab: isPrefab(src), Scene: s, env: env, reg: s.Registry()}
	for _, w := range rep.Warnings {
		d.Problems = append(d.Problems, Problem{Node: w.Path, Message: w.Msg})
	}
	for _, u := range rep.Unknown {
		d.Problems = append(d.Problems, Problem{Message: "component " + u + " is not registered; it is kept unchanged"})
	}
	return d, nil
}

func Wrap(s *scene.Scene, name string, prefab bool, env Env) *Document {
	return &Document{Name: name, Prefab: prefab, Scene: s, env: env, reg: s.Registry()}
}

func (d *Document) Bytes() []byte {
	if d.Prefab {
		return sceneio.SavePrefab(d.Scene)
	}
	return sceneio.Save(d.Scene)
}

func (d *Document) library() *sceneio.Library {
	if d.env.Library == nil {
		d.env.Library = sceneio.NewLibrary(d.reg, d.Scene.Resolver())
	}
	return d.env.Library
}

func (d *Document) assetID(text string) (ids.AssetID, error) {
	if id, err := ids.ParseAssetID(text); err == nil {
		return id, nil
	}
	if d.env.Lookup != nil {
		if id, ok := d.env.Lookup(text); ok {
			return id, nil
		}
	}
	return ids.AssetID{}, errNoAsset(text)
}
