package prefab

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"

	"aego/core/ids"
)

const MaxDepth = 8

type Key string

func LocalKey(id ids.NodeID) Key {
	return Key(id.String())
}

func ParseKey(s string) (Key, bool) {
	if s == "" {
		return "", false
	}
	for _, p := range strings.Split(s, "/") {
		if _, err := ids.ParseNodeID(p); err != nil {
			return "", false
		}
	}
	return Key(s), true
}

func (k Key) Nested() bool {
	return strings.IndexByte(string(k), '/') >= 0
}

func (k Key) Instance() Key {
	s := string(k)
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return Key(s[:i])
	}
	return ""
}

func (k Key) Local() (ids.NodeID, bool) {
	s := string(k)
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	id, err := ids.ParseNodeID(s)
	return id, err == nil
}

func (k Key) ID() ids.NodeID {
	if !k.Nested() {
		if id, err := ids.ParseNodeID(string(k)); err == nil {
			return id
		}
	}
	sum := sha256.Sum256([]byte("aego.prefab/" + string(k)))
	return ids.NodeID{Hi: binary.BigEndian.Uint64(sum[0:8]), Lo: binary.BigEndian.Uint64(sum[8:16])}
}

func Rekey(k, root, at Key) Key {
	if k == root {
		return at
	}
	return at + "/" + k
}
