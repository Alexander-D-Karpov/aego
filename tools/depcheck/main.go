package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

type rule struct {
	pkg      string
	allow    []string
	external []string
}

func with(base []string, more ...string) []string {
	return append(slices.Clone(base), more...)
}

var scriptDev = []string{
	"core/...", "schema", "format/...",
	"script/abi", "script/source", "script/token", "script/diag", "script/lexer", "script/ast",
	"script/parser", "script/types", "script/resolver", "script/checker", "script/asm",
	"script/compiler", "script/bytecode", "script/build",
}

var scriptRuntime = []string{
	"core/...", "schema",
	"script/abi", "script/source", "script/diag", "script/bytecode", "script/verify", "script/vm",
}

var scriptCompilerOnly = []string{
	"script/token", "script/lexer", "script/ast", "script/parser", "script/types", "script/resolver",
	"script/checker", "script/asm", "script/compiler", "script/build", "script/harness",
}

var rules = []rule{
	{"core/...", []string{"core/..."}, nil},
	{"schema", []string{"core/..."}, nil},
	{"format/...", []string{"core/...", "schema"}, nil},
	{"platform", []string{"core/...", "platform/input"}, nil},
	{"platform/input", []string{"core/..."}, nil},
	{"platform/sdl", []string{"core/...", "platform", "platform/input"}, nil},
	{"platform/headless", []string{"core/...", "platform", "platform/input"}, nil},
	{"gpu", []string{"core/..."}, nil},
	{"gpu/null", []string{"core/...", "gpu"}, nil},
	{"gpu/gl33", []string{"core/...", "gpu", "gpu/gl33/..."}, nil},
	{"gpu/gl33/gl", nil, nil},
	{"gpu/gl33/internal/...", []string{"core/..."}, nil},
	{"vfs", []string{"core/...", "vfs/osfs"}, nil},
	{"vfs/osfs", []string{"core/..."}, nil},
	{"engine", []string{"core/...", "platform/...", "gpu/...", "vfs/...", "engine/res"}, nil},
	{"engine/res", []string{"core/...", "vfs"}, nil},
	{"engine/bootstrap", []string{"core/...", "project"}, nil},
	{"render", []string{"core/...", "gpu", "vfs", "render/..."}, nil},
	{"render/module", []string{"core/...", "gpu", "engine", "render"}, nil},
	{"render/...", []string{"core/...", "gpu", "vfs", "render/..."}, nil},
	{"asset", []string{"core/...", "gpu", "format/...", "render/texture", "render/mesh"}, nil},
	{"asset/loader", []string{"core/...", "gpu", "vfs", "asset", "render/texture", "render/mesh"}, nil},
	{"asset/pipeline", []string{"core/...", "asset", "format/...", "project", "vfs", "render/texture"}, nil},
	{"asset/importers/...", []string{"core/...", "gpu", "asset", "asset/pipeline", "format/...", "render/texture", "render/mesh"}, nil},
	{"project", []string{"core/...", "format/...", "platform/input"}, nil},
	{"scene", []string{"core/...", "schema", "asset", "render/..."}, nil},
	{"scene/components", []string{"core/...", "schema", "asset", "gpu", "render/...", "scene", "format/..."}, nil},
	{"scene/sceneio", []string{"core/...", "schema", "asset", "format/...", "scene", "scene/components"}, nil},
	{"scene/systems", []string{"core/...", "schema", "asset", "gpu", "engine", "render", "render/...", "scene", "scene/components"}, nil},
	{"ui", []string{"core/...", "gpu", "platform/input", "render", "render/...", "schema", "scene", "asset", "format/...", "ui/..."}, nil},
	{"ui/...", []string{"core/...", "gpu", "platform/input", "render", "render/...", "schema", "scene", "asset", "format/...", "ui/..."}, nil},
	{"editor/...", []string{"core/...", "platform", "platform/input", "gpu", "render", "render/...", "ui/...", "schema", "scene", "scene/...", "asset", "asset/...", "format/...", "project", "engine", "editor/..."}, nil},
	{"app/edit", []string{"..."}, nil},
	{"app/supervise", []string{"..."}, nil},
	{"sdk", []string{"core/...", "engine", "schema", "scene", "scene/components", "asset", "asset/...", "render", "render/module", "platform/input", "script/bind"}, nil},
	{"app/run", []string{"..."}, nil},
	{"demos/...", []string{"..."}, nil},
	{"cmd/...", []string{"..."}, nil},
	{"tests/...", []string{"..."}, nil},
	{"tools/...", []string{"..."}, nil},
	{"examples/...", []string{"sdk"}, nil},
	{"script/abi", []string{"core/...", "schema"}, nil},
	{"script/source", scriptDev, nil},
	{"script/token", scriptDev, nil},
	{"script/diag", scriptDev, nil},
	{"script/lexer", scriptDev, nil},
	{"script/ast", scriptDev, nil},
	{"script/parser", scriptDev, nil},
	{"script/types", scriptDev, nil},
	{"script/resolver", scriptDev, nil},
	{"script/checker", scriptDev, nil},
	{"script/asm", scriptDev, nil},
	{"script/compiler", scriptDev, nil},
	{"script/build", scriptDev, nil},
	{"script/harness", with(scriptDev, "script/bind", "script/verify", "script/vm"), nil},
	{"script/bytecode", scriptRuntime, nil},
	{"script/verify", scriptRuntime, nil},
	{"script/vm", scriptRuntime, nil},
	{"script/bind", with(scriptRuntime, "scene", "engine", "asset", "platform/input"), nil},
	{"script/stdlib", with(scriptRuntime, "script/bind", "scene", "scene/...", "engine", "asset", "platform/input"), nil},
	{"script/runtime", with(scriptRuntime, "script/bind", "scene", "scene/...", "engine", "asset", "asset/...", "platform/input"), nil},
	{"script/module", with(scriptRuntime, "script/bind", "script/runtime", "script/stdlib", "engine", "scene"), nil},
	{"asset/importers/script", with(scriptDev, "asset", "asset/pipeline"), nil},
}

var osFileFuncs = map[string]bool{
	"Open": true, "OpenFile": true, "Create": true, "CreateTemp": true,
	"ReadFile": true, "WriteFile": true, "ReadDir": true, "MkdirAll": true,
	"Mkdir": true, "Remove": true, "RemoveAll": true, "Rename": true, "Stat": true,
}

var fsAllowed = []string{
	"vfs/...",
	"platform/...",
	"tools/...",
	"cmd/...",
	"project",
	"asset/pipeline",
	"engine/bootstrap",
	"app/run",
	"app/edit",
	"app/supervise",
	"editor/assets",
	"editor/document",
	"editor/session",
	"editor/state",
	"demos/asset-hot-reload",
}

var denied = []struct {
	pkg  string
	deny []string
}{
	{"app/run", []string{"editor/..."}},
	{"runtime/...", []string{"editor/..."}},
	{"sdk", []string{"editor/..."}},
	{"script/bytecode", scriptCompilerOnly},
	{"script/verify", scriptCompilerOnly},
	{"script/vm", scriptCompilerOnly},
	{"script/bind", scriptCompilerOnly},
	{"script/stdlib", scriptCompilerOnly},
	{"script/runtime", scriptCompilerOnly},
	{"script/module", scriptCompilerOnly},
}

var cgoAllowed = []string{"platform/sdl", "gpu/gl33/gl"}

var serviceTypes = map[string]bool{
	"*engine.Engine": true, "engine.Engine": true, "*engine.Builder": true,
	"gpu.Device": true, "platform.Platform": true, "platform.Window": true,
	"*log.Logger": true, "vfs.FS": true, "*vfs.VFS": true, "*input.State": true,
}

var servicePkgs = map[string]bool{
	"engine": true, "gpu": true, "gl33": true, "null": true, "platform": true,
	"sdl": true, "headless": true, "log": true, "vfs": true,
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	violations, err := Check(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "depcheck:", err)
		os.Exit(2)
	}
	for _, v := range violations {
		fmt.Println(v)
	}
	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "depcheck: %d violation(s)\n", len(violations))
		os.Exit(1)
	}
	fmt.Println("depcheck: ok")
}

func Check(root string) ([]string, error) {
	module, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	pkgs, err := listPackages(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range pkgs {
		out = append(out, checkImports(module, p)...)
	}
	src, err := checkSources(root)
	if err != nil {
		return nil, err
	}
	out = append(out, src...)
	sort.Strings(out)
	return out, nil
}

type pkgInfo struct {
	path    string
	imports []string
}

func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", errors.New("module directive not found in go.mod")
}

func listPackages(root string) ([]pkgInfo, error) {
	cmd := exec.Command("go", "list", "-e", "-f", "{{.ImportPath}}\t{{join .Imports \" \"}}", "./...")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, stderr.String())
	}
	var pkgs []pkgInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		p := pkgInfo{path: parts[0]}
		if len(parts) == 2 && parts[1] != "" {
			p.imports = strings.Fields(parts[1])
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

func relPkg(module, path string) (string, bool) {
	if path == module {
		return "", true
	}
	if rest, ok := strings.CutPrefix(path, module+"/"); ok {
		return rest, true
	}
	return "", false
}

func match(pattern, p string) bool {
	if pattern == "..." {
		return true
	}
	if base, ok := strings.CutSuffix(pattern, "/..."); ok {
		return p == base || strings.HasPrefix(p, base+"/")
	}
	return p == pattern
}

func allowed(patterns []string, p string) bool {
	for _, pat := range patterns {
		if match(pat, p) {
			return true
		}
	}
	return false
}

func ruleFor(p string) (rule, bool) {
	best, bestScore := rule{}, -1
	for _, r := range rules {
		if !match(r.pkg, p) {
			continue
		}
		score := len(strings.TrimSuffix(r.pkg, "/...")) * 2
		if !strings.HasSuffix(r.pkg, "/...") {
			score++
		}
		if score > bestScore {
			best, bestScore = r, score
		}
	}
	return best, bestScore >= 0
}

func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

func checkImports(module string, p pkgInfo) []string {
	local, ok := relPkg(module, p.path)
	if !ok || local == "" {
		return nil
	}
	r, ok := ruleFor(local)
	if !ok {
		return []string{fmt.Sprintf("%s: no dependency rule for package", local)}
	}
	var out []string
	for _, imp := range p.imports {
		if imp == "C" {
			continue
		}
		if l, ok := relPkg(module, imp); ok {
			if !allowed(r.allow, l) {
				out = append(out, fmt.Sprintf("%s: forbidden import %s (rule %s)", local, l, r.pkg))
			}
			for _, d := range denied {
				if match(d.pkg, local) && allowed(d.deny, l) {
					out = append(out, fmt.Sprintf("%s: denied import %s", local, l))
				}
			}
			continue
		}
		if isStdlib(imp) {
			continue
		}
		if !allowed(r.external, imp) {
			out = append(out, fmt.Sprintf("%s: forbidden external import %s (rule %s)", local, imp, r.pkg))
		}
	}
	return out
}

func checkSources(root string) ([]string, error) {
	var out []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || name == "third_party" || name == "testdata" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		dir := filepath.ToSlash(filepath.Dir(rel))
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		out = append(out, inspectFile(fset, f, rel, dir)...)
		return nil
	})
	return out, err
}

func inspectFile(fset *token.FileSet, f *ast.File, file, dir string) []string {
	var out []string
	osName := ""
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p == "C" && !allowed(cgoAllowed, dir) {
			out = append(out, fmt.Sprintf("%s: cgo is only allowed in %v", file, cgoAllowed))
		}
		if p == "os" {
			osName = "os"
			if imp.Name != nil {
				osName = imp.Name.Name
			}
		}
	}
	if osName != "" && !allowed(fsAllowed, dir) {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if ok && id.Name == osName && osFileFuncs[sel.Sel.Name] {
				out = append(out, fmt.Sprintf("%s: os.%s outside vfs/platform", fset.Position(sel.Pos()), sel.Sel.Name))
			}
			return true
		})
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if vs.Type != nil && serviceTypes[types.ExprString(vs.Type)] {
				out = append(out, fmt.Sprintf("%s: global service variable %s", fset.Position(vs.Pos()), types.ExprString(vs.Type)))
				continue
			}
			for _, v := range vs.Values {
				if isServiceValue(v) {
					out = append(out, fmt.Sprintf("%s: global service variable", fset.Position(vs.Pos())))
					break
				}
			}
		}
	}
	return out
}

func isServiceValue(e ast.Expr) bool {
	if u, ok := e.(*ast.UnaryExpr); ok {
		e = u.X
	}
	switch x := e.(type) {
	case *ast.CallExpr:
		sel, ok := x.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && servicePkgs[id.Name] && strings.HasPrefix(sel.Sel.Name, "New")
	case *ast.CompositeLit:
		if x.Type == nil {
			return false
		}
		t := types.ExprString(x.Type)
		return serviceTypes[t] || serviceTypes["*"+t]
	}
	return false
}
