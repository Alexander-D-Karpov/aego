// Command i18n extracts the translatable texts of the code into the en catalogs and checks the other languages (D348).
//
//	go run ./tools/i18n extract   rewrites engine/i18n/messages/en.aetext and the module en catalogs
//	go run ./tools/i18n check     lists missing, unused and mismatched keys; -strict fails on missing ones
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "repository root")
	strict := flag.Bool("strict", false, "fail on keys without a translation")
	flag.Usage = func() { fmt.Fprintln(os.Stderr, "usage: i18n [-root dir] [-strict] extract|check") }
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	keys, errs := Scan(*root)
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, e)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
	switch flag.Arg(0) {
	case "extract":
		n, err := Extract(*root, keys)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("i18n extract: %d keys in %d catalogs\n", len(keys), n)
	case "check":
		rep, err := Check(*root, keys)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		rep.Print(os.Stdout)
		if rep.Failed(*strict) {
			os.Exit(1)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}
