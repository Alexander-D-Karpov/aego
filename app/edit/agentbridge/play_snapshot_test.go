package agentbridge

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/project"
)

func TestMaterializePlayCopiesIndependentFilesAndHardlinks(t *testing.T) {
	for _, linked := range []bool{false, true} {
		name := "independent"
		if linked {
			name = "hardlinked source names"
		}
		t.Run(name, func(t *testing.T) {
			root, dst := t.TempDir(), t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "scripts"), 0700); err != nil {
				t.Fatal(err)
			}
			first := filepath.Join(root, "scripts", "first.aes")
			second := filepath.Join(root, "scripts", "second.aes")
			if err := os.WriteFile(first, []byte("first on disk"), 0600); err != nil {
				t.Fatal(err)
			}
			secondBytes := []byte("second on disk")
			if linked {
				if err := os.Link(first, second); err != nil {
					t.Skipf("filesystem does not allow hardlinks: %v", err)
				}
				secondBytes = []byte("first on disk")
			} else if err := os.WriteFile(second, secondBytes, 0600); err != nil {
				t.Fatal(err)
			}
			privateDir := filepath.Join(root, ".aego", "cache")
			if err := os.MkdirAll(privateDir, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(privateDir, "original-only")
			if err := os.WriteFile(marker, []byte("do not touch"), 0600); err != nil {
				t.Fatal(err)
			}
			// Alternate root spellings must not make source and copy aliases.
			suffix := string(os.PathSeparator) + "."
			overlay := map[string][]byte{
				project.FileName:    []byte("project snapshot"),
				"scripts/first.aes": []byte("unsaved edit"),
			}
			if err := materializePlay(root+suffix, dst+suffix, overlay); err != nil {
				t.Fatal(err)
			}
			copyFirst := filepath.Join(dst, "scripts", "first.aes")
			copySecond := filepath.Join(dst, "scripts", "second.aes")
			for _, pair := range [][2]string{{first, copyFirst}, {second, copySecond}, {copyFirst, copySecond}} {
				a, err := os.Stat(pair[0])
				if err != nil {
					t.Fatal(err)
				}
				b, err := os.Stat(pair[1])
				if err != nil {
					t.Fatal(err)
				}
				if os.SameFile(a, b) {
					t.Fatalf("play files share an identity: %q and %q", pair[0], pair[1])
				}
			}
			check := func(path string, want []byte) {
				t.Helper()
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("%s = %q, want %q, err=%v", path, got, want, err)
				}
			}
			check(copyFirst, []byte("unsaved edit"))
			check(copySecond, secondBytes)
			check(marker, []byte("do not touch"))
			if _, err := os.Stat(filepath.Join(dst, ".aego")); !os.IsNotExist(err) {
				t.Fatalf("private editor files copied into play project: %v", err)
			}
			if err := os.WriteFile(copyFirst, []byte("run writes first"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(copySecond, []byte("run writes second"), 0600); err != nil {
				t.Fatal(err)
			}
			check(first, []byte("first on disk"))
			check(second, secondBytes)
			if err := os.WriteFile(first, []byte("external change"), 0600); err != nil {
				t.Fatal(err)
			}
			check(copyFirst, []byte("run writes first"))
			check(copySecond, []byte("run writes second"))
			check(marker, []byte("do not touch"))
		})
	}
}

func TestMaterializePlayCaseAliasDoesNotOverwriteSource(t *testing.T) {
	root, dst := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "scripts", "Hero.aes")
	if err := os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	aliasInfo, err := os.Stat(filepath.Join(root, "scripts", "hero.aes"))
	if os.IsNotExist(err) || err == nil && !os.SameFile(info, aliasInfo) {
		t.Skip("filesystem does not resolve case variants to the same file")
	}
	if err != nil {
		t.Fatal(err)
	}
	overlay := map[string][]byte{project.FileName: []byte("project"), "scripts/hero.aes": []byte("unsaved")}
	if err := materializePlay(root, dst, overlay); err == nil {
		t.Fatal("ambiguous case aliases were not rejected")
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "original" {
		t.Fatalf("case alias modified the source: %q, %v", got, err)
	}
}
