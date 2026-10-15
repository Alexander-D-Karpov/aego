package modulecheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var kernelAndEditor = []string{
	"aego/schema/...", "aego/scene/...", "aego/render/...", "aego/asset/...", "aego/engine/...",
	"aego/script/...", "aego/ui/...", "aego/project/...",
	"aego/editor/core/...", "aego/editor/ext/...", "aego/editor/panels/...",
}

func repoRoot(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func TestKernelDoesNotImportModules(t *testing.T) {
	args := append([]string{"list", "-deps", "-tags", "aego_nonative"}, kernelAndEditor...)
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(p, "aego/modules/") {
			t.Errorf("kernel or editor reaches %s", p)
		}
	}
}

var physicsWords = regexp.MustCompile(`(?i)physics|PHY0\d\d|Body2D|Collider`)

// Frame phases are a kernel concept (docs/extensions.md), not the module.
var phaseNames = map[string]bool{"PrePhysics": true, "Physics": true, "PostPhysics": true}

func TestKernelDoesNotNamePhysics(t *testing.T) {
	root := repoRoot(t)
	dirs := []string{"schema", "scene", "render", "asset", "engine", "script", "ui", "project", "editor/core", "editor/ext", "editor/panels"}
	fset := token.NewFileSet()
	for _, d := range dirs {
		filepath.WalkDir(filepath.Join(root, d), func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				if s, err := strconv.Unquote(lit.Value); err == nil && !phaseNames[s] && physicsWords.MatchString(s) {
					t.Errorf("%s: literal %q names physics", fset.Position(lit.Pos()), s)
				}
				return true
			})
			return nil
		})
	}
}
