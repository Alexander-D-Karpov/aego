package agentbridge

import (
	"aego/editor/core/document"
	"os"
	"path/filepath"
	"testing"
)

func TestMaterialSourceUsesLiveBufferAndConfinesReads(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	for _, name := range []string{"cold.aemat", "cold.aeshader"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("saved"), 0600); err != nil {
			t.Fatal(err)
		}
		text, err := materialText(docs, name)
		if err != nil || text != "saved" {
			t.Fatal(text, err)
		}
		d, err := docs.OpenScript(name)
		if err != nil {
			t.Fatal(err)
		}
		d.Code.ReplaceSnapshot([]byte("changing Strength in the editor"), false)
		text, err = materialText(docs, name)
		if err != nil || text != "changing Strength in the editor" {
			t.Fatal(text, err)
		}
	}
	secret := filepath.Join(t.TempDir(), "foreign.aeshader")
	if err := os.WriteFile(secret, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "link.aeshader")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"link.aeshader", "../cold.aemat", "/tmp/foo.aeshader", "main.go"} {
		if _, e := materialText(docs, name); e == nil {
			t.Fatal(name)
		}
	}
}
