package prefabs

import (
	"aego/core/ids"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

func NearestForTest(lib *sceneio.Library, id ids.AssetID, key prefab.Key) (ids.AssetID, bool) {
	return newBaker(lib).Nearest(id, key)
}
