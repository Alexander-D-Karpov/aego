package audio

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/core/ids"
	audiomod "aego/modules/audio"
)

// The embedded Resolver methods are not used by audio.Module. Audio provides
// only the immutable synthetic artifact for this test.
type previewPauseResolver struct {
	asset.Resolver
	id  ids.AssetID
	raw []byte
}

func (r *previewPauseResolver) Audio(id ids.AssetID) ([]byte, bool) {
	return r.raw, id == r.id
}

func TestPausePreviewLeavesGameVoiceRunning(t *testing.T) {
	samples := tone(440, 4*rate, .2).PCM
	wavData := wav.Encode(samples, rate, 1, wav.EncodeOptions{Float: true})
	oggData, err := os.ReadFile(filepath.Join("..", "..", "audio", "codec", "testdata", "tone.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := codec.Open(bytes.NewReader(oggData), "tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	info := decoder.Info()
	if err := decoder.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		streaming bool
		format    uint8
		rate      int
		channels  int
		frames    int64
		encoded   []byte
	}{
		{"memory", false, asset.AudioWAV, rate, 1, int64(len(samples)), wavData},
		{"wav-stream", true, asset.AudioWAV, rate, 1, int64(len(samples)), wavData},
		{"ogg-stream", true, asset.AudioOgg, info.SampleRate, info.Channels, info.Frames, oggData},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			id := ids.NewAssetID()
			resolver := &previewPauseResolver{id: id, raw: asset.EncodeAudio(&asset.AudioArtifact{
				Format: tc.format, Rate: tc.rate, Channels: tc.channels, Frames: tc.frames,
				Gain: 1, Bus: "sfx", Stream: tc.streaming, Data: tc.encoded,
			})}
			game := r.mod.Play(tone(220, 4*rate, .1), "sfx", audiomod.PlayOptions{Loop: true})
			if !game.Valid() {
				t.Fatal("could not create the independent game voice")
			}
			if err := r.mod.Preview(resolver, id, true); err != nil {
				t.Fatal(err)
			}
			var before float32
			for tries := 0; tries < 500; tries++ {
				r.frames(1)
				before, _ = r.mod.PreviewPosition()
				if before > .01 && r.mod.Mixer().Position(game) > .01 {
					break
				}
				time.Sleep(time.Millisecond) // Allow the real streaming feeder to fill its ring.
			}
			if before <= .01 {
				t.Fatal("synthetic preview did not start")
			}
			if err := r.mod.SetPreviewPaused(true); err != nil || !r.mod.PreviewPaused() {
				t.Fatal("preview pause was not queued", err)
			}
			r.frames(4) // Apply the command and let previously buffered blocks drain.
			paused, live := r.mod.PreviewPosition()
			gameBefore := r.mod.Mixer().Position(game)
			r.frames(8)
			again, stillLive := r.mod.PreviewPosition()
			gameAfter := r.mod.Mixer().Position(game)
			if !live || !stillLive || paused != again || gameAfter <= gameBefore {
				t.Fatalf("pause affected the wrong voice: preview %v -> %v, game %v -> %v", paused, again, gameBefore, gameAfter)
			}
			if err := r.mod.SetPreviewPaused(false); err != nil || r.mod.PreviewPaused() {
				t.Fatal("preview resume was not queued", err)
			}
			r.frames(8)
			resumed, live := r.mod.PreviewPosition()
			if !live || resumed <= paused {
				t.Fatalf("preview did not resume: %v -> %v", paused, resumed)
			}
			if err := r.mod.SetPreviewPaused(true); err != nil {
				t.Fatal(err)
			}
			if err := r.mod.StopPreview(); err != nil {
				t.Fatal("could not stop preview", err)
			}
			if r.mod.PreviewPaused() || !r.mod.PreviewClip().IsZero() {
				t.Fatal("stopping preview retained its pause state")
			}
			if err := r.mod.Preview(resolver, id, true); err != nil || r.mod.PreviewPaused() {
				t.Fatal("a new preview inherited the old pause state", err)
			}
		})
	}
}

func TestPausePreviewReportsQueueBackpressure(t *testing.T) {
	r := newRig(t)
	if err := r.mod.SetPreviewPaused(true); err == nil {
		t.Fatal("paused without an active preview")
	}
	if err := r.mod.Preview(nil, asset.BuiltinMelody, true); err != nil {
		t.Fatal(err)
	}
	r.frames(3)
	queue := r.mod.Mixer().Commands()
	count := 0
	for count < 4096 && queue.Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
		count++
	}
	if count == 4096 {
		t.Fatal("fixture did not fill the bounded command queue")
	}
	if err := r.mod.SetPreviewPaused(true); err == nil || r.mod.PreviewPaused() {
		t.Fatal("reported pause despite a rejected command")
	}
	r.frames(3)
	if err := r.mod.SetPreviewPaused(true); err != nil || !r.mod.PreviewPaused() {
		t.Fatal("could not retry pause after the queue drained", err)
	}
	r.frames(3)
	count = 0
	for count < 4096 && queue.Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
		count++
	}
	if count == 4096 {
		t.Fatal("fixture did not fill the command queue for resume")
	}
	if err := r.mod.SetPreviewPaused(false); err == nil || !r.mod.PreviewPaused() {
		t.Fatal("reported resume despite a rejected command")
	}
	r.frames(3)
	if err := r.mod.SetPreviewPaused(false); err != nil || r.mod.PreviewPaused() {
		t.Fatal("could not retry resume after the queue drained", err)
	}
}

func TestPausePreviewKeepsRapidCommandsOrdered(t *testing.T) {
	r := newRig(t)
	if err := r.mod.Preview(nil, asset.BuiltinMelody, true); err != nil {
		t.Fatal(err)
	}
	r.frames(3)
	queue := r.mod.Mixer().Commands()
	before := queue.Len()
	if err := r.mod.SetPreviewPaused(true); err != nil {
		t.Fatal(err)
	}
	if err := r.mod.SetPreviewPaused(true); err != nil || queue.Len() != before+1 {
		t.Fatal("repeated pause enqueued a duplicate command", err)
	}
	if err := r.mod.SetPreviewPaused(false); err != nil || queue.Len() != before+2 {
		t.Fatal("rapid resume was not queued after pause", err)
	}
	r.frames(4)
	start, live := r.mod.PreviewPosition()
	r.frames(8)
	end, stillLive := r.mod.PreviewPosition()
	if !live || !stillLive || r.mod.PreviewPaused() || end <= start {
		t.Fatalf("ordered pause/resume left playback paused: %v -> %v", start, end)
	}
}

func fillPreviewCommandQueue(t *testing.T, r *rig) int {
	t.Helper()
	queue := r.mod.Mixer().Commands()
	count := 0
	for count < 4096 && queue.Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
		count++
	}
	if count == 4096 {
		t.Fatal("fixture did not fill the bounded command queue")
	}
	return count
}

func TestStopPreviewKeepsControlWhenQueueIsFull(t *testing.T) {
	for _, paused := range []bool{false, true} {
		t.Run(map[bool]string{false: "playing", true: "paused"}[paused], func(t *testing.T) {
			r := newRig(t)
			if err := r.mod.Preview(nil, asset.BuiltinMelody, true); err != nil {
				t.Fatal(err)
			}
			r.frames(3)
			if paused {
				if err := r.mod.SetPreviewPaused(true); err != nil {
					t.Fatal(err)
				}
				r.frames(3)
			}
			fillPreviewCommandQueue(t, r)
			if err := r.mod.StopPreview(); err == nil {
				t.Fatal("did not report the rejected Stop")
			}
			if _, live := r.mod.PreviewPosition(); !live || r.mod.PreviewClip() != asset.BuiltinMelody || r.mod.PreviewPaused() != paused {
				t.Fatal("rejected Stop lost the live preview or its pause state")
			}
			r.frames(3)
			if err := r.mod.StopPreview(); err != nil {
				t.Fatal("could not retry Stop", err)
			}
			r.frames(3)
			if !r.mod.PreviewClip().IsZero() || r.mod.PreviewPaused() || r.mod.Mixer().Active() != 0 {
				t.Fatal("retrying Stop did not stop and release the preview")
			}
		})
	}
}

func TestPreviewReportsRejectedStartAndPreservesPriorVoice(t *testing.T) {
	t.Run("new preview", func(t *testing.T) {
		r := newRig(t)
		fillPreviewCommandQueue(t, r)
		if err := r.mod.Preview(nil, asset.BuiltinMelody, true); err == nil {
			t.Fatal("reported successful Play although the command queue was full")
		}
		if !r.mod.PreviewClip().IsZero() || r.mod.PreviewPaused() {
			t.Fatal("rejected Play left a phantom preview")
		}
	})
	t.Run("replacement", func(t *testing.T) {
		r := newRig(t)
		if err := r.mod.Preview(nil, asset.BuiltinMelody, true); err != nil {
			t.Fatal(err)
		}
		r.frames(3)
		if err := r.mod.SetPreviewPaused(true); err != nil {
			t.Fatal(err)
		}
		r.frames(3)
		fillPreviewCommandQueue(t, r)
		if err := r.mod.Preview(nil, asset.BuiltinHum, true); err == nil {
			t.Fatal("reported replacement despite a rejected Stop")
		}
		if _, live := r.mod.PreviewPosition(); !live || r.mod.PreviewClip() != asset.BuiltinMelody || !r.mod.PreviewPaused() {
			t.Fatal("rejected replacement lost control of the original paused preview")
		}
	})
	t.Run("only stop fits", func(t *testing.T) {
		r := newRig(t)
		if err := r.mod.Preview(nil, asset.BuiltinMelody, true); err != nil {
			t.Fatal(err)
		}
		r.frames(3)
		count := fillPreviewCommandQueue(t, r)
		r.frames(3)
		queue := r.mod.Mixer().Commands()
		if queue.Len() != 0 || count < 2 {
			t.Fatal("fixture did not drain the queue")
		}
		for range count - 1 {
			if !queue.Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
				t.Fatal("could not leave exactly one slot for Stop")
			}
		}
		if err := r.mod.Preview(nil, asset.BuiltinHum, true); err == nil {
			t.Fatal("reported replacement when Play could not be queued")
		}
		if !r.mod.PreviewClip().IsZero() || r.mod.PreviewPaused() {
			t.Fatal("rejected replacement left a phantom preview")
		}
		r.frames(3)
		if r.mod.Mixer().Active() != 0 {
			t.Fatal("old preview was not stopped after accepting Stop")
		}
	})
	t.Run("invalid replacement", func(t *testing.T) {
		r := newRig(t)
		if err := r.mod.Preview(nil, asset.BuiltinMelody, true); err != nil {
			t.Fatal(err)
		}
		r.frames(3)
		if err := r.mod.SetPreviewPaused(true); err != nil {
			t.Fatal(err)
		}
		if err := r.mod.Preview(nil, ids.AssetID{}, false); err == nil {
			t.Fatal("accepted a missing clip")
		}
		if _, live := r.mod.PreviewPosition(); !live || r.mod.PreviewClip() != asset.BuiltinMelody || !r.mod.PreviewPaused() {
			t.Fatal("invalid replacement disturbed the existing paused preview")
		}
	})
}
