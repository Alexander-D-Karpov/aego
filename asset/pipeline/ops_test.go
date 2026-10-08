package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveKeepsIDAndMeta(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	pl.Scan()
	pl.ImportAll()
	id, _ := pl.Lookup("assets/a.txt")

	if err := pl.Move(id, "assets/sub/b.txt"); err != nil {
		t.Fatal(err)
	}
	if path, _ := pl.PathOf(id); path != "assets/sub/b.txt" {
		t.Fatalf("path = %s", path)
	}
	if _, err := os.Stat(filepath.Join(p.Root, "assets", "sub", "b.txt.meta")); err != nil {
		t.Fatal("meta did not move with the file")
	}
	if e, ok := pl.Entry(id); !ok || e.Source != "assets/sub/b.txt" {
		t.Fatal("index entry not updated")
	}
	if err := pl.Move(id, "assets/sub/b.png"); err == nil {
		t.Fatal("extension change accepted")
	}
	if err := pl.Move(id, "../outside.txt"); err == nil {
		t.Fatal("move outside the roots accepted")
	}
	fresh := newPipeline(t, p, &fakeImporter{version: 1})
	fresh.Scan()
	if again, _ := fresh.Lookup("assets/sub/b.txt"); again != id {
		t.Fatal("id changed after the move")
	}
}

func TestMoveDir(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/chars/a.txt", "a")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	pl.Scan()
	id, _ := pl.Lookup("assets/chars/a.txt")
	if err := pl.MoveDir("assets/chars", "assets/heroes"); err != nil {
		t.Fatal(err)
	}
	if path, _ := pl.PathOf(id); path != "assets/heroes/a.txt" {
		t.Fatalf("path = %s", path)
	}
	if err := pl.MoveDir("assets", "assets/x"); err == nil {
		t.Fatal("root move accepted")
	}
}

func TestDuplicateAndDelete(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	pl.Scan()
	pl.ImportAll()
	id, _ := pl.Lookup("assets/a.txt")

	dup, batch, err := pl.Duplicate(id)
	if err != nil {
		t.Fatal(err)
	}
	if dup == id || len(batch.Imported) != 1 {
		t.Fatal("duplicate must get a fresh id and be imported")
	}
	if path, _ := pl.PathOf(dup); path != "assets/a copy.txt" {
		t.Fatalf("duplicate path = %s", path)
	}
	del, err := pl.Delete(id)
	if err != nil || len(del.Removed) != 1 {
		t.Fatalf("delete: %v %v", del, err)
	}
	if e, ok := pl.Entry(id); !ok || !e.Missing {
		t.Fatal("deleted asset must stay in the index as missing")
	}
	if _, ok := pl.PathOf(id); ok {
		t.Fatal("deleted asset is still tracked")
	}
}

func TestImportExternal(t *testing.T) {
	p := newProject(t)
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	pl.Scan()
	src := filepath.Join(t.TempDir(), "dropped.txt")
	os.WriteFile(src, []byte("external"), 0o644)
	bad := filepath.Join(t.TempDir(), "dropped.bin")
	os.WriteFile(bad, []byte("x"), 0o644)

	created, batch, err := pl.ImportExternal([]string{src, bad}, "assets/incoming")
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || len(batch.Imported) != 1 {
		t.Fatalf("created=%v imported=%d", created, len(batch.Imported))
	}
	if path, _ := pl.PathOf(created[0]); path != "assets/incoming/dropped.txt" {
		t.Fatalf("path = %s", path)
	}
	again, _, err := pl.ImportExternal([]string{src}, "assets/incoming")
	if err != nil || len(again) != 1 {
		t.Fatal(err)
	}
	if path, _ := pl.PathOf(again[0]); path != "assets/incoming/dropped 2.txt" {
		t.Fatalf("second import path = %s", path)
	}
	if _, _, err := pl.ImportExternal([]string{bad}, "assets"); err == nil {
		t.Fatal("unsupported file accepted")
	}
}

func TestAssetsAndDirs(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/x/a.txt", "a")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	pl.Scan()
	pl.ImportAll()
	if err := pl.Mkdir("assets/empty"); err != nil {
		t.Fatal(err)
	}
	dirs := pl.Dirs()
	want := map[string]bool{"assets": true, "assets/x": true, "assets/empty": true, "scenes": true}
	for _, d := range dirs {
		delete(want, d)
	}
	if len(want) != 0 {
		t.Fatalf("missing dirs %v in %v", want, dirs)
	}
	list := pl.Assets()
	if len(list) != 1 || list[0].Path != "assets/x/a.txt" || list[0].Name != "assets/x/a" {
		t.Fatalf("assets = %+v", list)
	}
}
