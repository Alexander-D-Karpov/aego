package audio

import (
	"bytes"
	"crypto/sha256"
	"slices"
	"strconv"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
)

const (
	clipKey        = "audio.clip@"
	previewClipKey = "audio.preview.clip@"
)

var (
	errNoClip   = errs.New(errs.AssetNotFound, "audio: the clip is nil")
	errNoOutput = errs.New(errs.AssetLoadFailed, "audio: there is no audio output")
	errNoSource = errs.New(errs.AssetNotMounted, "audio: clips need a mounted asset loader")
)

// Clip exposes the decoded clip for inspection. Its artifact, encoded bytes,
// memory and PCM are read-only to callers, and are accessed on the module
// owner thread. Preview checks some accidental changes, not concurrent writes
// or in-place PCM mutations.
type Clip struct {
	ID      ids.AssetID
	Art     *asset.AudioArtifact
	Mem     *mix.Memory
	Bus     int
	Gain    float32
	Seconds float32
	proof   memoryPreviewProof
}

// memoryPreviewProof binds a cached memory decode to the encoded source and
// the structural memory view from which it was made. It does not protect
// against in-place changes to PCM or concurrent mutation of public slices.
type memoryPreviewProof struct {
	checked bool
	valid   bool
	digest  [sha256.Size]byte
	id      ids.AssetID
	source  asset.AudioArtifact // Data is not retained a second time.
	memory  *mix.Memory
	state   mix.Memory
}

func (p *memoryPreviewProof) matches(c *Clip, a *asset.AudioArtifact, digest [sha256.Size]byte) bool {
	if !p.valid || c == nil || c.ID != p.id || a == nil || digest != p.digest || c.Mem != p.memory || c.Mem == nil {
		return false
	}
	want, mem := &p.source, c.Mem
	if a.Format != want.Format || a.Rate != want.Rate || a.Channels != want.Channels || a.Frames != want.Frames ||
		a.Stream != want.Stream || a.Loop != want.Loop || a.LoopStart != want.LoopStart || a.LoopEnd != want.LoopEnd ||
		mem.Rate != p.state.Rate || mem.Channels != p.state.Channels || mem.Loop != p.state.Loop ||
		mem.LoopStart != p.state.LoopStart || mem.LoopEnd != p.state.LoopEnd || len(mem.PCM) != len(p.state.PCM) {
		return false
	}
	return len(mem.PCM) == 0 || &mem.PCM[0] == &p.state.PCM[0]
}

type clipCacheKey struct {
	id      ids.AssetID
	rate    int
	preview bool
}

func busIndex(name string) int {
	if i := slices.Index(Buses, name); i >= 0 {
		return i
	}
	return int(BusSfx)
}

func decodeClip(id ids.AssetID, raw []byte, rate int) (*Clip, error) {
	return decodeClipMode(id, raw, rate, true)
}

func decodeClipMode(id ids.AssetID, raw []byte, rate int, withProof bool) (*Clip, error) {
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
	defer d.Close()
	info := d.Info()
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
	if withProof {
		c.proof.checked = true
		validFormat := a.Format == asset.AudioWAV && wav.Match(a.Data) ||
			a.Format == asset.AudioOgg && len(a.Data) >= 4 && string(a.Data[:4]) == "OggS"
		if validFormat && info.SampleRate == a.Rate && info.Channels == a.Channels && info.Frames == a.Frames &&
			info.Frames > 0 && len(a.Data) <= MaxPrivatePreviewPayloadBytes {
			// Hash only editor/private-preview decodes, not ordinary game loads.
			// The bounded hash is synchronous and must precede public access.
			c.proof = memoryPreviewProof{checked: true, valid: true, digest: sha256.Sum256(a.Data), id: id, source: *a, memory: mem, state: *mem}
			c.proof.source.Data = nil
		}
	}
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

func (m *Module) previewCacheKey(rate int) string {
	if rate != m.previewKeyRate || m.previewKey == "" {
		m.previewKeyRate, m.previewKey = rate, previewClipKey+strconv.Itoa(rate)
	}
	return m.previewKey
}

func buildClip(src asset.AudioSource, id ids.AssetID, rate int, withProof bool) (any, error) {
	raw, ok := src.Audio(id)
	if !ok {
		return nil, errs.New(errs.AssetNotFound, "audio: "+id.Short()+" is not an imported sound", log.F("asset", id.String()))
	}
	return decodeClipMode(id, raw, rate, withProof)
}

func (m *Module) decode(dec asset.Decoder, src asset.AudioSource, id ids.AssetID, rate int, key string, withProof bool) (*Clip, error) {
	if cache, ok := dec.(cachedDecoder); ok {
		if v, hit, err := cache.Cached(id, key); hit {
			if err != nil {
				return nil, err
			}
			return v.(*Clip), nil
		}
	}
	v, err := dec.Decoded(id, key, func() (any, error) { return buildClip(src, id, rate, withProof) })
	if err != nil {
		return nil, err
	}
	return v.(*Clip), nil
}

func (m *Module) clip(res asset.Resolver, id ids.AssetID) (*Clip, error) {
	return m.clipMode(res, id, false)
}

// A game-cached memory clip has no decode-time proof. If the editor opens it
// later, prepare a separate saved-source clip under the same loader's normal
// invalidation lifecycle; never replace the memory used by existing voices.
func (m *Module) clipMode(res asset.Resolver, id ids.AssetID, withProof bool) (*Clip, error) {
	switch {
	case id.IsZero():
		return nil, errNoClip
	case m.mixer == nil:
		return nil, errNoOutput
	}
	rate := m.mixer.SampleRate()
	if raw, ok := asset.BuiltinAudio(id); ok {
		c, err := m.cached(id, rate, false, func() (any, error) { return decodeClipMode(id, raw, rate, withProof) })
		if err != nil || !withProof || c.Mem == nil || c.proof.checked {
			return c, err
		}
		return m.cached(id, rate, true, func() (any, error) { return decodeClipMode(id, raw, rate, true) })
	}
	src, ok := res.(asset.AudioSource)
	if !ok {
		return nil, errNoSource
	}
	dec, ok := res.(asset.Decoder)
	if !ok {
		c, err := m.cached(id, rate, false, func() (any, error) { return buildClip(src, id, rate, withProof) })
		if err != nil || !withProof || c.Mem == nil || c.proof.checked {
			return c, err
		}
		return m.cached(id, rate, true, func() (any, error) { return buildClip(src, id, rate, true) })
	}
	c, err := m.decode(dec, src, id, rate, m.cacheKey(rate), withProof)
	if err != nil || !withProof || c.Mem == nil || c.proof.checked {
		return c, err
	}
	return m.decode(dec, src, id, rate, m.previewCacheKey(rate), true)
}

func (m *Module) cached(id ids.AssetID, rate int, preview bool, build func() (any, error)) (*Clip, error) {
	k := clipCacheKey{id: id, rate: rate, preview: preview}
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
