package save

import "aego/core/ids"

const spawnTag = 0x4145535041574e00

func PersistentID(n uint64) ids.NodeID {
	return ids.NodeID{Hi: spawnTag, Lo: n}
}

func IsPersistent(id ids.NodeID) bool {
	return id.Hi == spawnTag
}

func Counter(id ids.NodeID) uint64 {
	if !IsPersistent(id) {
		return 0
	}
	return id.Lo
}
