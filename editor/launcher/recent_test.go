package launcher

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"aego/project"
)

func TestRecent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "aego", "recent")
	base := time.Unix(1_700_000_000, 0)
	var roots []string
	for i := 0; i < MaxRecent+2; i++ {
		root := filepath.Join(t.TempDir(), "p"+strconv.Itoa(i))
		os.MkdirAll(root, 0o755)
		os.WriteFile(filepath.Join(root, project.FileName), []byte("project version=1 {\n}\n"), 0o644)
		roots = append(roots, root)
		if err := Touch(file, root, "P"+strconv.Itoa(i), base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	list, err := LoadRecent(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != MaxRecent || list[0].Path != roots[len(roots)-1] {
		t.Fatalf("len=%d first=%s", len(list), list[0].Path)
	}
	Touch(file, roots[5], "P5", base.Add(time.Hour))
	list, _ = LoadRecent(file)
	if list[0].Path != roots[5] || len(list) != MaxRecent {
		t.Fatal("reopened project did not move to the top")
	}
	os.RemoveAll(roots[5])
	list, _ = LoadRecent(file)
	if !list[0].Missing {
		t.Fatal("deleted project is not marked missing")
	}
	Forget(file, roots[5])
	list, _ = LoadRecent(file)
	if list[0].Path == roots[5] {
		t.Fatal("forget kept the entry")
	}
}
