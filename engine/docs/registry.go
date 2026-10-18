//go:build !aego_runtime

package docs

import (
	"io/fs"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"aego/engine/module"
	"aego/engine/res"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/diag"
)

type Kind uint8

const (
	Page Kind = iota
	Module
	Function
	Type
	Field
	Method
	Signal
	Code
	Const
	Enum
)

const (
	BuiltinID      = "aego/builtin"
	ComponentsStub = "engine/components"
	DiagnosticsID  = "diagnostics"
	ModulePrefix   = "module/"
	KeywordPrefix  = "keyword/"
	StubPrefix     = "stub:"
	EngineOwner    = "engine"
	scriptOwner    = "script"
)

var kindNames = [...]string{"page", "module", "function", "type", "field", "method", "signal", "code", "const", "enum"}

var kindRank = [...]int{Function: 0, Type: 1, Method: 2, Field: 3, Const: 4, Enum: 5, Signal: 6, Module: 7, Page: 8, Code: 9}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "kind" + strconv.Itoa(int(k))
}

type Param struct {
	Name string
	Type string
}

type Location struct {
	Path   string
	Line   int
	Stub   string
	Symbol string
}

type Snippet struct {
	Lang string
	Code string
	Run  bool
	Line int
}

type Entry struct {
	ID        string
	Parent    string
	Kind      Kind
	Owner     string
	Title     string
	Summary   string
	Body      string
	Signature string
	Params    []Param
	Example   string
	Location  Location
	Links     []string
	Keys      []string
	Requires  []string
	Snippets  []Snippet
	Order     int
}

type Problem struct {
	ID      string
	Error   bool
	Message string
}

type Sources struct {
	Bindings *manifest.Manifest
	Schema   *schema.Registry
	Modules  []module.Info
	Docs     []module.DocSource
	Owners   map[string]string
	Project  string
	Scripts  []ScriptSymbol
}

type Registry struct {
	entries  []Entry
	byID     map[string]int
	children map[string][]int
	keys     map[string]int
	builtin  map[string]bool
	problems []Problem
}

func StubPath(module string) string {
	return StubPrefix + module + ".aes"
}

func KeywordID(key string) string {
	attr := strings.HasPrefix(key, "@")
	s := strings.ToLower(strings.TrimPrefix(key, "@"))
	if i := strings.IndexByte(s, '<'); i >= 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), "-")
	if attr {
		s += "-attr"
	}
	return KeywordPrefix + s
}

func Build(m *module.Manifest, b *bind.Registry, s *schema.Registry) (*Registry, error) {
	bm, err := manifest.FromRegistry(b, s)
	if err != nil {
		return nil, err
	}
	return FromManifest(bm, s, m), nil
}

func SourcesOf(bm *manifest.Manifest, s *schema.Registry, m *module.Manifest) Sources {
	src := Sources{Bindings: bm, Schema: s, Docs: []module.DocSource{EngineDocs()}}
	if m != nil {
		src.Modules, src.Owners = m.Modules, m.ScriptOwners
		src.Docs = append(src.Docs, m.Docs...)
	}
	return src
}

func FromManifest(bm *manifest.Manifest, s *schema.Registry, m *module.Manifest) *Registry {
	return Assemble(SourcesOf(bm, s, m))
}

func EngineDocs() module.DocSource {
	sub, err := fs.Sub(res.FS(), "docs")
	if err != nil {
		sub = res.FS()
	}
	return module.DocSource{Owner: EngineOwner, FS: sub}
}

func (r *Registry) Keywords() map[string]string {
	out := make(map[string]string, len(r.keys))
	for k, i := range r.keys {
		out[k] = Plain(r.entries[i].Body)
	}
	return out
}

func Assemble(src Sources) *Registry {
	r := &Registry{
		byID:     map[string]int{},
		children: map[string][]int{},
		keys:     map[string]int{},
		builtin:  map[string]bool{},
	}
	for _, mi := range src.Modules {
		r.builtin[mi.ID] = mi.Builtin
		r.add(Entry{ID: ModulePrefix + mi.ID, Kind: Module, Owner: mi.ID, Title: mi.ID})
	}
	r.add(Entry{ID: BuiltinID, Kind: Module, Owner: scriptOwner, Title: "Builtin types", Location: Location{Stub: StubPath(BuiltinID)}})
	r.add(Entry{ID: DiagnosticsID, Kind: Page, Owner: scriptOwner, Title: "Diagnostics"})
	if src.Bindings != nil {
		r.bindings(src.Bindings, src.Owners)
		r.handles(src.Bindings, src.Owners)
	}
	if src.Schema != nil {
		r.components(src.Schema, src.Bindings)
	}
	r.codes()
	r.scripts(src.Project, src.Scripts)
	for _, d := range src.Docs {
		r.pages(d)
	}
	r.index()
	return r
}

func (r *Registry) handles(m *manifest.Manifest, owners map[string]string) {
	for _, h := range m.Handles {
		i := strings.LastIndexByte(h.Name, '.')
		mod, name := h.Name[:i], h.Name[i+1:]
		r.add(Entry{
			ID: h.Name, Parent: mod, Kind: Type, Owner: r.ownerOf(mod, owners), Title: name,
			Signature: "type " + name, Body: h.Doc,
			Location: Location{Stub: StubPath(mod), Symbol: "type:" + name},
		})
	}
	for _, hm := range m.HandleMethods {
		i := strings.LastIndexByte(hm.Handle, '.')
		mod, name := hm.Handle[:i], hm.Handle[i+1:]
		ps := params(hm.Sig, hm.Params)
		r.add(Entry{
			ID: hm.Handle + "." + hm.Name, Parent: hm.Handle, Kind: Method, Owner: r.ownerOf(mod, owners), Title: name + "." + hm.Name,
			Signature: signature("func ("+name+") "+hm.Name, ps, hm.Sig.Result), Params: ps, Body: hm.Doc, Example: hm.Example,
			Location: Location{Path: hm.Loc, Stub: StubPath(mod), Symbol: "method:" + name + "." + hm.Name},
		})
	}
}

func (r *Registry) add(e Entry) int {
	if i, dup := r.byID[e.ID]; dup {
		r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "declared twice, first by " + r.entries[i].Owner})
		return i
	}
	r.byID[e.ID] = len(r.entries)
	r.entries = append(r.entries, e)
	return len(r.entries) - 1
}

func (r *Registry) known(id string) bool {
	_, ok := r.builtin[id]
	return ok
}

func (r *Registry) ownerOf(path string, owners map[string]string) string {
	if o, ok := owners[path]; ok {
		return o
	}
	if first, _, _ := strings.Cut(path, "/"); r.known(first) {
		return first
	}
	if strings.HasPrefix(path, "aego/") {
		return scriptOwner
	}
	return ""
}

func moduleParent(owner string) string {
	if owner == "" {
		return ""
	}
	return ModulePrefix + owner
}

func params(s abi.Signature, names []string) []Param {
	out := make([]Param, len(s.Params))
	for i, p := range s.Params {
		n := "p" + strconv.Itoa(i)
		if i < len(names) {
			n = names[i]
		}
		out[i] = Param{Name: n, Type: p.String()}
	}
	return out
}

func signature(head string, ps []Param, result abi.TypeRef) string {
	var b strings.Builder
	b.WriteString(head)
	b.WriteByte('(')
	for i, p := range ps {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name + " " + p.Type)
	}
	b.WriteByte(')')
	if result.Kind != abi.Void {
		b.WriteString(" " + result.String())
	}
	return b.String()
}

func (r *Registry) bindings(m *manifest.Manifest, owners map[string]string) {
	for _, mod := range m.Modules {
		owner := r.ownerOf(mod.Path, owners)
		stub := StubPath(mod.Path)
		r.add(Entry{ID: mod.Path, Parent: moduleParent(owner), Kind: Module, Owner: owner, Title: mod.Path, Location: Location{Stub: stub}})
		for _, f := range mod.Funcs {
			ps := params(f.Sig, f.Params)
			r.add(Entry{
				ID: mod.Path + "." + f.Name, Parent: mod.Path, Kind: Function, Owner: owner, Title: f.Name,
				Signature: signature("func "+f.Name, ps, f.Sig.Result), Params: ps, Body: f.Doc, Example: f.Example,
				Location: Location{Path: f.Loc, Stub: stub, Symbol: "func:" + f.Name},
			})
		}
		for _, c := range mod.Consts {
			r.add(Entry{
				ID: mod.Path + "." + c.Name, Parent: mod.Path, Kind: Const, Owner: owner, Title: c.Name,
				Signature: "const " + c.Name + " " + c.Val.Type.String(), Body: c.Doc,
				Location: Location{Stub: stub, Symbol: "const:" + c.Name},
			})
		}
	}
	for _, e := range m.Enums {
		owner := r.ownerOf(e.Module, owners)
		r.add(Entry{
			ID: e.Module + "." + e.Name, Parent: e.Module, Kind: Enum, Owner: owner, Title: e.Name,
			Signature: "enum " + e.Name, Body: e.Doc,
			Location: Location{Stub: StubPath(e.Module), Symbol: "enum:" + e.Name},
		})
	}
	for _, me := range m.Methods {
		recv := me.Recv.String()
		ps := params(me.Sig, me.Params)
		r.add(Entry{
			ID: BuiltinID + "." + recv + "." + me.Name, Parent: BuiltinID, Kind: Method, Owner: scriptOwner, Title: recv + "." + me.Name,
			Signature: signature("func ("+recv+") "+me.Name, ps, me.Sig.Result), Params: ps, Body: me.Doc, Example: me.Example,
			Location: Location{Path: me.Loc, Stub: StubPath(BuiltinID), Symbol: "method:" + recv + "." + me.Name},
		})
	}
}

func exportName(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func (r *Registry) components(s *schema.Registry, m *manifest.Manifest) {
	stub := StubPath(ComponentsStub)
	for _, td := range s.Components() {
		var mc *manifest.Component
		if m != nil {
			mc, _ = m.Component(td.FullName)
		}
		if td.Owner == "" && mc == nil {
			continue
		}
		title := td.DisplayName
		if title == "" {
			title = td.FullName[strings.LastIndexByte(td.FullName, '.')+1:]
		}
		r.add(Entry{
			ID: td.FullName, Parent: moduleParent(td.Owner), Kind: Type, Owner: td.Owner, Title: title,
			Signature: "component " + td.FullName, Body: td.Doc,
			Location: Location{Stub: stub, Symbol: "component:" + td.FullName},
		})
		for i := range td.Fields {
			fd := &td.Fields[i]
			if !fd.Flags.Has(schema.FlagEditor) && !td.Scripted(fd) {
				continue
			}
			name := exportName(fd.Name)
			sig := "field " + name
			loc := Location{Stub: stub}
			if mc != nil {
				if f, ok := mc.Field(name); ok {
					sig += " " + f.Type.String()
					if f.ReadOnly {
						sig += " readonly"
					}
					loc.Symbol = "field:" + td.FullName + "." + name
				}
			}
			r.add(Entry{
				ID: td.FullName + "." + name, Parent: td.FullName, Kind: Field, Owner: td.Owner, Title: name,
				Signature: sig, Body: fd.Doc, Location: loc,
			})
		}
	}
}

func (r *Registry) codes() {
	for _, c := range diag.Codes() {
		h, _ := diag.Explain(c)
		r.add(Entry{
			ID: c, Parent: DiagnosticsID, Kind: Code, Owner: scriptOwner, Title: c,
			Summary: h.What, Body: h.What + "\n\nFix: " + h.Fix,
		})
	}
}

func (r *Registry) pages(src module.DocSource) {
	var files []string
	err := fs.WalkDir(src.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		r.problems = append(r.problems, Problem{ID: src.Owner, Error: true, Message: "docs source cannot be read: " + err.Error()})
		return
	}
	for _, f := range files {
		data, err := fs.ReadFile(src.FS, f)
		if err != nil {
			r.problems = append(r.problems, Problem{ID: src.Owner + ":" + f, Error: true, Message: err.Error()})
			continue
		}
		recs, err := parseRecords(string(data))
		if err != nil {
			r.problems = append(r.problems, Problem{ID: src.Owner + ":" + f, Error: true, Message: err.Error()})
			continue
		}
		for _, rec := range recs {
			r.page(src.Owner, f, rec)
		}
	}
}

func (r *Registry) page(owner, file string, rec record) {
	if i, ok := r.byID[rec.id]; ok {
		e := &r.entries[i]
		if e.Kind == Page {
			r.problems = append(r.problems, Problem{ID: rec.id, Error: true, Message: "page declared twice, again in " + owner + ":" + file})
			return
		}
		e.Body = joinBody(e.Body, rec.body)
		e.Keys = append(e.Keys, rec.keys...)
		if rec.summary != "" {
			e.Summary = rec.summary
		}
		return
	}
	title := rec.title
	if title == "" {
		title = heading(rec.body)
	}
	if title == "" {
		title = rec.id
	}
	r.add(Entry{
		ID: rec.id, Parent: rec.parent, Kind: Page, Owner: owner, Title: title, Summary: rec.summary,
		Body: rec.body, Keys: rec.keys, Requires: rec.requires, Order: rec.order,
		Location: Location{Path: owner + ":" + file, Line: rec.line},
	})
}

func joinBody(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "\n\n" + b
}

func (r *Registry) index() {
	for i := range r.entries {
		e := &r.entries[i]
		if e.Summary == "" {
			e.Summary = summary(e.Body)
		}
		for _, l := range refs(e.Body) {
			if !slices.Contains(e.Links, l) {
				e.Links = append(e.Links, l)
			}
		}
		e.Snippets = snippets(e.Body)
		if e.Parent != "" {
			if _, ok := r.byID[e.Parent]; !ok {
				if e.Kind == Page {
					r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "parent " + e.Parent + " does not exist"})
				}
				e.Parent = ""
			}
		}
		if e.Parent != "" {
			r.children[e.Parent] = append(r.children[e.Parent], i)
		}
		for _, k := range e.Keys {
			if j, dup := r.keys[k]; dup {
				r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "key " + strconv.Quote(k) + " is already used by " + r.entries[j].ID})
				continue
			}
			r.keys[k] = i
		}
	}
	for _, list := range r.children {
		sort.SliceStable(list, func(a, b int) bool {
			x, y := &r.entries[list[a]], &r.entries[list[b]]
			if x.Order != y.Order {
				return x.Order < y.Order
			}
			if x.Kind != y.Kind {
				return kindRank[x.Kind] < kindRank[y.Kind]
			}
			return x.Title < y.Title
		})
	}
}

func (r *Registry) Len() int {
	return len(r.entries)
}

func (r *Registry) All() []Entry {
	return slices.Clone(r.entries)
}

func (r *Registry) Get(id string) (Entry, bool) {
	i, ok := r.byID[id]
	if !ok {
		return Entry{}, false
	}
	return r.entries[i], true
}

const KeyPrefix = "key:"

func (r *Registry) Resolve(ref string) (Entry, bool) {
	if k, ok := strings.CutPrefix(ref, KeyPrefix); ok {
		return r.ByKey(k)
	}
	return r.Get(strings.TrimPrefix(ref, "ref:"))
}

func (r *Registry) ByKey(key string) (Entry, bool) {
	i, ok := r.keys[key]
	if !ok {
		return Entry{}, false
	}
	return r.entries[i], true
}

func (r *Registry) Children(id string) []Entry {
	list := r.children[id]
	out := make([]Entry, len(list))
	for k, i := range list {
		out[k] = r.entries[i]
	}
	return out
}

func (r *Registry) Roots() []Entry {
	var out []Entry
	for i := range r.entries {
		if r.entries[i].Parent == "" {
			out = append(out, r.entries[i])
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Order != out[b].Order {
			return out[a].Order < out[b].Order
		}
		return out[a].Title < out[b].Title
	})
	return out
}

func (r *Registry) Problems() []Problem {
	return r.problems
}

func (r *Registry) Check(strict bool) []Problem {
	out := slices.Clone(r.problems)
	for i := range r.entries {
		e := &r.entries[i]
		for _, l := range e.Links {
			if _, ok := r.byID[l]; !ok {
				out = append(out, Problem{ID: e.ID, Error: true, Message: "broken link ref:" + l})
			}
		}
		switch {
		case (e.Kind == Function || e.Kind == Method) && strings.TrimSpace(e.Body) == "":
			out = append(out, Problem{ID: e.ID, Error: strict && e.Owner != "" && !r.builtin[e.Owner], Message: "has no documentation"})
		case e.Kind != Module && e.Summary == "":
			out = append(out, Problem{ID: e.ID, Message: "empty summary"})
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Error != out[b].Error {
			return out[a].Error
		}
		return out[a].ID < out[b].ID
	})
	return out
}
