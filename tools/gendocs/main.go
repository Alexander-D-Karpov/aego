package main

import (
	"flag"
	"fmt"
	"os"

	"aego/tools/gendocs/site"
)

func main() {
	in := flag.String("in", "", "documentation export written by aego docs export")
	out := flag.String("out", "site", "output directory")
	title := flag.String("title", "", "site title, defaults to the export title")
	play := flag.Bool("playground", false, "add Run buttons that load play/aego.wasm")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: gendocs -in export.json [-out dir] [-title name] [-playground]")
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
	rep, err := site.Build(exp, *out, site.Options{Playground: *play})
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
