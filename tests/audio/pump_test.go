package audio

import (
	"testing"
	"time"

	"aego/audio/mix"
	"aego/audio/pump"
	"aego/platform"
	"aego/platform/headless"
)

func TestPumpHeadless(t *testing.T) {
	plat := headless.New(headless.Options{FrameTime: 1.0 / 60})
	out, err := plat.OpenAudio(platform.AudioSpec{SampleRate: rate, Channels: 2, Frames: 512})
	if err != nil {
		t.Fatal(err)
	}
	m := newMixer(t)
	m.Play(mix.PlayParams{Memory: tone(440, rate, 0.5), Gain: 1, Loop: true})
	p := pump.New(m, out, pump.Config{SampleRate: rate, Channels: 2, Block: 512, Target: 3})
	poll := func() { plat.PollEvents(func(platform.Event) {}) }
	p.Fill()
	for i := 0; i < 60; i++ {
		poll()
		p.Fill()
		if q := out.Queued(); q < 512 || q > 4*512 {
			t.Fatalf("frame %d: %d frames queued", i, q)
		}
	}
	if s := p.Stats(); s.Underruns != 0 {
		t.Fatalf("%d underruns in a second", s.Underruns)
	}
	for i := 0; i < 10; i++ {
		poll()
	}
	if q := out.Queued(); q != 0 {
		t.Fatalf("%d frames left after 10 frames without filling", q)
	}
	p.Fill()
	if s := p.Stats(); s.Underruns != 1 {
		t.Fatalf("%d underruns after the queue ran dry, want 1", s.Underruns)
	}
	for i := 0; i < 10; i++ {
		poll()
		p.Fill()
	}
	if s := p.Stats(); s.Underruns != 1 || out.Queued() < 512 {
		t.Fatalf("the pump did not recover: %+v, %d queued", s, out.Queued())
	}
	if out.(*headless.Audio).Consumed() < rate {
		t.Fatal("the headless output consumed less than a second")
	}
}

func TestPumpGoroutine(t *testing.T) {
	plat := headless.New(headless.Options{})
	out, err := plat.OpenAudio(platform.AudioSpec{SampleRate: rate, Channels: 2, Frames: 512})
	if err != nil {
		t.Fatal(err)
	}
	m := newMixer(t)
	m.Play(mix.PlayParams{Memory: tone(440, rate, 0.5), Gain: 1, Loop: true})
	p := pump.New(m, out, pump.Config{SampleRate: rate, Channels: 2, Block: 512, Target: 3})
	p.Start()
	time.Sleep(200 * time.Millisecond)
	p.Stop()
	if s := p.Stats(); s.Blocks < 3 || s.MixMax <= 0 {
		t.Fatalf("stats %+v", s)
	}
	p.Close()
}
