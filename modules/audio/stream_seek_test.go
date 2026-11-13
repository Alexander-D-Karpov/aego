package audio

import (
	"bytes"
	"math"
	"testing"
	"time"

	"aego/asset"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/core/ids"
)

func newStreamPreviewFixture(t *testing.T, loop bool) (*Module, *mix.Mixer, ids.AssetID, []float32) {
	t.Helper()
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Voices: 4, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	pcm := make([]float32, 400*2)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i)*.031)) * .4
	}
	res := &previewLifecycleResolver{id: id, raw: asset.EncodeAudio(&asset.AudioArtifact{
		Format: asset.AudioWAV, Rate: 48000, Channels: 2, Frames: 400, Gain: 1, Stream: true,
		LoopStart: 100, LoopEnd: 300, Data: wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true}),
	})}
	if err := m.Preview(res, id, loop); err != nil {
		t.Fatal(err)
	}
	mx.Mix(nil)
	t.Cleanup(func() { m.endPreview() })
	return m, mx, id, pcm
}

func waitStreamSeek(t *testing.T, m *Module, until func() bool) {
	t.Helper()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !until() {
		m.pollStreamSeek()
		if err := m.TakePreviewStreamSeekError(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-timeout.C:
			t.Fatal("stream seek did not reach the expected state")
		case <-tick.C:
		}
	}
}

func TestStreamPreviewSeekRendersExactLoopPCMAndKeepsGameVoice(t *testing.T) {
	m, mx, id, pcm := newStreamPreviewFixture(t, true)
	gamePCM := make([]float32, 1000)
	for i := range gamePCM {
		gamePCM[i] = .05
	}
	game := mx.Play(mix.PlayParams{Memory: &mix.Memory{PCM: gamePCM, Rate: 48000, Channels: 1}, Bus: int(BusSfx), Gain: 1})
	if !game.Valid() || m.SetPreviewPaused(true) != nil {
		t.Fatal("could not prepare game and paused preview")
	}
	mx.Mix(nil)
	old, oldCancel := m.preview, m.previewCancel
	if err := m.SeekPreviewStream(298); err != nil {
		t.Fatal(err)
	}
	if err := m.SeekPreviewStream(200); err == nil {
		t.Fatal("accepted a second concurrent preparation")
	}
	waitStreamSeek(t, m, func() bool { return m.streamSeek.pending != nil })
	next := m.streamSeek.pending.next
	if !mx.Live(old) || mx.Started(next) || m.PreviewClip() != id {
		t.Fatal("preview changed before the replacement reached the mixer")
	}
	mx.Mix(nil)
	m.update(0)
	if m.preview != next || mx.Live(old) || !mx.Started(next) || !mx.Live(game) || !m.PreviewPaused() {
		t.Fatal("replacement did not preserve preview and game ownership")
	}
	select {
	case <-oldCancel:
	default:
		t.Fatal("old feeder was retained after replacement acknowledgement")
	}
	if pos, live := m.PreviewPosition(); !live || math.Abs(float64(pos)-298.0/48000) > 1e-6 {
		t.Fatalf("paused position = %v/%v", pos, live)
	}
	if err := m.SetPreviewPaused(false); err != nil {
		t.Fatal(err)
	}
	ref, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Voices: 4, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	wantPreview := ref.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: 48000, Channels: 2,
		LoopStart: 100, LoopEnd: 300}, Bus: int(BusUI), Gain: 1, Loop: true})
	wantGame := ref.Play(mix.PlayParams{Memory: &mix.Memory{PCM: gamePCM, Rate: 48000, Channels: 1}, Bus: int(BusSfx), Gain: 1})
	if !wantPreview.Valid() || !wantGame.Valid() || !ref.SeekMemory(wantPreview, 298) {
		t.Fatal("could not create independent PCM reference")
	}
	got, want := make([]float32, 20), make([]float32, 20)
	mx.Mix(got)
	ref.Mix(want)
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("sample %d = %v, reference %v", i, got[i], want[i])
		}
	}
	if pos, live := m.PreviewPosition(); !live || math.Abs(float64(pos)-108.0/48000) > 1e-6 {
		t.Fatalf("position did not wrap with loop: %v/%v", pos, live)
	}
	if mx.Position(game) != ref.Position(wantGame) {
		t.Fatal("stream seek moved the game voice")
	}
}

func TestStreamPreviewSeekCancellationKeepsOriginalVoice(t *testing.T) {
	m, mx, _, _ := newStreamPreviewFixture(t, false)
	old, cancel := m.preview, m.previewCancel
	if err := m.SeekPreviewStream(100); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPreviewPaused(true); err != nil {
		t.Fatal(err)
	}
	mx.Mix(nil)
	waitStreamSeek(t, m, func() bool { return !m.PreviewStreamSeekPending() })
	if m.preview != old || !mx.Live(old) || !m.PreviewPaused() {
		t.Fatal("cancelled seek displaced the original preview")
	}
	select {
	case <-cancel:
		t.Fatal("cancelled seek stopped the original feeder")
	default:
	}
}

func TestStreamPreviewSeekRejectsExhaustedGenerationAndStaleWork(t *testing.T) {
	m, mx, _, _ := newStreamPreviewFixture(t, true)
	old, oldCancel := m.preview, m.previewCancel
	m.previewEpoch = ^uint64(0) - 1
	if err := m.SeekPreviewStream(100); err == nil || m.PreviewStreamSeekPending() || m.PreviewEpoch() != ^uint64(0)-1 {
		t.Fatal("accepted an asynchronous seek with a saturating generation")
	}
	m.previewEpoch = 1
	if err := m.SeekPreviewStream(300); err == nil || m.PreviewStreamSeekPending() {
		t.Fatal("accepted a seek past the active loop end")
	}
	if err := m.SeekPreviewStream(100); err != nil {
		t.Fatal(err)
	}
	// Simulate a newer accepted operation before the asynchronous result is
	// delivered, including the case where its feeder was already primed.
	m.bumpPreviewEpoch()
	waitStreamSeek(t, m, func() bool { return !m.PreviewStreamSeekPending() })
	if m.preview != old || !mx.Live(old) || m.streamSeek.pending != nil {
		t.Fatal("stale prepared seek displaced the original voice")
	}
	select {
	case <-oldCancel:
		t.Fatal("stale seek cancelled the original feeder")
	default:
	}
}

func TestStreamPreviewSeekRejectsChangedOwnedSource(t *testing.T) {
	m, mx, _, _ := newStreamPreviewFixture(t, false)
	old, oldCancel, epoch := m.preview, m.previewCancel, m.PreviewEpoch()
	changed := bytes.Clone(m.previewArtifact.Data)
	changed[len(changed)-1] ^= 1
	m.previewArtifact.Data = changed // The already-running reader keeps its old slice.
	if err := m.SeekPreviewStream(100); err == nil || m.PreviewStreamSeekPending() || m.PreviewEpoch() != epoch {
		t.Fatal("accepted a changed preview source snapshot")
	}
	if m.preview != old || !mx.Live(old) {
		t.Fatal("source conflict displaced the preview")
	}
	select {
	case <-oldCancel:
		t.Fatal("source conflict cancelled the active feeder")
	default:
	}
}

func TestStreamPreviewSeekPreservesAcceptedGain(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Voices: 4, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	res := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.3, true)}
	clip, err := m.Clip(res, id)
	if err != nil {
		t.Fatal(err)
	}
	clip.Gain = .25 // Deliberately differs from the artifact's encoded gain.
	if err := m.Preview(res, id, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.endPreview() })
	mx.Mix(nil)
	if err := m.SeekPreviewStream(100); err != nil {
		t.Fatal(err)
	}
	waitStreamSeek(t, m, func() bool { return m.streamSeek.pending != nil })
	mx.Mix(nil)
	m.update(0)
	ref, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Voices: 4, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]float32, 8192)
	for i := range pcm {
		pcm[i] = .3
	}
	idRef := ref.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: 48000, Channels: 1}, Bus: int(BusUI), Gain: .25})
	if !idRef.Valid() || !ref.SeekMemory(idRef, 100) {
		t.Fatal("could not create gain reference")
	}
	got, want := make([]float32, 32), make([]float32, 32)
	mx.Mix(got)
	ref.Mix(want)
	for i := range got {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Fatalf("sample %d = %v, gain reference %v", i, got[i], want[i])
		}
	}
}

func TestStreamPreviewSeekHandlesRejectedReplacementAndDeadOldVoice(t *testing.T) {
	for _, oldDies := range []bool{false, true} {
		name := "invalid-new-stream"
		if oldDies {
			name = "old-ends-first"
		}
		t.Run(name, func(t *testing.T) {
			m, mx, _, _ := newStreamPreviewFixture(t, false)
			old, oldCancel := m.preview, m.previewCancel
			game := mx.Play(mix.PlayParams{Memory: &mix.Memory{PCM: make([]float32, 4096), Rate: 48000, Channels: 1}, Gain: 1})
			mx.Mix(nil)
			if err := m.SeekPreviewStream(100); err != nil {
				t.Fatal(err)
			}
			if oldDies && !mx.Kill(old) {
				t.Fatal("could not queue the old voice ending")
			}
			waitStreamSeek(t, m, func() bool { return m.streamSeek.pending != nil })
			pending := m.streamSeek.pending
			if !oldDies {
				pending.prepared.stream.Rate = 0 // Mixer rejects this before touching the old voice.
			}
			mx.Mix(nil)
			m.update(0)
			if m.TakePreviewStreamSeekError() == nil || mx.Live(pending.next) || !mx.Live(game) {
				t.Fatal("replacement failure was not reported or disturbed the game")
			}
			if oldDies {
				if mx.Live(old) || !m.PreviewClip().IsZero() {
					t.Fatal("finished old voice was retained as the preview")
				}
			} else {
				if !mx.Live(old) || m.preview != old {
					t.Fatal("rejected replacement displaced the live old voice")
				}
				select {
				case <-oldCancel:
					t.Fatal("rejected replacement cancelled the old feeder")
				default:
				}
			}
		})
	}
}

func TestStreamPreviewSeekWaitsForRemoteAcknowledgement(t *testing.T) {
	cfg := mix.Config{SampleRate: 48000, Channels: 2, Voices: 4, Buses: BusDescs()}
	mx, err := mix.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mix.NewRemoteClient(mx)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := mix.NewRemoteMixer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	transfer := func() {
		t.Helper()
		for {
			command, ok, err := client.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				return
			}
			if err := remote.Accept(command); err != nil {
				t.Fatal(err)
			}
		}
	}
	apply := func() {
		t.Helper()
		batch, err := remote.Drain()
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Apply(batch); err != nil {
			t.Fatal(err)
		}
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	res := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.3, true)}
	if err := m.Preview(res, id, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.endPreview() })
	transfer()
	if err := remote.Mix(nil); err != nil {
		t.Fatal(err)
	}
	apply()
	old, oldCancel := m.preview, m.previewCancel
	if err := m.SeekPreviewStream(100); err != nil {
		t.Fatal(err)
	}
	waitStreamSeek(t, m, func() bool { return m.streamSeek.pending != nil })
	next := m.streamSeek.pending.next
	transfer()
	if err := remote.Mix(nil); err != nil {
		t.Fatal(err)
	}
	m.reconcileStreamSeek()
	if m.preview != old || mx.Started(next) {
		t.Fatal("unacknowledged worker replacement changed local ownership")
	}
	select {
	case <-oldCancel:
		t.Fatal("old feeder stopped before the worker batch")
	default:
	}
	apply()
	m.update(0)
	if m.preview != next || !mx.Started(next) || mx.Live(old) {
		t.Fatal("validated worker batch did not transfer preview ownership")
	}
	select {
	case <-oldCancel:
	default:
		t.Fatal("acknowledged worker replacement retained the old feeder")
	}
}

func TestStreamPreviewSeekShutdownCancelsUnclaimedResult(t *testing.T) {
	m, _, _, _ := newStreamPreviewFixture(t, false)
	if err := m.SeekPreviewStream(100); err != nil {
		t.Fatal(err)
	}
	result := m.streamSeek.result
	m.Shutdown() // No later update/poll is required to cancel the worker result.
	select {
	case got := <-result:
		if got.prepared != nil {
			select {
			case <-got.prepared.stop:
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not cancel an unclaimed feeder")
			}
			select {
			case <-got.prepared.done:
			case <-time.After(5 * time.Second):
				t.Fatal("unclaimed feeder did not finish")
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish the seek worker")
	}
}

func TestStreamPreviewStopIsAtomicWhileReplacementPending(t *testing.T) {
	for _, fullQueue := range []bool{false, true} {
		name := "accepted"
		if fullQueue {
			name = "queue-full"
		}
		t.Run(name, func(t *testing.T) {
			m, mx, _, _ := newStreamPreviewFixture(t, false)
			game := mx.Play(mix.PlayParams{Memory: &mix.Memory{PCM: make([]float32, 4096), Rate: 48000, Channels: 1}, Gain: 1})
			mx.Mix(nil)
			old, oldCancel := m.preview, m.previewCancel
			if err := m.SeekPreviewStream(100); err != nil {
				t.Fatal(err)
			}
			waitStreamSeek(t, m, func() bool { return m.streamSeek.pending != nil })
			pending := m.streamSeek.pending
			if fullQueue {
				count := 0
				for count < 4096 && mx.Commands().Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
					count++
				}
				if count == 4096 {
					t.Fatal("fixture did not fill the queue")
				}
				if err := m.StopPreview(); err == nil || m.streamSeek.pending != pending || !mx.Live(old) {
					t.Fatal("rejected stop changed pending ownership")
				}
				for _, ch := range []<-chan struct{}{oldCancel, pending.prepared.stop} {
					select {
					case <-ch:
						t.Fatal("rejected stop cancelled a live feeder")
					default:
					}
				}
				mx.Mix(nil)
			}
			if err := m.StopPreview(); err != nil {
				t.Fatal(err)
			}
			mx.Mix(make([]float32, 2048))
			if mx.Live(old) || mx.Live(pending.next) || !mx.Live(game) || !m.PreviewClip().IsZero() {
				t.Fatal("accepted stop retained preview or stopped the game voice")
			}
			select {
			case <-pending.prepared.stop:
			default:
				t.Fatal("accepted stop retained the replacement feeder")
			}
		})
	}
}
