package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Layer rules are the allowlists inside low and kernel (ARCHITECTURE §4).
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
	"schema", "scene", "scene/...", "asset", "asset/...", "format/...", "project", "engine", "engine/module", "engine/problems", "engine/docs", "engine/recorder", "engine/settings", "engine/settings/settingsui", "editor/...",
	"data", "save", "contracts/persistence",
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
	{"platform/web", []string{"core/...", "platform", "platform/input"}, nil},
	{"gpu", []string{"core/..."}, nil},
	{"gpu/null", []string{"core/...", "gpu"}, nil},
	{"gpu/webgl2", []string{"core/...", "gpu", "gpu/internal/..."}, nil},
	{"gpu/gl33", []string{"core/...", "gpu", "gpu/gl33/...", "gpu/internal/..."}, nil},
	{"gpu/gl33/gl", nil, nil},
	{"gpu/internal/...", []string{"core/..."}, nil},
	{"vfs", []string{"core/...", "vfs/osfs"}, nil},
	{"vfs/osfs", []string{"core/..."}, nil},
	{"vfs/exportfs", []string{"core/...", "vfs", "vfs/osfs"}, nil},
	{"runtime/bundle", []string{"core/...", "asset", "format/..."}, nil},
	{"runtime/player", []string{"core/...", "app/assembly", "app/assembly/builtin", "app/crash", "asset", "asset/loader",
		"contracts/...", "engine", "engine/...", "format/...", "modules/input", "modules/scripting/systems", "platform",
		"render", "render/host", "runtime/bundle", "save", "scene", "scene/frame", "scene/sceneio", "schema",
		"script/bind", "script/bytecode", "script/runtime", "script/vm", "sdk", "vfs", "vfs/..."}, nil},
	{"app/export", []string{"..."}, nil},
	{"app/export/...", []string{"core/...", "app/export/icon"}, nil},
	{"engine/recorder", []string{"core/..."}, nil},
	{"engine", []string{"core/...", "platform/...", "gpu/...", "vfs/...", "engine/res", "engine/recorder"}, nil},
	{"engine/res", []string{"core/...", "vfs"}, nil},
	{"app/bootstrap", []string{"core/...", "project"}, nil},
	{"render", []string{"core/...", "gpu", "vfs", "render/..."}, nil},
	{"render/host", []string{"core/...", "gpu", "engine", "render"}, nil},
	{"render/...", []string{"core/...", "gpu", "vfs", "render/..."}, nil},
	{"asset", []string{"core/...", "gpu", "format/...", "render/texture", "render/mesh"}, nil},
	{"asset/loader", []string{"core/...", "gpu", "vfs", "asset", "render/texture", "render/mesh"}, nil},
	{"asset/pipeline", []string{"core/...", "asset", "format/...", "project", "vfs", "render/texture"}, nil},
	{"asset/importers/...", []string{"core/...", "gpu", "asset", "asset/pipeline", "format/...", "render/texture", "render/mesh"}, nil},
	{"data", []string{"core/...", "asset", "format/aetext", "schema", "scene/sceneio", "scene/prefab"}, nil},
	{"save", []string{"core/...", "format/aetext", "scene", "platform"}, nil},
	{"project", []string{"core/...", "format/...", "platform/input"}, nil},
	{"scene", []string{"core/...", "schema", "asset", "render/..."}, nil},
	{"scene/scriptcomp", []string{"core/...", "schema", "scene", "format/..."}, nil},
	{"scene/frame", []string{"core/...", "schema", "asset", "gpu", "engine", "engine/recorder", "render", "render/...", "scene"}, nil},
	{"scene/sceneio", []string{"core/...", "schema", "asset", "format/...", "scene", "scene/scriptcomp", "scene/prefab"}, nil},
	{"scene/prefab", []string{"core/...", "schema"}, nil},
	{"scene/prefab/author", []string{"core/...", "schema", "scene/prefab"}, nil},
	{"modules/render2d", []string{"core/...", "asset", "engine", "engine/module", "gpu", "render/...", "scene", "scene/frame", "schema"}, nil},
	{"modules/render2d/editor", with(editorAllow, "modules/render2d"), nil},
	{"modules/render3d", []string{"core/...", "asset", "asset/pipeline", "asset/importers/obj", "engine", "engine/module", "gpu", "render", "render/...", "scene", "scene/frame", "schema"}, nil},
	{"modules/render3d/editor", with(editorAllow, "modules/render3d"), nil},
	{"ui", []string{"core/...", "gpu", "platform/input", "render", "render/...", "schema", "scene", "asset", "format/...", "ui/..."}, nil},
	{"ui/...", []string{"core/...", "gpu", "platform/input", "render", "render/...", "schema", "scene", "asset", "format/...", "ui/..."}, nil},
	{"editor/...", editorAllow, nil},
	{"editor/core/code/...", with(editorAllow, "script/token"), nil},
	{"editor/shell/...", with(editorAllow, "modules/..."), nil},
	{"editor/panels/scripteditor", with(editorAllow, "script/langsvc", "script/workspace", "script/source", "script/diag"), nil},
	{"app", []string{"..."}, nil},
	{"app/docstool", []string{"..."}, nil},
	{"app/edit", []string{"..."}, nil},
	{"app/projects", []string{"..."}, nil},
	{"app/supervise", []string{"..."}, nil},
	{"modules/physics", []string{"core/...", "engine", "engine/module", "engine/problems", "schema", "scene", "modules/physics/world", "modules/physics/components", "modules/physics/systems", "modules/physics/stdlib", "modules/physics/settings", "contracts/..."}, nil},
	{"modules/physics/world", []string{"core/..."}, nil},
	{"modules/physics/components", []string{"core/...", "schema", "scene"}, nil},
	{"modules/physics/systems", []string{"core/...", "engine", "scene", "schema", "modules/physics/world", "modules/physics/components", "contracts/..."}, nil},
	{"modules/physics/stdlib", []string{"core/...", "engine", "engine/module", "scene", "schema", "script/bind", "script/abi", "script/vm", "contracts/...", "modules/physics/world", "modules/physics/components", "modules/physics/systems"}, nil},
	{"modules/physics/settings", []string{"core/...", "engine/module", "format/...", "modules/physics/world"}, nil},
	{"modules/physics/editor", with(editorAllow, "modules/physics", "modules/physics/components", "modules/physics/systems", "modules/physics/world", "modules/physics/settings", "contracts/..."), nil},
	{"modules/input", []string{"core/...", "engine", "engine/module", "engine/recorder", "engine/settings", "format/...", "platform/input", "scene", "scene/frame", "schema", "script/bind", "script/abi", "script/vm", "contracts/inputactions"}, nil},
	{"contracts/inputactions", []string{"core/..."}, nil},
	{"modules/input/editor", with(editorAllow, "modules/input"), nil},
	{"modules/debug", []string{"core/...", "engine", "engine/module", "scene", "scene/frame", "schema", "render", "render/...", "script/bind", "script/vm", "contracts/..."}, nil},
	{"modules/debug/editor", with(editorAllow, "modules/debug", "contracts/..."), nil},
	{"contracts/scriptevents", []string{"core/...", "scene", "script/abi", "script/vm"}, nil},
	{"contracts/scriptfields", []string{"core/...", "scene", "script/abi", "script/vm"}, nil},
	{"contracts/scriptwait", []string{"core/..."}, nil},
	{"contracts/signals", []string{"core/...", "scene", "script/vm"}, nil},
	{"contracts/scriptpool", []string{"core/...", "scene"}, nil},
	{"contracts/persistence", []string{"core/...", "engine", "format/aetext", "save", "scene", "script/abi", "script/vm"}, nil},
	{"contracts/physics2d", []string{"core/...", "scene"}, nil},
	{"contracts/debugdraw", []string{"core/...", "scene"}, nil},
	{"engine/problems", []string{"core/...", "scene"}, nil},
	{"engine/settings", []string{"core/...", "engine/problems", "format/...", "schema"}, nil},
	{"engine/settings/settingsui", []string{"core/...", "engine/settings", "platform/input", "schema", "ui/..."}, nil},
	{"engine/module", []string{"core/...", "engine", "engine/res", "engine/problems", "engine/settings", "format/...", "schema", "scene", "asset", "script/abi", "script/bind", "contracts/scriptevents", "contracts/signals", "contracts/persistence", "vfs"}, nil},
	{"engine/base", []string{"core/...", "engine", "engine/module", "schema", "scene", "scene/...", "engine/problems"}, nil},
	{"engine/docs", []string{"core/...", "schema", "engine/module", "engine/res", "script/abi", "script/bind", "script/bind/manifest", "script/diag"}, nil},
	{"sdk", []string{"core/...", "engine", "engine/module", "engine/problems", "engine/settings", "schema", "scene", "asset", "asset/...", "render", "render/host", "modules/...", "platform/input", "script/bind", "contracts/...", "script/vm", "data"}, nil},
	{"app/crash", []string{"core/...", "engine", "engine/recorder", "format/..."}, nil},
	{"app/run", []string{"..."}, nil},
	{"app/playground", []string{"core/...", "asset", "asset/loader", "engine", "engine/base", "engine/module", "engine/recorder",
		"engine/docs", "format/...", "app/assembly", "app/assembly/playground", "modules/scripting/systems", "modules/render2d",
		"platform", "platform/input", "render", "render/...", "scene", "scene/...", "schema", "script/...", "vfs"}, nil},
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
	{"script/runtime", with(scriptRuntime, "script/bind", "script/coroutine", "script/signal", "scene", "scene/...", "engine", "engine/recorder", "asset", "asset/...", "platform/input", "contracts/scriptevents", "contracts/scriptfields", "contracts/signals", "contracts/persistence"), nil},
	{"script/reload", with(scriptRuntime, "script/runtime"), nil},
	{"modules/scripting/stdlib", with(scriptRuntime, "script/bind", "script/stdlib", "scene", "scene/...", "engine", "asset", "platform/input", "contracts/physics2d"), nil},
	{"modules/scripting/systems", with(scriptRuntime, "script/bind", "script/runtime", "script/stdlib", "modules/scripting/stdlib", "engine", "scene", "contracts/scriptevents", "contracts/scriptfields", "contracts/scriptwait", "contracts/signals", "script/signal", "contracts/scriptpool", "contracts/persistence"), nil},
	{"modules/scripting", with(scriptRuntime, "engine", "engine/module", "scene", "scene/scriptcomp", "script/bind", "modules/scripting/systems", "modules/scripting/stdlib", "script/stdlib", "asset/pipeline", "asset/importers/script", "script/workspace", "contracts/scriptevents", "contracts/scriptfields", "contracts/scriptwait", "contracts/signals", "contracts/scriptpool", "contracts/persistence"), nil},
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
	"save", // slot files next to the player profile (D262)
	"asset/pipeline",
	"app/bootstrap",
	"app",
	"app/docstool",
	"app/run",
	"app/crash",
	"app/edit",
	"app/projects",
	"app/supervise",
	"runtime/player",
	"app/export",
	"app/export/...",
	"editor/core/assetops",
	"editor/core/prefabs",
	"editor/core/dataedit",
	"editor/core/preview",
	"editor/core/vcs",
	"editor/core/capture",
	"editor/core/refs",
	"editor/panels/assets",
	"editor/core/document",
	"editor/shell/launcher",
	"editor/panels/scripteditor",
	"editor/core/session",
	"editor/core/state",
	"editor/panels/welcome",
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
	{"engine/module", []string{"app/...", "engine/base"}},
	{"modules/render2d", []string{"modules/render3d", "modules/render3d/...", "script/...", "editor/...", "modules/render2d/editor"}},
	{"modules/render3d", []string{"modules/render2d", "modules/render2d/...", "script/...", "editor/...", "modules/render3d/editor"}},
	{"modules/scripting", []string{"modules/render2d", "modules/render2d/...", "modules/render3d", "modules/render3d/...", "editor/...", "modules/physics", "modules/physics/..."}},
	{"sdk", []string{"app/...", "engine/base"}},
	{"editor/...", []string{"app/...", "engine/base", "modules/scripting"}},
	{"app/run", []string{"editor/..."}},
	{"app/projects", []string{"editor/..."}},
	{"engine/base", []string{"modules/render3d", "modules/render3d/...", "asset/pipeline", "asset/importers/...", "editor/..."}},
	{"app/playground", []string{"platform/sdl", "gpu/gl33", "gpu/gl33/...", "asset/pipeline", "asset/importers/...", "modules/render3d", "modules/render3d/...", "editor/..."}},
	{"runtime/...", []string{"editor/..."}},
	{"runtime/...", []string{"asset/pipeline", "asset/importers/...", "project", "app/run", "app/edit", "app/export", "app/export/..."}},
	{"sdk", []string{"editor/..."}},
	{"modules/physics/world", []string{"scene", "scene/...", "schema", "render", "render/...", "engine", "engine/...", "script/...", "contracts/..."}},
	{"modules/physics/...", []string{"modules/render2d", "modules/render2d/...", "modules/render3d", "modules/render3d/...", "app/...", "asset/pipeline", "asset/importers/..."}},
	{"modules/physics/components", []string{"script/...", "engine", "engine/..."}},
	{"modules/physics/systems", []string{"script/...", "editor/..."}},
	{"modules/physics", []string{"modules/physics/editor", "project"}},
	{"modules/physics/systems", []string{"modules/physics/editor", "project"}},
	{"modules/physics/settings", []string{"project", "editor/...", "script/..."}},
	{"modules/physics/stdlib", []string{"script/runtime", "script/compiler", "script/checker", "editor/...", "render", "render/..."}},
	{"modules/input", []string{"editor/...", "modules/input/editor", "project", "app/..."}},
	{"contracts/...", []string{"modules/physics/...", "engine", "engine/...", "editor/...", "render", "render/..."}},
	{"script/bytecode", scriptCompilerOnly},
	{"script/verify", scriptCompilerOnly},
	{"script/vm", scriptCompilerOnly},
	{"script/bind", scriptCompilerOnly},
	{"script/coroutine", scriptCompilerOnly},
	{"script/signal", scriptCompilerOnly},
	{"script/stdlib", scriptCompilerOnly},
	{"script/runtime", scriptCompilerOnly},
	{"modules/scripting/systems", scriptCompilerOnly},
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
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution|parser.ParseComments)
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
		if p == "syscall/js" && !jsOnly(f, file) {
			out = append(out, fmt.Sprintf("%s: syscall/js in a file that also builds outside js", file))
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

// layerRule returns the layer rule an import from -> to breaks, or "".
func layerRule(from, to string) string {
	r, ok := ruleFor(from)
	if ok && !allowed(r.allow, to) {
		return "layer:" + r.pkg
	}
	for _, d := range denied {
		if match(d.pkg, from) && allowed(d.deny, to) {
			return "denied:" + d.pkg
		}
	}
	return ""
}

func externalRule(from, imp string) string {
	r, ok := ruleFor(from)
	if ok && !allowed(r.external, imp) {
		return "layer:" + r.pkg
	}
	return ""
}

func jsOnly(f *ast.File, file string) bool {
	base := strings.TrimSuffix(path.Base(file), ".go")
	if strings.HasSuffix(base, "_js") || strings.HasSuffix(base, "_js_wasm") {
		return true
	}
	for _, cg := range f.Comments {
		if cg.Pos() > f.Package {
			break
		}
		for _, c := range cg.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}
			x, err := constraint.Parse(c.Text)
			if err != nil {
				return false
			}
			others := func(v bool) func(string) bool {
				return func(tag string) bool { return tag != "js" && tag != "wasm" && v }
			}
			return !x.Eval(others(true)) && !x.Eval(others(false))
		}
	}
	return false
}
