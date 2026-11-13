package audio

import (
	"bytes"
	"errors"
	"testing"

	"aego/asset"
	"aego/audio/codec/wav"
	"aego/audio/mix"
	"aego/core/ids"
)

func TestPreviewSourceMatchesCurrentOrPrivateArtifact(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "stream"}[stream], func(t *testing.T) {
			mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
			if err != nil {
				t.Fatal(err)
			}
			m := &Module{mixer: mx}
			id := ids.NewAssetID()
			raw := privatePreviewArtifact(.3, stream)
			res := &previewLifecycleResolver{id: id, raw: raw}
			clip, err := m.Clip(res, id)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.PreviewArtifact(id, raw, false); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { m.endPreview() })
			if m.previewArtifact == clip.Art || !m.PreviewSourceMatches(clip) {
				t.Fatal("equivalent private source did not match the displayed clip")
			}
			other := *clip
			changed := *clip.Art
			changed.Data = bytes.Clone(changed.Data)
			other.Art = &changed
			changed.Gain = 2 // Import settings do not change source PCM coordinates.
			if !m.PreviewSourceMatches(&other) {
				t.Fatal("settings-only changes incorrectly changed source identity")
			}
			changed.Data[len(changed.Data)-1] ^= 1
			if m.PreviewSourceMatches(&other) {
				t.Fatal("different source bytes matched the active preview")
			}
			other.ID = ids.NewAssetID()
			if m.PreviewSourceMatches(&other) || m.PreviewSourceMatches(nil) {
				t.Fatal("invalid clip identity matched the preview")
			}
			if err := m.StopPreview(); err != nil || m.PreviewSourceMatches(clip) {
				t.Fatal("stopped preview still matched a clip", err)
			}
		})
	}
}

func TestPreviewSourceMatchRejectsInPlaceClipMutation(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "stream"}[stream], func(t *testing.T) {
			mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
			if err != nil {
				t.Fatal(err)
			}
			m := &Module{mixer: mx}
			id := ids.NewAssetID()
			raw := privatePreviewArtifact(.3, stream)
			res := &previewLifecycleResolver{id: id, raw: raw}
			clip, err := m.Clip(res, id)
			if err != nil || m.Preview(res, id, false) != nil || !m.PreviewSourceMatches(clip) {
				t.Fatal("could not start matching preview", err)
			}
			t.Cleanup(func() { m.endPreview() })
			// Replace only the exported clip's in-memory slice header. The
			// original stream reader and synthetic source bytes are untouched.
			changed := bytes.Clone(clip.Art.Data)
			changed[len(changed)-1] ^= 1
			clip.Art.Data = changed
			if m.PreviewSourceMatches(clip) {
				t.Fatal("mutated clip pointer still matched its old preview source")
			}
		})
	}
}

func TestPreviewRejectsChangedCachedMemorySourceBeforeStoppingCurrentVoice(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Clip, []byte)
	}{
		{"source-bytes-in-place", func(c *Clip, replacement []byte) { copy(c.Art.Data, replacement) }},
		{"source-slice-replaced", func(c *Clip, replacement []byte) { c.Art.Data = bytes.Clone(replacement) }},
		{"asset-id", func(c *Clip, _ []byte) { c.ID = ids.NewAssetID() }},
		{"source-format", func(c *Clip, _ []byte) { c.Art.Format++ }},
		{"source-rate", func(c *Clip, _ []byte) { c.Art.Rate++ }},
		{"source-channels", func(c *Clip, _ []byte) { c.Art.Channels++ }},
		{"source-frames", func(c *Clip, _ []byte) { c.Art.Frames-- }},
		{"source-stream", func(c *Clip, _ []byte) { c.Art.Stream = !c.Art.Stream }},
		{"source-loop", func(c *Clip, _ []byte) { c.Art.Loop = !c.Art.Loop }},
		{"memory-pointer", func(c *Clip, _ []byte) { m := *c.Mem; c.Mem = &m }},
		{"memory-buffer", func(c *Clip, _ []byte) { c.Mem.PCM = append([]float32(nil), c.Mem.PCM...) }},
		{"memory-rate", func(c *Clip, _ []byte) { c.Mem.Rate++ }},
		{"memory-loop", func(c *Clip, _ []byte) { c.Mem.LoopEnd-- }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
			if err != nil {
				t.Fatal(err)
			}
			m := &Module{mixer: mx}
			t.Cleanup(func() { m.endPreview() })
			id := ids.NewAssetID()
			res := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.3, false)}
			clip, err := m.Clip(res, id)
			if err != nil || clip.Mem == nil || clip.Mem.PCM[100] <= 0 {
				t.Fatal("could not cache the original memory clip", err)
			}
			other := &previewLifecycleResolver{id: ids.NewAssetID(), raw: privatePreviewArtifact(.6, false)}
			if err := m.Preview(other, other.id, false); err != nil {
				t.Fatal(err)
			}
			mx.Mix(nil)
			old, epoch := m.preview, m.PreviewEpoch()
			replacement, err := asset.DecodeAudio(privatePreviewArtifact(-.3, false))
			if err != nil || len(replacement.Data) != len(clip.Art.Data) {
				t.Fatal("invalid replacement fixture", err)
			}
			tc.change(clip, replacement.Data)
			if err := m.Preview(res, id, false); !errors.Is(err, ErrPreviewSeekSourceChanged) {
				t.Fatalf("changed cached memory source was not rejected: %v", err)
			}
			if m.preview != old || m.PreviewEpoch() != epoch || !mx.Live(old) {
				t.Fatal("rejected preview disturbed the existing voice")
			}
		})
	}
}

func TestPreviewAcceptsRestoredCachedMemorySource(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	t.Cleanup(func() { m.endPreview() })
	id := ids.NewAssetID()
	res := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.3, false)}
	clip, err := m.Clip(res, id)
	if err != nil {
		t.Fatal(err)
	}
	original := bytes.Clone(clip.Art.Data)
	clip.Art.Data[len(clip.Art.Data)-1] ^= 1
	if err := m.Preview(res, id, false); !errors.Is(err, ErrPreviewSeekSourceChanged) {
		t.Fatal("changed encoded source was not rejected", err)
	}
	// A byte-identical replacement is safe; memory is still the decoded original.
	clip.Art.Data = original
	if err := m.Preview(res, id, false); err != nil || !m.PreviewSourceMatches(clip) {
		t.Fatal("restored encoded source was not accepted", err)
	}
	if err := m.RequestPreviewMemorySeek(clip, 100); err != nil {
		t.Fatal("restored source could not seek", err)
	}
	mx.Mix(nil)
	if position, live := m.PreviewPosition(); !live || position != float32(100)/48000 {
		t.Fatalf("restored preview sought to %v/%v", position, live)
	}
}

func TestPreviewMemoryWithoutDecodeProofDoesNotClaimSeekSafety(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	t.Cleanup(func() { m.endPreview() })
	clip, err := decodeClip(ids.NewAssetID(), privatePreviewArtifact(.3, false), 48000)
	if err != nil {
		t.Fatal(err)
	}
	clip.proof = memoryPreviewProof{} // e.g. an encoded source exceeds the proof budget
	if err := m.previewClip(clip, false); err != nil {
		t.Fatal("an unverified clip should still support ordinary playback", err)
	}
	mx.Mix(nil)
	if m.PreviewSourceMatches(clip) || !errors.Is(m.RequestPreviewMemorySeek(clip, 100), ErrPreviewSeekSourceChanged) {
		t.Fatal("unverified cached PCM was incorrectly certified for seeking")
	}
	if !mx.Live(m.preview) {
		t.Fatal("rejecting an unverified seek stopped ordinary playback")
	}
}

type previewCachedResolver struct {
	*previewLifecycleResolver
	decoded map[string]any
}

func (r *previewCachedResolver) Cached(_ ids.AssetID, key string) (any, bool, error) {
	v, ok := r.decoded[key]
	return v, ok, nil
}

func (r *previewCachedResolver) Decoded(_ ids.AssetID, key string, build func() (any, error)) (any, error) {
	if v, ok := r.decoded[key]; ok {
		return v, nil
	}
	v, err := build()
	if err == nil {
		r.decoded[key] = v
	}
	return v, err
}

func TestPreviewProofDoesNotHashGameClipAndRebuildsForEditor(t *testing.T) {
	for _, withDecoder := range []bool{false, true} {
		name := "module-cache"
		if withDecoder {
			name = "resolver-cache"
		}
		t.Run(name, func(t *testing.T) {
			mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
			if err != nil {
				t.Fatal(err)
			}
			m := &Module{mixer: mx}
			t.Cleanup(func() { m.endPreview() })
			id := ids.NewAssetID()
			src := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.3, false)}
			var res asset.Resolver = src
			var cache *previewCachedResolver
			if withDecoder {
				cache = &previewCachedResolver{previewLifecycleResolver: src, decoded: make(map[string]any)}
				res = cache
			}
			game, err := m.clip(res, id)
			if err != nil || game.Mem == nil || game.Mem.PCM[100] <= 0 || game.proof.valid {
				t.Fatal("game clip decode must not compute an editor proof", err)
			}
			voice := mx.Play(mix.PlayParams{Memory: game.Mem, Gain: 1, Loop: true})
			mx.Mix(nil)
			if !voice.Valid() || !mx.Live(voice) {
				t.Fatal("could not start the game voice")
			}
			replacement, err := asset.DecodeAudio(privatePreviewArtifact(-.3, false))
			if err != nil || len(game.Art.Data) != len(replacement.Data) {
				t.Fatal("invalid replacement fixture", err)
			}
			// Simulate the old unsealed cache referring to changed encoded bytes.
			// Rebuilding for the editor must not alter the live game's PCM.
			copy(game.Art.Data, replacement.Data)
			editor, err := m.Clip(res, id)
			if err != nil || editor == game || editor.Mem == game.Mem || !editor.proof.valid || editor.Mem.PCM[100] >= 0 {
				t.Fatal("editor did not prepare a coherent separate memory clip", err)
			}
			if again, err := m.Clip(res, id); err != nil || again != editor {
				t.Fatal("editor clip was not reused", err)
			}
			if again, err := m.clip(res, id); err != nil || again != game || game.Mem.PCM[100] <= 0 || !mx.Live(voice) {
				t.Fatal("editor lookup changed the game cache or voice", err)
			}
			if err := m.Preview(res, id, false); err != nil || !m.PreviewSourceMatches(editor) {
				t.Fatal("coherent editor preview did not start", err)
			}
			if err := m.RequestPreviewMemorySeek(editor, 100); err != nil {
				t.Fatal("coherent editor preview did not seek", err)
			}
			mx.Mix(nil)
			if !mx.Live(voice) {
				t.Fatal("preview seek stopped the game voice")
			}
			if cache != nil {
				// Real loader reloads invalidate both cache keys. A fresh editor
				// lookup must never return a clip from the old generation.
				clear(cache.decoded)
				src.raw = privatePreviewArtifact(.2, false)
				fresh, err := m.Clip(res, id)
				if err != nil || fresh == editor || !fresh.proof.valid || fresh.Mem.PCM[100] <= 0 {
					t.Fatal("editor retained a clip after cache invalidation", err)
				}
				if gameAgain, err := m.clip(res, id); err != nil || gameAgain != fresh || !mx.Live(voice) {
					t.Fatal("new generation did not share its editor-first clip or preserved game voice", err)
				}
			}
		})
	}
}

func TestPreviewProofSharesClipWhenEditorLoadsFirst(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	res := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.3, false)}
	editor, err := m.Clip(res, id)
	if err != nil || !editor.proof.valid {
		t.Fatal("editor clip lacks its decode-time proof", err)
	}
	game, err := m.clip(res, id)
	if err != nil || game != editor {
		t.Fatal("a clean editor-first clip was unnecessarily decoded twice", err)
	}
}

func TestPreviewProofGameCachedLookupDoesNotAllocate(t *testing.T) {
	for _, withDecoder := range []bool{false, true} {
		name := "module-cache"
		if withDecoder {
			name = "resolver-cache"
		}
		t.Run(name, func(t *testing.T) {
			mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
			if err != nil {
				t.Fatal(err)
			}
			m := &Module{mixer: mx}
			id := ids.NewAssetID()
			src := &previewLifecycleResolver{id: id, raw: privatePreviewArtifact(.3, false)}
			var res asset.Resolver = src
			if withDecoder {
				res = &previewCachedResolver{previewLifecycleResolver: src, decoded: make(map[string]any)}
			}
			original, err := m.clip(res, id)
			if err != nil {
				t.Fatal(err)
			}
			var got *Clip
			var lookupErr error
			allocs := testing.AllocsPerRun(100, func() { got, lookupErr = m.clip(res, id) })
			if lookupErr != nil || got != original || allocs != 0 {
				t.Fatalf("cached game lookup: clip=%p/%p, allocs=%v, error=%v", got, original, allocs, lookupErr)
			}
		})
	}
}

func TestPreviewMemorySeekKeepsPauseLoopAndOtherVoices(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	pcm := make([]float32, 128)
	for i := range pcm {
		pcm[i] = float32(i) / 256
	}
	res := &previewLifecycleResolver{id: id, raw: asset.EncodeAudio(&asset.AudioArtifact{
		Format: asset.AudioWAV, Rate: 48000, Channels: 1, Frames: int64(len(pcm)), Gain: 1,
		LoopStart: 10, LoopEnd: 80, Data: wav.Encode(pcm, 48000, 1, wav.EncodeOptions{Float: true}),
	})}
	if err := m.Preview(res, id, true); err != nil {
		t.Fatal(err)
	}
	game := mx.Play(mix.PlayParams{Memory: &mix.Memory{PCM: pcm, Rate: 48000, Channels: 1}, Gain: 1})
	if !game.Valid() || m.SetPreviewPaused(true) != nil {
		t.Fatal("could not start game and paused preview voices")
	}
	mx.Mix(make([]float32, 16))
	gameBefore := mx.Position(game)
	frames, rate, ok := m.PreviewMemorySeekRange()
	if !ok || frames != 80 || rate != 48000 {
		t.Fatalf("seek range = (%d, %d, %v)", frames, rate, ok)
	}
	before := m.PreviewEpoch()
	for _, frame := range []int64{-1, 80, 128} {
		if err := m.SeekPreviewMemory(frame); err == nil || m.PreviewEpoch() != before {
			t.Fatalf("invalid frame %d changed preview state", frame)
		}
	}
	if err := m.SeekPreviewMemory(37); err != nil || m.PreviewEpoch() <= before {
		t.Fatal("seek was not accepted", err)
	}
	mx.Mix(nil)
	pos, live := m.PreviewPosition()
	if !live || pos != float32(37)/48000 || !m.PreviewPaused() || mx.Position(game) != gameBefore {
		t.Fatalf("seek disturbed playback: position=%v live=%v paused=%v game=%v/%v",
			pos, live, m.PreviewPaused(), mx.Position(game), gameBefore)
	}
	if err := m.SetPreviewPaused(false); err != nil {
		t.Fatal(err)
	}
	mx.Mix(make([]float32, 2))
	if pos, live = m.PreviewPosition(); !live || pos != float32(38)/48000 {
		t.Fatalf("resume did not advance from the requested frame: %v/%v", pos, live)
	}
	if err := m.StopPreview(); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := m.PreviewMemorySeekRange(); ok || !mx.Live(game) {
		t.Fatal("stopping preview retained seek state or stopped the game voice")
	}
}

func TestPreviewMemorySeekRejectsStreamAndQueueBackpressure(t *testing.T) {
	mx, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2, Buses: BusDescs()})
	if err != nil {
		t.Fatal(err)
	}
	m := &Module{mixer: mx}
	id := ids.NewAssetID()
	if err := m.PreviewArtifact(id, privatePreviewArtifact(.3, true), true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if m.previewCancel != nil {
			m.endPreview()
		}
	})
	voice, cancel, before := m.preview, m.previewCancel, m.PreviewEpoch()
	if _, _, ok := m.PreviewMemorySeekRange(); ok {
		t.Fatal("stream reported a memory seek range")
	}
	if err := m.SeekPreviewMemory(12); err == nil || m.preview != voice || m.previewCancel != cancel || m.PreviewEpoch() != before {
		t.Fatal("unsupported stream seek changed preview ownership")
	}
	if err := m.StopPreview(); err != nil {
		t.Fatal(err)
	}
	if err := m.PreviewArtifact(id, privatePreviewArtifact(.3, false), false); err != nil {
		t.Fatal(err)
	}
	before = m.PreviewEpoch()
	count := 0
	for count < 4096 && mx.Commands().Push(mix.Command{Kind: mix.CmdBusMute, Bus: -1}) {
		count++
	}
	if count == 4096 {
		t.Fatal("fixture did not fill the queue")
	}
	if err := m.SeekPreviewMemory(12); err == nil || m.PreviewEpoch() != before {
		t.Fatal("rejected seek changed the epoch")
	}
	mx.Mix(nil)
	if err := m.SeekPreviewMemory(12); err != nil || m.PreviewEpoch() <= before {
		t.Fatal("could not retry seek after draining the queue", err)
	}
	mx.Mix(nil)
	if pos, live := m.PreviewPosition(); !live || pos != float32(12)/48000 {
		t.Fatalf("retry did not reach mixer: %v/%v", pos, live)
	}
	if err := m.StopPreview(); err != nil {
		t.Fatal(err)
	}
}
