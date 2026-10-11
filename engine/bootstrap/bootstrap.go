package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/project"
)

const SDKVersion = "1"

type Mode uint8

const (
	ModeRun Mode = iota
	ModeRuntime
	ModeManifest
)

const manifestTemplate = `package main

import (
	"os"

	"aego/engine/builtin"
	"aego/script/bind/manifest"
	"aego/sdk"
	native "%s/native"
)

func main() {
	if len(os.Args) < 2 {
		os.Stderr.WriteString("usage: manifest <out>\n")
		os.Exit(2)
	}
	m, err := builtin.Manifest(sdk.Adapt(native.Module{}))
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	if err := os.WriteFile(os.Args[1], manifest.Encode(m), 0o644); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(3)
	}
}
`

type Target struct {
	OS   string
	Arch string
	Tags []string
}

func HostTarget() Target {
	return Target{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

type Key string

func NeedsNative(p *project.Project) bool {
	if !p.Native {
		return false
	}
	entries, err := os.ReadDir(p.NativeDir())
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

const mainTemplate = `package main

import (
	"aego/app/run"
	native "%s/native"
)

func main() {
	run.Main(native.Module{})
}
`

func Generate(p *project.Project, mode Mode) (string, error) {
	dir, tmpl := p.GeneratedDir(), mainTemplate
	if mode == ModeManifest {
		dir, tmpl = filepath.Join(p.GeneratedDir(), "manifest"), manifestTemplate
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", errs.Wrap(err, errs.VFSIO, "cannot create the generated directory")
	}
	path := filepath.Join(dir, "main.go")
	content := []byte(fmt.Sprintf(tmpl, p.Module))
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(content) {
		return path, nil
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", errs.Wrap(err, errs.VFSIO, "cannot write the generated main")
	}
	return path, nil
}

const keepBuilds = 3

func Build(p *project.Project, t Target, mode Mode, engineVersion string, lg *log.Logger) (string, error) {
	if lg == nil {
		lg = log.Nop()
	}
	cl := lg.With(log.Engine)
	key, err := CacheKey(p, t, engineVersion)
	if err != nil {
		return "", err
	}
	outDir := filepath.Join(p.BuildDir(), string(key))
	binary := filepath.Join(outDir, "game")
	if t.OS == "windows" {
		binary += ".exe"
	}
	if _, err := os.Stat(binary); err == nil {
		cl.Debug("native build is up to date", log.F("key", string(key)))
		return binary, nil
	}
	if _, err := Generate(p, mode); err != nil {
		return "", err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", errs.Wrap(err, errs.VFSIO, "cannot create the build directory")
	}
	args := []string{"build", "-trimpath", "-o", binary}
	if len(t.Tags) > 0 {
		args = append(args, "-tags", strings.Join(t.Tags, ","))
	}
	args = append(args, "./.aego/generated")
	cmd := exec.Command("go", args...)
	cmd.Dir = p.Root
	cmd.Env = append(os.Environ(), "GOOS="+t.OS, "GOARCH="+t.Arch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(outDir)
		return "", errs.New(errs.BuildFailed, "go build failed",
			log.F("output", strings.TrimSpace(string(output))))
	}
	cl.Info("native build finished", log.F("key", string(key)), log.F("binary", binary))
	pruneBuilds(p.BuildDir(), string(key), keepBuilds)
	return binary, nil
}

func pruneBuilds(dir, current string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type build struct {
		name string
		mod  int64
	}
	var builds []build
	for _, e := range entries {
		if !e.IsDir() || e.Name() == current {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		builds = append(builds, build{name: e.Name(), mod: info.ModTime().UnixNano()})
	}
	sort.Slice(builds, func(i, j int) bool { return builds[i].mod > builds[j].mod })
	for i := keep - 1; i < len(builds); i++ {
		os.RemoveAll(filepath.Join(dir, builds[i].name))
	}
}
