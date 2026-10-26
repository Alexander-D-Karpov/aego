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
  --engine dir     path to the aego source tree
  --release-json   pack the export and write the signed release.json its copies update from
  --channel c      update channel of that release: stable or nightly
  --base-url url   where the archive is downloaded from, update.url/<channel> by default
  --key-file file  private key that signs release.json, from aego release keygen`)
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
	releaseJSON := fs.Bool("release-json", false, i18n.Tr("cli.export.flag.release_json", "pack the export and write the signed release.json its copies update from"))
	channel := fs.String("channel", "stable", i18n.Tr("cli.export.flag.channel", "update channel of the release: stable or nightly"))
	baseURL := fs.String("base-url", "", i18n.Tr("cli.export.flag.base_url", "where the archive is downloaded from; update.url/<channel> by default"))
	keyFile := fs.String("key-file", "", i18n.Tr("cli.export.flag.key_file", "private key that signs release.json"))
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
	if *releaseJSON && !*check {
		if err := exportRelease(p, rep, *channel, *baseURL, *keyFile); err != nil {
			return fail(err)
		}
	}
	return 0
}
