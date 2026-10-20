package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"aego/render/shaderx"
)

func main() {
	root := flag.String("root", ".", "engine source tree")
	check := flag.Bool("check", false, "fail when a generated shader is out of date instead of writing it")
	flag.Parse()
	dir := filepath.Join(*root, "engine", "res", "shaders")
	out := filepath.Join(dir, shaderx.ESDir)
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	var files []string
	for _, pat := range []string{"*.vert", "*.frag"} {
		m, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			fail(err)
		}
		files = append(files, m...)
	}
	stale := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			fail(err)
		}
		es, err := shaderx.Translate(string(src), filepath.Base(f))
		if err != nil {
			fail(err)
		}
		dst := filepath.Join(out, filepath.Base(f))
		if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, []byte(es)) {
			continue
		}
		if *check {
			fmt.Fprintln(os.Stderr, "genshaders: out of date:", dst)
			stale++
			continue
		}
		if err := os.WriteFile(dst, []byte(es), 0o644); err != nil {
			fail(err)
		}
		fmt.Println("wrote", dst)
	}
	if stale > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genshaders:", err)
	os.Exit(1)
}
