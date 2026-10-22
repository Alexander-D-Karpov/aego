package export

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"aego/app/export/icon"
	"aego/app/export/winres"
	"aego/app/proc"
	"aego/core/errs"
	"aego/core/log"
	"aego/runtime/bundle"
)

const runtimeTag = "aego_runtime"

const nativeMain = `package main

import (
	"aego/runtime/player"
	native "%s/native"
)

func main() {
	player.Main(&native.Module{})
}
`

const plainMain = `package main

import "aego/runtime/player"

func main() {
	player.Main()
}
`

func (j *job) cc() string {
	if j.opt.CC != "" {
		return j.opt.CC
	}
	return os.Getenv("CC_" + strings.ToUpper(j.t.OS) + "_" + strings.ToUpper(j.t.Arch))
}

func (j *job) sdlDir() string {
	return filepath.Join(j.opt.Engine, ".cache", "native", j.t.OS+"_"+j.t.Arch, "lib")
}

func hasSDL(dir string) bool {
	for _, n := range []string{"libSDL3.a", "SDL3.lib"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

func (j *job) toolchain() error {
	if j.opt.Engine == "" {
		return errs.New(errs.BuildFailed, "export needs the engine source tree, pass --engine or set AEGO_ROOT")
	}
	if _, err := exec.LookPath("go"); err != nil {
		return errs.New(errs.BuildFailed, "export needs the Go toolchain on this machine; the exported game does not, see docs/export.md")
	}
	if j.t.Web() {
		root, err := goRoot()
		if err != nil {
			return err
		}
		if _, err := wasmExecPath(root); err != nil {
			return err
		}
		j.goroot = root
		return nil
	}
	cc := j.cc()
	if !j.t.Host() && cc == "" {
		return errs.New(errs.BuildFailed, "exporting to "+j.t.String()+" from "+runtime.GOOS+"/"+runtime.GOARCH+
			" needs a C compiler for the target: pass --cc or set CC_"+strings.ToUpper(j.t.OS)+"_"+strings.ToUpper(j.t.Arch)+
			" (mingw, zig cc or osxcross), see docs/export.md")
	}
	if cc != "" {
		if _, err := exec.LookPath(strings.Fields(cc)[0]); err != nil {
			return errs.New(errs.BuildFailed, "the C compiler "+cc+" is not in PATH")
		}
	}
	if !hasSDL(j.sdlDir()) {
		cmd := "make bootstrap"
		if !j.t.Host() {
			cmd = "GOOS=" + j.t.OS + " GOARCH=" + j.t.Arch + " CC='" + cc + "' make bootstrap"
		}
		return errs.New(errs.BuildFailed, "SDL3 for "+j.t.String()+" is not built; run `"+cmd+"` in "+j.opt.Engine, log.F("lib", j.sdlDir()))
	}
	return nil
}

func (j *job) mainDir() (dir, root, pkg string, src []byte) {
	if j.native {
		dir = filepath.Join(j.p.GeneratedDir(), "runtime")
		return dir, j.p.Root, "./.aego/generated/runtime", []byte(fmt.Sprintf(nativeMain, j.p.Module))
	}
	sum := sha256.Sum256([]byte(j.p.Root))
	key := hex.EncodeToString(sum[:6])
	dir = filepath.Join(j.opt.Engine, ".cache", "export", key)
	return dir, j.opt.Engine, "./.cache/export/" + key, []byte(plainMain)
}

func (j *job) resources(dir string) error {
	old, _ := filepath.Glob(filepath.Join(dir, "*.syso"))
	for _, f := range old {
		os.Remove(f)
	}
	if j.t.OS != "windows" {
		return nil
	}
	var icons []winres.Icon
	if j.icon != nil {
		for _, n := range winres.IconSizes {
			data, err := icon.PNG(icon.Sized(j.icon, n))
			if err != nil {
				return err
			}
			icons = append(icons, winres.Icon{Size: n, PNG: data})
		}
	}
	v := GameVersion(j.p)
	data, err := winres.Syso(j.t.Arch, icons, winres.Info{
		Product: j.p.Name, Description: j.p.Name, Original: j.l.exe, Internal: j.p.Module,
		VersionText: v, Version: winres.ParseVersion(v),
	}, true)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "rsrc_windows_"+j.t.Arch+".syso"), data, 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the Windows resources")
	}
	return nil
}

func (j *job) build() error {
	dir, root, pkg, src := j.mainDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create the runtime build directory")
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), src, 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the runtime main")
	}
	if err := j.resources(dir); err != nil {
		return err
	}
	bin := j.l.binary()
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create the binary directory")
	}
	args := []string{"build", "-buildvcs=false", "-tags", runtimeTag, "-o", bin}
	ld := ""
	if j.profile == bundle.Release {
		args = append(args, "-trimpath")
		ld = "-s -w"
		if j.t.OS == "windows" {
			ld += " -H=windowsgui"
		}
	}
	if j.t.OS == "windows" {
		ld = strings.TrimSpace(ld + ` -extldflags "-static"`)
	}
	if ld != "" {
		args = append(args, "-ldflags", ld)
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	proc.Hide(cmd)
	cgo := "CGO_ENABLED=1"
	if j.t.Web() {
		cgo = "CGO_ENABLED=0"
	}
	cmd.Env = append(os.Environ(), "GOOS="+j.t.OS, "GOARCH="+j.t.Arch, cgo)
	if cc := j.cc(); cc != "" && !j.t.Web() {
		cmd.Env = append(cmd.Env, "CC="+cc)
	}
	if j.t.OS == "darwin" {
		cmd.Env = append(cmd.Env, "MACOSX_DEPLOYMENT_TARGET=11.0")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errs.New(errs.BuildFailed, "go build of the runtime failed:\n"+strings.TrimSpace(string(out)))
	}
	st, err := os.Stat(bin)
	if err != nil {
		return errs.Wrap(err, errs.BuildFailed, "the runtime binary is missing after the build")
	}
	j.rep.Binary = j.rel(bin)
	j.rep.BinarySize = st.Size()
	return nil
}

func (j *job) rel(path string) string {
	r, err := filepath.Rel(j.out, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(r)
}
