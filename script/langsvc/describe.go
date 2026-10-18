package langsvc

import (
	"strconv"
	"strings"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/bind/manifest"
	"aego/script/checker"
	"aego/script/resolver"
	"aego/script/types"
)

func typeString(t *types.Type) string {
	if t == nil {
		return "invalid"
	}
	return t.String()
}

func funcSig(f *checker.Func) string {
	var b strings.Builder
	b.WriteString("func ")
	if f.Script != nil {
		b.WriteString("(" + f.Script.Name + ") ")
	}
	b.WriteString(f.Name + "(")
	for i, p := range f.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name + " " + typeString(p.Type))
	}
	b.WriteByte(')')
	if f.Result != nil && f.Result != types.Void {
		b.WriteString(" " + f.Result.String())
	}
	if f.Decl != nil && f.Decl.Coroutine {
		b.WriteString(" coroutine")
	}
	return b.String()
}

func typeDecl(t *types.Type) string {
	switch t.Kind {
	case types.KStruct:
		parts := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			parts[i] = f.Name + " " + typeString(f.Type)
		}
		return "type " + t.Local + " struct { " + strings.Join(parts, "; ") + " }"
	case types.KEnum:
		return "enum " + t.Local + " { " + strings.Join(t.Members, ", ") + " }"
	}
	return "type " + t.Name
}

func scriptField(f *checker.ScriptField) string {
	s := "var " + f.Name + " " + typeString(f.Type)
	if f.Default != nil {
		s += " = " + f.Default.String()
	}
	if f.Export {
		s = "@export " + s
	}
	return s
}

func describe(o *resolver.Object) string {
	t := typeString(o.Type)
	switch o.Kind {
	case resolver.ObjFunc:
		if f, ok := o.Data.(*checker.Func); ok {
			return funcSig(f)
		}
	case resolver.ObjSignal:
		if sg, ok := o.Data.(*checker.ScriptSignal); ok {
			return signalDecl(sg)
		}
	case resolver.ObjVar:
		return "var " + o.Name + " " + t
	case resolver.ObjParam:
		return "param " + o.Name + " " + t
	case resolver.ObjField:
		if f, ok := o.Data.(*checker.ScriptField); ok {
			return scriptField(f)
		}
		return "field " + o.Name + " " + t
	case resolver.ObjConst:
		s := "const " + o.Name + " " + t
		if o.Const != nil {
			s += " = " + o.Const.String()
		}
		return s
	case resolver.ObjType:
		if o.Type != nil {
			return typeDecl(o.Type)
		}
	case resolver.ObjScript:
		if st, ok := o.Data.(*checker.ScriptType); ok {
			return "script " + st.FullName
		}
		return "script " + o.Name
	case resolver.ObjPkg:
		return "import " + strconv.Quote(o.Target)
	case resolver.ObjNode:
		return "var node NodeRef"
	case resolver.ObjBuiltin:
		if o.Name == "len" {
			return "func len(v string | []T) int"
		}
		return "func append(s []T, v ...T) []T"
	case resolver.ObjGeneric:
		return "type " + o.Name + "<T>"
	}
	return o.Name
}

func docOf(o *resolver.Object) string {
	switch d := o.Decl.(type) {
	case *ast.ConstDecl:
		return d.Doc
	case *ast.StructDecl:
		return d.Doc
	case *ast.EnumDecl:
		return d.Doc
	case *ast.FuncDecl:
		return d.Doc
	case *ast.ScriptDecl:
		return d.Doc
	case *ast.ScriptVar:
		return d.Doc
	case *ast.Field:
		return d.Doc
	case *ast.SignalDecl:
		return d.Doc
	}
	return ""
}

func constValue(c abi.Const) string {
	switch c.Type.Kind {
	case abi.Bool:
		return strconv.FormatBool(c.Bool)
	case abi.Float:
		return strconv.FormatFloat(c.Float, 'g', -1, 64)
	case abi.String:
		return strconv.Quote(c.Str)
	}
	return strconv.FormatInt(c.Int, 10)
}

func funcDoc(m *manifest.Manifest, module, name string) manifest.Func {
	f, _ := m.FuncDoc(module, name)
	return f
}

func methodDoc(m *manifest.Manifest, recv abi.Kind, name string) manifest.Method {
	me, _ := m.MethodDoc(recv, name)
	return me
}

func assetDisplay(kind string) string {
	if kind == "" {
		return kind
	}
	return strings.ToUpper(kind[:1]) + kind[1:]
}

func abiType(t abi.TypeRef) string {
	switch t.Kind {
	case abi.Enum, abi.Struct, abi.Handle:
		return t.Name[strings.LastIndexByte(t.Name, '/')+1:]
	case abi.AssetRef:
		return "AssetRef<" + assetDisplay(t.Name) + ">"
	case abi.ComponentRef:
		return "ComponentRef<" + t.Name[strings.LastIndexByte(t.Name, '.')+1:] + ">"
	case abi.Slice:
		if t.Elem != nil {
			return "[]" + abiType(*t.Elem)
		}
	}
	return t.String()
}

func hostSig(prefix string, s abi.Signature, names []string) string {
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteByte('(')
	for i, p := range s.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		if i < len(names) {
			b.WriteString(names[i] + " ")
		}
		b.WriteString(abiType(p))
	}
	b.WriteByte(')')
	if s.Result.Kind != abi.Void {
		b.WriteString(" " + abiType(s.Result))
	}
	return b.String()
}

func hostDoc(doc, example string) string {
	if example == "" {
		return doc
	}
	ex := "Example:\n  " + strings.ReplaceAll(example, "\n", "\n  ")
	if doc == "" {
		return ex
	}
	return doc + "\n\n" + ex
}

func componentDoc(m *manifest.Manifest, name string) string {
	if c, ok := m.Component(name); ok {
		return c.Doc
	}
	return ""
}

func fieldDoc(m *manifest.Manifest, comp, field string) string {
	if c, ok := m.Component(comp); ok {
		if f, ok := c.Field(field); ok {
			return f.Doc
		}
	}
	return ""
}
