package player

import (
	"flag"
	"os"
	"path/filepath"
	goruntime "runtime"

	"aego/runtime/bundle"
	"aego/sdk"
)

func DefaultData() string {
	exe, err := os.Executable()
	if err != nil {
		return bundle.DataDir
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	dir := filepath.Dir(exe)
	if goruntime.GOOS == "darwin" {
		res := filepath.Join(dir, "..", "Resources", bundle.DataDir)
		if _, err := os.Stat(filepath.Join(res, bundle.ManifestFile)); err == nil {
			return filepath.Clean(res)
		}
	}
	return filepath.Join(dir, bundle.DataDir)
}

func Main(mods ...sdk.Module) {
	fs := flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ExitOnError)
	data := fs.String("data", "", "game data directory, next to the binary when empty")
	profile := fs.String("profile", "", "player settings profile, default when empty")
	windowed := fs.Bool("windowed", false, "start in a window, overriding the player settings")
	fullscreen := fs.Bool("fullscreen", false, "start in fullscreen, overriding the player settings")
	scenePath := fs.String("scene", "", "start scene by asset id or path, for development")
	logFile := fs.String("log", "", "write the log to this file")
	tracePath := fs.String("trace", "", "write a Chrome trace of the last frames to this file on exit")
	replayPath := fs.String("replay", "", "replay an input recording")
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames and exit")
	debugFlag := fs.Bool("debug", false, "verbose logging and gpu debug output")
	seed := fs.Uint64("seed", 0, "world random seed")
	fs.Parse(os.Args[1:])
	if *windowed && *fullscreen {
		fail("--windowed and --fullscreen cannot be used together")
	}
	cfg := Config{
		Data: *data, Profile: *profile, Scene: *scenePath, LogFile: *logFile, Trace: *tracePath, Replay: *replayPath,
		Headless: *headless, Frames: *frames, Debug: *debugFlag, Seed: *seed,
	}
	switch {
	case *windowed:
		cfg.Window = WindowedMode
	case *fullscreen:
		cfg.Window = FullMode
	}
	if *headless && *frames > 0 {
		cfg.FrameTime = 1.0 / 60
	}
	if cfg.Data == "" {
		cfg.Data = DefaultData()
	} else if abs, err := filepath.Abs(cfg.Data); err == nil {
		cfg.Data = abs
	}
	p, err := Open(cfg, sdk.AdaptAll(mods)...)
	if err != nil {
		fail(err.Error())
	}
	if err := p.Run(); err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	os.Stderr.WriteString("game: " + msg + "\n")
	os.Exit(1)
}
