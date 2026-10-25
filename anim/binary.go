package anim

import (
	"encoding/binary"
	"math"
	"strconv"

	"aego/core/curve"
	"aego/core/errs"
	"aego/core/ids"
	"aego/schema"
)

const (
	binMagic      = "AEAN"
	BinaryVersion = 1
	minTrack      = 14
	minKey        = 78
	minEvent      = 13
)

const (
	flagNumeric = 1 << iota
	flagSimulation
	flagRestore
	flagRoot
)

var le = binary.LittleEndian

func putF32(b []byte, f float32) []byte {
	return le.AppendUint32(b, math.Float32bits(f))
}

func putStr(b []byte, s string) []byte {
	b = le.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

func trackFlags(t *Track) byte {
	var f byte
	if t.Numeric {
		f |= flagNumeric
	}
	if t.Authority == AuthSimulation {
		f |= flagSimulation
	}
	if t.OnEnd == EndRestore {
		f |= flagRestore
	}
	if t.Root {
		f |= flagRoot
	}
	return f
}

func putKey(b []byte, k *Key) []byte {
	b = putF32(b, k.T)
	for _, x := range k.V.L {
		b = putF32(b, x)
	}
	b = le.AppendUint64(b, uint64(k.V.I))
	b = le.AppendUint64(b, k.V.A.Hi)
	b = le.AppendUint64(b, k.V.A.Lo)
	for _, x := range k.In {
		b = putF32(b, x)
	}
	for _, x := range k.Out {
		b = putF32(b, x)
	}
	return append(b, byte(k.Interp), byte(k.Tangent))
}

func Marshal(c *Clip) []byte {
	b := make([]byte, 0, 32+len(c.Tracks)*(minTrack+2*minKey))
	b = append(b, binMagic...)
	b = le.AppendUint32(b, BinaryVersion)
	b = putF32(b, c.Length)
	b = putF32(b, c.FPS)
	b = append(b, byte(c.Loop))
	b = le.AppendUint32(b, uint32(len(c.Tracks)))
	for i := range c.Tracks {
		t := &c.Tracks[i]
		b = putStr(b, t.Target)
		b = putStr(b, t.Property)
		b = append(b, byte(t.Kind), trackFlags(t))
		b = le.AppendUint32(b, uint32(len(t.Keys)))
		for j := range t.Keys {
			b = putKey(b, &t.Keys[j])
		}
	}
	b = le.AppendUint32(b, uint32(len(c.Events)))
	for i := range c.Events {
		e := &c.Events[i]
		b = putF32(b, e.T)
		b = putStr(b, e.Name)
		b = putF32(b, e.Payload)
		b = append(b, byte(e.Mode))
	}
	return b
}

type reader struct {
	b   []byte
	bad bool
}

func (r *reader) take(n int) []byte {
	if r.bad || n < 0 || len(r.b) < n {
		r.bad = true
		return nil
	}
	p := r.b[:n]
	r.b = r.b[n:]
	return p
}

func (r *reader) u8() byte {
	if p := r.take(1); p != nil {
		return p[0]
	}
	return 0
}

func (r *reader) u32() uint32 {
	if p := r.take(4); p != nil {
		return le.Uint32(p)
	}
	return 0
}

func (r *reader) u64() uint64 {
	if p := r.take(8); p != nil {
		return le.Uint64(p)
	}
	return 0
}

func (r *reader) f32() float32 {
	return math.Float32frombits(r.u32())
}

func (r *reader) str() string {
	return string(r.take(int(r.u32())))
}

func (r *reader) count(min int) int {
	n := int(r.u32())
	if r.bad || n*min > len(r.b) {
		r.bad = true
		return 0
	}
	return n
}

func (r *reader) key(k *Key) {
	k.T = r.f32()
	for i := range k.V.L {
		k.V.L[i] = r.f32()
	}
	k.V.I = int64(r.u64())
	hi := r.u64()
	k.V.A = ids.AssetID{Hi: hi, Lo: r.u64()}
	for i := range k.In {
		k.In[i] = r.f32()
	}
	for i := range k.Out {
		k.Out[i] = r.f32()
	}
	k.Interp = curve.Interp(r.u8())
	k.Tangent = curve.Tangent(r.u8())
}

func sized[T any](n int) []T {
	if n == 0 {
		return nil
	}
	return make([]T, n)
}

func badArtifact(msg string) error {
	return errs.New(errs.AssetBadArtifact, msg)
}

func Unmarshal(data []byte) (*Clip, error) {
	if len(data) < 8 || string(data[:4]) != binMagic {
		return nil, badArtifact("not an animation clip artifact")
	}
	r := reader{b: data[4:]}
	if v := r.u32(); v != BinaryVersion {
		return nil, badArtifact("unsupported animation clip artifact version " + strconv.FormatUint(uint64(v), 10))
	}
	c := &Clip{}
	c.Length = r.f32()
	c.FPS = r.f32()
	c.Loop = LoopMode(r.u8())
	c.Tracks = sized[Track](r.count(minTrack))
	for i := range c.Tracks {
		t := &c.Tracks[i]
		t.Target = r.str()
		t.Property = r.str()
		t.Kind = schema.Kind(r.u8())
		f := r.u8()
		t.Numeric = f&flagNumeric != 0
		t.Root = f&flagRoot != 0
		if f&flagSimulation != 0 {
			t.Authority = AuthSimulation
		}
		if f&flagRestore != 0 {
			t.OnEnd = EndRestore
		}
		t.Keys = sized[Key](r.count(minKey))
		for j := range t.Keys {
			r.key(&t.Keys[j])
		}
	}
	c.Events = sized[Event](r.count(minEvent))
	for i := range c.Events {
		e := &c.Events[i]
		e.T = r.f32()
		e.Name = r.str()
		e.Payload = r.f32()
		e.Mode = Mode(r.u8())
		e.ID = -1
	}
	switch {
	case r.bad || len(r.b) != 0:
		return nil, badArtifact("animation clip artifact is truncated or has trailing bytes")
	case c.Loop > LoopPingPong:
		return nil, badArtifact("animation clip artifact has an unknown loop mode")
	}
	for i := range c.Tracks {
		if !Supported(c.Tracks[i].Kind) {
			return nil, badArtifact("animation clip artifact has a track of an unsupported kind")
		}
	}
	return c, nil
}
