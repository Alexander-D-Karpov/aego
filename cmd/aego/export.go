package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"aego/app/export"
	"aego/app/projects"
	"aego/runtime/bundle"
)

const exportUsage = `usage: aego export [dir] [flags]

  --target host|linux|windows|darwin[/amd64|/arm64]   default host
  --development | --release                           default development
  --out dir        output directory, default <project>/exports/<os>-<arch>-<profile>
  --cc compiler    C compiler for a cross target, or CC_<OS>_<ARCH>
  --check          run the pre-export checks only
  --json           print the report as JSON
  --engine dir     path to the aego source tree`

func cmdExport(args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	target := fs.String("target", "host", "target platform")
	release := fs.Bool("release", false, "release profile")
	development := fs.Bool("development", false, "development profile")
	out := fs.String("out", "", "output directory")
	cc := fs.String("cc", "", "C compiler for the target")
	check := fs.Bool("check", false, "run the pre-export checks only")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	engineDir := fs.String("engine", "", "path to the aego source tree")
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintln(os.Stderr, exportUsage)
		return 2
	}
	if *release && *development {
		fmt.Fprintln(os.Stderr, "aego: --release and --development cannot be used together")
		return 2
	}
	profile := bundle.Development
	if *release {
		profile = bundle.Release
	}
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	root := ""
	if !*check {
		if root, err = projects.EngineRoot(*engineDir); err != nil {
			return fail(err)
		}
	}
	var logw io.Writer
	if !*asJSON {
		logw = os.Stderr
	}
	rep, err := export.Export(export.Options{
		Project: p, Engine: root, Target: *target, Profile: profile, Out: *out, CC: *cc, CheckOnly: *check, Log: logw,
	})
	if rep == nil {
		return fail(err)
	}
	if *asJSON {
		if jerr := rep.JSON(os.Stdout); jerr != nil {
			return fail(jerr)
		}
	} else {
		rep.Print(os.Stdout)
		if err != nil && !rep.Blocked {
			fmt.Fprintln(os.Stderr, "aego:", err)
		}
	}
	switch {
	case rep.Blocked:
		return 2
	case err != nil:
		return 1
	}
	if *check && !*asJSON {
		fmt.Println("export check: ok")
	}
	return 0
}
