package audio

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"path"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/audio/codec"
	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
)

const (
	Version     = 1
	CodeFormat  = "AUD001"
	streamBytes = 2 << 20
	streamSecs  = 10
	readFrames  = 4096
)

type Importer struct {
	FFmpeg string
}

func New() *Importer {
	return &Importer{}
}

func (*Importer) ID() string {
	return "audio"
}

func (*Importer) Version() int {
	return Version
}

func (*Importer) Extensions() []string {
	return []string{".wav", ".ogg", ".mp3"}
}

func (*Importer) Defaults() pipeline.Settings {
	return pipeline.Settings{
		"stream":    aetext.Ident("auto"),
		"loopStart": aetext.Int(-1),
		"loopEnd":   aetext.Int(-1),
		"gain":      aetext.Float64(1),
		"bus":       aetext.Ident("sfx"),
	}
}

func fail(ctx *pipeline.ImportContext, msg string, cause error) error {
	if cause == nil {
		return errs.New(errs.AssetImportFailed, CodeFormat+": "+msg, log.F("path", ctx.Path))
	}
	return errs.Wrap(cause, errs.AssetImportFailed, CodeFormat+": "+msg).With(log.F("path", ctx.Path))
}

func (i *Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	src := ctx.Source
	if strings.EqualFold(path.Ext(ctx.Path), ".mp3") {
		ogg, err := i.convert(src)
		if err != nil {
			return nil, fail(ctx, "mp3 is converted with ffmpeg, which is not available; install it or convert the file: ffmpeg -i in.mp3 -c:a libvorbis out.ogg", err)
		}
		src = ogg
	}
	d, err := codec.Open(bytes.NewReader(src), ctx.Path)
	if err != nil {
		return nil, fail(ctx, "cannot read the sound; wav (PCM or float) and ogg vorbis, mono or stereo, are supported", err)
	}
	defer d.Close()
	info := d.Info()
	frames := info.Frames
	if frames < 0 {
		if frames, err = count(d, info.Channels); err != nil {
			return nil, fail(ctx, "cannot decode the sound", err)
		}
	}
	s := ctx.Meta.Settings
	var ls, le int64
	if l, ok := d.(codec.Looper); ok {
		if a, b, ok := l.Loop(); ok {
			ls, le = a, b
		}
	}
	if v := s.Int("loopStart", -1); v >= 0 {
		ls = int64(v)
	}
	if v := s.Int("loopEnd", -1); v >= 0 {
		le = int64(v)
	}
	format := asset.AudioWAV
	if bytes.HasPrefix(src, []byte("OggS")) {
		format = asset.AudioOgg
	}
	art := asset.EncodeAudio(&asset.AudioArtifact{
		Format: format, Rate: info.SampleRate, Channels: info.Channels, Frames: frames,
		LoopStart: ls, LoopEnd: le, Gain: float32(s.Float("gain", 1)), Bus: s.String("bus", "sfx"),
		Stream: streamed(s, len(src), frames, info.SampleRate), Data: src,
	})
	return &pipeline.Result{Kind: asset.ArtAudio, Artifacts: []pipeline.Produced{{Kind: asset.ArtAudio, Data: art}}}, nil
}

func streamed(s pipeline.Settings, size int, frames int64, rate int) bool {
	if v, ok := s["stream"]; ok && v.Kind == aetext.VBool {
		return v.Bool
	}
	switch s.String("stream", "auto") {
	case "on", "true":
		return true
	case "off", "false":
		return false
	}
	return size > streamBytes || float64(frames) > streamSecs*float64(rate)
}

func count(d codec.Decoder, ch int) (int64, error) {
	buf := make([]float32, readFrames*ch)
	var n int64
	for {
		k, err := d.Read(buf)
		n += int64(k)
		if errors.Is(err, io.EOF) || (err == nil && k == 0) {
			break
		}
		if err != nil {
			return n, err
		}
	}
	return n, d.SeekFrame(0)
}

func (i *Importer) convert(src []byte) ([]byte, error) {
	bin := i.FFmpeg
	if bin == "" {
		p, err := exec.LookPath("ffmpeg")
		if err != nil {
			return nil, err
		}
		bin = p
	}
	cmd := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-vn", "-c:a", "libvorbis", "-q:a", "5", "-f", "ogg", "pipe:1")
	cmd.Stdin = bytes.NewReader(src)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, errors.New(strings.TrimSpace(stderr.String()) + ": " + err.Error())
	}
	return out.Bytes(), nil
}
