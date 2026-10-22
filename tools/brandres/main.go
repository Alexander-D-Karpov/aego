package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aego/app/export/icon"
	"aego/app/export/winres"
)

var icoSizes = []int{16, 32, 48, 256}

func main() {
	dir := flag.String("png", "app/brand/png", "directory with aego-<size>.png")
	ico := flag.String("ico", "", "write a .ico file")
	syso := flag.String("syso", "", "comma-separated main package directories that get a Windows resource object")
	arch := flag.String("arch", "amd64", "Windows architecture of the resource object")
	version := flag.String("version", "", "version shown in the file properties")
	flag.Parse()
	paths, _ := filepath.Glob(filepath.Join(*dir, "aego-*.png"))
	set, err := icon.LoadSet(paths...)
	if err != nil {
		fail(err)
	}
	if *ico != "" {
		data, err := icon.ICO(set, icoSizes)
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(*ico, data, 0o644); err != nil {
			fail(err)
		}
	}
	if *syso == "" {
		return
	}
	icons := make([]winres.Icon, 0, len(winres.IconSizes))
	for _, n := range winres.IconSizes {
		data, err := icon.PNG(icon.Sized(set, n))
		if err != nil {
			fail(err)
		}
		icons = append(icons, winres.Icon{Size: n, PNG: data})
	}
	for _, pkg := range strings.Split(*syso, ",") {
		name := filepath.Base(pkg)
		desc := "Aego command line"
		if strings.HasSuffix(name, "editor") {
			desc = "Aego"
		}
		data, err := winres.Syso(*arch, icons, winres.Info{
			Product: "Aego", Description: desc, Original: name + ".exe", Internal: name,
			VersionText: *version, Version: winres.ParseVersion(*version),
		}, true)
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(filepath.Join(pkg, "rsrc_windows_"+*arch+".syso"), data, 0o644); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "brandres:", err)
	os.Exit(1)
}
