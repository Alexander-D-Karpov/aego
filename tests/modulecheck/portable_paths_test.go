package modulecheck

import (
	"os/exec"
	"strings"
	"testing"
)

// windowsPathComponent checks names in the Git tree, not the host filesystem:
// Windows cannot even check out a tree containing these names.
func windowsPathComponent(name string) bool {
	if name == "" || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return false
	}
	if strings.ContainsAny(name, `<>:"\|?*`) {
		return false
	}
	for _, c := range name {
		if c < 32 {
			return false
		}
	}
	base, _, _ := strings.Cut(name, ".")
	base = strings.ToUpper(strings.TrimRight(base, " "))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		suffix := base[3:]
		if len(suffix) == 1 && suffix[0] >= '1' && suffix[0] <= '9' || suffix == "¹" || suffix == "²" || suffix == "³" {
			return false
		}
	}
	return true
}

func TestWindowsPathComponents(t *testing.T) {
	for _, name := range []string{"aux.go", "CON", "lpt1.txt", "COM9", "COM¹.txt", "LPT²", "bad.", "bad ", `bad:name`} {
		if windowsPathComponent(name) {
			t.Errorf("%q should not be portable", name)
		}
	}
	for _, name := range []string{"auxiliary.go", "com10", "lpt0.txt", "console.go"} {
		if !windowsPathComponent(name) {
			t.Errorf("%q should be portable", name)
		}
	}
}

func TestGitTreeIsWindowsPortable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := repoRoot(t)
	probe := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	probe.Dir = root
	if err := probe.Run(); err != nil {
		t.Skip("not a Git checkout")
	}
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]string)
	for _, path := range strings.Split(string(out), "\x00") {
		if path == "" {
			continue
		}
		parts := strings.Split(path, "/")
		for i, part := range parts {
			prefix := strings.Join(parts[:i+1], "/")
			if !windowsPathComponent(part) {
				t.Errorf("%s contains a Windows-incompatible name %q", path, part)
			}
			folded := strings.ToLower(prefix)
			if other, ok := seen[folded]; ok && other != prefix {
				t.Errorf("case-insensitive path collision: %s and %s", other, prefix)
			} else {
				seen[folded] = prefix
			}
		}
	}
}
