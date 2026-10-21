package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalList(t *testing.T) {
	file := filepath.Join(t.TempDir(), "external.txt")
	text := "# header\naudio/codec/vorbis -> github.com/jfreymuth/oggvorbis, pure Go decoder\nbroken line\n"
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	list, bad, err := loadExternal(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || len(bad) != 1 {
		t.Fatalf("%d entries, %d bad lines", len(list), len(bad))
	}
	c := &collector{ext: list}
	if r := c.externalRule("audio/codec/vorbis", "github.com/jfreymuth/oggvorbis"); r != "" {
		t.Fatalf("listed import rejected: %s", r)
	}
	if r := c.externalRule("audio/mix", "github.com/jfreymuth/oggvorbis"); r != "external-import" {
		t.Fatalf("unlisted package: %q", r)
	}
	if r := c.externalRule("audio/codec/vorbis", "github.com/other/lib"); r != "external-import" {
		t.Fatalf("unlisted import: %q", r)
	}
	if u := unusedExternal(list); len(u) != 0 {
		t.Fatalf("used entry reported unused: %v", u)
	}
}
