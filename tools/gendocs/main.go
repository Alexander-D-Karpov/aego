package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"aego/tools/gendocs/site"
	"aego/tools/gendocs/tryassets"
)

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func main() {
	in := flag.String("in", "", "documentation export written by aego docs export")
	out := flag.String("out", "site", "output directory")
	title := flag.String("title", "", "site title, defaults to the export title")
	play := flag.Bool("playground", false, "add Run buttons that load play/aego.wasm")
	studio := flag.Bool("studio", false, "link an already-built Art Studio in studio/")
	studioURL := flag.String("studio-url", "", "link a verified, version-pinned standalone Studio release")
	studioGuides := flag.Bool("studio-guides", false, "link pre-integrated Studio guides in asset-studio/")
	examplesURL := flag.String("examples-url", "https://aego.akarpov.ru/", "canonical HTTPS docs root for complete source example packages; empty disables packaging")
	examplesVersion := flag.String("examples-version", "1.0.0", "release version of the example catalog; increment when publishing changed sources")
	examplesLicense := flag.String("examples-license", "NOASSERTION", "actual SPDX/source license of documentation examples; never inferred from dependencies")
	examplesLicenseFile := flag.String("examples-license-file", "", "file containing reviewed terms to include in every example archive")
	examplesProduction := flag.Bool("examples-production", false, "require an explicit known license for official example publication")
	repo := flag.String("repo", "", "source repository linked from the header and footer")
	edit := flag.String("edit", "", "base URL of Edit this page links, defaults to <repo>/src/branch/main")
	version := flag.String("version", "", "engine version shown in the footer")
	channel := flag.String("channel", "development", "documentation status, for example stable or development; not an engine feature guarantee")
	commit := flag.String("commit", "", "commit shown in the footer")
	hero := flag.String("hero", "", "screenshot shown on the landing page")
	shots := flag.String("shots", "", "directory of editor screenshots that pages show as shots/<name>.png")
	poster := flag.String("poster", "", "image shown before the home page demo starts")
	demo := flag.Bool("demo", false, "link the web demo at demo/ from the landing page")
	playVersion := flag.String("play-version", "", "cache key of play/aego.wasm")
	tryVersion := flag.String("try-version", "", "cache key of try/aego-docs.wasm")
	icons := flag.String("icons", "", "comma-separated directories with <id>.svg icons")
	prepareTry := flag.String("prepare-try", "", "source .assets project to prepare as adjacent .aetry runtime resources")
	checkTry := flag.Bool("check-try", false, "verify prepared Try artifacts against a fresh source import")
	flag.Parse()
	if *prepareTry != "" {
		target, err := tryassets.Generate(*prepareTry, *checkTry)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gendocs:", err)
			os.Exit(1)
		}
		fmt.Println("gendocs: Try artifacts", target)
		return
	}
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: gendocs -in export.json [-out dir] [-title name] [-playground] [-repo url] [-version v] [-commit sha] [-hero png]")
		os.Exit(2)
	}
	exp, err := site.Load(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
	if *title != "" {
		exp.Title = *title
	}
	var licenseText string
	if *examplesLicenseFile != "" {
		data, err := os.ReadFile(*examplesLicenseFile)
		if err != nil || len(data) == 0 || len(data) > 64<<10 || !utf8.Valid(data) || strings.TrimSpace(string(data)) == "" {
			fmt.Fprintln(os.Stderr, "gendocs: example license file is unreadable, empty, invalid UTF-8, or larger than 64 KiB")
			os.Exit(1)
		}
		licenseText = string(data)
	}
	var rel *site.Release
	if *repo != "" {
		if rel, err = site.FetchRelease(*repo); err != nil {
			fmt.Fprintln(os.Stderr, "gendocs: no download buttons:", err)
		}
	}
	rep, err := site.Build(exp, *out, site.Options{
		ExamplesURL: *examplesURL, ExamplesVersion: *examplesVersion, ExamplesLicense: *examplesLicense, ExamplesLicenseText: licenseText, ExamplesProduction: *examplesProduction, Channel: *channel,
		Playground: *play, Studio: *studio, StudioURL: *studioURL, StudioGuides: *studioGuides, Repo: *repo, Edit: *edit, Version: *version, Commit: *commit, Hero: *hero, Poster: *poster, Shots: *shots, Demo: *demo, PlayVersion: *playVersion, TryVersion: *tryVersion, Release: rel, Icons: splitList(*icons), Time: time.Now(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
	for _, b := range rep.Broken {
		fmt.Fprintln(os.Stderr, "gendocs: broken link", b)
	}
	fmt.Printf("gendocs: %d pages, %d entries -> %s\n", rep.Pages, rep.Entries, *out)
	if len(rep.Broken) > 0 {
		os.Exit(1)
	}
}
