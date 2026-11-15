package windows

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compile from a separate working directory, using the real installer and PATH
// helper. In particular, Windows makensis rejects mixed path separators even
// when the same script compiles with the Linux version of makensis.
func TestNSISPackagesPathHelper(t *testing.T) {
	makensis, err := exec.LookPath("makensis")
	if err != nil {
		t.Skip("makensis is not installed")
	}

	root := t.TempDir()
	source := filepath.Join(root, "installer source with spaces")
	stage := filepath.Join(root, "stage with spaces")
	work := filepath.Join(root, "other working directory")
	for _, dir := range []string{source, stage, work} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"aego.nsi", "path.ps1"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"aego.exe", "aego-editor.exe", "VERSION", "README.txt"} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte("placeholder\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	compile := func(out string) ([]byte, error) {
		cmd := exec.Command(makensis, "-V4", "-DVERSION=test", "-DSRC="+stage,
			"-DOUT="+filepath.Join(root, out), filepath.Join(source, "aego.nsi"))
		cmd.Dir = work
		return cmd.CombinedOutput()
	}
	output, err := compile("setup.exe")
	if err != nil {
		t.Fatalf("compile installer: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `File: "path.ps1"->"aego-path.ps1"`) {
		t.Fatalf("installer did not package the PATH helper:\n%s", output)
	}
	if info, err := os.Stat(filepath.Join(root, "setup.exe")); err != nil || info.Size() == 0 {
		t.Fatalf("installer output missing or empty: %v", err)
	}

	if err := os.Remove(filepath.Join(source, "path.ps1")); err != nil {
		t.Fatal(err)
	}
	output, err = compile("missing-helper.exe")
	if err == nil || !strings.Contains(string(output), "path.ps1") || !strings.Contains(string(output), "no files found") {
		t.Fatalf("a missing PATH helper must fail compilation: %v\n%s", err, output)
	}
}
