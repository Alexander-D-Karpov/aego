package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	pkgPath     = "aego/engine/i18n"
	commandPath = "aego/editor/core/command"
)

// Key is one translatable text found in the code.
type Key struct {
	ID     string
	En     string
	One    string
	Plural bool
	Pos    string
}

var skipDirs = map[string]bool{".cache": true, ".git": true, "testdata": true, "node_modules": true, "internal": true, "site": true}

type file struct {
	path string
	dir  string
	ast  *ast.File
}

// Scan collects every i18n.Tr, TrN, T and Note call and command.Desc/Info literal; a key or text that is not a literal is an error.
func Scan(root string) (map[string]*Key, []error) {
	keys := map[string]*Key{}
	var errs []error
	fset := token.NewFileSet()
	var files []file
	consts := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && (skipDirs[d.Name()] || hasMod(p)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		dir, _ := filepath.Rel(root, filepath.Dir(p))
		dir = filepath.ToSlash(dir)
		collectConsts(f, dir, consts)
		files = append(files, file{path: p, dir: dir, ast: f})
		return nil
	})
	add := func(k *Key) {
		if !validKey(k.ID) {
			errs = append(errs, fmt.Errorf("%s: key %q is not editor.*, launcher.*, settings.*, cli.*, schema.*, enum.* or i18n.<module>.*", k.Pos, k.ID))
			return
		}
		if old, ok := keys[k.ID]; ok {
			if old.En != k.En || old.One != k.One || old.Plural != k.Plural {
				errs = append(errs, fmt.Errorf("%s: key %q has other English text than at %s: %q vs %q", k.Pos, k.ID, old.Pos, k.En, old.En))
			}
			return
		}
		keys[k.ID] = k
	}
	for _, fl := range files {
		f := fl.ast
		if cmd := importAs(f, commandPath); cmd != "" || fl.dir == "editor/core/command" {
			commandKeys(f, fl.dir, cmd, consts, func(pos token.Pos, id, field, en string) {
				at := fset.Position(pos)
				rel, _ := filepath.Rel(root, at.Filename)
				key := "editor.command." + id
				if field == "Tip" {
					key += ".tip"
				}
				add(&Key{ID: key, En: en, Pos: fmt.Sprintf("%s:%d", filepath.ToSlash(rel), at.Line)})
			})
		}
		widgetTr(f, fl.dir, func(pos token.Pos, id, en string, ok bool) {
			at := fset.Position(pos)
			rel, _ := filepath.Rel(root, at.Filename)
			where := fmt.Sprintf("%s:%d", filepath.ToSlash(rel), at.Line)
			if !ok {
				errs = append(errs, fmt.Errorf("%s: widgets.Tr needs a literal key and English text", where))
				return
			}
			add(&Key{ID: id, En: en, Pos: where})
		})
		name := importAs(f, pkgPath)
		if name == "" || fl.dir == "engine/i18n" {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != name {
				return true
			}
			pos := fset.Position(call.Pos())
			rel, _ := filepath.Rel(root, pos.Filename)
			where := fmt.Sprintf("%s:%d", filepath.ToSlash(rel), pos.Line)
			var k *Key
			switch sel.Sel.Name {
			case "Tr", "T", "Note":
				if len(call.Args) < 2 {
					return true
				}
				id, ok1 := lit(call.Args[0])
				en, ok2 := lit(call.Args[1])
				if !ok1 || !ok2 {
					errs = append(errs, fmt.Errorf("%s: %s.%s needs a literal key and English text", where, name, sel.Sel.Name))
					return true
				}
				k = &Key{ID: id, En: en, Pos: where}
			case "TrN":
				if len(call.Args) < 4 {
					return true
				}
				id, ok1 := lit(call.Args[0])
				one, ok2 := lit(call.Args[2])
				other, ok3 := lit(call.Args[3])
				if !ok1 || !ok2 || !ok3 {
					errs = append(errs, fmt.Errorf("%s: %s.TrN needs a literal key and English forms", where, name))
					return true
				}
				k = &Key{ID: id, En: other, One: one, Plural: true, Pos: where}
			default:
				return true
			}
			add(k)
			return true
		})
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
	return keys, errs
}

func hasMod(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}

func importAs(f *ast.File, path string) string {
	for _, im := range f.Imports {
		if p, _ := strconv.Unquote(im.Path.Value); p == path {
			if im.Name != nil {
				return im.Name.Name
			}
			return path[strings.LastIndexByte(path, '/')+1:]
		}
	}
	return ""
}

// collectConsts records the string constants of a file as dir.Name.
func collectConsts(f *ast.File, dir string, out map[string]string) {
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			vs := sp.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if i < len(vs.Values) {
					if s, ok := lit(vs.Values[i]); ok {
						out[dir+"."+n.Name] = s
					}
				}
			}
		}
	}
}

// resolve is the value of a literal, a constant of the file's package or a qualified constant of another package.
func resolve(e ast.Expr, f *ast.File, dir string, consts map[string]string) (string, bool) {
	if s, ok := lit(e); ok {
		return s, true
	}
	switch v := e.(type) {
	case *ast.Ident:
		s, ok := consts[dir+"."+v.Name]
		return s, ok
	case *ast.SelectorExpr:
		x, ok := v.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			name := p[strings.LastIndexByte(p, '/')+1:]
			if im.Name != nil {
				name = im.Name.Name
			}
			if name == x.Name && strings.HasPrefix(p, "aego/") {
				s, ok := consts[strings.TrimPrefix(p, "aego/")+"."+v.Sel.Name]
				return s, ok
			}
		}
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			a, ok1 := resolve(v.X, f, dir, consts)
			b, ok2 := resolve(v.Y, f, dir, consts)
			return a + b, ok1 && ok2
		}
	}
	return "", false
}

// commandKeys reports the Name and Tip of every command.Desc and command.Info literal whose ID resolves.
func commandKeys(f *ast.File, dir, cmd string, consts map[string]string, emit func(pos token.Pos, id, field, en string)) {
	isCmd := func(t ast.Expr) bool {
		switch v := t.(type) {
		case *ast.SelectorExpr:
			x, ok := v.X.(*ast.Ident)
			return ok && x.Name == cmd && (v.Sel.Name == "Desc" || v.Sel.Name == "Info")
		case *ast.Ident:
			return dir == "editor/core/command" && (v.Name == "Desc" || v.Name == "Info")
		case *ast.StarExpr:
			return false
		}
		return false
	}
	visit := func(cl *ast.CompositeLit) {
		var id string
		texts := map[string]*ast.BasicLit{}
		for _, el := range cl.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			k, _ := kv.Key.(*ast.Ident)
			if k == nil {
				continue
			}
			switch k.Name {
			case "ID":
				id, _ = resolve(kv.Value, f, dir, consts)
			case "Name", "Tip":
				if b, ok := kv.Value.(*ast.BasicLit); ok {
					texts[k.Name] = b
				}
			}
		}
		if id == "" {
			return
		}
		for field, b := range texts {
			if s, ok := lit(b); ok && s != "" {
				emit(b.Pos(), id, field, s)
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if isCmd(cl.Type) {
			visit(cl)
			return true
		}
		var elt ast.Expr
		switch t := cl.Type.(type) {
		case *ast.ArrayType:
			elt = t.Elt
		case *ast.MapType:
			elt = t.Value
		}
		if elt != nil && isCmd(elt) {
			for _, e := range cl.Elts {
				if kv, ok := e.(*ast.KeyValueExpr); ok {
					e = kv.Value
				}
				if c, ok := e.(*ast.CompositeLit); ok && c.Type == nil {
					visit(c)
				}
			}
		}
		return true
	})
}

func lit(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		a, ok1 := lit(v.X)
		b, ok2 := lit(v.Y)
		return a + b, ok1 && ok2
	case *ast.ParenExpr:
		return lit(v.X)
	}
	return "", false
}

var prefixes = []string{"editor.", "launcher.", "settings.", "cli.", "schema.", "enum.", "i18n."}

func validKey(k string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(k, p) && len(k) > len(p) && !strings.HasSuffix(k, ".") && !strings.Contains(k, " ") {
			return true
		}
	}
	return false
}

// widgetTr reports the widgets.Tr calls of a file, and the bare Tr calls inside ui/widgets.
func widgetTr(f *ast.File, dir string, emit func(pos token.Pos, id, en string, ok bool)) {
	w := importAs(f, "aego/ui/widgets")
	inside := dir == "ui/widgets"
	if w == "" && !inside {
		return
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if !inside || fn.Name != "Tr" {
				return true
			}
		case *ast.SelectorExpr:
			x, ok := fn.X.(*ast.Ident)
			if !ok || w == "" || x.Name != w || fn.Sel.Name != "Tr" {
				return true
			}
		default:
			return true
		}
		id, ok1 := lit(call.Args[0])
		en, ok2 := lit(call.Args[1])
		emit(call.Pos(), id, en, ok1 && ok2)
		return true
	})
}
