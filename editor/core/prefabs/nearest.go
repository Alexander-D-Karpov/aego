package prefabs

import (
	"aego/core/ids"
	"aego/project/refactor"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

func NearestForTest(lib *sceneio.Library, id ids.AssetID, key prefab.Key) (ids.AssetID, bool) {
	return refactor.Nearest(lib, id, key)
}
