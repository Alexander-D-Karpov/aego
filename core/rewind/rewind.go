package rewind

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"math"
)

type Key interface {
	Size() int
}

type Keyframer interface {
	Capture(reuse Key) Key
	Restore(k Key)
	Hash(h *Hasher)
}

type AfterRestorer interface {
	AfterRestore()
}

type Provider interface {
	Keyframer() Keyframer
}

func Of(v any) (Keyframer, bool) {
	if k, ok := v.(Keyframer); ok {
		return k, true
	}
	if p, ok := v.(Provider); ok {
		if k := p.Keyframer(); k != nil {
			return k, true
		}
	}
	return nil, false
}

type Hasher struct {
	h   hash.Hash
	buf [8]byte
}

func NewHasher() *Hasher {
	return &Hasher{h: sha256.New()}
}

func (h *Hasher) Reset() {
	h.h.Reset()
}

func (h *Hasher) U64(v uint64) {
	binary.LittleEndian.PutUint64(h.buf[:], v)
	h.h.Write(h.buf[:])
}

func (h *Hasher) U32(v uint32) {
	h.U64(uint64(v))
}

func (h *Hasher) I64(v int64) {
	h.U64(uint64(v))
}

func (h *Hasher) Int(v int) {
	h.U64(uint64(v))
}

func (h *Hasher) F32(v float32) {
	h.U64(uint64(math.Float32bits(v)))
}

func (h *Hasher) F64(v float64) {
	h.U64(math.Float64bits(v))
}

func (h *Hasher) Bool(v bool) {
	if v {
		h.U64(1)
		return
	}
	h.U64(0)
}

func (h *Hasher) String(s string) {
	h.Int(len(s))
	io.WriteString(h.h, s)
}

func (h *Hasher) Bytes(b []byte) {
	h.Int(len(b))
	h.h.Write(b)
}

func (h *Hasher) Sum() [32]byte {
	var out [32]byte
	h.h.Sum(out[:0])
	return out
}
