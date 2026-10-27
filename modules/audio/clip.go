package audio

import (
	"bytes"
	"slices"
	"strconv"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/mix"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
)

const clipKey = "audio.clip@"

var (
	errNoClip   = errs.New(errs.AssetNotFound, "audio: the clip is nil")
	errNoOutput = errs.New(errs.AssetLoadFailed, "audio: there is no audio output")
	errNoSource = errs.New(errs.AssetNotMounted, "audio: clips need a mounted asset loader")
)

type Clip struct {
	ID      ids.AssetID
	Art     *asset.AudioArtifact
	Mem     *mix.Memory
	Bus     int
	Gain    float32
	Seconds float32
}

type clipCacheKey struct {
	id   ids.AssetID
	rate int
}

func busIndex(name string) int {
	if i := slices.Index(Buses, name); i >= 0 {
		return i
	}
	return int(BusSfx)
}

func decodeClip(id ids.AssetID, raw []byte, rate int) (*Clip, error) {
	a, err := asset.DecodeAudio(raw)
	if err != nil {
		return nil, err
	}
	c := &Clip{ID: id, Art: a, Bus: busIndex(a.Bus), Gain: a.Gain, Seconds: float32(a.Frames) / float32(a.Rate)}
	if a.Stream {
		return c, nil
	}
	d, err := codec.Open(bytes.NewReader(a.Data), "")
	if err != nil {
		return nil, err
	}
	mem, err := codec.DecodeAll(d, rate)
	if err != nil {
		return nil, err
	}
	if a.LoopEnd > 0 && a.LoopEnd <= a.Frames && a.LoopStart < a.LoopEnd {
		mem.LoopStart = a.LoopStart * int64(mem.Rate) / int64(a.Rate)
		mem.LoopEnd = a.LoopEnd * int64(mem.Rate) / int64(a.Rate)
	}
	mem.Loop = a.Loop
	c.Mem = mem
	return c, nil
}

func (c *Clip) stream(loop bool) (*mix.Stream, chan struct{}, error) {
	a := c.Art
	d, err := codec.Open(bytes.NewReader(a.Data), "")
	if err != nil {
		return nil, nil, err
	}
	st := mix.NewStream(a.Rate, a.Channels, a.Rate)
	cancel := make(chan struct{})
	o := codec.StreamOptions{Loop: loop}
	if a.LoopEnd > 0 && a.LoopEnd <= a.Frames && a.LoopStart < a.LoopEnd {
		o.LoopStart, o.LoopEnd = a.LoopStart, a.LoopEnd
	}
	codec.Stream(d, st, o, cancel)
	return st, cancel, nil
}

type cachedDecoder interface {
	Cached(id ids.AssetID, key string) (any, bool, error)
}

func (m *Module) cacheKey(rate int) string {
	if rate != m.keyRate || m.key == "" {
		m.keyRate, m.key = rate, clipKey+strconv.Itoa(rate)
	}
	return m.key
}

func buildClip(src asset.AudioSource, id ids.AssetID, rate int) (any, error) {
	raw, ok := src.Audio(id)
	if !ok {
		return nil, errs.New(errs.AssetNotFound, "audio: "+id.Short()+" is not an imported sound", log.F("asset", id.String()))
	}
	return decodeClip(id, raw, rate)
}

func (m *Module) decode(dec asset.Decoder, src asset.AudioSource, id ids.AssetID, rate int, key string) (*Clip, error) {
	v, err := dec.Decoded(id, key, func() (any, error) { return buildClip(src, id, rate) })
	if err != nil {
		return nil, err
	}
	return v.(*Clip), nil
}

func (m *Module) clip(res asset.Resolver, id ids.AssetID) (*Clip, error) {
	switch {
	case id.IsZero():
		return nil, errNoClip
	case m.mixer == nil:
		return nil, errNoOutput
	}
	rate := m.mixer.SampleRate()
	if raw, ok := asset.BuiltinAudio(id); ok {
		return m.cached(id, rate, func() (any, error) { return decodeClip(id, raw, rate) })
	}
	src, ok := res.(asset.AudioSource)
	if !ok {
		return nil, errNoSource
	}
	dec, ok := res.(asset.Decoder)
	if !ok {
		return m.cached(id, rate, func() (any, error) { return buildClip(src, id, rate) })
	}
	key := m.cacheKey(rate)
	if c, ok := dec.(cachedDecoder); ok {
		if v, hit, err := c.Cached(id, key); hit {
			if err != nil {
				return nil, err
			}
			return v.(*Clip), nil
		}
	}
	return m.decode(dec, src, id, rate, key)
}

func (m *Module) cached(id ids.AssetID, rate int, build func() (any, error)) (*Clip, error) {
	k := clipCacheKey{id: id, rate: rate}
	if c, ok := m.clips[k]; ok {
		return c, nil
	}
	v, err := build()
	if err != nil {
		return nil, err
	}
	if m.clips == nil {
		m.clips = map[clipCacheKey]*Clip{}
	}
	c := v.(*Clip)
	m.clips[k] = c
	return c, nil
}

func (m *Module) warnAsset(id ids.AssetID, err error) {
	if m.warned[id] || m.log == nil {
		return
	}
	if m.warned == nil {
		m.warned = map[ids.AssetID]bool{}
	}
	m.warned[id] = true
	m.log.Warn("cannot play a sound", log.F("asset", id.String()), log.F("error", err.Error()))
}
