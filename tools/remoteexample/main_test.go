package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func invoke(args ...string) (string, error) {
	var out bytes.Buffer
	err := run(context.Background(), args, &out, &out)
	return out.String(), err
}
func TestHelpAndUnknownCommand(t *testing.T) {
	if out, err := invoke("help"); err != nil || !strings.Contains(out, "no code execution") {
		t.Fatal(out, err)
	}
	if _, err := invoke("exec"); err == nil {
		t.Fatal("unknown command accepted")
	}
}
func TestLinkInspectNeverApprovesOrCreatesStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "unused")
	link, err := invoke("link", "-url", "https://example.com/m.json", "-mode", "scene", "-root", root)
	if err != nil {
		t.Fatal(err)
	}
	out, err := invoke("inspect", "-link", strings.TrimSpace(link), "-root", root)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if json.Unmarshal([]byte(out), &fields) != nil || fields["approved"] != false || fields["mode"] != "scene" {
		t.Fatal(out)
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("inspecting link had disk side effects", err)
	}
}
func TestInvalidOptionsAreRejectedBeforeStoreCreation(t *testing.T) {
	for _, args := range [][]string{{"clone", "-url", "file:///etc/passwd"}, {"open", "-url", "https://example.com/m.json", "-approve-origin", "https://example.com/path"}, {"clone", "-url", "https://example.com/m.json", "-approve-origin", "https://*.example.com"}, {"reopen"}, {"check"}, {"recent", "unexpected"}} {
		root := filepath.Join(t.TempDir(), "unused")
		args = append(args, "-root", root)
		if _, err := invoke(args...); err == nil {
			t.Fatal(args)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("bad options created store", args)
		}
	}
}
func TestRecentEmptyAndUntrustedNetwork(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	out, err := invoke("recent", "-root", root)
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatal(out, err)
	}
	if _, err = invoke("clone", "-root", root, "-url", "https://example.com/m.json"); err == nil || !strings.Contains(err.Error(), "explicit confirmation") {
		t.Fatal(err)
	}
}
func TestUnsafeDeepLinkParametersRejected(t *testing.T) {
	if _, err := invoke("inspect", "-link", "aego://example?manifest=https://example.com/m.json&exec=sh"); err == nil {
		t.Fatal("command accepted")
	}
}
