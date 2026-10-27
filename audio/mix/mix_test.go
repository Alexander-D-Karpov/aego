package mix

import (
	"encoding/binary"
	"flag"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden PCM files")

const rate = 48000

func sine(freq float64, frames, ch int, amp float32) *Memory {
	pcm := make([]float32, frames*ch)
	for f := 0; f < frames; f++ {
		s := amp * float32(math.Sin(2*math.Pi*freq*float64(f)/rate))
		for c := 0; c < ch; c++ {
			pcm[f*ch+c] = s
		}
	}
	return &Memory{PCM: pcm, Rate: rate, Channels: ch}
}

func dc(v float32, frames int) *Memory {
	pcm := make([]float32, frames)
	for i := range pcm {
		pcm[i] = v
	}
	return &Memory{PCM: pcm, Rate: rate, Channels: 1}
}

func newMixer(tb testing.TB, buses ...BusDesc) *Mixer {
	tb.Helper()
	m, err := New(Config{SampleRate: rate, Channels: 2, Buses: buses})
	if err != nil {
		tb.Fatal(err)
	}
	return m
}

func render(m *Mixer, frames int) []float32 {
	out := make([]float32, frames*2)
	m.Mix(out)
	return out
}

func events(m *Mixer) []Event {
	var out []Event
	for {
		e, ok := m.Events().Pop()
		if !ok {
			return out
		}
		out = append(out, e)
	}
}

func golden(t *testing.T, name string, got []float32) {
	t.Helper()
	file := filepath.Join("testdata", name+".f32")
	if *update {
		buf := make([]byte, len(got)*4)
		for i, v := range got {
			binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
		}
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, buf, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%v; run go test ./audio/mix -run Golden -update", err)
	}
	if len(data) != len(got)*4 {
		t.Fatalf("%s has %d samples, got %d", name, len(data)/4, len(got))
	}
	for i := range got {
		want := math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
		if d := math.Abs(float64(got[i] - want)); d > 1e-5 {
			t.Fatalf("%s sample %d = %v, want %v", name, i, got[i], want)
		}
	}
}

func frequency(out []float32) float64 {
	first, last, n := -1, 0, 0
	for i := 1; i < len(out)/2; i++ {
		if out[2*(i-1)] <= 0 && out[2*i] > 0 {
			if first < 0 {
				first = i
			}
			last = i
			n++
		}
	}
	if n < 2 {
		return 0
	}
	return float64(n-1) * rate / float64(last-first)
}

func maxDelta(out []float32) float64 {
	d := 0.0
	for i := 2; i < len(out); i += 2 {
		d = max(d, math.Abs(float64(out[i]-out[i-2])))
	}
	return d
}

func TestGolden(t *testing.T) {
	cases := []struct {
		name             string
		pan, gain, pitch float32
	}{
		{"pan_left", -1, 0.5, 1},
		{"pan_center", 0, 0.5, 1},
		{"pan_right", 1, 0.5, 1},
		{"pitch2", 0, 0.5, 2},
	}
	src := sine(440, rate, 1, 1)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newMixer(t)
			if !m.Play(PlayParams{Memory: src, Gain: c.gain, Pan: c.pan, Pitch: c.pitch, Loop: true}).Valid() {
				t.Fatal("play refused")
			}
			out := render(m, 4096)
			golden(t, c.name, out)
			switch c.pan {
			case -1:
				for i := 1; i < len(out); i += 2 {
					if out[i] != 0 {
						t.Fatalf("right channel %v at full left pan", out[i])
					}
				}
			case 1:
				for i := 0; i < len(out); i += 2 {
					if math.Abs(float64(out[i])) > 1e-6 {
						t.Fatalf("left channel %v at full right pan", out[i])
					}
				}
			}
		})
	}
}

func TestPitchDoublesFrequency(t *testing.T) {
	m := newMixer(t)
	m.Play(PlayParams{Memory: sine(440, rate, 1, 1), Gain: 0.5, Pitch: 2, Loop: true})
	f := frequency(render(m, 4096))
	if math.Abs(f-880)/880 > 0.01 {
		t.Fatalf("pitch 2 gives %.1f Hz, want 880", f)
	}
}

func TestGainRamp(t *testing.T) {
	m := newMixer(t)
	id := m.Play(PlayParams{Memory: dc(0.5, 4800), Gain: 1, Loop: true})
	render(m, 1024)
	m.SetGain(id, 0, 0)
	out := render(m, 2048)
	if d := maxDelta(out); d > 0.02 {
		t.Fatalf("gain step %.4f between samples", d)
	}
	if out[len(out)-2] != 0 {
		t.Fatalf("gain did not reach zero: %v", out[len(out)-2])
	}
}

func TestBusRamp(t *testing.T) {
	m := newMixer(t)
	m.Play(PlayParams{Memory: dc(0.5, 4800), Gain: 1, Loop: true})
	render(m, 1024)
	m.SetBusGain(0, 0)
	out := render(m, 2048)
	if d := maxDelta(out); d > 0.02 {
		t.Fatalf("bus gain step %.4f between samples", d)
	}
	if g := m.BusGain(0); g != 0 {
		t.Fatalf("bus gain %v after the ramp", g)
	}
}

func TestBusConfig(t *testing.T) {
	bad := map[string][]BusDesc{
		"cycle":     {{ID: "master"}, {ID: "a", Parent: "b"}, {ID: "b", Parent: "a"}},
		"duplicate": {{ID: "master"}, {ID: "a", Parent: "master"}, {ID: "a", Parent: "master"}},
		"parent":    {{ID: "master"}, {ID: "a", Parent: "nope"}},
		"two roots": {{ID: "master"}, {ID: "other"}},
		"no root":   {{ID: "a", Parent: "b"}, {ID: "b", Parent: "a"}},
	}
	for name, buses := range bad {
		if _, err := New(Config{SampleRate: rate, Buses: buses}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestMuteSolo(t *testing.T) {
	m := newMixer(t, BusDesc{ID: "master"}, BusDesc{ID: "music", Parent: "master"}, BusDesc{ID: "sfx", Parent: "master"})
	music, _ := m.Bus("music")
	sfx, _ := m.Bus("sfx")
	m.Play(PlayParams{Memory: dc(0.25, 4800), Bus: music, Gain: 1, Loop: true})
	m.Play(PlayParams{Memory: dc(0.125, 4800), Bus: sfx, Gain: 1, Loop: true})
	k := float32(math.Sqrt2 / 2)
	check := func(what string, want float32) {
		t.Helper()
		out := render(m, 2048)
		if got := out[len(out)-2]; math.Abs(float64(got-want)) > 1e-5 {
			t.Fatalf("%s: %v, want %v", what, got, want)
		}
	}
	check("both", (0.25+0.125)*k)
	m.SetBusMute(music, true)
	check("music muted", 0.125*k)
	m.SetBusMute(music, false)
	m.SetBusSolo(music, true)
	check("music solo", 0.25*k)
	m.SetBusSolo(music, false)
	check("solo off", (0.25+0.125)*k)
}

func TestStealing(t *testing.T) {
	m := newMixer(t)
	mem := dc(0.001, 4800)
	ids := make([]VoiceID, DefaultVoices)
	for i := range ids {
		ids[i] = m.Play(PlayParams{Memory: mem, Gain: 1, Loop: true, Priority: 1})
		if !ids[i].Valid() {
			t.Fatalf("voice %d refused", i)
		}
	}
	render(m, 64)
	if m.Active() != DefaultVoices {
		t.Fatalf("%d active voices", m.Active())
	}
	low := m.Play(PlayParams{Memory: mem, Gain: 1, Loop: true, Priority: 0})
	render(m, 64)
	ev := events(m)
	if len(ev) != 1 || ev[0].Kind != EvRejected || ev[0].Voice != low {
		t.Fatalf("lower priority: %+v", ev)
	}
	m.Play(PlayParams{Memory: mem, Gain: 1, Loop: true, Priority: 1})
	render(m, 64)
	ev = events(m)
	if len(ev) != 1 || ev[0].Kind != EvStolen || ev[0].Voice != ids[0] {
		t.Fatalf("equal priority: %+v, want the oldest %+v stolen", ev, ids[0])
	}
	m.SetGain(ids[0], 0, 0)
	m.Stop(ids[0], 0)
	render(m, 64)
	if ev := events(m); len(ev) != 0 || m.Active() != DefaultVoices {
		t.Fatalf("stale id changed the mixer: %+v, %d active", ev, m.Active())
	}
}

func TestStaleID(t *testing.T) {
	m := newMixer(t)
	id := m.Play(PlayParams{Memory: dc(0.5, 10), Gain: 1})
	render(m, 64)
	ev := events(m)
	if len(ev) != 1 || ev[0].Kind != EvFinished || ev[0].Voice != id {
		t.Fatalf("events %+v", ev)
	}
	if m.find(id) >= 0 {
		t.Fatal("finished id still resolves")
	}
	next := m.Play(PlayParams{Memory: dc(0.5, 4800), Gain: 1, Loop: true})
	render(m, 64)
	m.Stop(id, 0)
	render(m, 1024)
	if m.find(next) < 0 || m.Active() != 1 {
		t.Fatal("a stale stop hit another voice")
	}
}

func TestQueueRace(t *testing.T) {
	m := newMixer(t)
	const total = 100000
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < total; {
			if m.SetBusGain(0, float32(i%2)) {
				i++
			} else {
				runtime.Gosched()
			}
		}
	}()
	buf := make([]float32, 256)
	for m.applied < total {
		m.Mix(buf)
	}
	wg.Wait()
}

func TestQueueOverflow(t *testing.T) {
	m := newMixer(t)
	refused := 0
	for i := 0; i < 2*queueSize; i++ {
		if !m.SetBusGain(0, 1) {
			refused++
		}
	}
	if refused == 0 || m.Commands().Dropped() != uint64(refused) {
		t.Fatalf("refused %d, dropped %d", refused, m.Commands().Dropped())
	}
	render(m, 64)
	if !m.SetBusGain(0, 1) {
		t.Fatal("queue still full after Mix")
	}
}

func TestStreamStarved(t *testing.T) {
	m := newMixer(t)
	s := NewStream(rate, 1, 4096)
	m.Play(PlayParams{Stream: s, Gain: 1})
	out := render(m, 512)
	for _, v := range out {
		if v != 0 {
			t.Fatal("a starved stream is not silent")
		}
	}
	count := func() int {
		n := 0
		for _, e := range events(m) {
			if e.Kind == EvStarved {
				n++
			}
		}
		return n
	}
	if n := count(); n != 1 {
		t.Fatalf("%d Starved events, want 1", n)
	}
	render(m, rate/2)
	if n := count(); n != 0 {
		t.Fatalf("%d Starved events within a second", n)
	}
	render(m, rate)
	if n := count(); n != 1 {
		t.Fatalf("%d Starved events after a second, want 1", n)
	}
}

func TestStreamPlaysAndEnds(t *testing.T) {
	m := newMixer(t)
	s := NewStream(rate, 1, 8192)
	src := sine(440, 4800, 1, 0.5)
	if n := s.Ring.Write(src.PCM); n != 4800 {
		t.Fatalf("ring took %d frames", n)
	}
	s.SetEOF()
	id := m.Play(PlayParams{Stream: s, Gain: 1})
	out := render(m, 4096)
	if peak, _ := m.Meters(0); peak < 0.3 {
		t.Fatalf("stream peak %v", peak)
	}
	_ = out
	render(m, 2048)
	ev := events(m)
	if len(ev) != 1 || ev[0].Kind != EvFinished || ev[0].Voice != id {
		t.Fatalf("events %+v", ev)
	}
}

func loaded(tb testing.TB) (*Mixer, []float32) {
	m := newMixer(tb)
	mem := sine(440, rate, 1, 0.1)
	for i := 0; i < 60; i++ {
		m.Play(PlayParams{Memory: mem, Gain: 0.1, Pan: float32(i%3 - 1), Pitch: 0.5 + float32(i)/40, Loop: true})
	}
	fill := make([]float32, 60000*2)
	for i := 0; i < 4; i++ {
		s := NewStream(44100, 2, 1<<16)
		s.Ring.Write(fill)
		m.Play(PlayParams{Stream: s, Gain: 0.1, Pitch: 1})
	}
	buf := make([]float32, 512*2)
	m.Mix(buf)
	if m.Active() != 64 {
		tb.Fatalf("%d active voices", m.Active())
	}
	return m, buf
}

func TestMixAllocs(t *testing.T) {
	m, buf := loaded(t)
	if n := testing.AllocsPerRun(200, func() { m.Mix(buf) }); n != 0 {
		t.Fatalf("Mix allocates %v times per call", n)
	}
}

func BenchmarkMix64(b *testing.B) {
	m, buf := loaded(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m.Mix(buf)
	}
}

func TestBusLayers(t *testing.T) {
	m := newMixer(t)
	m.Play(PlayParams{Memory: dc(0.5, 4800), Gain: 1, Loop: true})
	m.SetBusLayer(0, LayerSession, 0.5, 0)
	m.SetBusLayer(0, LayerWorld, 0.5, 0.1)
	render(m, rate/5)
	if g := m.BusGain(0); math.Abs(float64(g-0.25)) > 1e-5 {
		t.Fatalf("user × session × world = %v, want 0.25", g)
	}
	m.SetBusGain(0, 0.5)
	render(m, rate/10)
	if g := m.BusGain(0); math.Abs(float64(g-0.125)) > 1e-5 {
		t.Fatalf("after user 0.5: %v", g)
	}
}

func TestVoicePositionAndKill(t *testing.T) {
	m := newMixer(t)
	id := m.Play(PlayParams{Memory: dc(0.5, rate), Gain: 1})
	render(m, rate/2)
	if p := m.Position(id); math.Abs(float64(p-0.5)) > 0.02 {
		t.Fatalf("position %v s, want 0.5", p)
	}
	m.Kill(id)
	render(m, 64)
	ev := events(m)
	if len(ev) != 1 || ev[0].Kind != EvFinished || m.Live(id) {
		t.Fatalf("kill: %+v, live %v", ev, m.Live(id))
	}
}

func directPair(tb testing.TB, mem *Memory, loop bool) (*Mixer, voice, voice) {
	m := newMixer(tb)
	m.Play(PlayParams{Memory: mem, Gain: 0.7, Pan: 0.3, Loop: loop, FadeIn: 0.005})
	buf := make([]float32, 64*2)
	m.Mix(buf)
	for i := range m.voices {
		if m.voices[i].state == voicePlaying {
			return m, m.voices[i], m.voices[i]
		}
	}
	tb.Fatal("no playing voice")
	return nil, voice{}, voice{}
}

// The direct path for pitch 1 gives the same samples, position, gain and end as Hermite, through fades, loops and the stop ramp.
func TestDirectPathMatchesHermite(t *testing.T) {
	for _, ch := range []int{1, 2} {
		for _, loop := range []bool{false, true} {
			mem := sine(440, rate/20, ch, 0.5)
			if loop {
				mem.LoopStart, mem.LoopEnd = 100, mem.Frames()-50
			}
			m, a, b := directPair(t, mem, loop)
			if b.step != 1 {
				t.Fatalf("step %v, the direct path needs 1", b.step)
			}
			const n = 256
			accA, accB := make([]float32, n*2), make([]float32, n*2)
			for block := 0; block < 40; block++ {
				if block == 20 {
					a.rampTo(0, 300)
					b.rampTo(0, 300)
					a.stopping, b.stopping = true, true
				}
				clear(accA)
				clear(accB)
				da := m.renderMemory(&a, accA, n)
				db := m.renderMemoryDirect(&b, accB, n)
				if da != db || a.pos != b.pos || a.gain != b.gain {
					t.Fatalf("ch=%d loop=%v block %d: done %v/%v pos %v/%v gain %v/%v", ch, loop, block, da, db, a.pos, b.pos, a.gain, b.gain)
				}
				for i := range accA {
					if accA[i] != accB[i] {
						t.Fatalf("ch=%d loop=%v block %d sample %d: %v != %v", ch, loop, block, i, accA[i], accB[i])
					}
				}
				if da {
					break
				}
			}
		}
	}
}

func loadedPitch1(tb testing.TB) (*Mixer, []float32) {
	m := newMixer(tb)
	mem := sine(440, rate/10, 1, 0.5)
	for i := 0; i < 64; i++ {
		m.Play(PlayParams{Memory: mem, Gain: 0.1, Pan: float32(i%3 - 1), Pitch: 1, Loop: true})
	}
	buf := make([]float32, 512*2)
	m.Mix(buf)
	if m.Active() != 64 {
		tb.Fatalf("%d active voices", m.Active())
	}
	return m, buf
}

func TestMixPitch1Allocs(t *testing.T) {
	m, buf := loadedPitch1(t)
	if n := testing.AllocsPerRun(200, func() { m.Mix(buf) }); n != 0 {
		t.Fatalf("Mix allocates %v times per call", n)
	}
}

func BenchmarkMix64Pitch1(b *testing.B) {
	m, buf := loadedPitch1(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m.Mix(buf)
	}
}

// Skipping a stretch leaves voices where mixing it would: positions, loops, ramps, stops and ends with their events.
func TestSkipMatchesMix(t *testing.T) {
	setup := func() (*Mixer, VoiceID) {
		m := newMixer(t)
		m.Play(PlayParams{Memory: dc(0.5, rate/4), Gain: 1})
		m.Play(PlayParams{Memory: sine(440, rate/3, 2, 0.3), Gain: 0.8, Pitch: 1.3, Loop: true, FadeIn: 0.1})
		id := m.Play(PlayParams{Memory: dc(0.2, rate), Gain: 1})
		render(m, 256)
		events(m)
		m.Stop(id, 0.05)
		return m, id
	}
	a, _ := setup()
	b, _ := setup()
	for i := 0; i < 20; i++ {
		render(a, 1200)
		b.Skip(1200)
		if a.Active() != b.Active() {
			t.Fatalf("step %d: %d active after mixing, %d after skipping", i, a.Active(), b.Active())
		}
		for vi := range a.voices {
			va, vb := &a.voices[vi], &b.voices[vi]
			if va.state != vb.state || math.Abs(va.pos-vb.pos) > 1e-3 || math.Abs(float64(va.gain-vb.gain)) > 1e-4 {
				t.Fatalf("step %d voice %d: mixed %v at %.4f gain %.5f, skipped %v at %.4f gain %.5f", i, vi, va.state, va.pos, va.gain, vb.state, vb.pos, vb.gain)
			}
		}
		ea, eb := events(a), events(b)
		if len(ea) != len(eb) {
			t.Fatalf("step %d: events %+v after mixing, %+v after skipping", i, ea, eb)
		}
	}
	if a.Active() != 1 {
		t.Fatalf("%d voices left, want the loop", a.Active())
	}
}

// A skipped stream drops what is decoded and leaves the rest to the feeder, holds it until it plays again and ends at its end.
func TestSkipStream(t *testing.T) {
	m := newMixer(t)
	s := NewStream(rate, 1, 8192)
	s.Ring.Write(make([]float32, 6000))
	id := m.Play(PlayParams{Stream: s, Gain: 1})
	render(m, 1000)
	m.Skip(3000)
	if !s.Held() || s.TakeSkip() != 0 {
		t.Fatal("a skip within the decoded frames went to the feeder or did not hold the stream")
	}
	m.Skip(4000)
	if got := s.TakeSkip(); got != 1999 {
		t.Fatalf("the feeder skips %d frames, want 1999", got)
	}
	render(m, 64)
	if s.Held() {
		t.Fatal("mixing did not release the stream")
	}
	m.Skip(100)
	s.SetEOF()
	m.Skip(100)
	ev := events(m)
	if len(ev) == 0 || ev[len(ev)-1].Kind != EvFinished || ev[len(ev)-1].Voice != id {
		t.Fatalf("events %+v, want the stream finished", ev)
	}
}
