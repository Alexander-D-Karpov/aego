package mix

import "testing"

func TestStopStarvedStreamCompletesFadeWithoutSourceSamples(t *testing.T) {
	for _, tc := range []struct {
		name string
		fade float32
	}{{"default", 0}, {"explicit", .05}} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMixer(t)
			stream := NewStream(rate, 1, 4096) // Neither producer nor EOF.
			voice := m.Play(PlayParams{Stream: stream, Gain: 1})
			other := m.Play(PlayParams{Memory: dc(.2, rate), Gain: 1, Loop: true})
			render(m, 64)
			if !m.Live(voice) || !m.Live(other) {
				t.Fatal("fixture did not start the starved and independent voices")
			}
			events(m) // Discard the expected initial starvation notice.
			if !m.Stop(voice, tc.fade) {
				t.Fatal("could not queue Stop")
			}
			// Use the mixer's actual float32-to-frame conversion, including its
			// truncation of the default 10 ms ramp at this sample rate.
			frames := int(m.seconds(tc.fade, gainRamp))
			if frames < 2 {
				t.Fatal("invalid test fade")
			}
			render(m, frames-1)
			if !m.Live(voice) || !m.Live(other) {
				t.Fatal("stopping stream ended before its fade duration")
			}
			render(m, 1)
			if !m.Live(voice) {
				t.Fatal("voice must finish on the frame after the fade")
			}
			render(m, 1)
			if m.Live(voice) || !m.Live(other) {
				t.Fatal("silent stopping stream was not released, or another voice stopped")
			}
			got := events(m)
			if len(got) != 1 || got[0].Kind != EvFinished || got[0].Voice != voice {
				t.Fatalf("unexpected events after Stop: %+v", got)
			}
		})
	}
}
