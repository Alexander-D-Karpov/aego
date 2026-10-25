package main

import (
	"aego/engine/i18n"
	"flag"
	"fmt"
	"io"
	"os"

	"aego/app/export"
	"aego/app/projects"
	"aego/runtime/bundle"
)

func exportUsage() string {
	return i18n.Tr("cli.export.usage", `usage: aego export [dir] [flags]

  --target host|linux|windows|darwin[/amd64|/arm64]|web   default host
  --development | --release                           default development
  --out dir        output directory, default <project>/exports/<os>-<arch>-<profile>
  --cc compiler    C compiler for a cross target, or CC_<OS>_<ARCH>
  --check          run the pre-export checks only
  --json           print the report as JSON
  --engine dir     path to the aego source tree`)
}

func cmdExport(args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	target := fs.String("target", "host", i18n.Tr("cli.export.flag.target", "target platform"))
	release := fs.Bool("release", false, i18n.Tr("cli.export.flag.release", "release profile"))
	development := fs.Bool("development", false, i18n.Tr("cli.export.flag.development", "development profile"))
	out := fs.String("out", "", i18n.Tr("cli.export.flag.out", "output directory"))
	cc := fs.String("cc", "", i18n.Tr("cli.export.flag.cc", "C compiler for the target"))
	check := fs.Bool("check", false, i18n.Tr("cli.export.flag.check", "run the pre-export checks only"))
	asJSON := fs.Bool("json", false, i18n.Tr("cli.export.flag.json", "print the report as JSON"))
	engineDir := fs.String("engine", "", i18n.Tr("cli.export.flag.engine", "path to the aego source tree"))
	dir, rest := firstArg(args)
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintln(os.Stderr, exportUsage())
		return 2
	}
	if *release && *development {
		fmt.Fprintln(os.Stderr, "aego:", i18n.Tr("cli.export.profiles", "--release and --development cannot be used together"))
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
