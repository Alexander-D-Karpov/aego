package sysopen

import (
	"os/exec"
	"path/filepath"
	"runtime"

	"aego/platform"
)

func command(target string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	}
	return exec.Command("xdg-open", target)
}

func Open(target string) error {
	return command(target).Start()
}

func Reveal(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	case "windows":
		return exec.Command("explorer", "/select,", path).Start()
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}

func RevealLabel() string {
	switch runtime.GOOS {
	case "darwin":
		return "Show in Finder"
	case "windows":
		return "Show in Explorer"
	}
	return "Show in Files"
}

func Picker(win platform.Window, kind platform.DialogKind, filter, pattern string) func(start string, done func(string)) {
	fd, ok := win.(platform.FileDialogs)
	if !ok {
		return nil
	}
	return func(start string, done func(string)) {
		_ = fd.ShowFileDialog(platform.FileDialog{Kind: kind, Start: start, Filter: filter, Pattern: pattern}, func(p string, ok bool) {
			if ok && p != "" {
				done(p)
			}
		})
	}
}
