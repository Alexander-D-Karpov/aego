package iconuse

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var iconLike = regexp.MustCompile(`^[a-z]+\.[a-z0-9_]+$`)

func callName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

func collect(fset *token.FileSet, f *ast.File, out map[string]string) {
	lit := func(e ast.Expr) {
		bl, ok := e.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return
		}
		s, err := strconv.Unquote(bl.Value)
		if err != nil || !iconLike.MatchString(s) {
			return
		}
		if _, seen := out[s]; !seen {
			out[s] = fset.Position(bl.Pos()).String()
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			switch name := callName(x.Fun); {
			case name == "IconAt" && len(x.Args) >= 3:
				lit(x.Args[2])
			case (name == "Button" || name == "TintButton") && len(x.Args) >= 4:
				lit(x.Args[1])
			case name == "Describe" && len(x.Args) >= 2:
				lit(x.Args[1])
			case name == "MenuItemIcon" && len(x.Args) >= 4:
				lit(x.Args[3])
			case name == "SubMenuIcon" && len(x.Args) >= 3:
				lit(x.Args[2])
			}
		case *ast.KeyValueExpr:
			if id, ok := x.Key.(*ast.Ident); ok && (id.Name == "Icon" || id.Name == "icon") {
				lit(x.Value)
			}
		case *ast.FuncDecl:
			if x.Body != nil && strings.Contains(strings.ToLower(x.Name.Name), "icon") {
				ast.Inspect(x.Body, func(m ast.Node) bool {
					if r, ok := m.(*ast.ReturnStmt); ok {
						for _, e := range r.Results {
							lit(e)
						}
					}
					return true
				})
			}
		}
		return true
	})
}

func TestEditorIconsAreListed(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	dirs := []string{filepath.Join(root, "editor"), filepath.Join(root, "ui")}
	mods, _ := filepath.Glob(filepath.Join(root, "modules", "*", "editor"))
	dirs = append(dirs, mods...)
	found := map[string]string{}
	fset := token.NewFileSet()
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			collect(fset, f, found)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	listed := map[string]bool{}
	for _, u := range Editor {
		if listed[u.ID] {
			t.Errorf("icon %s is listed twice", u.ID)
		}
		listed[u.ID] = true
	}
	for id, at := range found {
		if !listed[id] {
			t.Errorf("%s uses icon %s, add it to iconuse.Editor", at, id)
		}
	}
}

func TestReservationsNotExpired(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "ARCHITECTURE.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range Reserved {
		done := regexp.MustCompile(`(?m)^### [0-9.]+ Stage ` + regexp.QuoteMeta(r.Until) + `([^0-9]|$)`)
		if done.Match(data) {
			t.Errorf("icon %s was reserved until Stage %s (%s); that stage is in ARCHITECTURE.md, use the icon or drop the reservation", r.ID, r.Until, r.Reason)
		}
	}
}
