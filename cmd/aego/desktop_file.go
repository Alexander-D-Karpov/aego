package main

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"aego/engine/i18n"
)

// desktopFileArg converts a single local file URI from an OS desktop entry.
// Remote hosts, credentials, flags and ambiguous query/fragment data are not
// project paths and must never be interpreted as editor commands.
func desktopFileArg(args []string) (string, error) {
	if len(args) != 1 || len(args[0]) > 32768 {
		return "", errors.New(i18n.Tr("cli.edit.desktop.one_uri", "A desktop file request requires exactly one local URI."))
	}
	u, err := url.Parse(args[0])
	if err != nil || u == nil || u.Scheme != "file" || u.Opaque != "" || u.User != nil ||
		(u.Host != "" && !strings.EqualFold(u.Host, "localhost")) || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsRune(u.Path, 0) {
		return "", errors.New(i18n.Tr("cli.edit.desktop.invalid_uri", "The desktop file URI is invalid or nonlocal."))
	}
	name := u.Path
	if runtime.GOOS == "windows" {
		// Do not turn a file:////host/share URI into a Windows UNC path.
		if len(name) < 4 || name[0] != '/' || name[2] != ':' || name[3] != '/' ||
			!((name[1] >= 'A' && name[1] <= 'Z') || (name[1] >= 'a' && name[1] <= 'z')) {
			return "", errors.New(i18n.Tr("cli.edit.desktop.local_drive", "The desktop file URI must name a local drive."))
		}
		name = name[1:]
	}
	name = filepath.FromSlash(name)
	if !filepath.IsAbs(name) {
		return "", errors.New(i18n.Tr("cli.edit.desktop.absolute_path", "The desktop file URI must have an absolute path."))
	}
	return name, nil
}
