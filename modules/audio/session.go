package audio

import (
	"crypto/sha256"
	"errors"
	"math"
	"slices"

	"aego/asset"
	"aego/audio/mix"
	"aego/core/ids"
	"aego/engine/module"
	"aego/schema"
)

func (m *Module) busOK(b Bus) bool {
	return int(b) < busCount
}

func (m *Module) applySession(b Bus) {
	if m.mixer == nil || !m.busOK(b) {
		return
	}
	g := m.session[b]
	if m.smute[b] {
		g = 0
	}
	m.mixer.SetBusLayer(int(b), mix.LayerSession, g, 0)
}

func (m *Module) BusVolume(b Bus) float32 {
	if !m.busOK(b) {
		return 0
	}
	return m.session[b]
}

func (m *Module) SetBusVolume(b Bus, v float32) {
	if !m.busOK(b) || v != v {
		return
	}
	m.session[b] = max(v, 0)
	m.applySession(b)
}

func (m *Module) BusMuted(b Bus) bool {
	return m.busOK(b) && m.smute[b]
}

func (m *Module) SetBusMuted(b Bus, on bool) {
	if !m.busOK(b) {
		return
	}
	m.smute[b] = on
	m.applySession(b)
}

func (m *Module) enabledApply(schema.Value) error {
	m.syncEnabled()
	return nil
}

func (m *Module) syncEnabled() {
	if m.mixer == nil || m.st == nil || m.enabled == 0 {
		return
	}
	if m.st.Bool(m.enabled) {
		m.mixer.ResumeAll()
	} else {
		m.mixer.PauseAll()
	}
}

var errNoSettings = errors.New("audio: the settings are not available")

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if ctx.World == nil {
		return nil
	}
	ctx.World.SetView(MixerID, m)
	if !ctx.Play || ctx.Scene == nil {
		return nil
	}
	s := NewService(m, ctx.Scene, ctx.World)
	ctx.World.SetService(ServiceID, s)
	if s.hub != nil {
		s.hub.Probe(typeSound, s.alive)
	}
	m.worlds = append(m.worlds, s)
	return nil
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if ctx.World == nil {
		return
	}
	if ctx.Play {
		if s := Of(ctx.World); s != nil {
			s.close()
			if s.hub != nil {
				s.hub.Probe(typeSound, nil)
			}
			m.worlds = slices.DeleteFunc(m.worlds, func(x *Service) bool { return x == s })
		}
		ctx.World.RemoveService(ServiceID)
	}
	ctx.World.RemoveService(MixerID)
}

func (m *Module) BusSolo(b Bus) bool {
	return m.busOK(b) && m.solo[b]
}

func (m *Module) SetBusSolo(b Bus, on bool) {
	if !m.busOK(b) || m.mixer == nil {
		return
	}
	m.solo[b] = on
	m.mixer.SetBusSolo(int(b), on)
}

func (m *Module) ResetSession() {
	for i := range m.session {
		m.session[i], m.smute[i] = 1, false
		m.applySession(Bus(i))
		if m.solo[i] {
			m.SetBusSolo(Bus(i), false)
		}
	}
}

func (m *Module) SaveSession() error {
	if m.st == nil || len(m.vol) != len(Buses) {
		return errNoSettings
	}
	tx := m.st.Begin()
	for i, b := range Buses {
		v := min(m.st.Float(m.vol[i])*float64(m.session[i]), 1)
		if err := tx.SetID(VolumeKey(b), schema.Float64(v)); err != nil {
			return err
		}
	}
	if err := tx.Apply(); err != nil {
		return err
	}
	for i := range m.session {
		m.session[i] = 1
		m.applySession(Bus(i))
	}
	return nil
}

func (m *Module) Worlds() []*Service {
	return m.worlds
}

func (m *Module) Preview(res asset.Resolver, id ids.AssetID, loop bool) error {
	c, err := m.clipMode(res, id, true)
	if err != nil {
		return err
	}
	return m.previewClip(c, loop)
}

func (m *Module) previewClip(c *Clip, loop bool) error {
	// Validate before stopping the current preview: a cached decoded memory
	// clip may no longer correspond to the publicly exposed source bytes.
	var snapshot asset.AudioArtifact
	var digest [sha256.Size]byte
	hasArtifact := c.Art != nil
	guard := false
	if hasArtifact {
		snapshot = *c.Art
		guard = len(snapshot.Data) > 0 && len(snapshot.Data) <= MaxPrivatePreviewPayloadBytes
		if guard {
			digest = sha256.Sum256(snapshot.Data)
		}
	}
	if c.proof.valid && (!guard || !c.proof.matches(c, &snapshot, digest)) {
		return ErrPreviewSeekSourceChanged
	}
	if c.Mem != nil && !c.proof.valid {
		// Without a decode-time proof, a hash first taken at Play cannot prove
		// that cached PCM was produced from these encoded bytes.
		guard = false
	}
	if err := m.StopPreview(); err != nil {
		return err
	}
	p := mix.PlayParams{Bus: int(BusUI), Gain: c.Gain, Pitch: 1, Loop: loop, Priority: musicPriority}
	var cancel chan struct{}
	if c.Mem != nil {
		p.Memory = c.Mem
	} else {
		var st *mix.Stream
		var err error
		st, cancel, err = c.stream(loop)
		if err != nil {
			return err
		}
		p.Stream = st
	}
	voice := m.mixer.Play(p)
	if !voice.Valid() {
		if cancel != nil {
			close(cancel)
		}
		return errors.New("audio: could not queue preview playback")
	}
	m.preview, m.previewID, m.previewCancel = voice, c.ID, cancel
	if hasArtifact {
		m.previewArtifact = &snapshot
		m.previewDigest, m.previewGuard = digest, guard
	}
	m.previewGain, m.previewLoop = c.Gain, loop
	if c.Mem != nil {
		m.previewFrames, m.previewRate = c.Mem.Frames(), c.Mem.Rate
		if (loop || c.Mem.Loop) && c.Mem.LoopEnd > 0 && c.Mem.LoopEnd < m.previewFrames {
			m.previewFrames = c.Mem.LoopEnd
		}
	}
	m.bumpPreviewEpoch()
	return nil
}

// StopPreview relinquishes ownership only after Stop has been queued, or the
// voice is already dead. If the bounded queue is full the caller can retry;
// a live stream is not cancelled while its voice is still playing.
func (m *Module) StopPreview() error {
	m.cancelStreamSeekPreparation()
	m.reconcileStreamSeek()
	if m.mixer != nil {
		if p := m.streamSeek.pending; p != nil {
			if (m.mixer.Live(p.old) || m.mixer.Live(p.next)) && !m.mixer.StopPair(p.old, p.next, 0) {
				return errors.New("audio: preview command queue is full")
			}
		} else if m.mixer.Live(m.preview) && !m.mixer.Stop(m.preview, 0) {
			return errors.New("audio: preview command queue is full")
		}
	}
	m.endPreview()
	return nil
}

func (m *Module) closePreviewFeeder() {
	if m.previewClose != nil {
		m.previewClose()
		m.streamSeek.track(m.previewDone)
	} else if m.previewCancel != nil {
		close(m.previewCancel)
	}
	m.previewCancel, m.previewClose, m.previewDone = nil, nil, nil
}

func (m *Module) endPreview() {
	m.cancelStreamSeekPreparation()
	if p := m.streamSeek.pending; p != nil {
		p.prepared.cancel()
		m.streamSeek.track(p.prepared.done)
		m.streamSeek.pending = nil
	}
	m.closePreviewFeeder()
	m.preview, m.previewID = mix.VoiceID{}, ids.AssetID{}
	m.previewPaused = false
	m.previewFrames, m.previewRate = 0, 0
	m.previewArtifact, m.previewDigest, m.previewGuard = nil, [32]byte{}, false
	m.previewGain, m.previewLoop, m.previewTime = 0, false, nil
	m.bumpPreviewEpoch()
}

func (m *Module) bumpPreviewEpoch() {
	if m.previewEpoch < ^uint64(0) {
		m.previewEpoch++
	}
}

// PreviewEpoch changes when accepted preview ownership/commands or output
// reopen change. It is read on the module owner thread. The saturated value
// ^uint64(0) cannot guard asynchronous work and must be treated as stale.
func (m *Module) PreviewEpoch() uint64 {
	if m == nil {
		return ^uint64(0)
	}
	return m.previewEpoch
}

func (m *Module) PreviewClip() ids.AssetID {
	return m.previewID
}

// PreviewSourceMatches checks whether a displayed clip has the same in-memory
// source bytes and layout as the current preview, even for a private preview
// with different import settings. It does not check the disk or provide CAS.
func (m *Module) PreviewSourceMatches(c *Clip) bool {
	if m == nil || m.mixer == nil || !m.mixer.Live(m.preview) || c == nil || c.ID != m.previewID ||
		m.previewArtifact == nil || !m.previewGuard || c.Art == nil {
		return false
	}
	a, b := m.previewArtifact, c.Art
	return a.Format == b.Format && a.Rate == b.Rate && a.Channels == b.Channels && a.Frames == b.Frames &&
		len(b.Data) > 0 && len(b.Data) <= MaxPrivatePreviewPayloadBytes && sha256.Sum256(b.Data) == m.previewDigest
}

func (m *Module) PreviewPosition() (float32, bool) {
	if m.mixer == nil || !m.mixer.Live(m.preview) {
		return 0, false
	}
	position := m.mixer.Position(m.preview)
	if view := m.previewTime; view != nil && view.rate > 0 {
		frame := float64(view.origin) + float64(position)*float64(view.rate)
		if view.loop && view.loopEnd > view.loopStart && frame >= float64(view.loopEnd) {
			frame = float64(view.loopStart) + math.Mod(frame-float64(view.loopStart), float64(view.loopEnd-view.loopStart))
		}
		return float32(frame / float64(view.rate)), true
	}
	return position, true
}

// PreviewMemorySeekRange returns the exclusive playable frame bound and sample
// rate of the current memory preview. Streamed previews are not seekable by
// this API. The frame coordinates refer to the decoded (possibly resampled)
// memory, not necessarily to the original asset's sample rate.
func (m *Module) PreviewMemorySeekRange() (frames int64, rate int, ok bool) {
	if m == nil || m.mixer == nil || !m.mixer.Live(m.preview) || m.previewRate <= 0 || m.previewFrames <= 0 {
		return 0, 0, false
	}
	return min(m.previewFrames, 1<<53+1), m.previewRate, true
}

// SeekPreviewMemory queues a frame position for the current memory preview,
// without affecting scene/game voices or its paused state. A nil error means
// the command was accepted by the bounded queue, not that it has reached the
// mixer or that previously buffered output has drained.
func (m *Module) SeekPreviewMemory(frame int64) error {
	frames, _, ok := m.PreviewMemorySeekRange()
	if !ok {
		return errors.New("audio: there is no active memory preview to seek")
	}
	if frame < 0 || frame >= frames {
		return errors.New("audio: preview seek frame is outside the playable range")
	}
	if !m.mixer.SeekMemory(m.preview, frame) {
		return errors.New("audio: preview command queue is full")
	}
	m.bumpPreviewEpoch()
	return nil
}

// SetPreviewPaused queues a pause or resume for the preview voice only. The
// state represents an accepted command, not confirmation that previously
// buffered output has drained. Scene and game voices are unaffected.
func (m *Module) SetPreviewPaused(paused bool) error {
	m.reconcileStreamSeek()
	if m.streamSeek.pending != nil {
		return errors.New("audio: a stream seek is awaiting the mixer")
	}
	if m.mixer == nil || !m.mixer.Live(m.preview) {
		return errors.New("audio: there is no active preview to pause or resume")
	}
	if m.previewPaused == paused {
		return nil
	}
	var queued bool
	if paused {
		queued = m.mixer.Pause(m.preview)
	} else {
		queued = m.mixer.Resume(m.preview)
	}
	if !queued {
		return errors.New("audio: preview command queue is full")
	}
	m.cancelStreamSeekPreparation()
	m.previewPaused = paused
	m.bumpPreviewEpoch()
	return nil
}

// PreviewPaused reports the requested state of a still-live preview voice.
func (m *Module) PreviewPaused() bool {
	return m.mixer != nil && m.mixer.Live(m.preview) && m.previewPaused
}

// Clip returns the source for editor inspection. If a memory clip was already
// decoded for game playback without an editor proof, it decodes a separate
// cached view rather than replacing memory used by live game voices.
func (m *Module) Clip(res asset.Resolver, id ids.AssetID) (*Clip, error) {
	return m.clipMode(res, id, true)
}
