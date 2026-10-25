// Command i18n writes the en catalogs from the code (extract) and checks the other languages against them (check, -strict) (D348).
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
	data, err := Data()
	if err != nil {
		errs = append(errs, err)
	}
	for id, k := range data {
		if _, dup := keys[id]; !dup {
			keys[id] = k
		}
	}
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
