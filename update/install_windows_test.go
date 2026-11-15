package update

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const setupMarker = "\nAEGO_UPDATE_TEST_SETUP_VERSION="

// The test binary doubles as a real executable installer, so the Windows
// update path can run without a shell script or an external compiler.
func TestMain(m *testing.M) {
	if os.Getenv("AEGO_UPDATE_TEST_SETUP") != "1" {
		os.Exit(m.Run())
	}
	if err := runTestSetup(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runTestSetup() error {
	if len(os.Args) != 3 || os.Args[1] != "/S" || os.Args[2] != "/UPDATE" {
		return fmt.Errorf("unexpected setup arguments: %q", os.Args[1:])
	}
	path, err := os.Executable()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	i := strings.LastIndex(string(b), setupMarker)
	if i < 0 {
		return fmt.Errorf("setup version marker missing")
	}
	version := strings.TrimSpace(string(b[i+len(setupMarker):]))
	f, err := os.OpenFile(os.Getenv("AEGO_UPDATE_TEST_LOG"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(f, "%s %s\n", version, strings.Join(os.Args[1:], " "))
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr
}

func testWindowsSetup(t *testing.T, dst, version string) {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, src)
	if err == nil {
		_, err = fmt.Fprintln(out, setupMarker+version)
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}

func runWindowsSetup(t *testing.T, cmd *exec.Cmd, log string) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no installer command")
	}
	cmd.Env = append(os.Environ(), "AEGO_UPDATE_TEST_SETUP=1", "AEGO_UPDATE_TEST_LOG="+log)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run installer: %v: %s", err, output)
	}
}

func TestWindowsInstallerUpdateAndCrashedTrial(t *testing.T) {
	t.Setenv("APPIMAGE", "")
	dir := t.TempDir()
	root := filepath.Join(dir, "Aego")
	state := filepath.Join(dir, "updates")
	log := filepath.Join(dir, "runs.txt")
	setup := filepath.Join(root, SetupFile)
	testWindowsSetup(t, setup, "0.11.0")
	for name, content := range map[string]string{"VERSION": "0.11.0\n", "uninstall.exe": "stub", "aego-editor.exe": "stub"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	in, err := Detect(filepath.Join(root, "aego-editor.exe"))
	if err != nil || in.Kind != KindInstaller {
		t.Fatalf("detect Windows installer: %+v %v", in, err)
	}
	in.Keep = filepath.Join(state, "previous")
	next := filepath.Join(dir, "next-setup.exe")
	testWindowsSetup(t, next, "0.12.0")

	a, err := in.Apply(next, "0.12.0")
	if err != nil || a.Previous != "0.11.0" || !in.HasPrevious() {
		t.Fatalf("apply: %+v %v", a, err)
	}
	runWindowsSetup(t, a.Exit, log)
	testWindowsSetup(t, setup, "0.12.0") // the installer replaces its saved copy
	restored, err := in.Restore()
	if err != nil {
		t.Fatal(err)
	}
	runWindowsSetup(t, restored.Exit, log)

	testWindowsSetup(t, setup, "0.11.0")
	a, err = in.Apply(next, "0.12.0")
	if err != nil {
		t.Fatal(err)
	}
	runWindowsSetup(t, a.Exit, log)
	testWindowsSetup(t, setup, "0.12.0")
	BeginTrial(state, a)
	now := time.Now()
	if back, _, err := Start(state, in, "0.12.0", now); err != nil || back != "" {
		t.Fatalf("first trial: %q %v", back, err)
	}
	back, exit, err := Start(state, in, "0.12.0", now.Add(time.Second))
	if err != nil || back != "0.12.0" {
		t.Fatalf("crashed trial: %q %v", back, err)
	}
	runWindowsSetup(t, exit, log)
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "0.12.0 /S /UPDATE\n0.11.0 /S /UPDATE\n0.12.0 /S /UPDATE\n0.11.0 /S /UPDATE\n"
	if string(got) != want {
		t.Fatalf("installers ran as\n%s\nwant\n%s", got, want)
	}
}
