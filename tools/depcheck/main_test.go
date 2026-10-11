package main

import "testing"

func TestEditorFileException(t *testing.T) {
	imp := []string{"aego/editor/context", "aego/engine/module"}
	if v := checkFile("aego", "sdk", "sdk/editor.go", imp); len(v) != 0 {
		t.Fatalf("sdk/editor.go may import the editor: %v", v)
	}
	if v := checkFile("aego", "sdk", "sdk/script.go", imp); len(v) == 0 {
		t.Fatal("another sdk file importing the editor was accepted")
	}
	if v := checkFile("aego", "app/run", "app/run/run.go", []string{"aego/editor/context"}); len(v) == 0 {
		t.Fatal("app/run importing the editor was accepted")
	}
}
