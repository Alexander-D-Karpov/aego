package main

import (
	"bytes"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"

	audioimp "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/audio/codec/wav"
	"aego/core/ids"
	"aego/format/aetext"
)

const rate = 48000

type clip struct {
	file string
	id   string
	bus  string
	gain float64
	ogg  bool
	pcm  func() []float32
}

type event struct {
	file string
	id   string
	body string
}

const (
	idJumpA  = "a0d10000000000000000000000000001"
	idJumpB  = "a0d10000000000000000000000000002"
	idHit    = "a0d10000000000000000000000000003"
	idDie    = "a0d10000000000000000000000000004"
	idMusic  = "a0d10000000000000000000000000005"
	idEvJump = "a0d10000000000000000000000000011"
	idEvHit  = "a0d10000000000000000000000000012"
	idEvDie  = "a0d10000000000000000000000000013"
)

var clips = []clip{
	{"jump_a.wav", idJumpA, "sfx", 1, false, func() []float32 { return sweep(320, 720, 0.16, 0.08, 0, 1) }},
	{"jump_b.wav", idJumpB, "sfx", 1, false, func() []float32 { return sweep(360, 860, 0.15, 0.07, 0, 2) }},
	{"hit.wav", idHit, "sfx", 1, false, hit},
	{"enemy_die.wav", idDie, "sfx", 1, false, func() []float32 { return sweep(700, 140, 0.4, 0.22, 0.15, 3) }},
	{"music.ogg", idMusic, "music", 0.7, true, music},
}

var events = []event{
	{"jump.aedata", idEvJump, fmt.Sprintf(`data version=1 type="audio.AudioEvent" {
  variants=[asset(%q), asset(%q)]
  volume=0.7
  volumeRandom=0.15
  pitch=1
  pitchRandom=0.06
  bus=sfx
  priority=20
  maxVoices=2
  steal=oldest
  cooldown=0.05
}
`, idJumpA, idJumpB)},
	{"hit.aedata", idEvHit, fmt.Sprintf(`data version=1 type="audio.AudioEvent" {
  variants=[asset(%q)]
  volume=0.9
  pitch=1
  pitchRandom=0.05
  bus=sfx
  priority=40
  maxVoices=2
  steal=oldest
}
`, idHit)},
	{"enemy_die.aedata", idEvDie, fmt.Sprintf(`data version=1 type="audio.AudioEvent" {
  variants=[asset(%q)]
  volume=0.9
  pitch=1
  pitchRandom=0.08
  bus=sfx
  priority=30
  maxVoices=3
  steal=oldest
  spatial=true
  minDistance=3
  maxDistance=18
  rolloff={ ease=outQuad }
  panWidth=0.8
}
`, idDie)},
}

func main() {
	out := flag.String("out", "examples/platformer/assets/audio", "directory for the clips, events and their .meta files")
	ffmpeg := flag.String("ffmpeg", "ffmpeg", "ffmpeg binary for the ogg music")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	for _, c := range clips {
		data := wav.Encode(c.pcm(), rate, 1, wav.EncodeOptions{Bits: 16})
		if c.ogg {
			var err error
			if data, err = encodeOgg(*ffmpeg, data); err != nil {
				fail(err)
			}
		}
		path := filepath.Join(*out, c.file)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fail(err)
		}
		s := audioimp.New().Defaults()
		s["bus"] = aetext.Ident(c.bus)
		s["gain"] = aetext.Float64(c.gain)
		if c.ogg {
			s["stream"] = aetext.Ident("off")
		}
		if err := writeMeta(path, c.id, audioimp.New().ID(), audioimp.Version, s); err != nil {
			fail(err)
		}
		fmt.Printf("%s %d bytes\n", path, len(data))
	}
	for _, e := range events {
		path := filepath.Join(*out, e.file)
		if err := os.WriteFile(path, []byte(e.body), 0o644); err != nil {
			fail(err)
		}
		if err := writeMeta(path, e.id, "data", 1, pipeline.Settings{}); err != nil {
			fail(err)
		}
		fmt.Println(path)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gentones:", err)
	os.Exit(1)
}

func writeMeta(path, id, importer string, version int, s pipeline.Settings) error {
	aid, err := ids.ParseAssetID(id)
	if err != nil {
		return err
	}
	return os.WriteFile(path+pipeline.MetaSuffix, pipeline.WriteMeta(&pipeline.Meta{ID: aid, Importer: importer, Version: version, Settings: s}), 0o644)
}

func encodeOgg(bin string, src []byte) ([]byte, error) {
	cmd := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-f", "wav", "-i", "pipe:0", "-c:a", "libvorbis", "-q:a", "3", "-f", "ogg", "pipe:1")
	cmd.Stdin = bytes.NewReader(src)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, stderr.String())
	}
	return out.Bytes(), nil
}

type noise uint32

func (n *noise) next() float64 {
	x := uint32(*n)
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	*n = noise(x)
	return float64(x)/math.MaxUint32*2 - 1
}

func edge(i, n, w int) float64 {
	switch {
	case i < w:
		return float64(i) / float64(w)
	case n-i < w:
		return float64(n-i) / float64(w)
	}
	return 1
}

func sweep(f0, f1, dur, decay, grit float64, seed uint32) []float32 {
	n := int(dur * rate)
	out := make([]float32, n)
	src := noise(seed*2654435761 + 1)
	phase := 0.0
	for i := range out {
		t := float64(i) / rate
		f := f0 * math.Pow(f1/f0, t/dur) * (1 + 0.03*math.Sin(2*math.Pi*18*t))
		phase += 2 * math.Pi * f / rate
		s := math.Sin(phase) + 0.25*math.Sin(3*phase) + grit*src.next()
		out[i] = float32(0.5 * s * math.Exp(-t/decay) * edge(i, n, rate/400))
	}
	return out
}

func hit() []float32 {
	n := int(0.2 * rate)
	out := make([]float32, n)
	src := noise(0x51ab)
	phase := 0.0
	for i := range out {
		t := float64(i) / rate
		phase += 2 * math.Pi * (140 - 70*t/0.2) / rate
		s := 0.7*math.Sin(phase)*math.Exp(-t/0.08) + 0.5*src.next()*math.Exp(-t/0.025)
		out[i] = float32(0.7 * s * edge(i, n, rate/400))
	}
	return out
}

func midi(n int) float64 {
	return 440 * math.Pow(2, float64(n-69)/12)
}
