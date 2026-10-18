package resolver

import (
	"path"
	"sort"
	"strings"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/source"
	"aego/script/types"
)

type ObjKind uint8

const (
	ObjBad ObjKind = iota
	ObjConst
	ObjType
	ObjFunc
	ObjScript
	ObjPkg
	ObjVar
	ObjParam
	ObjField
	ObjBuiltin
	ObjNode
	ObjGeneric
	ObjSignal
)

type Object struct {
	Kind   ObjKind
	Name   string
	Pkg    string
	Ident  *ast.Ident
	Decl   ast.Node
	File   *ast.File
	Type   *types.Type
	Const  *types.Const
	Target string
	Host   bool
	Index  int
	Used   bool
	Data   any
}

func (o *Object) Exported() bool {
	return abi.Exported(o.Name)
}

func (o *Object) Span() source.Span {
	if o.Ident != nil {
		return o.Ident.Sp
	}
	if o.Decl != nil {
		return o.Decl.Span()
	}
	return source.Span{}
}

type Scope struct {
	Parent *Scope
	objs   map[string]*Object
	order  []*Object
}

func NewScope(parent *Scope) *Scope {
	return &Scope{Parent: parent, objs: map[string]*Object{}}
}

func (s *Scope) Insert(o *Object) *Object {
	if prev, ok := s.objs[o.Name]; ok {
		return prev
	}
	s.objs[o.Name] = o
	s.order = append(s.order, o)
	return nil
}

func (s *Scope) LookupLocal(name string) *Object {
	return s.objs[name]
}

func (s *Scope) Lookup(name string) *Object {
	for sc := s; sc != nil; sc = sc.Parent {
		if o, ok := sc.objs[name]; ok {
			return o
		}
	}
	return nil
}

func (s *Scope) Objects() []*Object {
	return s.order
}

func Universe() *Scope {
	s := NewScope(nil)
	for name, t := range map[string]*types.Type{
		"bool": types.Bool, "int": types.Int, "float": types.Float, "string": types.String,
		"Vec2": types.Vec2, "Vec3": types.Vec3, "Color": types.Color, "Quat": types.Quat, "Rect": types.Rect,
		"Transform2D": types.Transform2D, "Transform3D": types.Transform3D, "NodeRef": types.NodeRef,
		"Coroutine": types.Coroutine, "Connection": types.Connection,
	} {
		s.Insert(&Object{Kind: ObjType, Name: name, Type: t})
	}
	s.Insert(&Object{Kind: ObjGeneric, Name: "AssetRef"})
	s.Insert(&Object{Kind: ObjGeneric, Name: "TableRef"})
	s.Insert(&Object{Kind: ObjGeneric, Name: "ComponentRef"})
	s.Insert(&Object{Kind: ObjGeneric, Name: "ScriptRef"})
	s.Insert(&Object{Kind: ObjBuiltin, Name: "len"})
	s.Insert(&Object{Kind: ObjBuiltin, Name: "append"})
	return s
}

type Package struct {
	Path       string
	Name       string
	Files      []*ast.File
	Scope      *Scope
	FileScopes map[*ast.File]*Scope
	Deps       []string
	Cyclic     bool
	importAt   map[string]source.Span
}

type Table struct {
	Universe *Scope
	Packages map[string]*Package
	Order    []string
}

type resolver struct {
	tab     *Table
	surface abi.Surface
	diags   []diag.Diagnostic
}

func pkgSpan(files []*ast.File) source.Span {
	if len(files) == 0 {
		return source.Span{}
	}
	if files[0].Package != nil {
		return files[0].Package.Sp
	}
	return files[0].Sp
}

func Resolve(sm *source.Manager, files map[string][]*ast.File, surface abi.Surface) (*Table, []diag.Diagnostic) {
	r := &resolver{tab: &Table{Universe: Universe(), Packages: map[string]*Package{}}, surface: surface}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	// Object order follows file order, so sort files by path.
	for _, fs := range files {
		sort.Slice(fs, func(i, j int) bool { return sm.Path(fs[i].ID) < sm.Path(fs[j].ID) })
	}
	for _, p := range paths {
		r.declare(p, files[p])
		if surface != nil && surface.Module(p) {
			r.errorf(diag.ModuleConflict, pkgSpan(files[p]),
				"%s is both a script directory under scripts/ and an engine or native module; rename the script directory", p)
		}
	}
	for _, p := range paths {
		r.imports(r.tab.Packages[p])
	}
	r.order(paths)
	return r.tab, r.diags
}

func (r *resolver) errorf(code string, sp source.Span, format string, args ...any) {
	r.diags = append(r.diags, diag.Errorf(code, sp, format, args...))
}

func (r *resolver) declare(p string, files []*ast.File) {
	pkg := &Package{Path: p, Files: files, Scope: NewScope(r.tab.Universe), FileScopes: map[*ast.File]*Scope{}, importAt: map[string]source.Span{}}
	r.tab.Packages[p] = pkg
	for i, f := range files {
		if i == 0 && (p == "aego" || strings.HasPrefix(p, "aego/")) {
			r.errorf(diag.ReservedPackage, f.Sp, "package path %s is reserved for the engine", p)
		}
		if f.Package != nil {
			switch {
			case pkg.Name == "":
				pkg.Name = f.Package.Name
			case f.Package.Name != pkg.Name:
				r.errorf(diag.PackageName, f.Package.Sp, "package %s, expected %s like the other files in %s", f.Package.Name, pkg.Name, p)
			}
		}
		for _, d := range f.Decls {
			r.declareDecl(pkg, f, d)
		}
	}
	if pkg.Name != "" && pkg.Name != path.Base(p) {
		sp := files[0].Sp
		if files[0].Package != nil {
			sp = files[0].Package.Sp
		}
		r.errorf(diag.PackageName, sp, "package %s must be named %s after its directory", pkg.Name, path.Base(p))
	}
}

func (r *resolver) declareDecl(pkg *Package, f *ast.File, d ast.Decl) {
	var obj *Object
	switch d := d.(type) {
	case *ast.ConstDecl:
		obj = &Object{Kind: ObjConst, Ident: d.Name}
	case *ast.StructDecl:
		obj = &Object{Kind: ObjType, Ident: d.Name, Type: types.NewStruct(pkg.Path, d.Name.Name)}
	case *ast.EnumDecl:
		names := make([]string, 0, len(d.Members))
		seen := map[string]bool{}
		for _, m := range d.Members {
			if seen[m.Name] {
				r.errorf(diag.DuplicateDecl, m.Sp, "enum member %s is declared twice", m.Name)
				continue
			}
			seen[m.Name] = true
			names = append(names, m.Name)
		}
		obj = &Object{Kind: ObjType, Ident: d.Name, Type: types.NewEnum(pkg.Path, d.Name.Name, names)}
	case *ast.FuncDecl:
		obj = &Object{Kind: ObjFunc, Ident: d.Name}
	case *ast.ScriptDecl:
		obj = &Object{Kind: ObjScript, Ident: d.Name}
	default:
		return
	}
	if obj.Ident.Name == "_" {
		return
	}
	obj.Name, obj.Pkg, obj.Decl, obj.File = obj.Ident.Name, pkg.Path, d, f
	if prev := pkg.Scope.Insert(obj); prev != nil {
		r.diags = append(r.diags, diag.Errorf(diag.DuplicateDecl, obj.Ident.Sp, "%s redeclared in package %s", obj.Name, pkg.Path).
			WithNote(prev.Span(), "previous declaration"))
	}
}

func (r *resolver) imports(pkg *Package) {
	deps := map[string]bool{}
	for _, f := range pkg.Files {
		fs := NewScope(pkg.Scope)
		pkg.FileScopes[f] = fs
		for _, imp := range f.Imports {
			obj := &Object{Kind: ObjPkg, Name: path.Base(imp.Path), Pkg: pkg.Path, Decl: imp, File: f}
			switch {
			case imp.Path == pkg.Path:
				r.errorf(diag.SelfImport, imp.Sp, "package %s imports itself", pkg.Path)
				obj.Used = true
			case r.tab.Packages[imp.Path] != nil:
				obj.Target = imp.Path
				deps[imp.Path] = true
				if _, ok := pkg.importAt[imp.Path]; !ok {
					pkg.importAt[imp.Path] = imp.Sp
				}
			case r.surface != nil && r.surface.Module(imp.Path):
				obj.Target, obj.Host = imp.Path, true
			default:
				r.errorf(diag.UnknownImport, imp.Sp, "cannot find package %q", imp.Path)
				obj.Used = true
			}
			if prev := fs.Insert(obj); prev != nil {
				r.diags = append(r.diags, diag.Errorf(diag.DuplicateImport, imp.Sp, "%s is imported twice", obj.Name).
					WithNote(prev.Span(), "previous import"))
				continue
			}
			if d := pkg.Scope.LookupLocal(obj.Name); d != nil {
				r.diags = append(r.diags, diag.Errorf(diag.ImportConflict, imp.Sp, "%s is both an import and a declaration in %s", obj.Name, pkg.Path).
					WithNote(d.Span(), "declared here"))
			}
		}
	}
	for d := range deps {
		pkg.Deps = append(pkg.Deps, d)
	}
	sort.Strings(pkg.Deps)
}

func (r *resolver) order(paths []string) {
	state := map[string]uint8{}
	var stack []string
	var visit func(p string)
	visit = func(p string) {
		if state[p] != 0 {
			return
		}
		state[p] = 1
		stack = append(stack, p)
		pkg := r.tab.Packages[p]
		for _, d := range pkg.Deps {
			if state[d] == 1 {
				start := 0
				for i, s := range stack {
					if s == d {
						start = i
					}
				}
				chain := append(append([]string(nil), stack[start:]...), d)
				r.errorf(diag.ImportCycle, pkg.importAt[d], "import cycle: %s", strings.Join(chain, " → "))
				// Mark the whole cycle so the checker skips its bodies.
				for _, cp := range chain {
					if pk := r.tab.Packages[cp]; pk != nil {
						pk.Cyclic = true
					}
				}
				continue
			}
			visit(d)
		}
		stack = stack[:len(stack)-1]
		state[p] = 2
		r.tab.Order = append(r.tab.Order, p)
	}
	for _, p := range paths {
		visit(p)
	}
}
