package run

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"strings"

	"aego/core/log"
	"aego/core/rewind"
	"aego/engine"
	"aego/project"
	"aego/sdk"
)

const (
	defaultDetFrames = 600
	defaultDetEvery  = 60
	defaultDetSeed   = 1
	defaultDetStep   = 1.0 / 60
)

type DeterminismOptions struct {
	Frames    int
	Every     int
	Seed      uint64
	FrameTime float64
	ScenePath string
}

type DeterminismReport struct {
	Frames   int
	Diverged int
	Hash     [32]byte
	Covered  []string
	Skipped  []string
}

type checkpoint struct {
	frame int
	sum   [32]byte
}

func (o DeterminismOptions) withDefaults() DeterminismOptions {
	if o.Frames <= 0 {
		o.Frames = defaultDetFrames
	}
	if o.Every <= 0 {
		o.Every = defaultDetEvery
	}
	if o.Seed == 0 {
		o.Seed = defaultDetSeed
	}
	if o.FrameTime <= 0 {
		o.FrameTime = defaultDetStep
	}
	return o
}

func WorldHash(w *engine.World, h *rewind.Hasher) (sum [32]byte, covered, skipped []string) {
	h.Reset()
	w.Time.Hash(h)
	if w.Random != nil {
		h.U64(w.Random.State())
	}
	w.EachOwner(func(id string, v any) {
		k, ok := rewind.Of(v)
		if !ok {
			skipped = append(skipped, id)
			return
		}
		covered = append(covered, id)
		h.String(id)
		k.Hash(h)
	})
	return h.Sum(), covered, skipped
}

func determinismPass(p *project.Project, o DeterminismOptions, mods []sdk.Module) ([]checkpoint, []string, []string, error) {
	a, err := Open(p, Config{Headless: true, FrameTime: o.FrameTime, Seed: o.Seed, ScenePath: o.ScenePath, Logger: log.Nop(), NoProfile: true}, mods...)
	if err != nil {
		return nil, nil, nil, err
	}
	defer a.Close()
	w := a.Engine.MainWorld()
	h := rewind.NewHasher()
	driver := a.Driver()
	var out []checkpoint
	var covered, skipped []string
	for f := 1; f <= o.Frames; f++ {
		if !a.Engine.Step(driver) {
			return nil, nil, nil, fmt.Errorf("the game stopped at frame %d: %v", f, a.Engine.Err())
		}
		if f%o.Every == 0 || f == o.Frames {
			var sum [32]byte
			sum, covered, skipped = WorldHash(w, h)
			out = append(out, checkpoint{frame: f, sum: sum})
		}
	}
	return out, covered, skipped, nil
}

func Determinism(p *project.Project, o DeterminismOptions, mods ...sdk.Module) (DeterminismReport, error) {
	o = o.withDefaults()
	first, covered, skipped, err := determinismPass(p, o, mods)
	if err != nil {
		return DeterminismReport{}, err
	}
	second, _, _, err := determinismPass(p, o, mods)
	if err != nil {
		return DeterminismReport{}, err
	}
	rep := DeterminismReport{Frames: o.Frames, Covered: covered, Skipped: skipped}
	for i := range first {
		if i >= len(second) || first[i].sum != second[i].sum {
			rep.Diverged = first[i].frame
			break
		}
	}
	if n := len(first); n > 0 {
		rep.Hash = first[n-1].sum
	}
	return rep, nil
}

func WantsDeterminism(args []string) bool {
	for _, a := range args {
		switch {
		case a == "-determinism" || a == "--determinism":
			return true
		case strings.HasPrefix(a, "-determinism=") || strings.HasPrefix(a, "--determinism="):
			return !strings.HasSuffix(a, "=false")
		}
	}
	return false
}

func determinismTool(p *project.Project, args []string, stdout, stderr io.Writer, mods []sdk.Module) int {
	fs := flag.NewFlagSet("test --determinism", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Bool("determinism", true, "run the main scene twice and compare state hashes")
	frames := fs.Int("frames", defaultDetFrames, "frames per run")
	every := fs.Int("every", defaultDetEvery, "frames between hash checkpoints")
	seed := fs.Uint64("seed", defaultDetSeed, "random seed of both runs")
	scenePath := fs.String("scene", "", "scene file instead of the main scene")
	verbose := fs.Bool("v", false, "list the services the hash covers and skips")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rep, err := Determinism(p, DeterminismOptions{Frames: *frames, Every: *every, Seed: *seed, ScenePath: *scenePath}, mods...)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	if *verbose {
		fmt.Fprintln(stdout, "covered: "+strings.Join(rep.Covered, ", "))
		if len(rep.Skipped) > 0 {
			fmt.Fprintln(stdout, "not covered: "+strings.Join(rep.Skipped, ", "))
		}
	}
	if rep.Diverged > 0 {
		fmt.Fprintf(stderr, "determinism: the runs diverge by frame %d of %d\n", rep.Diverged, rep.Frames)
		return 1
	}
	fmt.Fprintf(stdout, "determinism: ok, %d frames, %s\n", rep.Frames, hex.EncodeToString(rep.Hash[:8]))
	return 0
}
