package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"aego/tools/gendocs/site"
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
	repo := flag.String("repo", "", "source repository linked from the header and footer")
	edit := flag.String("edit", "", "base URL of Edit this page links, defaults to <repo>/src/branch/main")
	version := flag.String("version", "", "engine version shown in the footer")
	commit := flag.String("commit", "", "commit shown in the footer")
	hero := flag.String("hero", "", "screenshot shown on the landing page")
	shots := flag.String("shots", "", "directory of editor screenshots that pages show as shots/<name>.png")
	poster := flag.String("poster", "", "image shown before the home page demo starts")
	demo := flag.Bool("demo", false, "link the web demo at demo/ from the landing page")
	playVersion := flag.String("play-version", "", "cache key of play/aego.wasm")
	tryVersion := flag.String("try-version", "", "cache key of try/aego-docs.wasm")
	icons := flag.String("icons", "", "comma-separated directories with <id>.svg icons")
	flag.Parse()
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
	var rel *site.Release
	if *repo != "" {
		if rel, err = site.FetchRelease(*repo); err != nil {
			fmt.Fprintln(os.Stderr, "gendocs: no download buttons:", err)
		}
	}
	rep, err := site.Build(exp, *out, site.Options{
		Playground: *play, Repo: *repo, Edit: *edit, Version: *version, Commit: *commit, Hero: *hero, Poster: *poster, Shots: *shots, Demo: *demo, PlayVersion: *playVersion, TryVersion: *tryVersion, Release: rel, Icons: splitList(*icons), Time: time.Now(),
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
