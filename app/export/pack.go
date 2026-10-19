package export

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"aego/app/export/icon"
	"aego/app/export/macos"
	"aego/core/errs"
)

const desktopEntry = "[Desktop Entry]\nType=Application\nName=%s\nExec=sh -c 'cd \"$(dirname \"%%k\")\" && exec ./%s'\nTerminal=false\nCategories=Game;\n"

func (j *job) pack() error {
	switch j.t.OS {
	case "linux":
		return j.packLinux()
	case "darwin":
		return j.packDarwin()
	}
	return nil
}

func (j *job) packLinux() error {
	if j.icon != nil {
		data, err := icon.PNG(icon.Resize(j.icon, 256, 256))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(j.out, "icon.png"), data, 0o644); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot write the icon")
		}
	}
	entry := fmt.Sprintf(desktopEntry, j.p.Name, j.l.exe)
	if err := os.WriteFile(filepath.Join(j.out, j.l.exe+".desktop"), []byte(entry), 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the .desktop file")
	}
	return nil
}

func (j *job) packDarwin() error {
	info := macos.Info{
		Name: j.p.Name, Executable: j.l.exe, ID: macos.BundleID(j.p.Module),
		Version: GameVersion(j.p), MinOS: "11.0", Category: "public.app-category.games",
	}
	if err := macos.Fill(j.l.app, info, j.icon); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return nil
	}
	path, err := exec.LookPath("codesign")
	if err != nil {
		return nil
	}
	out, err := exec.Command(path, "--force", "--deep", "--sign", "-", j.l.app).CombinedOutput()
	if err != nil {
		return errs.New(errs.BuildFailed, "ad-hoc signing failed: "+strings.TrimSpace(string(out)))
	}
	return nil
}
