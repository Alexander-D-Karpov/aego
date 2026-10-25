package prefs

import (
	"os"
	"path/filepath"
	"testing"

	"aego/ui/theme"
)

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	if p, err := Load(dir); err != nil || p.Theme != theme.ModeSystem {
		t.Fatalf("a missing file loads %v, %v; want System", p.Theme, err)
	}
	if err := Save(dir, &Prefs{Theme: theme.ModeLight, Language: "ru"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, File))
	p, err := Load(dir)
	if err != nil || p.Theme != theme.ModeLight || p.Language != "ru" {
		t.Fatalf("loaded %v, %v from:\n%s", p.Theme, err, data)
	}
}
