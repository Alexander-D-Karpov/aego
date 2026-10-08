package supervise

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const sdkPath = "aego/sdk"

type entry struct {
	name string
	list bool
}

func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "module")
		if !ok || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		return strings.Trim(strings.TrimSpace(rest), `"`), nil
	}
	return "", errors.New("go.mod has no module line")
}

func importName(f *ast.File, path string) string {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != path {
			continue
		}
		if imp.Name == nil {
			return filepath.Base(path)
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			return ""
		}
		return imp.Name.Name
	}
	return ""
}

func isModule(e ast.Expr, pkg string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Module" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}

func entryCalls(dir string) ([]entry, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	seen := map[string]bool{}
	var out []entry
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		pkg := importName(f, sdkPath)
		if pkg == "" {
			continue
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() || fn.Type.TypeParams != nil {
				continue
			}
			if len(fn.Type.Params.List) != 0 || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 || len(fn.Type.Results.List[0].Names) > 1 {
				continue
			}
			if seen[fn.Name.Name] {
				continue
			}
			switch t := fn.Type.Results.List[0].Type.(type) {
			case *ast.SelectorExpr:
				if isModule(t, pkg) {
					out = append(out, entry{name: fn.Name.Name})
					seen[fn.Name.Name] = true
				}
			case *ast.ArrayType:
				if t.Len == nil && isModule(t.Elt, pkg) {
					out = append(out, entry{name: fn.Name.Name, list: true})
					seen[fn.Name.Name] = true
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("native package exports no module: add func Modules() []sdk.Module or func Module() sdk.Module")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func mainSource(nativePkg string, calls []entry) []byte {
	var b strings.Builder
	b.WriteString("package main\n\nimport (\n\t\"aego/app/edit\"\n\t\"aego/sdk\"\n\n\tnative \"")
	b.WriteString(nativePkg)
	b.WriteString("\"\n)\n\nfunc main() {\n\tvar mods []sdk.Module\n")
	for _, c := range calls {
		b.WriteString("\tmods = append(mods, native.")
		b.WriteString(c.name)
		if c.list {
			b.WriteString("()...)\n")
		} else {
			b.WriteString("())\n")
		}
	}
	b.WriteString("\tedit.Main(mods...)\n}\n")
	return []byte(b.String())
}
