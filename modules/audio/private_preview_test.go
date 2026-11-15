package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
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

func TestPreparedPrivatePreviewRejectsChangedOutputWithoutStoppingVoice(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	oldID, newID := ids.NewAssetID(), ids.NewAssetID()
	old := &previewLifecycleResolver{id: oldID, raw: privatePreviewArtifact(.5, false)}
	if err := m.Preview(old, oldID, true); err != nil {
		t.Fatal(err)
	}
	mx.Mix(make([]float32, 512))
	if err := m.SetPreviewPaused(true); err != nil {
		t.Fatal(err)
	}
	voice := m.preview
	raw := privatePreviewArtifact(-.5, false)
	type preparedResult struct {
		value *PreparedPreview
		err   error
	}
	done := make(chan preparedResult, 1)
	go func() {
		value, err := PreparePreviewArtifact(context.Background(), newID, raw, 48000)
		done <- preparedResult{value, err}
	}()
	got := <-done
	if got.err != nil || got.value == nil {
		t.Fatal("worker could not prepare private audio", got.err)
	}
	for i := range raw {
		raw[i] = 0
	}
	mx.SetSampleRate(44100)
	if err := m.PreviewPrepared(got.value, true); err == nil {
		t.Fatal("accepted preview prepared for the wrong output sample rate")
	}
	if m.preview != voice || m.PreviewClip() != oldID || !m.PreviewPaused() || !mx.Live(voice) {
		t.Fatal("rejected prepared preview displaced the existing voice")
	}
	mx.SetSampleRate(48000)
	if err := m.PreviewPrepared(got.value, true); err != nil || m.PreviewClip() != newID || m.PreviewPaused() {
		t.Fatal("valid prepared preview could not replace the old voice", err)
	}
	out := make([]float32, 512)
	mx.Mix(out)
	var sum float64
	for _, sample := range out {
		sum += float64(sample)
	}
	if sum >= 0 {
		t.Fatalf("prepared preview used mutated or old audio: %v", sum)
	}
	if err := m.StopPreview(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparePrivatePreviewRejectsCancelledContext(t *testing.T) {
	id := ids.NewAssetID()
	raw := privatePreviewArtifact(-.5, false)
	request, cancel := context.WithCancel(context.Background())
	cancel()
	if prepared, err := PreparePreviewArtifact(request, id, raw, 48000); prepared != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation: %+v %v", prepared, err)
	}
	var absent context.Context // Exercise the API's explicit nil-context rejection.
	if prepared, err := PreparePreviewArtifact(absent, id, raw, 48000); prepared != nil || err == nil {
		t.Fatalf("accepted a nil context: %+v %v", prepared, err)
	}
}

func TestPreviewEpochTracksOnlyAcceptedControlChanges(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	resolver := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.5, false)}
	initial := m.PreviewEpoch()
	if err := m.Preview(resolver, id, true); err != nil || m.PreviewEpoch() <= initial {
		t.Fatal("accepted play did not change the preview epoch", err)
	}
	before := m.PreviewEpoch()
	if err := m.SetPreviewPaused(true); err != nil || m.PreviewEpoch() <= before {
		t.Fatal("accepted pause did not change the preview epoch", err)
	}
	before = m.PreviewEpoch()
	if err := m.SetPreviewPaused(true); err != nil || m.PreviewEpoch() != before {
		t.Fatal("a redundant pause changed the preview epoch", err)
	}
	queue := mx.Commands()
	count := 0
	for count < 4096 && queue.Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
		count++
	}
	if count == 4096 {
		t.Fatal("fixture did not fill the command queue")
	}
	if err := m.SetPreviewPaused(false); err == nil || m.PreviewEpoch() != before {
		t.Fatal("rejected resume changed the preview epoch")
	}
	mx.Mix(make([]float32, 512))
	if err := m.SetPreviewPaused(false); err != nil || m.PreviewEpoch() <= before {
		t.Fatal("accepted resume did not change the preview epoch", err)
	}
	before = m.PreviewEpoch()
	if err := m.StopPreview(); err != nil || m.PreviewEpoch() <= before {
		t.Fatal("accepted stop did not change the preview epoch", err)
	}
	m.previewEpoch = ^uint64(0) - 1
	m.bumpPreviewEpoch()
	m.bumpPreviewEpoch()
	if m.PreviewEpoch() != ^uint64(0) {
		t.Fatal("preview epoch wrapped instead of failing closed")
	}
}
