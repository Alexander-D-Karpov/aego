package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	runtimePkgs := flag.Bool("runtime-pkgs", false, "print the packages of the aego_runtime build and exit")
	verbose := flag.Bool("v", false, "print the time spent")
	flag.Parse()
	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	if *runtimePkgs {
		pkgs, err := RuntimePackages(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "depcheck:", err)
			os.Exit(2)
		}
		fmt.Println(strings.Join(pkgs, "\n"))
		return
	}
	start := time.Now()
	cat, err := builtinCatalog()
	if err != nil {
		fmt.Fprintln(os.Stderr, "depcheck: module catalog:", err)
		os.Exit(2)
	}
	violations, err := Check(Config{Root: root, Catalog: cat})
	if err != nil {
		fmt.Fprintln(os.Stderr, "depcheck:", err)
		os.Exit(2)
	}
	for _, v := range violations {
		fmt.Println(v)
	}
	if *verbose {
		fmt.Fprintf(os.Stderr, "depcheck: %s\n", time.Since(start).Round(time.Millisecond))
	}
	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "depcheck: %d violation(s)\n", len(violations))
		os.Exit(1)
	}
	fmt.Println("depcheck: ok")
}
