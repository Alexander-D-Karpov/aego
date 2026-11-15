package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/engine/i18n"
	"aego/project/remoteexample/cli"
)

func examplesCommand(args ...string) (string, string, error) {
	var out, diagnostics bytes.Buffer
	err := runRemoteExamples(context.Background(), args, &out, &diagnostics)
	return out.String(), diagnostics.String(), err
}

func TestExamplesCLIHelpAndUntrustedLink(t *testing.T) {
	bundle := i18n.Default()
	previous := bundle.Language()
	bundle.SetLanguage(i18n.English)
	t.Cleanup(func() { bundle.SetLanguage(previous) })
	out, _, err := examplesCommand("help")
	if err != nil || !strings.Contains(out, "aego examples clone") || !strings.Contains(out, "no code execution") {
		t.Fatal(out, err)
	}
	if strings.TrimSpace(out) != strings.ReplaceAll(cli.HelpText, "remoteexample ", "aego examples ") {
		t.Fatal("installed and standalone remote example help diverged")
	}
	bundle.SetLanguage(i18n.Russian)
	out, _, err = examplesCommand("help")
	if err != nil || !strings.Contains(out, "aego examples clone") || !strings.Contains(out, "только данные") {
		t.Fatalf("Russian help is missing: %s, %v", out, err)
	}
	root := filepath.Join(t.TempDir(), "not-created")
	link, _, err := examplesCommand("link", "--url", "https://example.com/manifest.json", "--mode", "scene", "--root", root)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err = examplesCommand("inspect", "--link", strings.TrimSpace(link), "--root", root)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Mode     string `json:"mode"`
		Approved bool   `json:"approved"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil || info.Mode != "scene" || info.Approved {
		t.Fatalf("link parsing incorrectly granted trust: %s, %v", out, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("link inspection created a store: %v", err)
	}
}

func TestExamplesCLIDoesNotAllowCompatibilityOverrides(t *testing.T) {
	for _, override := range [][]string{{"--engine", "99.0.0"}, {"--modules", "arbitrary"}} {
		root := filepath.Join(t.TempDir(), "not-created")
		args := append([]string{"clone", "--root", root, "--url", "https://example.com/manifest.json"}, override...)
		if _, _, err := examplesCommand(args...); err == nil {
			t.Fatalf("installed CLI accepted compatibility override %v", override)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("invalid options created a store: %v", err)
		}
	}
}

func TestExamplesCLIRequiresOriginApproval(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	_, _, err := examplesCommand("clone", "--root", root, "--url", "https://example.com/manifest.json")
	if err == nil || !strings.Contains(err.Error(), "explicit confirmation") {
		t.Fatalf("unapproved origin was accepted: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("an unapproved clone created a store: %v", err)
	}
}
