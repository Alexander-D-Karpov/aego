package main

import (
	"bytes"
	"encoding/json"
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
	"script/compiler", "script/bytecode",
}

var scriptRuntime = []string{
	"core/...", "schema",
	"script/abi", "script/source", "script/diag", "script/bytecode", "script/verify", "script/vm",
}

var scriptCompilerOnly = []string{
	"script/token", "script/lexer", "script/ast", "script/parser", "script/types", "script/resolver",
	"script/checker", "script/asm", "script/compiler", "script/harness",
	"script/workspace", "script/langsvc",
}

var editorAllow = []string{
	"core/...", "platform", "platform/input", "platform/clip", "gpu", "render", "render/...", "ui/...",
	"schema", "scene", "scene/...", "asset", "asset/...", "format/...", "project", "engine", "engine/module", "engine/docs", "editor/...",
}

var rules = []rule{
	{"core/...", []string{"core/..."}, nil},
	{"schema", []string{"core/..."}, nil},
	{"format/...", []string{"core/...", "schema"}, nil},
	{"platform", []string{"core/...", "platform/input"}, nil},
	{"platform/clip", []string{"core/...", "platform"}, nil},
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
	{"scene/scriptcomp", []string{"core/...", "schema", "scene", "format/..."}, nil},
	{"scene/frame", []string{"core/...", "schema", "asset", "gpu", "engine", "render", "render/...", "scene"}, nil},
	{"scene/sceneio", []string{"core/...", "schema", "asset", "format/...", "scene", "scene/scriptcomp"}, nil},
	{"render2d", []string{"core/...", "asset", "engine", "engine/module", "gpu", "render/...", "scene", "scene/frame", "schema"}, nil},
	{"render2d/editor", with(editorAllow, "render2d"), nil},
	{"render3d", []string{"core/...", "asset", "asset/pipeline", "asset/importers/obj", "engine", "engine/module", "gpu", "render", "render/...", "scene", "scene/frame", "schema"}, nil},
	{"render3d/editor", with(editorAllow, "render3d"), nil},
	{"ui", []string{"core/...", "gpu", "platform/input", "render", "render/...", "schema", "scene", "asset", "format/...", "ui/..."}, nil},
	{"ui/...", []string{"core/...", "gpu", "platform/input", "render", "render/...", "schema", "scene", "asset", "format/...", "ui/..."}, nil},
	{"editor/...", editorAllow, nil},
	{"editor/code/...", with(editorAllow, "script/token"), nil},
	{"editor/builtins", with(editorAllow, "render2d", "render2d/editor", "render3d", "render3d/editor"), nil},
	{"editor/scripteditor", with(editorAllow, "script/langsvc", "script/workspace", "script/source", "script/diag"), nil},
	{"app", []string{"..."}, nil},
	{"app/docstool", []string{"..."}, nil},
	{"app/edit", []string{"..."}, nil},
	{"app/projects", []string{"..."}, nil},
	{"app/supervise", []string{"..."}, nil},
	{"engine/module", []string{"core/...", "engine", "engine/res", "schema", "scene", "asset", "script/bind"}, nil},
	{"engine/builtin/headless", []string{"core/...", "engine", "engine/module", "schema", "scene", "scene/...", "render2d", "script/engine", "script/stdlib"}, nil},
	{"engine/builtin", []string{"core/...", "engine", "engine/module", "engine/builtin/headless", "schema", "scene", "scene/...", "render2d", "render3d", "script/engine", "script/bind", "script/bind/manifest", "script/stdlib"}, nil},
	{"engine/docs", []string{"core/...", "schema", "engine/module", "engine/res", "script/abi", "script/bind", "script/bind/manifest", "script/diag"}, nil},
	{"sdk", []string{"core/...", "engine", "engine/module", "schema", "scene", "asset", "asset/...", "render", "render/module", "render2d", "render3d", "platform/input", "script/bind"}, nil},
	{"app/run", []string{"..."}, nil},
	{"app/playground", []string{"core/...", "engine", "engine/builtin/headless", "engine/docs", "engine/module", "platform/input", "render2d", "scene", "scene/...", "schema", "script/..."}, nil},
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
	{"script/harness", with(scriptDev, "script/bind", "script/verify", "script/vm", "script/workspace"), nil},
	{"script/bytecode", scriptRuntime, nil},
	{"script/verify", scriptRuntime, nil},
	{"script/vm", scriptRuntime, nil},
	{"script/bind", with(scriptRuntime, "scene", "engine", "asset", "platform/input"), nil},
	{"script/stdlib", with(scriptRuntime, "script/bind", "scene", "scene/...", "engine", "asset", "platform/input"), nil},
	{"script/coroutine", scriptRuntime, nil},
	{"script/signal", scriptRuntime, nil},
	{"script/runtime", with(scriptRuntime, "script/bind", "script/coroutine", "script/signal", "scene", "scene/...", "engine", "asset", "asset/...", "platform/input"), nil},
	{"script/reload", with(scriptRuntime, "script/runtime"), nil},
	{"script/module", with(scriptRuntime, "script/bind", "script/runtime", "script/stdlib", "engine", "scene"), nil},
	{"script/engine", with(scriptRuntime, "engine", "engine/module", "scene", "scene/scriptcomp", "script/bind", "script/module", "script/stdlib", "asset/pipeline", "asset/importers/script", "script/workspace"), nil},
	{"asset/importers/script", with(scriptDev, "asset", "asset/pipeline", "script/workspace"), nil},
	{"script/bind/manifest", with(scriptRuntime, "script/bind", "script/runtime", "format/..."), nil},
	{"script/workspace", with(scriptDev, "script/bind/manifest"), nil},
	{"script/langsvc", with(scriptDev, "script/workspace", "script/bind/manifest"), nil},
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
	"app",
	"app/docstool",
	"app/run",
	"app/edit",
	"app/projects",
	"app/supervise",
	"editor/assets",
	"editor/document",
	"editor/launcher",
	"editor/scripteditor",
	"editor/session",
	"editor/state",
	"editor/welcome",
	"script/workspace",
	"demos/asset-hot-reload",
}

var denied = []struct {
	pkg  string
	deny []string
}{
	{"engine/module", []string{"editor/...", "asset/pipeline", "asset/importers/...", "app/..."}},
	{"scene/...", []string{"engine/module"}},
	{"schema", []string{"engine/module"}},
	{"engine/module", []string{"engine/builtin", "engine/builtin/headless"}},
	{"render2d", []string{"render3d", "render3d/...", "script/...", "editor/...", "render2d/editor"}},
	{"render3d", []string{"render2d", "render2d/...", "script/...", "editor/...", "render3d/editor"}},
	{"script/engine", []string{"render2d", "render2d/...", "render3d", "render3d/...", "editor/..."}},
	{"sdk", []string{"engine/builtin", "engine/builtin/headless"}},
	{"editor/...", []string{"engine/builtin", "engine/builtin/headless", "script/engine"}},
	{"app/run", []string{"editor/..."}},
	{"app/projects", []string{"editor/..."}},
	{"engine/builtin/headless", []string{"render3d", "render3d/...", "asset/pipeline", "asset/importers/...", "editor/..."}},
	{"app/playground", []string{"platform/sdl", "gpu/gl33", "gpu/gl33/...", "asset/pipeline", "asset/importers/...", "render3d", "render3d/...", "editor/..."}},
	{"runtime/...", []string{"editor/..."}},
	{"sdk", []string{"editor/..."}},
	{"script/bytecode", scriptCompilerOnly},
	{"script/verify", scriptCompilerOnly},
	{"script/vm", scriptCompilerOnly},
	{"script/bind", scriptCompilerOnly},
	{"script/coroutine", scriptCompilerOnly},
	{"script/signal", scriptCompilerOnly},
	{"script/stdlib", scriptCompilerOnly},
	{"script/runtime", scriptCompilerOnly},
	{"script/module", scriptCompilerOnly},
	{"script/reload", scriptCompilerOnly},
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

var runtimeRoots = []string{
	"engine", "engine/module", "engine/builtin", "scene", "scene/...", "render", "render/...",
	"render2d", "render3d", "script/engine", "script/runtime", "script/stdlib", "script/bind", "script/module", "sdk",
}

var runtimeForbidden = with(scriptCompilerOnly, "asset/pipeline", "asset/importers/...", "project", "editor/...", "app/...", "engine/docs")

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

var fileExceptions = map[string][]string{
	"sdk/editor.go": {"editor/..."},
}

func Check(root string) ([]string, error) {
	module, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	pkgs, err := listPackages(root, "")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range pkgs {
		local, ok := relPkg(module, p.path)
		if ok && local != "" {
			out = append(out, checkImports(module, local, local, p.imports, nil)...)
		}
	}
	rt, err := listPackages(root, "aego_runtime")
	if err != nil {
		return nil, err
	}
	out = append(out, checkRuntime(module, rt)...)
	ed, err := listPackages(root, "aego_editor")
	if err != nil {
		return nil, err
	}
	ev, err := checkEditor(module, ed)
	if err != nil {
		return nil, err
	}
	out = append(out, ev...)
	src, err := checkSources(root)
	if err != nil {
		return nil, err
	}
	out = append(out, src...)
	sort.Strings(out)
	return slices.Compact(out), nil
}

type pkgInfo struct {
	path    string
	dir     string
	files   []string
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

func listPackages(root, tags string) ([]pkgInfo, error) {
	cmd := exec.Command("go", "list", "-e", "-json", "-tags", tags, "./...")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, stderr.String())
	}
	var pkgs []pkgInfo
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p struct {
			ImportPath string
			Dir        string
			GoFiles    []string
			CgoFiles   []string
			Imports    []string
		}
		if err := dec.Decode(&p); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkgInfo{path: p.ImportPath, dir: p.Dir, files: append(p.GoFiles, p.CgoFiles...), imports: p.Imports})
	}
	return pkgs, nil
}

func checkEditor(module string, pkgs []pkgInfo) ([]string, error) {
	var out []string
	fset := token.NewFileSet()
	for _, p := range pkgs {
		local, ok := relPkg(module, p.path)
		if !ok || local == "" {
			continue
		}
		for _, name := range p.files {
			f, err := parser.ParseFile(fset, filepath.Join(p.dir, name), nil, parser.ImportsOnly)
			if err != nil {
				return nil, err
			}
			imps := make([]string, 0, len(f.Imports))
			for _, imp := range f.Imports {
				imps = append(imps, strings.Trim(imp.Path.Value, `"`))
			}
			out = append(out, checkFile(module, local, local+"/"+name, imps)...)
		}
	}
	return out, nil
}

func checkFile(module, local, file string, imports []string) []string {
	return checkImports(module, local, file, imports, fileExceptions[file])
}

func checkRuntime(module string, pkgs []pkgInfo) []string {
	var out []string
	for _, p := range pkgs {
		local, ok := relPkg(module, p.path)
		if !ok || !allowed(runtimeRoots, local) {
			continue
		}
		for _, imp := range p.imports {
			if l, ok := relPkg(module, imp); ok && allowed(runtimeForbidden, l) {
				out = append(out, fmt.Sprintf("%s: runtime build (aego_runtime) imports dev package %s", local, l))
			}
		}
	}
	return out
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

func checkImports(module, local, where string, imports []string, extra []string) []string {
	r, ok := ruleFor(local)
	if !ok {
		return []string{fmt.Sprintf("%s: no dependency rule for package", where)}
	}
	var out []string
	for _, imp := range imports {
		if imp == "C" {
			continue
		}
		if l, ok := relPkg(module, imp); ok {
			if allowed(extra, l) {
				continue
			}
			if !allowed(r.allow, l) {
				out = append(out, fmt.Sprintf("%s: forbidden import %s (rule %s)", where, l, r.pkg))
			}
			for _, d := range denied {
				if match(d.pkg, local) && allowed(d.deny, l) {
					out = append(out, fmt.Sprintf("%s: denied import %s", where, l))
				}
			}
			continue
		}
		if isStdlib(imp) {
			continue
		}
		if !allowed(r.external, imp) {
			out = append(out, fmt.Sprintf("%s: forbidden external import %s (rule %s)", where, imp, r.pkg))
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
