package persistence

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/format/aetext"
	"aego/save"
	"aego/scene"
	"aego/script/abi"
	"aego/script/vm"
)

const (
	ServiceID = "save.service"
	HostID    = "save.host"
	StoreID   = "save.store"
	ScriptsID = "script.persist"
	SpawnerID = "spawn.service"
)

type Host interface {
	SceneID() ids.AssetID
	LoadScene(scene ids.AssetID) error
}

type Section interface {
	Version() int
	Save(w *engine.World, out *save.Writer) error
	Load(w *engine.World, in *save.Reader) error
	Migrate(n *aetext.Node, from, to int) error
}

type ScriptValue struct {
	Script string
	Field  string
	Kind   abi.Kind
	Value  vm.Value
	Text   string
}

type Scripts interface {
	Prepare()
	Persisted(node scene.NodeHandle, dst []ScriptValue) []ScriptValue
	RestoreField(node scene.NodeHandle, v ScriptValue) bool
}

type Spawner interface {
	EachSpawned(fn func(root scene.NodeHandle, prefab ids.AssetID, pid ids.NodeID))
	PersistentID(h scene.NodeHandle) (ids.NodeID, bool)
	ByPersistentID(pid ids.NodeID) (scene.NodeHandle, bool)
	SpawnAs(prefab ids.AssetID, parent scene.NodeHandle, pos mathx.Vec2, rot float32, pid ids.NodeID) (scene.NodeHandle, error)
	Counter() uint64
	SetCounter(n uint64)
}

type Service interface {
	Save(slot string) error
	Load(slot string) error
	Checkpoint() error
	SetAutosave(seconds float64)
}
