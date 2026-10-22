package bootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"aego/app/proc"
	"aego/core/errs"
	"aego/core/log"
	"aego/project"
)

const manifestTag = "aego_nonative"

func ManifestTarget() Target {
	t := HostTarget()
	t.Tags = []string{manifestTag}
	return t
}

func ManifestPath(p *project.Project, key Key) string {
	return filepath.Join(p.CacheDir(), "script-bindings", string(key)+".aetext")
}

func BuildManifest(p *project.Project, engineVersion string, lg *log.Logger) (string, error) {
	if lg == nil {
		lg = log.Nop()
	}
	cl := lg.With(log.Engine)
	t := ManifestTarget()
	key, err := CacheKey(p, t, engineVersion+"+manifest")
	if err != nil {
		return "", err
	}
	out := ManifestPath(p, key)
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	if _, err := Generate(p, ModeManifest); err != nil {
		return "", err
	}
	for _, dir := range []string{filepath.Dir(out), p.BuildDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", errs.Wrap(err, errs.VFSIO, "cannot create the bindings cache")
		}
	}
	bin := filepath.Join(p.BuildDir(), "manifest-"+string(key))
	if t.OS == "windows" {
		bin += ".exe"
	}
	defer os.Remove(bin)
	build := exec.Command("go", "build", "-tags", strings.Join(t.Tags, ","), "-o", bin, "./.aego/generated/manifest")
	build.Dir = p.Root
	proc.Hide(build)
	if output, err := build.CombinedOutput(); err != nil {
		return "", errs.New(errs.BuildFailed, "native bindings helper does not compile",
			log.F("output", strings.TrimSpace(string(output))))
	}
	tmp := out + ".tmp"
	run := exec.Command(bin, tmp)
	run.Dir = p.Root
	proc.Hide(run)
	if output, err := run.CombinedOutput(); err != nil {
		os.Remove(tmp)
		return "", errs.New(errs.BuildFailed, "native module Register failed",
			log.F("output", strings.TrimSpace(string(output))))
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", errs.Wrap(err, errs.VFSIO, "cannot store the bindings manifest")
	}
	cl.Info("native bindings manifest built", log.F("key", string(key)))
	return out, nil
}
