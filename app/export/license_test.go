package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebExportGoLicenseLocations(t *testing.T) {
	prefix := t.TempDir()
	root := filepath.Join(prefix, "lib", "go")
	standard := filepath.Join(root, "LICENSE")
	packaged := filepath.Join(prefix, "share", "licenses", "go", "LICENSE")
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := goLicense(root); got != "" {
		t.Fatalf("missing license: got %q", got)
	}
	write(packaged, "distribution Go license")
	if got := goLicense(root); got != packaged {
		t.Fatalf("packaged Go license: got %q, want %q", got, packaged)
	}
	job := job{opt: Options{Engine: prefix}, t: Target{OS: "js", Arch: "wasm"}, out: filepath.Join(prefix, "export"), goroot: root}
	libs := job.libs()
	if len(libs) == 0 || libs[0].License != "licenses/Go.txt" {
		t.Fatalf("web manifest has no Go license: %+v", libs)
	}
	data, err := os.ReadFile(filepath.Join(job.out, filepath.FromSlash(libs[0].License)))
	if err != nil || string(data) != "distribution Go license" {
		t.Fatalf("exported Go license: %q, %v", data, err)
	}
	write(standard, "GOROOT Go license")
	if got := goLicense(root); got != standard {
		t.Fatalf("GOROOT license should take precedence: got %q, want %q", got, standard)
	}
}
