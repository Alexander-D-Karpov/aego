package audio

import (
	"bytes"
	"errors"
	"testing"

	"aego/asset"
	"aego/audio/mix"
	"aego/core/ids"
)

func guardedSeekFixture(t *testing.T, stream bool) (*Module, *mix.Mixer, *Clip) {
	t.Helper()
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Voices: 4, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	a, err := asset.DecodeAudio(privatePreviewArtifact(.3, stream))
	if err != nil {
		t.Fatal(err)
	}
	// Bytes after the WAV container's declared end keep the PCM fixture small
	// while requiring several owner-thread verification steps.
	a.Data = append(bytes.Clone(a.Data), make([]byte, 3*previewSeekHashChunk)...)
	res := &previewLifecycleResolver{id: id, raw: asset.EncodeAudio(a)}
	clip, err := m.Clip(res, id)
	if err != nil || m.Preview(res, id, false) != nil {
		t.Fatal("could not start preview", err)
	}
	if err := m.SetPreviewPaused(true); err != nil {
		t.Fatal(err)
	}
	mx.Mix(nil)
	t.Cleanup(func() { m.endPreview() })
	return m, mx, clip
}

func TestGuardedPreviewSeekChecksBoundedChunksAndKeepsOtherVoices(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "memory"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			m, mx, clip := guardedSeekFixture(t, stream)
			old := m.preview
			game := mx.Play(mix.PlayParams{Memory: &mix.Memory{PCM: make([]float32, 1000), Rate: 48000, Channels: 1}, Gain: 1})
			mx.Mix(nil)
			var err error
			if stream {
				err = m.RequestPreviewStreamSeek(clip, 100)
			} else {
				err = m.RequestPreviewMemorySeek(clip, 100)
			}
			if err != nil || m.streamSeek.guard == nil || m.streamSeek.guard.offset != previewSeekHashChunk {
				t.Fatalf("seek did not begin one bounded verification step: %v", err)
			}
			if !m.PreviewStreamSeekPending() || m.preview != old || !mx.Live(game) {
				t.Fatal("preparation moved the preview or game voice")
			}
			if stream && !samePreviewBytes(m.streamSeek.guard.displayed, m.streamSeek.guard.artifact.Data) {
				t.Fatal("identical source and preview did not share the private snapshot")
			}
			for m.streamSeek.guard != nil {
				before := m.streamSeek.guard.offset
				m.pollStreamSeek()
				if g := m.streamSeek.guard; g != nil && g.offset-before > previewSeekHashChunk {
					t.Fatal("verification exceeded its per-frame byte budget")
				}
			}
			if err := m.TakePreviewStreamSeekError(); err != nil {
				t.Fatal(err)
			}
			if stream {
				waitStreamSeek(t, m, func() bool { return m.streamSeek.pending != nil })
				next := m.streamSeek.pending.next
				mx.Mix(nil)
				m.update(0)
				if m.preview != next || mx.Live(old) {
					t.Fatal("verified stream did not replace the old preview")
				}
			} else {
				mx.Mix(nil)
			}
			if position, live := m.PreviewPosition(); !live || !m.PreviewPaused() || position != float32(100)/48000 || !mx.Live(game) {
				t.Fatalf("guarded seek changed the wrong voice: %v/%v", position, live)
			}
		})
	}
}

func TestGuardedPreviewSeekRejectsChangedSources(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, change := range []string{"before", "header", "unread-bytes"} {
			t.Run(map[bool]string{false: "memory/", true: "stream/"}[stream]+change, func(t *testing.T) {
				m, mx, clip := guardedSeekFixture(t, stream)
				old := m.preview
				displayed, a := *clip, *clip.Art
				a.Data = bytes.Clone(a.Data)
				displayed.Art = &a
				if change == "before" {
					a.Data[len(a.Data)-1] ^= 1
				}
				var err error
				if stream {
					err = m.RequestPreviewStreamSeek(&displayed, 100)
				} else {
					err = m.RequestPreviewMemorySeek(&displayed, 100)
				}
				if err != nil || m.streamSeek.guard == nil {
					t.Fatalf("fixture did not enter incremental verification: %v", err)
				}
				switch change {
				case "header":
					a.Data = bytes.Clone(a.Data)
				case "unread-bytes":
					a.Data[len(a.Data)-1] ^= 1
				}
				for m.streamSeek.guard != nil {
					m.pollStreamSeek()
				}
				if err := m.TakePreviewStreamSeekError(); !errors.Is(err, ErrPreviewSeekSourceChanged) || m.preview != old || !mx.Live(old) || m.streamSeek.pending != nil {
					t.Fatal("changed source was accepted or displaced the preview")
				}
			})
		}
	}
}

func TestGuardedPreviewMemorySeekRejectsMismatchedDecoderLayout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*asset.AudioArtifact)
	}{
		{"rate", func(a *asset.AudioArtifact) { a.Rate = 44100 }},
		{"channels", func(a *asset.AudioArtifact) { a.Channels = 2 }},
		{"frames", func(a *asset.AudioArtifact) { a.Frames++ }},
		{"format", func(a *asset.AudioArtifact) { a.Format = asset.AudioOgg }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
			if err != nil {
				t.Fatal(err)
			}
			m := &Module{mixer: mx}
			t.Cleanup(func() { m.endPreview() })
			a, err := asset.DecodeAudio(privatePreviewArtifact(.3, false))
			if err != nil {
				t.Fatal(err)
			}
			tc.change(a)
			id := ids.NewAssetID()
			res := &previewLifecycleResolver{id: id, raw: asset.EncodeAudio(a)}
			clip, err := m.Clip(res, id)
			if err != nil || clip.Mem == nil {
				t.Fatal("could not build the synthetic mismatched clip", err)
			}
			if err := m.Preview(res, id, false); err != nil {
				t.Fatal("ordinary playback must retain its existing behavior", err)
			}
			mx.Mix(nil)
			voice, epoch := m.preview, m.PreviewEpoch()
			if m.PreviewSourceMatches(clip) || !errors.Is(m.RequestPreviewMemorySeek(clip, 100), ErrPreviewSeekSourceChanged) {
				t.Fatal("guard accepted mismatched decoder and artifact layout")
			}
			if m.preview != voice || m.PreviewEpoch() != epoch || !mx.Live(voice) {
				t.Fatal("rejected seek disturbed ordinary playback")
			}
		})
	}
}

func TestGuardedPreviewSeekCancellationAndBackpressure(t *testing.T) {
	for _, stop := range []bool{false, true} {
		name := "resume"
		if stop {
			name = "stop"
		}
		t.Run(name, func(t *testing.T) {
			m, mx, clip := guardedSeekFixture(t, true)
			old := m.preview
			if err := m.RequestPreviewStreamSeek(clip, 100); err != nil || m.streamSeek.guard == nil {
				t.Fatal("could not start guarded seek", err)
			}
			if err := m.RequestPreviewStreamSeek(clip, 200); err == nil {
				t.Fatal("accepted overlapping guarded seeks")
			}
			if stop {
				if err := m.StopPreview(); err != nil {
					t.Fatal(err)
				}
			} else if err := m.SetPreviewPaused(false); err != nil {
				t.Fatal(err)
			}
			if m.streamSeek.guard != nil || m.streamSeek.request != nil {
				t.Fatal("accepted preview action retained a pending guard")
			}
			mx.Mix(nil)
			if !stop && (!mx.Live(old) || m.preview != old) {
				t.Fatal("cancelling verification displaced the preview")
			}
		})
	}
}

func TestGuardedMemorySeekQueueFailurePreservesBothVoices(t *testing.T) {
	m, mx, clip := guardedSeekFixture(t, false)
	old := m.preview
	game := mx.Play(mix.PlayParams{Memory: &mix.Memory{PCM: make([]float32, 1000), Rate: 48000, Channels: 1}, Gain: 1})
	mx.Mix(nil)
	before, live := m.PreviewPosition()
	if !live || m.RequestPreviewMemorySeek(clip, 100) != nil || m.streamSeek.guard == nil {
		t.Fatal("could not begin guarded memory seek")
	}
	count := 0
	for count < 4096 && mx.Commands().Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
		count++
	}
	if count == 4096 {
		t.Fatal("fixture did not fill the mixer command queue")
	}
	for m.streamSeek.guard != nil {
		m.pollStreamSeek()
	}
	if m.TakePreviewStreamSeekError() == nil {
		t.Fatal("full mixer queue did not report the failed seek")
	}
	mx.Mix(nil)
	if after, active := m.PreviewPosition(); !active || before != after || m.preview != old || !mx.Live(game) || !m.PreviewPaused() {
		t.Fatal("rejected seek changed preview or game ownership")
	}
}

func TestGuardedPreviewSeekDiscardsAStaleGeneration(t *testing.T) {
	m, mx, clip := guardedSeekFixture(t, true)
	old := m.preview
	if err := m.RequestPreviewStreamSeek(clip, 100); err != nil || m.streamSeek.guard == nil {
		t.Fatal("could not start guarded stream seek", err)
	}
	m.bumpPreviewEpoch() // Simulate a newer accepted preview action.
	m.pollStreamSeek()
	if m.streamSeek.guard != nil || m.streamSeek.request != nil || m.TakePreviewStreamSeekError() != nil ||
		m.preview != old || !mx.Live(old) {
		t.Fatal("stale verification displaced the current preview")
	}
}
