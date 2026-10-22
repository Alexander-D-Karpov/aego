package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"aego/project"
)

func TestDropAndCreateWith(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.FileName), []byte("project version=1 {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := New(Options{})
	if !l.Drop(filepath.Join(dir, project.FileName)) {
		t.Fatal("dropping project.aego did not open the project")
	}
	if root, _ := l.Result(); root != dir {
		t.Fatalf("result %q", root)
	}
	var got CreateRequest
	l = New(Options{CreateWith: func(r CreateRequest) (string, error) {
		got = r
		return r.Dir, nil
	}})
	l.native = true
	if _, err := l.Create("x", "2d", "", ""); err != nil || !got.Native {
		t.Fatalf("native flag lost: %+v %v", got, err)
	}
	l = New(Options{NativeHint: "no go", CreateWith: func(r CreateRequest) (string, error) {
		got = r
		return r.Dir, nil
	}})
	l.native = true
	if _, err := l.Create("x", "2d", "", ""); err != nil || got.Native {
		t.Fatal("native code was requested without Go")
	}
}
