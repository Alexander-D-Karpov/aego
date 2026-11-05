package engine

import (
	"os"
	"path/filepath"

	"aego/core/clock"
	"aego/core/errs"
	"aego/core/log"
	"aego/platform"
)

var Version = "0.12.5-dev"

type Config struct {
	Title             string
	Width             int
	Height            int
	Fullscreen        bool
	FixedSize         bool
	Hidden            bool
	VSync             platform.VSyncMode
	Headless          bool
	HeadlessFrameTime float64
	// FixedFrameTime makes every windowed frame this long whatever the clock says, so a hidden run with screenshots steps like a headless one.
	FixedFrameTime float64
	Debug          bool
	LogLevel       log.Level
	FixedHz        float64
	MaxFrameDelta  float64
	MaxFixedSteps  int
	Workers        int
	ResRoot        string
	UserRoot       string
	AppID          string
	Seed           uint64
	LoopScope      SystemScope
	Trace          bool
}

func (c Config) withDefaults() Config {
	if c.Title == "" {
		c.Title = "aego"
	}
	if c.Width <= 0 {
		c.Width = 1280
	}
	if c.Height <= 0 {
		c.Height = 720
	}
	if c.FixedHz <= 0 {
		c.FixedHz = 60
	}
	if c.MaxFrameDelta <= 0 {
		c.MaxFrameDelta = clock.DefaultMaxFrameDelta
	}
	if c.MaxFixedSteps <= 0 {
		c.MaxFixedSteps = clock.DefaultMaxSteps
	}
	if c.ResRoot == "" {
		c.ResRoot = "res"
	}
	if c.AppID == "" {
		c.AppID = "aego"
	}
	if c.LogLevel == 0 && !c.Debug {
		c.LogLevel = log.Info
	} else if c.Debug && c.LogLevel == 0 {
		c.LogLevel = log.Debug
	}
	if c.LoopScope == 0 {
		c.LoopScope = ScopePlay
	}
	return c
}

func (c Config) validate() error {
	if c.FixedHz <= 0 || c.FixedHz > 1000 {
		return errs.New(errs.EngineConfig, "FixedHz out of range", log.F("value", c.FixedHz))
	}
	if c.Width <= 0 || c.Height <= 0 {
		return errs.New(errs.EngineConfig, "window size must be positive")
	}
	return nil
}

func (c Config) userRoot() string {
	if c.UserRoot != "" {
		return c.UserRoot
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "aego", c.AppID)
}
