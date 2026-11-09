package edit

import (
	"os"
	"os/exec"
	"strings"

	"aego/app/proc"
	"aego/editor/core/session"
	"aego/project/remoteexample"
)

const (
	EnvEditorExe  = "AEGO_EDITOR_EXE"
	EnvEditorVerb = "AEGO_EDITOR_VERB"
)

func markEntry(verb string) {
	if os.Getenv(EnvEditorExe) != "" {
		return
	}
	if img := os.Getenv("APPIMAGE"); img != "" {
		os.Setenv(EnvEditorExe, img)
		os.Setenv(EnvEditorVerb, "")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	os.Setenv(EnvEditorExe, exe)
	os.Setenv(EnvEditorVerb, verb)
}

func relaunchArgs(verb, target, tab string) []string {
	if verb != "" {
		args := []string{verb}
		if target != "" {
			args = append(args, target)
		}
		if tab != "" {
			args = append(args, "--launcher", tab)
		}
		return args
	}
	var args []string
	if tab != "" {
		args = append(args, "-launcher", tab)
	}
	if target != "" {
		args = append(args, target)
	}
	return args
}

func withoutEnv(env []string, key string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}

func Relaunch(target, tab string) error {
	exe := os.Getenv(EnvEditorExe)
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return err
		}
	}
	cmd := exec.Command(exe, relaunchArgs(os.Getenv(EnvEditorVerb), target, tab)...)
	cmd.Env = withoutEnv(os.Environ(), session.EnvSupervised)
	proc.Hide(cmd)
	return cmd.Start()
}

// OpenExampleLink starts a separate restricted launcher without closing or
// saving the current project. Validation is repeated in the receiving entry;
// no shell, project path, additional flags or network request is derived here.
func OpenExampleLink(raw string) error {
	if _, err := remoteexample.ParseLink(raw); err != nil {
		return err
	}
	exe := os.Getenv(EnvEditorExe)
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return err
		}
	}
	cmd := exec.Command(exe, exampleLinkArgs(os.Getenv(EnvEditorVerb), raw)...)
	cmd.Env = withoutEnv(os.Environ(), session.EnvSupervised)
	proc.Hide(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Unlike a project switch, this editor stays alive; reap the separate
	// launcher when it eventually exits rather than retaining a child process.
	go func() { _ = cmd.Wait() }()
	return nil
}

func exampleLinkArgs(verb, raw string) []string {
	if verb != "" {
		return []string{verb, "--example-link", raw}
	}
	return []string{"--example-link", raw}
}
