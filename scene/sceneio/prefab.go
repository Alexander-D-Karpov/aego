package sceneio

import (
	"maps"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/prefab"
	"aego/schema"
)

const PrefabName = "engine.PrefabInstance"

var prefabTypeID = schema.HashName(PrefabName)

type PrefabInstance struct {
	Prefab  ids.AssetID               `aego:"-"`
	Owner   bool                      `aego:"-"`
	Nested  bool                      `aego:"-"`
	Missing bool                      `aego:"-"`
	Keys    map[ids.NodeID]prefab.Key `aego:"-"`
	Orphans prefab.Overrides          `aego:"-"`
	Extra   []*aetext.Node            `aego:"-"`
	Block   *aetext.Node              `aego:"-"`
}

func RegisterPrefab(t *scene.Types) error {
	if err := scene.RegisterComponent[PrefabInstance](t, schema.Name(PrefabName),
		schema.Doc("links a node to the prefab it was created from"), schema.Clone(clonePrefab)); err != nil {
		return err
	}
	if td, ok := t.Schema().ByName(PrefabName); ok {
		td.Hidden = true
	}
	return nil
}

func clonePrefab(dst, src *PrefabInstance) {
	dst.Keys = maps.Clone(src.Keys)
	dst.Orphans = src.Orphans.Clone()
	dst.Extra = cloneNodes(src.Extra)
	dst.Block = src.Block.Clone()
}

func InstanceOf(s *scene.Scene, h scene.NodeHandle) (*PrefabInstance, bool) {
	p, ok := s.GetByType(h, prefabTypeID)
	if !ok {
		return nil, false
	}
	return (*PrefabInstance)(p), true
}

func addInstance(s *scene.Scene, h scene.NodeHandle) (*PrefabInstance, bool) {
	p, err := s.AddByType(h, prefabTypeID)
	if err != nil {
		return nil, false
	}
	return (*PrefabInstance)(p), true
}

func PrefabRoot(s *scene.Scene, h scene.NodeHandle) (scene.NodeHandle, bool) {
	for c := h; !c.IsZero() && c != s.Root() && s.IsAlive(c); c = s.Parent(c) {
		if _, ok := InstanceOf(s, c); ok {
			return c, true
		}
	}
	return scene.NodeHandle{}, false
}

type Template struct {
	Prefab ids.AssetID
	Doc    []byte
}

func Instantiate(s *scene.Scene, parent scene.NodeHandle, t *Template) (scene.NodeHandle, error) {
	created, _, err := LoadInto(s, parent, t.Doc, "prefab "+t.Prefab.Short(), LoadOptions{})
	if err != nil {
		return scene.NodeHandle{}, err
	}
	at := parent
	if at.IsZero() {
		at = s.Root()
	}
	for _, h := range created {
		if s.Parent(h) != at {
			continue
		}
		if pi, ok := addInstance(s, h); ok {
			pi.Prefab, pi.Owner, pi.Nested = t.Prefab, false, false
		}
		return h, nil
	}
	return scene.NodeHandle{}, errs.New(errs.SceneIOParse, "prefab template has no root node", log.F("prefab", t.Prefab.String()))
}
