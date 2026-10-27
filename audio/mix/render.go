package mix

import (
	"math"

	"aego/audio/dsp"
)

func (m *Mixer) Mix(out []float32) {
	m.drain()
	for len(out) >= 2 {
		n := min(len(out)/2, m.cfg.Block)
		m.chunk(out[:n*2], n)
		out = out[n*2:]
	}
}

func (m *Mixer) drain() {
	for {
		c, ok := m.cmds.q.pop()
		if !ok {
			return
		}
		m.apply(&c)
	}
}

func (m *Mixer) apply(c *Command) {
	m.applied++
	switch c.Kind {
	case CmdPlay:
		m.play(c)
	case CmdBusGain, CmdBusMute, CmdBusSolo, CmdBusLayer:
		i := int(c.Bus)
		if i < 0 || i >= len(m.buses) {
			return
		}
		b := &m.buses[i]
		switch c.Kind {
		case CmdBusGain:
			b.user = max(c.Gain, 0)
			m.retarget(i, m.seconds(0, busRamp))
		case CmdBusLayer:
			g := max(c.Gain, 0)
			switch c.Layer {
			case LayerSession:
				b.session = g
			case LayerWorld:
				b.world = g
			default:
				b.user = g
			}
			m.retarget(i, m.seconds(c.Fade, busRamp))
		case CmdBusMute:
			b.mute = c.Flag
			m.retarget(i, m.seconds(0, busRamp))
		default:
			b.solo = c.Flag
			m.resolo()
		}
	case CmdPauseAll:
		m.paused = true
	case CmdResumeAll:
		m.paused = false
	default:
		m.voiceCmd(c)
	}
}

func (m *Mixer) seconds(s, def float32) float64 {
	if s <= 0 {
		s = def
	}
	return float64(s) * m.rate
}

func (m *Mixer) find(id VoiceID) int {
	if int(id.Index) >= len(m.handles) {
		return -1
	}
	h := &m.handles[id.Index]
	if h.gen.Load() != id.Gen {
		return -1
	}
	return int(h.voice)
}

func (m *Mixer) voiceCmd(c *Command) {
	vi := m.find(c.Voice)
	if vi < 0 {
		return
	}
	v := &m.voices[vi]
	switch c.Kind {
	case CmdKill:
		m.end(vi, EvFinished)
	case CmdStop:
		if v.state == voicePaused {
			m.end(vi, EvFinished)
			return
		}
		v.stopping = true
		v.rampTo(0, m.seconds(c.Fade, gainRamp))
	case CmdSetGain:
		if !v.stopping {
			v.rampTo(max(c.Gain, 0), m.seconds(c.Fade, gainRamp))
		}
	case CmdSetPan:
		v.setPan(c.Pan)
	case CmdSetPitch:
		v.setPitch(c.Pitch)
	case CmdPause:
		if v.state == voicePlaying {
			v.state = voicePaused
		}
	case CmdResume:
		if v.state == voicePaused {
			v.state = voicePlaying
		}
	}
}

func (m *Mixer) playable(c *Command) bool {
	if c.Bus < 0 || int(c.Bus) >= len(m.buses) {
		return false
	}
	switch {
	case c.Memory != nil && c.Stream == nil:
		s := c.Memory
		return (s.Channels == 1 || s.Channels == 2) && s.Rate > 0 && float64(s.Rate) <= MaxRateRatio*m.rate && s.Frames() > 0
	case c.Stream != nil && c.Memory == nil:
		s := c.Stream
		return (s.Channels == 1 || s.Channels == 2) && s.Rate > 0 && float64(s.Rate) <= MaxRateRatio*m.rate && !s.inUse
	}
	return false
}

func (m *Mixer) play(c *Command) {
	id := c.Voice
	if int(id.Index) >= len(m.handles) {
		return
	}
	h := &m.handles[id.Index]
	if h.gen.Load() != id.Gen || h.voice >= 0 {
		return
	}
	if !m.playable(c) {
		m.reject(id)
		return
	}
	vi := m.take(c.Priority)
	if vi < 0 {
		m.reject(id)
		return
	}
	v := &m.voices[vi]
	*v = voice{id: id, state: voicePlaying, mem: c.Memory, st: c.Stream, bus: int(c.Bus), priority: c.Priority, age: m.ages}
	m.ages++
	if c.Memory != nil {
		v.ch = c.Memory.Channels
		v.ratio = float64(c.Memory.Rate) / m.rate
		v.frames = c.Memory.Frames()
		v.ls, v.le = c.Memory.loopRange()
		v.loop = (c.Flag || c.Memory.Loop) && v.le > v.ls
	} else {
		v.ch = c.Stream.Channels
		v.ratio = float64(c.Stream.Rate) / m.rate
		c.Stream.reset()
		c.Stream.inUse = true
	}
	v.setPitch(c.Pitch)
	v.setPan(c.Pan)
	g := max(c.Gain, 0)
	if c.Fade > 0 {
		v.gain = 0
		v.rampTo(g, float64(c.Fade)*m.rate)
	} else {
		v.gain, v.target = g, g
	}
	h.voice = int32(vi)
}

func (m *Mixer) take(prio int32) int {
	if n := len(m.freeV); n > 0 {
		vi := int(m.freeV[n-1])
		m.freeV = m.freeV[:n-1]
		return vi
	}
	victim := -1
	for i := range m.voices {
		v := &m.voices[i]
		if victim < 0 {
			victim = i
			continue
		}
		w := &m.voices[victim]
		if v.priority < w.priority || (v.priority == w.priority && v.age < w.age) {
			victim = i
		}
	}
	if victim < 0 || m.voices[victim].priority > prio {
		return -1
	}
	m.end(victim, EvStolen)
	n := len(m.freeV)
	vi := int(m.freeV[n-1])
	m.freeV = m.freeV[:n-1]
	return vi
}

func (m *Mixer) release(h uint32) {
	hd := &m.handles[h]
	hd.voice = -1
	hd.pos.Store(0)
	hd.gain.Store(0)
	if hd.gen.Add(1) == 0 {
		hd.gen.Store(1)
	}
	m.hfree.push(h)
}

func (m *Mixer) reject(id VoiceID) {
	m.events.push(Event{Kind: EvRejected, Voice: id})
	m.release(id.Index)
}

func (m *Mixer) end(vi int, kind EventKind) {
	v := &m.voices[vi]
	m.events.push(Event{Kind: kind, Voice: v.id})
	if v.st != nil {
		v.st.inUse = false
	}
	m.release(v.id.Index)
	*v = voice{}
	m.freeV = append(m.freeV, int32(vi))
}

func (m *Mixer) starve(v *voice) {
	if v.starved && m.clock-v.starvedAt < uint64(m.rate) {
		return
	}
	v.starved, v.starvedAt = true, m.clock
	m.events.push(Event{Kind: EvStarved, Voice: v.id})
}

func (m *Mixer) retarget(i int, frames float64) {
	b := &m.buses[i]
	t := b.user * b.session * b.world
	if b.mute || !b.audible {
		t = 0
	}
	b.target = t
	n := int(frames)
	if n < 1 {
		b.gain, b.ramp = t, 0
		return
	}
	b.ramp = n
	b.delta = (t - b.gain) / float32(n)
}

func (m *Mixer) resolo() {
	anySolo := false
	for i := range m.buses {
		anySolo = anySolo || m.buses[i].solo
	}
	for i := range m.buses {
		aud := !anySolo || i == m.master || m.soloLinked(i)
		if aud != m.buses[i].audible {
			m.buses[i].audible = aud
			m.retarget(i, m.seconds(0, busRamp))
		}
	}
}

func (m *Mixer) soloLinked(i int) bool {
	for j := i; j >= 0; j = m.buses[j].parent {
		if m.buses[j].solo {
			return true
		}
	}
	for j := range m.buses {
		if !m.buses[j].solo {
			continue
		}
		for k := m.buses[j].parent; k >= 0; k = m.buses[k].parent {
			if k == i {
				return true
			}
		}
	}
	return false
}

func (m *Mixer) publish(v *voice) {
	h := &m.handles[v.id.Index]
	if rate := v.ratio * m.rate; rate > 0 {
		h.pos.Store(math.Float32bits(float32(v.pos / rate)))
	}
	h.gain.Store(math.Float32bits(v.gain))
}

func (m *Mixer) chunk(out []float32, n int) {
	for i := range m.buses {
		if m.buses[i].live {
			clear(m.buses[i].acc[:n*2])
			m.buses[i].live = false
		}
	}
	active := 0
	for vi := range m.voices {
		v := &m.voices[vi]
		if v.state == voiceFree {
			continue
		}
		active++
		if v.state != voicePlaying || m.paused {
			continue
		}
		acc := m.buses[v.bus].acc[:n*2]
		m.buses[v.bus].live = true
		var done bool
		if v.mem != nil {
			if v.step == 1 && v.pos == float64(int64(v.pos)) {
				done = m.renderMemoryDirect(v, acc, n)
			} else {
				done = m.renderMemory(v, acc, n)
			}
		} else {
			done = m.renderStream(v, acc, n)
		}
		if done {
			m.end(vi, EvFinished)
			active--
			continue
		}
		m.publish(v)
	}
	m.active.Store(int32(active))
	m.sum(out, n)
	m.clock += uint64(n)
}

func (m *Mixer) renderMemory(v *voice, acc []float32, n int) bool {
	for f := 0; f < n; f++ {
		if v.stopping && v.ramp == 0 {
			return true
		}
		i := int64(v.pos)
		if !v.loop && i >= v.frames {
			return true
		}
		t := float32(v.pos - float64(i))
		g := v.next()
		l := dsp.Hermite(v.sample(i-1, 0), v.sample(i, 0), v.sample(i+1, 0), v.sample(i+2, 0), t)
		r := l
		if v.ch == 2 {
			r = dsp.Hermite(v.sample(i-1, 1), v.sample(i, 1), v.sample(i+1, 1), v.sample(i+2, 1), t)
		}
		acc[2*f] += l * g * v.gl
		acc[2*f+1] += r * g * v.gr
		v.pos += v.step
		if v.loop && v.pos >= float64(v.le) {
			v.pos -= float64(v.le - v.ls)
		}
	}
	return false
}

// renderMemoryDirect reads a voice at pitch 1 on a whole frame: Hermite at t = 0 is y1, so the output is bit-identical without interpolation; outside a gain ramp it runs a flat loop to the end of the clip or loop.
func (m *Mixer) renderMemoryDirect(v *voice, acc []float32, n int) bool {
	pcm, ch := v.mem.PCM, v.ch
	i := int64(v.pos)
	f := 0
	for f < n {
		if v.stopping && v.ramp == 0 {
			v.pos = float64(i)
			return true
		}
		end := v.frames
		if v.loop {
			end = v.le
		}
		if i >= end {
			if !v.loop {
				v.pos = float64(i)
				return true
			}
			i = v.ls
			continue
		}
		if v.ramp > 0 {
			g := v.next()
			k := int(i) * ch
			l := pcm[k]
			r := l
			if ch == 2 {
				r = pcm[k+1]
			}
			acc[2*f] += l * g * v.gl
			acc[2*f+1] += r * g * v.gr
			f++
			i++
			continue
		}
		run := int(min(int64(n-f), end-i))
		g, gl, gr := v.gain, v.gl, v.gr
		src := pcm[int(i)*ch : (int(i)+run)*ch]
		dst := acc[2*f : 2*(f+run)]
		if ch == 2 {
			for j := 0; j < run; j++ {
				dst[2*j] += src[2*j] * g * gl
				dst[2*j+1] += src[2*j+1] * g * gr
			}
		} else {
			for j, s := range src {
				dst[2*j] += s * g * gl
				dst[2*j+1] += s * g * gr
			}
		}
		f += run
		i += int64(run)
	}
	v.pos = float64(i)
	return false
}

func (m *Mixer) renderStream(v *voice, acc []float32, n int) bool {
	s := v.st
	s.release()
	eof := s.eof.Load()
	s.fill(v.pos, v.step, n)
	drained := eof && s.Ring.Len() == 0
	end := s.start + int64(s.n)
	for f := 0; f < n; f++ {
		if v.stopping && v.ramp == 0 {
			return true
		}
		i := int64(v.pos)
		if i+2 >= end {
			if !drained {
				m.starve(v)
				return false
			}
			if i >= end {
				return true
			}
		}
		k := int(i - s.start)
		t := float32(v.pos - float64(i))
		g := v.next()
		l := dsp.Hermite(s.at(k-1, 0), s.at(k, 0), s.at(k+1, 0), s.at(k+2, 0), t)
		r := l
		if v.ch == 2 {
			r = dsp.Hermite(s.at(k-1, 1), s.at(k, 1), s.at(k+1, 1), s.at(k+2, 1), t)
		}
		acc[2*f] += l * g * v.gl
		acc[2*f+1] += r * g * v.gr
		v.pos += v.step
	}
	return false
}

// Skip advances every voice by frames output frames without rendering, as if the output were mixed and thrown away: commands apply, ramps and positions move, loops wrap and finished voices end with their events.
func (m *Mixer) Skip(frames int) {
	m.drain()
	frames = max(frames, 0)
	active := 0
	for vi := range m.voices {
		v := &m.voices[vi]
		if v.state == voiceFree {
			continue
		}
		active++
		if v.state != voicePlaying || m.paused || frames == 0 {
			continue
		}
		if m.skipVoice(v, frames) {
			m.end(vi, EvFinished)
			active--
			continue
		}
		m.publish(v)
	}
	m.active.Store(int32(active))
	for i := range m.buses {
		m.idle(&m.buses[i], frames)
	}
	m.clock += uint64(frames)
}

// skipVoice ends a voice when the mixer would within n frames, which checks the end before each frame, so a voice reaching it on the last frame ends on the next call.
func (m *Mixer) skipVoice(v *voice, n int) bool {
	if v.stopping && v.ramp < n {
		return true
	}
	if v.ramp > 0 {
		k := min(v.ramp, n)
		v.gain += v.delta * float32(k)
		if v.ramp -= k; v.ramp == 0 {
			v.gain = v.target
		}
	}
	to := v.pos + v.step*float64(n)
	if v.mem == nil {
		if v.st.skip(to) {
			return true
		}
		v.pos = to
		return false
	}
	if v.loop {
		if span := float64(v.le - v.ls); to >= float64(v.le) && span > 0 {
			to = float64(v.ls) + math.Mod(to-float64(v.ls), span)
		}
		v.pos = to
		return false
	}
	if int64(v.pos+v.step*float64(n-1)) >= v.frames {
		return true
	}
	v.pos = to
	return false
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// idle advances the gain ramp of a bus that got no samples this chunk.
func (m *Mixer) idle(b *bus, n int) {
	for f := 0; f < n && b.ramp > 0; f++ {
		b.gain += b.delta
		b.ramp--
		if b.ramp == 0 {
			b.gain = b.target
		}
	}
	b.peak.Store(0)
	b.rms.Store(0)
	b.level.Store(math.Float32bits(b.gain))
}

func (m *Mixer) sum(out []float32, n int) {
	for _, bi := range m.order {
		b := &m.buses[bi]
		acc := b.acc[:n*2]
		var dst []float32
		if bi != m.master {
			if !b.live {
				m.idle(b, n)
				continue
			}
			dst = m.buses[b.parent].acc[:n*2]
			m.buses[b.parent].live = true
		}
		var peak, sq float32
		for f := 0; f < n; f++ {
			if b.ramp > 0 {
				b.gain += b.delta
				b.ramp--
				if b.ramp == 0 {
					b.gain = b.target
				}
			}
			l, r := acc[2*f]*b.gain, acc[2*f+1]*b.gain
			if dst == nil {
				l, r = dsp.SoftClip(l), dsp.SoftClip(r)
				out[2*f], out[2*f+1] = l, r
			} else {
				dst[2*f] += l
				dst[2*f+1] += r
			}
			if a := abs32(l); a > peak {
				peak = a
			}
			if a := abs32(r); a > peak {
				peak = a
			}
			sq += l*l + r*r
		}
		b.peak.Store(math.Float32bits(peak))
		b.rms.Store(math.Float32bits(float32(math.Sqrt(float64(sq / float32(2*n))))))
		b.level.Store(math.Float32bits(b.gain))
	}
}
