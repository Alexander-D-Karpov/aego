package pump

import (
	"testing"
	"testing/synctest"
	"time"

	"aego/audio/mix"
)

type sink struct {
	channels int
	muted    bool
	writes   int
}

func (s *sink) Write(samples []float32) int {
	s.writes++
	return len(samples) / s.channels
}
func (*sink) Queued() int   { return 0 }
func (*sink) Close()        {}
func (s *sink) Muted() bool { return s.muted }

func TestPumpSleepsWithoutVoices(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2})
		if err != nil {
			t.Fatal(err)
		}
		p := New(m, &sink{channels: 2}, Config{})
		p.Start()
		defer p.Stop()
		synctest.Wait()
		idle := p.Stats().Blocks
		if idle == 0 {
			t.Fatal("the pump did not prime the output")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := p.Stats().Blocks; got != idle {
			t.Fatalf("an idle pump mixed %d blocks in a second", got-idle)
		}
		m.PauseAll()
		synctest.Wait()
		if got := p.Stats().Blocks; got == idle {
			t.Fatal("a command did not wake the pump")
		}
	})
}

// While the output is muted the pump writes nothing and moves voices on by the time that passed, so they still end.
func TestPumpSkipsWhileMuted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2})
		if err != nil {
			t.Fatal(err)
		}
		out := &sink{channels: 2, muted: true}
		p := New(m, out, Config{})
		pcm := make([]float32, 48000)
		id := m.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: 48000, Channels: 1}, Gain: 1})
		p.Start()
		defer p.Stop()
		time.Sleep(500 * time.Millisecond)
		synctest.Wait()
		if !m.Live(id) {
			t.Fatal("a one-second voice ended after half a second")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if m.Live(id) || m.Active() != 0 {
			t.Fatal("the voice did not end while muted")
		}
		if out.writes != 0 {
			t.Fatalf("the pump wrote %d blocks to a muted output", out.writes)
		}
	})
}
