package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/edit"
	"aego/app/projects"
	"aego/core/log"
	"aego/editor/core/preview"
	"aego/format/aetext"
	"aego/script/bind"
)

const allowEnv = "AEGO_PREVIEWS_ALLOW_HARDWARE"

var errFatal = errors.New("fatal")

func main() {
	root := flag.String("root", ".", "aego source tree")
	frames := flag.Int("frames", 600, "frames to wait for one preview")
	flag.Parse()
	runtime.LockOSThread()
	abs, err := filepath.Abs(*root)
	if err != nil {
		fail(err)
	}
	tmp, err := os.MkdirTemp("", "aego-previews")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(tmp)
	bad := false
	run := func(name string, opt projects.Options, dst string) {
		opt.Dir = filepath.Join(tmp, strings.ReplaceAll(name, "/", "_"))
		opt.Module = "preview"
		if err := render(opt, dst, *frames); err != nil {
			if errors.Is(err, errFatal) {
				fail(err)
			}
			fmt.Fprintf(os.Stderr, "genpreviews: %s: %v\n", name, err)
			bad = true
		}
	}
	for _, t := range projects.Templates {
		run("template "+t.ID, projects.Options{Template: t.ID}, filepath.Join(abs, "engine", "res", "templates", t.ID, projects.PreviewFile))
	}
	for _, ex := range projects.Examples(abs) {
		run("example "+ex, projects.Options{Example: ex, Engine: abs}, filepath.Join(abs, "examples", ex, projects.PreviewFile))
	}
	if bad {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genpreviews:", err)
	os.Exit(1)
}

func software(name string) error {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "null"):
		return fmt.Errorf("%w: no GL context; run it through make res, which starts xvfb with llvmpipe", errFatal)
	case strings.Contains(n, "llvmpipe"), strings.Contains(n, "softpipe"), strings.Contains(n, "swrast"), strings.Contains(n, "lavapipe"):
		return nil
	case os.Getenv(allowEnv) != "":
		fmt.Fprintf(os.Stderr, "genpreviews: rendering on %q, the PNGs will differ from the ones made with llvmpipe\n", name)
		return nil
	}
	return fmt.Errorf("%w: the committed previews are rendered with a software rasterizer so every machine produces the same bytes, got %q; run make res (xvfb + llvmpipe) or set %s=1", errFatal, name, allowEnv)
}

func render(opt projects.Options, dst string, frames int) error {
	p, err := projects.Create(opt)
	if err != nil {
		return err
	}
	if p.Native && p.MainScene.IsZero() {
		fmt.Printf("skip %s: a native project without a startup scene\n", dst)
		return nil
	}
	if p.Native {
		if comp, err := nativeComponent(p.Root); err != nil {
			return err
		} else if comp != "" {
			fmt.Printf("skip %s: a scene uses the native component %s\n", dst, comp)
			return nil
		}
		p.Native = false
	}
	if p.MainScene.IsZero() {
		return errors.New("the project has no startup scene")
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, NoWatch: true, Width: 640, Height: 400, Logger: log.Nop()})
	if err != nil {
		return fmt.Errorf("%w: %v", errFatal, err)
	}
	defer s.Close()
	if err := software(s.Engine.Caps().Renderer); err != nil {
		return err
	}
	s.Step()
	ctx := s.App.Context()
	pv := s.App.Previews()
	var data []byte
	for i := 0; i < frames && data == nil; i++ {
		pv.ByID(p.MainScene)
		s.Step()
		e, ok := ctx.Pipeline.Entry(p.MainScene)
		if !ok {
			continue
		}
		if key, ok := pv.Key(e); ok {
			data, _ = os.ReadFile(filepath.Join(p.CacheDir(), preview.CacheDir, key+".png"))
		}
	}
	if data == nil {
		return errors.New("the preview did not render in time")
	}
	if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, data) {
		fmt.Println("unchanged", dst)
		return nil
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", dst)
	return nil
}

// nativeComponent returns the first component of the project's scenes and prefabs that the built-in catalog does not register; native data types and assets do not matter for a preview.
func nativeComponent(root string) (string, error) {
	types, err := assembly.NewTypes()
	if err != nil {
		return "", err
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{}); err != nil {
		return "", err
	}
	known := map[string]bool{}
	for _, td := range types.Schema().Components() {
		known[td.FullName] = true
		for _, f := range td.Former {
			known[f] = true
		}
	}
	found := ""
	var walk func(ns []*aetext.Node)
	walk = func(ns []*aetext.Node) {
		for _, n := range ns {
			if found != "" {
				return
			}
			if n.Head == "component" {
				if !known[n.Label] {
					found = n.Label
					return
				}
			}
			walk(n.Blocks)
		}
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if ext := filepath.Ext(path); ext != ".aescene" && ext != ".aeprefab" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		doc, err := aetext.Parse(src, path)
		if err != nil {
			return err
		}
		walk(doc.Blocks)
		return nil
	})
	return found, err
}
