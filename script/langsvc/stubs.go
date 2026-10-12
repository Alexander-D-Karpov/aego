package langsvc

import (
	"sort"
	"strings"

	"aego/script/workspace"
)

const (
	StubPrefix     = "stub:"
	StubBuiltin    = "aego/builtin"
	StubComponents = "engine/components"
)

func StubPath(module string) string {
	return StubPrefix + module + ".aes"
}

type stubWriter struct {
	b     strings.Builder
	line  int
	lines map[string]int
	locs  map[string]string
}

func newStubWriter(module, pkg string) *stubWriter {
	w := &stubWriter{lines: map[string]int{}, locs: map[string]string{}}
	w.write("// Library stub for " + module + ", generated from the engine bindings. Read-only.")
	w.write("// Ctrl+Click a declaration to open its Go source when it is known.")
	w.lines[""] = w.line
	w.write("package " + pkg)
	w.write("")
	return w
}

func (w *stubWriter) write(s string) {
	w.b.WriteString(s)
	w.b.WriteByte('\n')
	w.line += strings.Count(s, "\n") + 1
}

func (w *stubWriter) comment(prefix, text string) {
	if text == "" {
		return
	}
	for _, l := range strings.Split(text, "\n") {
		if l == "" {
			w.write(prefix + "//")
			continue
		}
		w.write(prefix + "// " + l)
	}
}

func (w *stubWriter) symbol(key, decl, doc, example, loc string) {
	w.comment("", doc)
	if example != "" {
		if doc != "" {
			w.write("//")
		}
		w.write("// Example:")
		for _, l := range strings.Split(example, "\n") {
			w.write("//   " + l)
		}
	}
	w.lines[key] = w.line
	if loc != "" {
		w.locs[key] = loc
	}
	w.write(decl)
	w.write("")
}

func (w *stubWriter) stub() *Stub {
	return &Stub{Text: w.b.String(), Lines: w.lines, Locs: w.locs}
}

func (s *Service) Stub(path string) (*Stub, bool) {
	snap := s.Latest()
	if snap == nil {
		return nil, false
	}
	s.smu.Lock()
	defer s.smu.Unlock()
	if s.stubTab != snap.Bindings || s.stubDocs != snap.Docs {
		s.stubs = map[string]*Stub{}
		s.stubTab, s.stubDocs = snap.Bindings, snap.Docs
	}
	if st, ok := s.stubs[path]; ok {
		return st, true
	}
	st, ok := StubFor(snap, path)
	if !ok {
		return nil, false
	}
	s.stubs[path] = st
	return st, true
}

func StubFor(snap *workspace.Snapshot, path string) (*Stub, bool) {
	module := strings.TrimSuffix(strings.TrimPrefix(path, StubPrefix), ".aes")
	var st *Stub
	switch {
	case module == StubBuiltin:
		st = builtinStub(snap)
	case module == StubComponents:
		st = componentStub(snap)
	case snap.Bindings.Module(module):
		st = moduleStub(snap, module)
	default:
		return nil, false
	}
	st.Path = path
	return st, true
}

func StubPaths(snap *workspace.Snapshot) []string {
	tab := snap.Bindings
	seen := map[string]bool{}
	var mods []string
	add := func(m string) {
		if !seen[m] {
			seen[m] = true
			mods = append(mods, m)
		}
	}
	for _, f := range tab.FuncList() {
		add(f.Module)
	}
	for _, e := range tab.EnumList() {
		add(e.Module)
	}
	for _, k := range tab.ConstList() {
		add(k.Module)
	}
	sort.Strings(mods)
	out := []string{StubPath(StubBuiltin), StubPath(StubComponents)}
	for _, m := range mods {
		out = append(out, StubPath(m))
	}
	return out
}

func moduleStub(snap *workspace.Snapshot, module string) *Stub {
	tab := snap.Bindings
	w := newStubWriter(module, module[strings.LastIndexByte(module, '/')+1:])
	for _, fn := range tab.FuncList() {
		if fn.Module != module {
			continue
		}
		f := funcDoc(snap.Docs, module, fn.Name)
		w.symbol("func:"+fn.Name, hostSig("func "+fn.Name, fn.Sig, f.Params), f.Doc, f.Example, f.Loc)
	}
	for _, e := range tab.EnumList() {
		if e.Module != module {
			continue
		}
		var b strings.Builder
		b.WriteString("enum " + e.Name + " {")
		for _, m := range e.Members {
			b.WriteString("\n\t" + m.Name)
		}
		b.WriteString("\n}")
		w.symbol("enum:"+e.Name, b.String(), enumDoc(snap, module, e.Name), "", "")
	}
	for _, k := range tab.ConstList() {
		if k.Module != module {
			continue
		}
		doc := ""
		if mc, ok := snap.Docs.ConstDoc(module, k.Name); ok {
			doc = mc.Doc
		}
		w.symbol("const:"+k.Name, "const "+k.Name+" "+abiType(k.Val.Type)+" = "+constValue(k.Val), doc, "", "")
	}
	return w.stub()
}

func enumDoc(snap *workspace.Snapshot, module, name string) string {
	if snap.Docs == nil {
		return ""
	}
	for _, e := range snap.Docs.Enums {
		if e.Module == module && e.Name == name {
			return e.Doc
		}
	}
	return ""
}

func builtinStub(snap *workspace.Snapshot) *Stub {
	w := newStubWriter("the builtin types", "builtin")
	for _, name := range builtinOrder {
		decl := builtinDecl[name]
		if decl == "" {
			decl = "type " + name
		}
		w.symbol("type:"+name, decl, builtinText(snap, name), "", "")
	}
	w.symbol("func:len", "func len(v string | []T) int", builtinText(snap, "len"), "", "")
	w.symbol("func:append", "func append(s []T, v T) []T", builtinText(snap, "append"), "", "")
	w.symbol("method:NodeRef.Get", "func (NodeRef) Get<Component>() ComponentRef<Component>",
		"Get returns the component of the given type on the node, or nil.", "sr := node.Get<SpriteRenderer>()", "")
	w.symbol("method:NodeRef.Has", "func (NodeRef) Has<Component>() bool",
		"Has reports whether the node has a component of the given type.", "if node.Has<Camera2D>() {", "")
	for _, me := range snap.Bindings.MethodList() {
		md := methodDoc(snap.Docs, me.Recv, me.Name)
		key := "method:" + me.Recv.String() + "." + me.Name
		w.symbol(key, hostSig("func ("+me.Recv.String()+") "+me.Name, me.Sig, md.Params), md.Doc, md.Example, md.Loc)
	}
	return w.stub()
}

func componentStub(snap *workspace.Snapshot) *Stub {
	w := newStubWriter("the engine and native components", "components")
	for _, c := range snap.Bindings.AllComponents() {
		doc := componentDoc(snap.Docs, c.Name)
		w.comment("", doc)
		w.lines["component:"+c.Name] = w.line
		w.write("component " + c.Name + " {")
		for _, f := range c.Fields {
			w.comment("\t", fieldDoc(snap.Docs, c.Name, f.Name))
			w.lines["field:"+c.Name+"."+f.Name] = w.line
			ro := ""
			if f.ReadOnly {
				ro = " readonly"
			}
			w.write("\t" + f.Name + " " + abiType(f.Type) + ro)
		}
		w.write("}")
		w.write("")
	}
	return w.stub()
}
