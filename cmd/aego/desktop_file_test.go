package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDesktopFileArgPreservesLocalPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		name, err := desktopFileArg([]string{"file:///C:/Users/Me/My%20Game/project.aego"})
		if err != nil || name != filepath.FromSlash("C:/Users/Me/My Game/project.aego") {
			t.Fatalf("local URI = %q, %v", name, err)
		}
		return
	}
	for _, uri := range []string{"file:///tmp/My%20Game/project.aego", "file://localhost/tmp/My%20Game/project.aego"} {
		name, err := desktopFileArg([]string{uri})
		if err != nil || name != "/tmp/My Game/project.aego" {
			t.Fatalf("local URI %q = %q, %v", uri, name, err)
		}
	}
}

func TestDesktopFileArgRejectsRemoteAndInjectedArguments(t *testing.T) {
	for _, args := range [][]string{
		nil, {"--tool"}, {"file:relative"}, {"file://remote/path"},
		{"file:///tmp/a?flag=1"}, {"file:///tmp/a?"}, {"file:///tmp/a#fragment"},
		{"file:///tmp/a%00b"}, {"https://example.com/project.aego"},
		{"file:///tmp/project.aego", "--tool"},
	} {
		if name, err := desktopFileArg(args); err == nil {
			t.Errorf("accepted unsafe URI %q as %q", args, name)
		}
	}
	if runtime.GOOS == "windows" {
		if name, err := desktopFileArg([]string{"file:////server/share/project.aego"}); err == nil {
			t.Errorf("accepted remote UNC path %q", name)
		}
	}
}

func TestEditDesktopFileEntryRejectsInvalidArgsBeforeOpening(t *testing.T) {
	for _, args := range [][]string{
		{"--file-link"}, {"--file-link", "--tool"}, {"--file-link", "file://remote/project.aego"},
		{"--file-link", "file:///tmp/project.aego", "--tool", "run"},
	} {
		if code := cmdEdit(args); code == 0 {
			t.Errorf("desktop entry accepted unsafe arguments: %q", args)
		}
	}
}
