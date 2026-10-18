package fakephys

import (
	"unsafe"

	"aego/core/rewind"
)

var _ rewind.Keyframer = (*State)(nil)

type stateKey struct {
	ticks int
}

func (k *stateKey) Size() int {
	return int(unsafe.Sizeof(*k))
}

func (s *State) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*stateKey)
	if k == nil {
		k = &stateKey{}
	}
	k.ticks = s.Ticks
	return k
}

func (s *State) Restore(k rewind.Key) {
	s.Ticks = k.(*stateKey).ticks
}

func (s *State) Hash(h *rewind.Hasher) {
	h.String(ID)
	h.Int(s.Ticks)
}
