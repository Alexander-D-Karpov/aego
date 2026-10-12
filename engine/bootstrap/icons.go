package bootstrap

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/project"
)

func RunIcons(p *project.Project, engineVersion string, out io.Writer) (int, error) {
	t := ManifestTarget()
	key, err := CacheKey(p, t, engineVersion+"+icons")
	if err != nil {
		return 2, err
	}
	if _, err := Generate(p, ModeIcons); err != nil {
		return 2, err
	}
	if err := os.MkdirAll(p.BuildDir(), 0o755); err != nil {
		return 2, errs.Wrap(err, errs.VFSIO, "cannot create the build directory")
	}
	bin := filepath.Join(p.BuildDir(), "icons-"+string(key))
	if t.OS == "windows" {
		bin += ".exe"
	}
	defer os.Remove(bin)
	build := exec.Command("go", "build", "-tags", strings.Join(t.Tags, ","), "-o", bin, "./.aego/generated/icons")
	build.Dir = p.Root
	if output, err := build.CombinedOutput(); err != nil {
		return 2, errs.New(errs.BuildFailed, "native icons helper does not compile",
			log.F("output", strings.TrimSpace(string(output))))
	}
	run := exec.Command(bin)
	run.Dir, run.Stdout, run.Stderr = p.Root, out, out
	err = run.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return 2, errs.Wrap(err, errs.BuildFailed, "native icons helper failed")
	}
	return 0, nil
}
