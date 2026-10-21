package audio

import (
	"errors"
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
	m.StopPreview()
	c, err := m.clip(res, id)
	if err != nil {
		return err
	}
	p := mix.PlayParams{Bus: int(BusUI), Gain: c.Gain, Pitch: 1, Loop: loop, Priority: musicPriority}
	if c.Mem != nil {
		p.Memory = c.Mem
	} else {
		st, cancel, err := c.stream(loop)
		if err != nil {
			return err
		}
		p.Stream, m.previewCancel = st, cancel
	}
	m.preview, m.previewID = m.mixer.Play(p), id
	return nil
}

func (m *Module) StopPreview() {
	if m.preview.Valid() && m.mixer != nil {
		m.mixer.Stop(m.preview, 0)
	}
	m.endPreview()
}

func (m *Module) endPreview() {
	if m.previewCancel != nil {
		close(m.previewCancel)
		m.previewCancel = nil
	}
	m.preview, m.previewID = mix.VoiceID{}, ids.AssetID{}
}

func (m *Module) PreviewClip() ids.AssetID {
	return m.previewID
}

func (m *Module) PreviewPosition() (float32, bool) {
	if m.mixer == nil || !m.mixer.Live(m.preview) {
		return 0, false
	}
	return m.mixer.Position(m.preview), true
}

func (m *Module) Clip(res asset.Resolver, id ids.AssetID) (*Clip, error) {
	return m.clip(res, id)
}
