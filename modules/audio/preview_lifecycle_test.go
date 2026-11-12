package audio

import (
	"testing"

	"aego/asset"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/core/ids"
)

type previewLifecycleResolver struct {
	asset.Resolver
	id  ids.AssetID
	raw []byte
}

func (r *previewLifecycleResolver) Audio(id ids.AssetID) ([]byte, bool) {
	return r.raw, id == r.id
}

func TestPreviewStreamCancellationWaitsForAcceptedStop(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	pcm := make([]float32, 48000)
	src := &previewLifecycleResolver{id: id, raw: asset.EncodeAudio(&asset.AudioArtifact{
		Format: asset.AudioWAV, Rate: 48000, Channels: 1, Frames: int64(len(pcm)), Gain: 1, Stream: true,
		Data: wav.Encode(pcm, 48000, 1, wav.EncodeOptions{Float: true}),
	})}
	if err := m.Preview(src, id, true); err != nil {
		t.Fatal(err)
	}
	cancel, voice := m.previewCancel, m.preview
	if cancel == nil || !mx.Live(voice) {
		t.Fatal("stream preview did not retain its feeder")
	}
	t.Cleanup(func() {
		if m.previewCancel != nil {
			m.endPreview()
		}
	})
	queue := mx.Commands()
	count := 0
	for count < 4096 && queue.Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
		count++
	}
	if count == 4096 {
		t.Fatal("fixture did not fill the command queue")
	}
	if err := m.StopPreview(); err == nil || m.preview != voice || m.previewCancel != cancel {
		t.Fatal("rejected Stop lost the stream preview")
	}
	select {
	case <-cancel:
		t.Fatal("rejected Stop cancelled the live stream feeder")
	default:
	}
	mx.Mix(make([]float32, 1024)) // Drain Play and the full queue without an audio device.
	if err := m.StopPreview(); err != nil {
		t.Fatal("could not retry Stop", err)
	}
	select {
	case <-cancel:
	default:
		t.Fatal("accepted Stop did not cancel the stream feeder")
	}
	mx.Mix(make([]float32, 2048))
	if mx.Live(voice) || !m.PreviewClip().IsZero() {
		t.Fatal("accepted Stop did not release the stream voice")
	}
}
