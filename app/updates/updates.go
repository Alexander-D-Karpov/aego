// Package updates keeps Aego itself up to date with the update library: where its releases are, what this build is, the trial after an update (D362).
package updates

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"aego/engine"
	"aego/update"
)

const (
	// DefaultBase is Aego's releases on Gitea: <base>/<channel>/release.json, where the channel is the stable or nightly release tag.
	DefaultBase = "https://git.akarpov.ru/sanspie/aego/releases/download"
	// EnvBase points the editor at another release server, for tests and mirrors; the signature is checked all the same.
	EnvBase = "AEGO_UPDATE_URL"
	// EnvRolledBack tells a restored version which newer one crashed, so it can say so.
	EnvRolledBack = "AEGO_UPDATE_ROLLED_BACK"
)

// Dir keeps downloads, the release cache and the trial record: UserCacheDir/aego/updates.
func Dir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "aego", "updates")
}

// Build is this copy: its version and, for nightlies, when its commit was made.
func Build() update.Build {
	b := update.Build{Version: engine.Version}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.time" {
				b.Time, _ = time.Parse(time.RFC3339, s.Value)
			}
		}
	}
	return b
}

// Source is where this copy looks for releases of a channel.
func Source(ch update.Channel) *update.Source {
	base := os.Getenv(EnvBase)
	if base == "" {
		base = DefaultBase
	}
	return &update.Source{BaseURL: base, Channel: ch, Cache: filepath.Join(Dir(), "cache")}
}

// Installed is how this copy was installed.
func Installed() (update.Install, error) {
	exe, err := os.Executable()
	if err != nil {
		return update.Install{}, err
	}
	in, err := update.Detect(exe)
	in.Keep = filepath.Join(Dir(), "previous")
	return in, err
}

// assetKind is the asset an install updates from.
func assetKind(in update.Install) update.Kind {
	return in.Kind
}

// Fetch downloads the release's asset for this install into Dir()/<version>.
func Fetch(ctx context.Context, r *update.Release, in update.Install, progress update.Progress) (string, error) {
	if !r.Allowed(Build()) {
		return "", update.ErrTooOld
	}
	a, err := r.Asset(runtime.GOOS, runtime.GOARCH, assetKind(in))
	if err != nil {
		return "", err
	}
	return update.Download(ctx, nil, a, filepath.Join(Dir(), r.Version), progress)
}

// Apply installs a fetched release and starts its trial: the new version's first run keeps it, a crash in the first seconds rolls it back.
func Apply(in update.Install, file string, r *update.Release) (update.Applied, error) {
	a, err := in.Apply(file, r.Version)
	if err != nil {
		return a, err
	}
	return a, update.BeginTrial(Dir(), a)
}

// Boot runs first thing in the editor and the CLI: it rolls back a new version that crashed in its trial, and confirms one that has run long enough; exit is the Windows installer that does the rollback.
func Boot() (rolledBack string, exit *exec.Cmd, confirm func(), err error) {
	confirm = func() {}
	in, ierr := Installed()
	if ierr != nil {
		return "", nil, confirm, nil
	}
	back, exit, err := update.Start(Dir(), in, engine.Version, time.Now())
	if err != nil || back != "" {
		return back, exit, confirm, err
	}
	update.Clean(Dir(), engine.Version)
	timer := time.AfterFunc(update.TrialTime, func() { update.Confirm(Dir(), engine.Version) })
	return "", nil, func() {
		timer.Stop()
		update.Confirm(Dir(), engine.Version)
	}, nil
}

// ErrNotInstalled says why a copy cannot update itself, for the editor to show.
var ErrNotInstalled = errors.New("this copy of Aego was not installed by its installer, install.sh or the AppImage, so it cannot update itself; download the new version from the install page")

// Guard runs first in main: a version that crashed in its trial restores the previous one and runs it with the same arguments.
func Guard(args []string) (confirm func(), code int, done bool) {
	back, exit, confirm, err := Boot()
	if err != nil || back == "" {
		return confirm, 0, false
	}
	if exit != nil {
		// the previous version's installer replaces this one and starts the editor itself
		if err := exit.Start(); err != nil {
			return confirm, 0, false
		}
		return confirm, 0, true
	}
	in, err := Installed()
	if err != nil {
		return confirm, 0, false
	}
	cmd := exec.Command(in.Launch, args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), EnvRolledBack+"="+back)
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return confirm, exit.ExitCode(), true
		}
		return confirm, 1, true
	}
	return confirm, 0, true
}

// RolledBack is the version that crashed and was undone before this run, for a message.
func RolledBack() string {
	return os.Getenv(EnvRolledBack)
}
