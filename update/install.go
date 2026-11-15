package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Install is how the running copy was installed, which decides how an update replaces it.
type Install struct {
	Kind Kind
	// Exe is the running executable; Root is what an update replaces: share/aego, the AppImage, the install folder or the .app.
	Exe  string
	Root string
	// Launch is what starts the installed version after an update: bin/aego of the current version, the AppImage, the .app.
	Launch string
	// Folder is an exported game unpacked anywhere: an update swaps the whole folder and keeps the old one next to it.
	Folder bool
	// Keep is where the installer of the replaced version is kept, updates/previous, for an installer install to restore.
	Keep string
}

// SetupFile is the copy of itself the Windows installer leaves in the install folder, so the next update can keep it.
const SetupFile = "setup.exe"

// DetectGame finds how an exported game was installed: an AppImage, a .app, or the folder its executable is in.
func DetectGame(exe string) Install {
	if img := os.Getenv("APPIMAGE"); img != "" {
		return Install{Kind: KindAppImage, Exe: exe, Root: img, Launch: img}
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		real = exe
	}
	if i := strings.Index(real, ".app/Contents/MacOS/"); i >= 0 {
		app := real[:i+4]
		return Install{Kind: KindApp, Exe: real, Root: app, Launch: app}
	}
	return Install{Kind: KindTarball, Exe: real, Root: filepath.Dir(real), Launch: real, Folder: true}
}

// Detect finds how the executable was installed: an AppImage, the versioned tarball layout of install.sh, the NSIS folder or a .app.
func Detect(exe string) (Install, error) {
	return detectPlatform(exe, runtime.GOOS)
}

// detectPlatform also lets the Linux tarball layout be exercised on other
// Unix hosts without treating that layout as an install for the host OS.
func detectPlatform(exe, goos string) (Install, error) {
	if img := os.Getenv("APPIMAGE"); img != "" {
		return Install{Kind: KindAppImage, Exe: exe, Root: img, Launch: img}, nil
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		real = exe
	}
	switch goos {
	case "windows":
		dir := filepath.Dir(real)
		if _, err := os.Stat(filepath.Join(dir, "uninstall.exe")); err == nil {
			return Install{Kind: KindInstaller, Exe: real, Root: dir, Launch: filepath.Join(dir, "aego-editor.exe")}, nil
		}
	case "darwin":
		if i := strings.Index(real, ".app/Contents/MacOS/"); i >= 0 {
			app := real[:i+4]
			return Install{Kind: KindApp, Exe: real, Root: app, Launch: app}, nil
		}
	default:
		ver := filepath.Dir(filepath.Dir(real))
		root := filepath.Dir(ver)
		if filepath.Base(filepath.Dir(real)) == "bin" {
			if _, err := os.Lstat(filepath.Join(root, "current")); err == nil {
				return Install{Kind: KindTarball, Exe: real, Root: root, Launch: filepath.Join(root, "current", "bin", filepath.Base(real))}, nil
			}
		}
	}
	return Install{Exe: real}, ErrNoInstall
}

// Applied is a finished update: what starts the new version and what the trial run needs to roll it back.
type Applied struct {
	Version  string
	Previous string
	// Exit is a command to run as the editor exits, for an install that cannot replace a running program (Windows).
	Exit *exec.Cmd
}

// Apply installs a downloaded asset over the install; the running program goes on until it restarts.
func (in Install) Apply(file, version string) (Applied, error) {
	switch {
	case in.Folder:
		return in.applyFolder(file, version)
	}
	switch in.Kind {
	case KindTarball:
		return in.applyTarball(file, version)
	case KindAppImage:
		return in.applyAppImage(file, version)
	case KindApp:
		return in.applyApp(file, version)
	case KindInstaller:
		return in.applyInstaller(file, version)
	}
	return Applied{}, ErrNoInstall
}

var errNoPrevious = errors.New("there is no previous version to restore")

// Restore switches back to the version before the last update; an installer install gets the kept installer to run at exit, as an update does.
func (in Install) Restore() (Applied, error) {
	if in.Folder {
		prev := previousOf(in.Root)
		if _, err := os.Stat(prev); err != nil {
			return Applied{}, errNoPrevious
		}
		return Applied{}, swap(in.Root, prev)
	}
	switch in.Kind {
	case KindTarball:
		prev, err := os.Readlink(filepath.Join(in.Root, "previous"))
		if err != nil {
			return Applied{}, errNoPrevious
		}
		cur, _ := os.Readlink(filepath.Join(in.Root, "current"))
		if err := relink(in.Root, "current", prev); err != nil {
			return Applied{}, err
		}
		if cur != "" {
			return Applied{Version: filepath.Base(prev)}, relink(in.Root, "previous", cur)
		}
		return Applied{Version: filepath.Base(prev)}, nil
	case KindAppImage, KindApp:
		prev := previousOf(in.Root)
		if _, err := os.Stat(prev); err != nil {
			return Applied{}, errNoPrevious
		}
		return Applied{}, swap(in.Root, prev)
	case KindInstaller:
		return in.restoreInstaller()
	}
	return Applied{}, ErrNoInstall
}

// HasPrevious reports whether Restore has something to go back to.
func (in Install) HasPrevious() bool {
	if in.Folder {
		_, err := os.Stat(previousOf(in.Root))
		return err == nil
	}
	switch in.Kind {
	case KindTarball:
		_, err := os.Readlink(filepath.Join(in.Root, "previous"))
		return err == nil
	case KindAppImage, KindApp:
		_, err := os.Stat(previousOf(in.Root))
		return err == nil
	case KindInstaller:
		_, err := os.Stat(filepath.Join(in.Keep, SetupFile))
		return in.Keep != "" && err == nil
	}
	return false
}

// previousOf is where the replaced AppImage or .app is kept: next to it, hidden.
func previousOf(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".previous")
}

// swap exchanges a file or folder with its kept previous one.
func swap(cur, prev string) error {
	tmp := cur + ".swap"
	if err := os.Rename(cur, tmp); err != nil {
		return err
	}
	if err := os.Rename(prev, cur); err != nil {
		os.Rename(tmp, cur)
		return err
	}
	return os.Rename(tmp, prev)
}

// relink points a symlink in root at target atomically: a new link renamed over the old one.
func relink(root, name, target string) error {
	tmp := filepath.Join(root, "."+name+".new")
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(root, name))
}

func (in Install) applyTarball(file, version string) (Applied, error) {
	dir := filepath.Join(in.Root, safe(version))
	tmp := dir + ".unpack"
	os.RemoveAll(tmp)
	if err := untar(file, tmp); err != nil {
		os.RemoveAll(tmp)
		return Applied{}, err
	}
	top, err := single(tmp)
	if err != nil {
		os.RemoveAll(tmp)
		return Applied{}, err
	}
	if _, err := os.Stat(filepath.Join(top, "bin")); err != nil {
		os.RemoveAll(tmp)
		return Applied{}, errors.New("the archive has no bin folder")
	}
	os.RemoveAll(dir)
	if err := os.Rename(top, dir); err != nil {
		os.RemoveAll(tmp)
		return Applied{}, err
	}
	os.RemoveAll(tmp)
	prev, _ := os.Readlink(filepath.Join(in.Root, "current"))
	if err := relink(in.Root, "current", safe(version)); err != nil {
		return Applied{}, err
	}
	if prev != "" && prev != safe(version) {
		if err := relink(in.Root, "previous", prev); err != nil {
			return Applied{}, err
		}
		in.prune(prev, safe(version))
	}
	return Applied{Version: version, Previous: prev}, nil
}

// prune removes versions other than the current and the previous one.
func (in Install) prune(keep ...string) {
	list, err := os.ReadDir(in.Root)
	if err != nil {
		return
	}
	for _, e := range list {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") || name == "current" || name == "previous" {
			continue
		}
		drop := true
		for _, k := range keep {
			if name == k {
				drop = false
			}
		}
		if drop {
			os.RemoveAll(filepath.Join(in.Root, name))
		}
	}
}

// applyFolder unpacks a game's archive next to its folder and swaps the folders; the running game keeps its open files on Linux and macOS.
func (in Install) applyFolder(file, version string) (Applied, error) {
	if runtime.GOOS == "windows" {
		return Applied{}, errors.New("a game folder updates on Windows through its installer")
	}
	tmp := in.Root + ".unpack"
	os.RemoveAll(tmp)
	unpack := untar
	if strings.HasSuffix(strings.ToLower(file), ".zip") {
		unpack = unzip
	}
	if err := unpack(file, tmp); err != nil {
		os.RemoveAll(tmp)
		return Applied{}, err
	}
	defer os.RemoveAll(tmp)
	top, err := single(tmp)
	if err != nil {
		return Applied{}, err
	}
	if _, err := os.Stat(filepath.Join(top, filepath.Base(in.Exe))); err != nil {
		return Applied{}, fmt.Errorf("the archive has no %s", filepath.Base(in.Exe))
	}
	prev := previousOf(in.Root)
	os.RemoveAll(prev)
	if err := os.Rename(in.Root, prev); err != nil {
		return Applied{}, err
	}
	if err := os.Rename(top, in.Root); err != nil {
		os.Rename(prev, in.Root)
		return Applied{}, err
	}
	return Applied{Version: version, Previous: prev}, nil
}

func (in Install) applyAppImage(file, version string) (Applied, error) {
	next := in.Root + ".new"
	if err := copyFile(file, next, 0o755); err != nil {
		return Applied{}, err
	}
	prev := previousOf(in.Root)
	os.Remove(prev)
	if err := os.Rename(in.Root, prev); err != nil {
		os.Remove(next)
		return Applied{}, err
	}
	if err := os.Rename(next, in.Root); err != nil {
		os.Rename(prev, in.Root)
		return Applied{}, err
	}
	return Applied{Version: version, Previous: prev}, nil
}

func (in Install) applyApp(file, version string) (Applied, error) {
	tmp := in.Root + ".unpack"
	os.RemoveAll(tmp)
	if err := unzip(file, tmp); err != nil {
		os.RemoveAll(tmp)
		return Applied{}, err
	}
	defer os.RemoveAll(tmp)
	app, err := single(tmp)
	if err != nil || !strings.HasSuffix(app, ".app") {
		return Applied{}, errors.New("the archive has no .app")
	}
	prev := previousOf(in.Root)
	os.RemoveAll(prev)
	if err := os.Rename(in.Root, prev); err != nil {
		return Applied{}, err
	}
	if err := os.Rename(app, in.Root); err != nil {
		os.Rename(prev, in.Root)
		return Applied{}, err
	}
	return Applied{Version: version, Previous: prev}, nil
}

// applyInstaller keeps the installed version's installer and runs the new one silently as the editor exits: Windows keeps the running program locked.
func (in Install) applyInstaller(file, version string) (Applied, error) {
	prev := ""
	if in.Keep != "" {
		if err := in.keepSetup(); err != nil {
			return Applied{}, err
		}
		if b, err := os.ReadFile(filepath.Join(in.Root, "VERSION")); err == nil {
			prev = strings.TrimSpace(string(b))
		}
	}
	return Applied{Version: version, Previous: prev, Exit: exec.Command(file, "/S", "/UPDATE")}, nil
}

// keepSetup copies the installed version's setup.exe to Keep; an install made before installers kept themselves has none, and then nothing is kept.
func (in Install) keepSetup() error {
	src := filepath.Join(in.Root, SetupFile)
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	if err := os.MkdirAll(in.Keep, 0o755); err != nil {
		return err
	}
	return copyFile(src, filepath.Join(in.Keep, SetupFile), 0o755)
}

// restoreInstaller runs the kept installer at exit from a copy, and keeps the current one in its place so a second Restore goes forward again.
func (in Install) restoreInstaller() (Applied, error) {
	kept := filepath.Join(in.Keep, SetupFile)
	if in.Keep == "" {
		return Applied{}, errNoPrevious
	}
	if _, err := os.Stat(kept); err != nil {
		return Applied{}, errNoPrevious
	}
	run := filepath.Join(filepath.Dir(in.Keep), "restore", SetupFile)
	if err := os.MkdirAll(filepath.Dir(run), 0o755); err != nil {
		return Applied{}, err
	}
	if err := copyFile(kept, run, 0o755); err != nil {
		return Applied{}, err
	}
	if err := in.keepSetup(); err != nil {
		return Applied{}, err
	}
	return Applied{Exit: exec.Command(run, "/S", "/UPDATE")}, nil
}

func safe(version string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, version)
}

// single is the one folder an archive unpacked to.
func single(dir string) (string, error) {
	list, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(list) != 1 || !list[0].IsDir() {
		return "", errors.New("the archive should hold one folder")
	}
	return filepath.Join(dir, list[0].Name()), nil
}

func inside(dir, name string) (string, error) {
	p := filepath.Join(dir, filepath.FromSlash(name))
	if !strings.HasPrefix(p, filepath.Clean(dir)+string(filepath.Separator)) {
		return "", fmt.Errorf("the archive has a path outside it: %s", name)
	}
	return p, nil
}

func untar(file, dir string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := inside(dir, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o777|0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, p); err != nil {
				return err
			}
		}
	}
}

func unzip(file, dir string) error {
	r, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		p, err := inside(dir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode()
		if mode&os.ModeSymlink != 0 {
			target, _ := io.ReadAll(in)
			in.Close()
			if err := os.Symlink(string(target), p); err != nil {
				return err
			}
			continue
		}
		out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm()|0o600)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// TrialTime is how long a new version must run before its update is kept; a crash sooner rolls it back (as D74 does for native rebuilds).
const TrialTime = 10 * time.Second

// trial is the record of an update that has not proved itself yet.
type trial struct {
	Version  string    `json:"version"`
	Previous string    `json:"previous"`
	Started  time.Time `json:"started,omitempty"`
}

func trialFile(dir string) string {
	return filepath.Join(dir, "trial.json")
}

// BeginTrial records an applied update; the new version's first run proves it or rolls it back.
func BeginTrial(dir string, a Applied) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, _ := json.Marshal(trial{Version: a.Version, Previous: a.Previous})
	return os.WriteFile(trialFile(dir), b, 0o644)
}

// Start runs first thing in the new version: a trial that started before and was never confirmed means that run crashed, so it rolls back and reports the version it left; exit is the installer to run for that on Windows.
func Start(dir string, in Install, running string, now time.Time) (rolledBack string, exit *exec.Cmd, err error) {
	data, err := os.ReadFile(trialFile(dir))
	if err != nil {
		return "", nil, nil
	}
	var t trial
	if json.Unmarshal(data, &t) != nil || Compare(t.Version, running) != 0 {
		os.Remove(trialFile(dir))
		return "", nil, nil
	}
	if !t.Started.IsZero() {
		os.Remove(trialFile(dir))
		a, err := in.Restore()
		if err != nil {
			return "", nil, err
		}
		return t.Version, a.Exit, nil
	}
	t.Started = now
	b, _ := json.Marshal(t)
	return "", nil, os.WriteFile(trialFile(dir), b, 0o644)
}

// Clean removes downloads of versions not newer than the running one and the copy a restore ran from; previous keeps its one installer.
func Clean(dir, running string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || name == "previous" || name == "cache" {
			continue
		}
		if name == "restore" || Compare(name, running) <= 0 {
			os.RemoveAll(filepath.Join(dir, name))
		}
	}
}

// Confirm keeps the update once its version has run for TrialTime or exited cleanly; a trial of another version is left alone.
func Confirm(dir, running string) {
	data, err := os.ReadFile(trialFile(dir))
	if err != nil {
		return
	}
	var t trial
	if json.Unmarshal(data, &t) != nil || Compare(t.Version, running) == 0 && !t.Started.IsZero() {
		os.Remove(trialFile(dir))
	}
}
