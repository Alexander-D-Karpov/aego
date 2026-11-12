package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"aego/asset"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/core/ids"
)

func privatePreviewArtifact(value float32, stream bool) []byte {
	pcm := make([]float32, 8192)
	for i := range pcm {
		pcm[i] = value
	}
	return asset.EncodeAudio(&asset.AudioArtifact{
		Format: asset.AudioWAV, Rate: 48000, Channels: 1, Frames: int64(len(pcm)),
		Gain: 1, Stream: stream, Data: wav.Encode(pcm, 48000, 1, wav.EncodeOptions{Float: true}),
	})
}

func TestPrivatePreviewUsesCandidateWithoutChangingSharedCache(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "memory"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
			if err != nil {
				t.Fatal(err)
			}
			m := &Module{mixer: mx}
			t.Cleanup(func() {
				if m.previewCancel != nil {
					m.endPreview()
				}
			})
			id := ids.NewAssetID()
			saved := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.5, false)}
			original, err := m.Clip(saved, id)
			if err != nil || original.Mem == nil || original.Mem.PCM[0] != .5 {
				t.Fatal("could not cache original audio", err)
			}
			candidate := privatePreviewArtifact(-.5, stream)
			if err := m.PreviewArtifact(id, candidate, true); err != nil {
				t.Fatal("could not start private preview", err)
			}
			// The module must not retain caller-owned bytes, including while a
			// stream feeder is using the payload on another goroutine.
			for i := range candidate {
				candidate[i] = 0
			}
			var total float64
			for attempt := 0; attempt < 100; attempt++ {
				out := make([]float32, 256)
				mx.Mix(out)
				for _, sample := range out {
					total += float64(sample)
				}
				if total != 0 {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if total >= 0 {
				t.Fatalf("preview played cached original instead of edited samples: sum=%v", total)
			}
			cached, err := m.Clip(saved, id)
			if err != nil || cached != original || cached.Mem.PCM[0] != .5 {
				t.Fatal("private preview changed the shared clip cache", err)
			}
		})
	}
}

func TestPrivatePreviewInvalidCandidateKeepsExistingVoice(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	saved := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.5, false)}
	if err := m.Preview(saved, id, true); err != nil {
		t.Fatal(err)
	}
	mx.Mix(make([]float32, 512))
	if err := m.SetPreviewPaused(true); err != nil {
		t.Fatal(err)
	}
	voice := m.preview
	valid := privatePreviewArtifact(-.5, true)
	for _, reason := range []string{"empty", "oversized", "wrong-format", "corrupt-payload", "wrong-rate", "wrong-frames", "invalid-loop", "non-finite-gain", "missing-id"} {
		t.Run(reason, func(t *testing.T) {
			candidate := bytes.Clone(valid)
			candidateID := id
			switch reason {
			case "empty":
				candidate = nil
			case "oversized":
				candidate = make([]byte, maxPrivatePreviewBytes+1)
			case "wrong-format":
				candidate[8] = asset.AudioOgg
			case "corrupt-payload":
				candidate[46] = 0
			case "wrong-rate":
				binary.LittleEndian.PutUint32(candidate[12:16], 44100)
			case "wrong-frames":
				binary.LittleEndian.PutUint64(candidate[16:24], 8191)
			case "invalid-loop":
				binary.LittleEndian.PutUint64(candidate[24:32], 10)
				binary.LittleEndian.PutUint64(candidate[32:40], 5)
			case "non-finite-gain":
				binary.LittleEndian.PutUint32(candidate[40:44], math.Float32bits(float32(math.NaN())))
			case "missing-id":
				candidateID = ids.AssetID{}
			}
			if err := m.PreviewArtifact(candidateID, candidate, true); err == nil {
				t.Fatal("accepted invalid candidate")
			}
			if m.preview != voice || m.PreviewClip() != id || !m.PreviewPaused() || !mx.Live(voice) {
				t.Fatal("invalid candidate displaced the existing preview")
			}
		})
	}
	if err := m.StopPreview(); err != nil {
		t.Fatal(err)
	}
}
