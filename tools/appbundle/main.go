package main

import (
	"flag"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"aego/app/export/icon"
	"aego/app/export/macos"
)

func main() {
	bin := flag.String("bin", "", "executable to put into the bundle")
	name := flag.String("name", "Aego Editor", "bundle name")
	id := flag.String("id", "dev.aego.editor", "bundle identifier")
	version := flag.String("version", "0.1.0", "bundle version")
	iconPath := flag.String("icon", "", "PNG icon or a directory of PNG sizes")
	out := flag.String("out", ".cache", "directory for the .app")
	sign := flag.Bool("sign", true, "ad-hoc sign when codesign is available")
	flag.Parse()
	if *bin == "" {
		fail("pass -bin")
	}
	exe := filepath.Base(*bin)
	app := filepath.Join(*out, *name+".app")
	if err := os.RemoveAll(app); err != nil {
		fail(err.Error())
	}
	dir := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err.Error())
	}
	if err := copyExe(*bin, filepath.Join(dir, exe)); err != nil {
		fail(err.Error())
	}
	var img image.Image
	if *iconPath != "" {
		paths := []string{*iconPath}
		if st, err := os.Stat(*iconPath); err == nil && st.IsDir() {
			paths, _ = filepath.Glob(filepath.Join(*iconPath, "*.png"))
		}
		set, err := icon.LoadSet(paths...)
		if err != nil {
			fail(err.Error())
		}
		img = set
	}
	info := macos.Info{Name: *name, Executable: exe, ID: *id, Version: *version, MinOS: "11.0", Category: "public.app-category.developer-tools",
		URLSchemes: []string{"aego"},
		Documents: []macos.DocType{
			{Name: "Aego Project", Extensions: []string{"aego"}},
			{Name: "Aego Scene", Extensions: []string{"aescene"}},
			{Name: "Aego Prefab", Extensions: []string{"aeprefab"}},
		}}
	if err := macos.Fill(app, info, img); err != nil {
		fail(err.Error())
	}
	if *sign && runtime.GOOS == "darwin" {
		if cs, err := exec.LookPath("codesign"); err == nil {
			if out, err := exec.Command(cs, "--force", "--deep", "--sign", "-", app).CombinedOutput(); err != nil {
				fail("codesign: " + strings.TrimSpace(string(out)))
			}
		}
	}
	fmt.Println(app)
}

func copyExe(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "appbundle:", msg)
	os.Exit(1)
}
