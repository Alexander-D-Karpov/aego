package audio

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"aego/asset"
	audioimp "aego/asset/importers/audio"
	"aego/asset/pipeline"
	_ "aego/audio/codec/vorbis"
	"aego/audio/codec/wav"
	"aego/core/ids"
	"aego/format/aetext"
)

func importSound(t *testing.T, imp *audioimp.Importer, path string, src []byte, set map[string]aetext.Value) (*asset.AudioArtifact, error) {
	t.Helper()
	meta := pipeline.NewMeta(imp.ID(), imp.Version(), imp.Defaults())
	for k, v := range set {
		meta.Settings[k] = v
	}
	res, err := imp.Import(&pipeline.ImportContext{ID: ids.NewAssetID(), Path: path, Source: src, Meta: meta})
	if err != nil {
		return nil, err
	}
	if res.Kind != asset.ArtAudio || len(res.Artifacts) != 1 {
		t.Fatalf("result %+v", res)
	}
	return asset.DecodeAudio(res.Artifacts[0].Data)
}

func TestImportWAV(t *testing.T) {
	pcm := tone(440, rate/2, 0.5).PCM
	src := wav.Encode(pcm, rate, 1, wav.EncodeOptions{Bits: 16, Loop: true, LoopStart: 100, LoopEnd: 2000})
	a, err := importSound(t, audioimp.New(), "beep.wav", src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Format != asset.AudioWAV || a.Rate != rate || a.Channels != 1 || a.Frames != rate/2 || a.Stream {
		t.Fatalf("artifact %+v", a)
	}
	if a.LoopStart != 100 || a.LoopEnd != 2000 || a.Bus != "sfx" || a.Gain != 1 {
		t.Fatalf("loop %d..%d bus %q gain %v", a.LoopStart, a.LoopEnd, a.Bus, a.Gain)
	}
	if !bytes.Equal(a.Data, src) {
		t.Fatal("the artifact does not carry the source bytes")
	}
	b, err := importSound(t, audioimp.New(), "beep.wav", src, map[string]aetext.Value{
		"stream": aetext.Bool(true), "loopStart": aetext.Int(0), "loopEnd": aetext.Int(500), "gain": aetext.Float64(0.5), "bus": aetext.Ident("ui"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !b.Stream || b.LoopStart != 0 || b.LoopEnd != 500 || b.Gain != 0.5 || b.Bus != "ui" {
		t.Fatalf("meta settings ignored: %+v", b)
	}
}

func TestImportLongIsStreamed(t *testing.T) {
	src := wav.Encode(make([]float32, rate*11), rate, 1, wav.EncodeOptions{Bits: 8})
	a, err := importSound(t, audioimp.New(), "long.wav", src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Stream {
		t.Fatal("an 11 second sound is not streamed")
	}
}

func TestImportMP3(t *testing.T) {
	_, err := importSound(t, &audioimp.Importer{FFmpeg: "/nonexistent/ffmpeg"}, "a.mp3", []byte("ID3\x04\x00\x00\x00\x00\x00\x00"), nil)
	if err == nil || !strings.Contains(err.Error(), audioimp.CodeFormat) {
		t.Fatalf("mp3 without ffmpeg: %v", err)
	}
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	src := wav.Encode(tone(440, rate/4, 0.5).PCM, rate, 1, wav.EncodeOptions{Bits: 16})
	cmd := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "wav", "-i", "pipe:0", "-f", "mp3", "pipe:1")
	cmd.Stdin = bytes.NewReader(src)
	mp3, err := cmd.Output()
	if err != nil {
		t.Skip("ffmpeg cannot encode mp3 here")
	}
	a, err := importSound(t, audioimp.New(), "a.mp3", mp3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Format != asset.AudioOgg || a.Frames < rate/5 {
		t.Fatalf("converted artifact %+v", a)
	}
}

func TestImportGarbage(t *testing.T) {
	_, err := importSound(t, audioimp.New(), "x.wav", []byte("RIFF\x00\x00\x00\x00WAVEjunk"), nil)
	if err == nil || !strings.Contains(err.Error(), audioimp.CodeFormat) {
		t.Fatalf("broken wav: %v", err)
	}
}

func TestArtifactRoundTrip(t *testing.T) {
	in := &asset.AudioArtifact{Format: asset.AudioOgg, Rate: 44100, Channels: 2, Frames: 1234, LoopStart: 5, LoopEnd: 600, Gain: 0.7, Bus: "music", Stream: true, Data: []byte("OggS…")}
	out, err := asset.DecodeAudio(asset.EncodeAudio(in))
	if err != nil {
		t.Fatal(err)
	}
	if out.Format != in.Format || out.Rate != in.Rate || out.Channels != in.Channels || out.Frames != in.Frames ||
		out.LoopStart != in.LoopStart || out.LoopEnd != in.LoopEnd || out.Gain != in.Gain || out.Bus != in.Bus ||
		out.Stream != in.Stream || !bytes.Equal(out.Data, in.Data) {
		t.Fatalf("round trip %+v", out)
	}
	if _, err := asset.DecodeAudio([]byte("AEAU")); err == nil {
		t.Fatal("a truncated artifact decodes")
	}
}
