//go:build !aego_runtime

package docs

import (
	"io/fs"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"aego/asset"
	"aego/engine/docs/democheck"
	"aego/engine/module"
	"aego/engine/res"
	"aego/engine/settings"
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
	EngineDocsDir  = "engine/res/docs"
	scriptOwner    = "script"
	unknownOrder   = 1000
	TryDir         = "try"
	TryExt         = ".aescene"
	TryScriptExt   = ".aes"
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
	Source string
}

type Snippet struct {
	Lang  string
	Code  string
	Run   bool
	Check bool
	Line  int
}

type Entry struct {
	ID        string
	Parent    string
	Kind      Kind
	Owner     string
	Group     string
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
	Field     *FieldInfo
	Component *ComponentInfo
	Demo      *democheck.Contract `json:",omitempty"`
	Related   []string
	Try       string
	TrySelect string
	// TryScript is the script next to the try scene; TryCode is its compiled bytecode, base64 modules joined by commas.
	TryScript string `json:",omitempty"`
	TryCode   string `json:",omitempty"`
	// Input is set when the try script reads aego/input, so the page asks for a click on the canvas first.
	Input bool `json:",omitempty"`
	// TryKeys are the keys the try script reads besides WASD and the arrows, for the hint over the canvas.
	TryKeys []string `json:",omitempty"`
	// TryPrefabs are the prefabs next to the try scene, by asset id.
	TryPrefabs map[string]string `json:",omitempty"`
	// TryArtifacts contains a bounded AETB payload prepared by the docs build.
	// JSON encodes it as base64; browser runtimes only decode runtime artifacts.
	TryArtifacts []byte `json:",omitempty"`
	Icon         string
	// EditorOnly pages are shown in the editor's docs panel and left out of the site.
	EditorOnly bool `json:",omitempty"`
	Diag       *DiagInfo
	// Translations are the entry in other languages, by language (D348).
	Translations map[string]Translation `json:",omitempty"`
}

type DiagFile struct {
	Path string
	Bad  string
	Good string
}

// DiagInfo is the example of a diagnostic code; Severity, At and the position come from compiling Bad.
type DiagInfo struct {
	Phase    string
	Fix      string
	File     string
	Bad      string
	Good     string
	Files    []DiagFile
	Why      string
	Severity string
	At       string
	Line     int
	Col      int
	EndLine  int
	EndCol   int
	Message  string
	// Example is the scene that produces a module code, read from the example file next to its page.
	Example       string `json:",omitempty"`
	ExampleFile   string `json:",omitempty"`
	ExampleScript string `json:",omitempty"`
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

	Sections    []*module.Section
	Settings    []settings.Desc
	EventOwners map[string]string
	Diagnose    func(code string, d *DiagInfo)
	// CompileTry turns the script of a try scene into the bytecode the docs runtime loads.
	CompileTry func(script string) (string, error)
}

type pageMeta struct {
	group   string
	ordered bool
	// allow lists lint patterns the page may match (D347).
	allow []string
}

type Registry struct {
	entries  []Entry
	byID     map[string]int
	children map[string][]int
	keys     map[string]int
	alias    map[string]string
	builtin  map[string]bool
	mods     map[string]module.Info
	meta     map[string]pageMeta
	extra    []Group
	evOwners map[string]string
	problems []Problem
	groupT   map[string]map[string]string
	trySeen  map[string]bool
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
		src.Sections, src.Settings, src.EventOwners = m.Sections, m.Settings, m.EventOwners
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
	return module.DocSource{Owner: EngineOwner, FS: sub, Dir: EngineDocsDir}
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
		mods:     map[string]module.Info{},
		meta:     map[string]pageMeta{},
	}
	for _, mi := range src.Modules {
		r.builtin[mi.ID] = mi.Builtin
		r.mods[mi.ID] = mi
	}
	r.add(Entry{ID: BuiltinID, Kind: Module, Owner: scriptOwner, Group: GroupScriptAPI, Order: -1, Title: "Builtin types", Location: Location{Stub: StubPath(BuiltinID)}})
	r.add(Entry{ID: DiagnosticsID, Kind: Page, Owner: scriptOwner, Group: GroupDiagnostics, Title: "Diagnostics",
		Body: "Every code the compiler, the script runtime and the engine report, with what it means and how to fix it."})
	if src.Bindings != nil {
		r.bindings(src.Bindings, src.Owners)
		r.handles(src.Bindings, src.Owners)
		r.hostSignals(src.Bindings, src.Owners)
	}
	r.evOwners = src.EventOwners
	if src.Schema != nil {
		r.components(src.Schema, src.Bindings)
	}
	if src.Bindings != nil {
		r.compMethods(src.Bindings)
		r.scriptEvents(src.Bindings)
	}
	r.projectSections(src.Sections)
	r.playerSettings(src.Settings)
	r.codes(src.Diagnose)
	r.scripts(src.Project, src.Scripts)
	for _, d := range src.Docs {
		// A component page may explicitly refer to the same Try scene that is
		// also discovered by tries(). Read its sidecars once per source, while
		// still rejecting separate competing contracts in different sources.
		r.trySeen = make(map[string]bool)
		r.pages(d)
		r.tries(d)
	}
	r.trySeen = nil
	r.checkCodes()
	r.compileTries(src.CompileTry)
	r.index()
	r.related()
	return r
}

func (r *Registry) docsOrder(owner string) int {
	if mi, ok := r.mods[owner]; ok {
		return mi.DocsOrder
	}
	return unknownOrder
}

func (r *Registry) moduleTitle(owner string) string {
	if mi, ok := r.mods[owner]; ok && mi.DisplayName != "" {
		return mi.DisplayName
	}
	return owner
}

// componentGuideOrder puts components of a module documented in a guide group after the guide pages.
const componentGuideOrder = 1000

func (r *Registry) modulePage(owner string) string {
	id := ModulePrefix + owner
	if _, ok := r.byID[id]; !ok {
		title := r.moduleTitle(owner)
		r.add(Entry{ID: id, Kind: Module, Owner: owner, Group: GroupComponents, Order: r.docsOrder(owner), Title: title,
			Body: "Components of the " + title + " module.", Icon: r.mods[owner].DocsIcon})
	}
	return id
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

func (r *Registry) hostSignals(m *manifest.Manifest, owners map[string]string) {
	for _, cs := range m.CompSignals {
		ps := params(abi.Signature{Params: cs.Params}, cs.Names)
		r.add(Entry{
			ID: cs.Comp + "." + cs.Name, Parent: cs.Comp, Kind: Signal, Title: cs.Name,
			Signature: signature("signal "+cs.Name, ps, abi.T(abi.Void)), Params: ps, Body: cs.Doc,
			Location: Location{Stub: StubPath(ComponentsStub), Symbol: "signal:" + cs.Comp + "." + cs.Name},
		})
	}
	for _, hs := range m.HandleSignals {
		i := strings.LastIndexByte(hs.Handle, '.')
		mod, short := hs.Handle[:i], hs.Handle[i+1:]
		ps := params(abi.Signature{Params: hs.Params}, hs.Names)
		r.add(Entry{
			ID: hs.Handle + "." + hs.Name, Parent: hs.Handle, Kind: Signal, Owner: r.ownerOf(mod, owners), Title: short + "." + hs.Name,
			Signature: signature("signal "+hs.Name, ps, abi.T(abi.Void)), Params: ps, Body: hs.Doc,
			Location: Location{Stub: StubPath(mod), Symbol: "signal:" + short + "." + hs.Name},
		})
	}
}

func (r *Registry) add(e Entry) int {
	if i, dup := r.byID[e.ID]; dup {
		r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "declared twice, first by " + r.entries[i].Owner})
		return i
	}
	r.byID[e.ID] = len(r.entries)
	r.alias = nil
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
	first, rest, _ := strings.Cut(path, "/")
	if r.known(first) {
		return first
	}
	if first == "aego" {
		if seg, _, _ := strings.Cut(rest, "/"); r.known(seg) {
			return seg
		}
		return scriptOwner
	}
	return ""
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
		r.add(Entry{ID: mod.Path, Kind: Module, Owner: owner, Group: GroupScriptAPI, Order: r.docsOrder(owner), Title: mod.Path, Location: Location{Stub: stub}})
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
		owner := td.Owner
		if owner == "" {
			owner = EngineOwner
		}
		title := td.DisplayName
		if title == "" {
			title = td.FullName[strings.LastIndexByte(td.FullName, '.')+1:]
		}
		parent, group, order := "", r.mods[owner].DocsGroup, 0
		if group == "" {
			parent = r.modulePage(owner)
		} else {
			order = componentGuideOrder
		}
		r.add(Entry{
			ID: td.FullName, Parent: parent, Group: group, Order: order, Kind: Type, Owner: owner, Title: title,
			Signature: "component " + td.FullName, Body: td.Doc, Component: r.componentInfo(td, owner), Icon: td.Icon,
			Location: Location{Stub: stub, Symbol: "component:" + td.FullName},
		})
		base := defaultsOf(td)
		for i := range td.Fields {
			fd := &td.Fields[i]
			if !fd.Flags.Has(schema.FlagEditor) && !td.Scripted(fd) {
				continue
			}
			name := exportName(fd.Name)
			sig := "field " + name
			info := fieldInfo(td, fd, base, true)
			info.Index, info.Label = i, schema.Label(fd)
			loc := Location{Stub: stub}
			if mc != nil {
				if f, ok := mc.Field(name); ok {
					sig += " " + f.Type.String()
					info.Type = f.Type.String()
					if f.ReadOnly && !info.Runtime && !slices.Contains(info.Notes, "read-only") {
						sig += " readonly"
						info.Notes = append(info.Notes, "read-only in scripts")
					}
					loc.Symbol = "field:" + td.FullName + "." + name
				}
			}
			info.Type = displayType(fd, info.Type)
			r.add(Entry{
				ID: td.FullName + "." + name, Parent: td.FullName, Kind: Field, Owner: owner, Title: name,
				Signature: sig, Body: fd.Doc, Location: loc, Field: info,
			})
		}
	}
}

func (r *Registry) codes(diagnose func(string, *DiagInfo)) {
	for _, c := range diag.Codes() {
		h, _ := diag.Explain(c)
		r.add(Entry{
			ID: c, Parent: DiagnosticsID, Kind: Code, Owner: scriptOwner, Title: c,
			Summary: h.What, Body: h.What + "\n\nFix: " + h.Fix, Diag: diagInfo(c, h, diagnose),
		})
	}
}

func diagInfo(code string, h diag.Help, diagnose func(string, *DiagInfo)) *DiagInfo {
	d := &DiagInfo{Phase: diag.Phase(code), Fix: h.Fix}
	ex, ok := diag.Example(code)
	if !ok {
		return d
	}
	d.Why = ex.Why
	if ex.Why == "" {
		d.File, d.Bad, d.Good = ex.MainFile(), strings.ToValidUTF8(ex.Bad, "\uFFFD"), ex.Good
		for _, f := range ex.Files {
			d.Files = append(d.Files, DiagFile{Path: f.Path, Bad: f.Bad, Good: f.Good})
		}
	}
	if diagnose != nil {
		diagnose(code, d)
	}
	return d
}

func (r *Registry) pages(src module.DocSource) {
	var files, translated []string
	err := fs.WalkDir(src.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			if translationLang(p) != "" {
				translated = append(translated, p)
			} else {
				files = append(files, p)
			}
		}
		return nil
	})
	defer r.translations(src, translated)
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
		source := ""
		if src.Dir != "" {
			source = path.Join(src.Dir, f)
		}
		for _, rec := range recs {
			if rec.code != "" {
				r.codePage(src, f, source, rec)
				continue
			}
			r.page(src.Owner, f, source, rec)
			if rec.try != "" {
				r.pageTry(src, rec)
			}
		}
	}
}

func (r *Registry) page(owner, file, source string, rec record) {
	if i, ok := r.byID[rec.id]; ok {
		e := &r.entries[i]
		if e.Kind == Page {
			r.problems = append(r.problems, Problem{ID: rec.id, Error: true, Message: "page declared twice, again in " + owner + ":" + file})
			return
		}
		e.Body = joinBody(e.Body, rec.body)
		e.Keys = append(e.Keys, rec.keys...)
		if rec.icon != "" {
			e.Icon = rec.icon
		}
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
		ID: rec.id, Parent: rec.parent, Kind: Page, Owner: owner, Group: rec.group, Title: title, Summary: rec.summary,
		Body: rec.body, Keys: rec.keys, Requires: rec.requires, Order: rec.order, Icon: rec.icon, EditorOnly: rec.editorOnly,
		Location: Location{Path: owner + ":" + file, Line: rec.line, Source: source},
	})
	r.meta[rec.id] = pageMeta{group: rec.group, ordered: rec.ordered, allow: rec.styleAllow}
	if rec.groupTitle != "" {
		r.declareGroup(rec.id, Group{ID: rec.group, Title: rec.groupTitle, Section: SectionGuides, Order: rec.groupOrder, Icon: rec.groupIcon})
	}
}

// Scopes of a module code: a scene code is reported by a component and needs an example scene that produces it; an asset code comes from importing or checking an asset; a play code comes from editing the scene during Play, which no scene file can show.
const (
	ScopeScene = "scene"
	ScopeAsset = "asset"
	ScopePlay  = "play"
)

// codePage registers diagnostics/<CODE>.md of a module as the code's entry under Diagnostics (D146).
func (r *Registry) codePage(src module.DocSource, file, source string, rec record) {
	if _, dup := r.byID[rec.code]; dup {
		r.problems = append(r.problems, Problem{ID: rec.code, Error: true, Message: "code page declared twice, again in " + src.Owner + ":" + file})
		return
	}
	scope := rec.scope
	if scope == "" {
		scope = ScopeScene
	}
	d := &DiagInfo{Phase: scope, Severity: rec.severity}
	switch scope {
	case ScopeScene, ScopeAsset, ScopePlay:
	default:
		r.problems = append(r.problems, Problem{ID: rec.code, Error: true, Message: "scope must be scene, asset or play"})
	}
	switch rec.severity {
	case "error", "warning", "info":
	default:
		r.problems = append(r.problems, Problem{ID: rec.code, Error: true, Message: "severity must be error, warning or info"})
	}
	if rec.example != "" {
		ex := path.Join(path.Dir(file), rec.example)
		data, err := fs.ReadFile(src.FS, ex)
		if err != nil {
			r.problems = append(r.problems, Problem{ID: rec.code, Error: true, Message: "example " + ex + " cannot be read: " + err.Error()})
		} else {
			d.Example, d.ExampleFile = string(data), path.Base(ex)
		}
		if sc, err := fs.ReadFile(src.FS, strings.TrimSuffix(ex, TryExt)+TryScriptExt); err == nil {
			d.ExampleScript = string(sc)
		}
	}
	r.add(Entry{
		ID: rec.code, Parent: DiagnosticsID, Kind: Code, Owner: src.Owner, Title: rec.code, Summary: rec.title,
		Body: rec.body, Diag: d, Location: Location{Path: src.Owner + ":" + file, Line: rec.line, Source: source},
	})
}

// checkCodes matches the codes modules declare in Info.Codes with their pages.
func (r *Registry) checkCodes() {
	declared := map[string]string{}
	for id, mi := range r.mods {
		for _, c := range mi.Codes {
			declared[c] = id
			i, ok := r.byID[c]
			if !ok || r.entries[i].Kind != Code {
				r.problems = append(r.problems, Problem{ID: c, Error: true, Message: "module " + id + " declares " + c + " but has no diagnostics/" + c + ".md"})
			}
		}
	}
	for i := range r.entries {
		e := &r.entries[i]
		if e.Kind != Code || e.Owner == scriptOwner {
			continue
		}
		if declared[e.ID] != e.Owner {
			r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "page for " + e.ID + " but module " + e.Owner + " does not list it in Info.Codes"})
		}
		if e.Diag.Phase == ScopeScene && e.Diag.Example == "" {
			r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: e.ID + " is reported by a component and needs an example scene (example: " + e.ID + ".aescene)"})
		}
	}
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
		trimSelfDoc(e)
		if e.Summary == "" {
			e.Summary = summary(e.Body)
			if e.Kind == Page {
				e.Summary = firstSentence(e.Summary)
			}
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
	for i := range r.entries {
		if r.entries[i].Parent == "" {
			r.spread(i, r.entries[i].Group, 0)
		}
	}
}

func (r *Registry) spread(i int, group string, depth int) {
	r.entries[i].Group = group
	if depth > 32 {
		return
	}
	for _, c := range r.children[r.entries[i].ID] {
		r.spread(c, group, depth+1)
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

func sortRoots(out []Entry) {
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Order != out[b].Order {
			return out[a].Order < out[b].Order
		}
		return out[a].Title < out[b].Title
	})
}

func (r *Registry) Roots() []Entry {
	var out []Entry
	for i := range r.entries {
		if r.entries[i].Parent == "" {
			out = append(out, r.entries[i])
		}
	}
	sortRoots(out)
	return out
}

func (r *Registry) GroupRoots(group string) []Entry {
	var out []Entry
	for i := range r.entries {
		if e := &r.entries[i]; e.Parent == "" && e.Group == group {
			out = append(out, *e)
		}
	}
	sortRoots(out)
	return out
}

func (r *Registry) Problems() []Problem {
	return r.problems
}

func (r *Registry) checkPages(out []Problem) []Problem {
	for id, m := range r.meta {
		if r.entries[r.byID[id]].Parent != "" {
			continue
		}
		g, ok := r.group(m.group)
		switch {
		case m.group == "":
			out = append(out, Problem{ID: id, Error: true, Message: "root page has no group; add group: one of " + strings.Join(r.guideGroups(), ", ")})
		case !ok || g.Section == SectionReference && g.ID != GroupCLI && g.ID != GroupFormats:
			out = append(out, Problem{ID: id, Error: true, Message: "group " + m.group + " is not a guide group (" + strings.Join(r.guideGroups(), ", ") + ")"})
		}
		if !m.ordered {
			out = append(out, Problem{ID: id, Error: true, Message: "root page has no order"})
		}
	}
	for _, mi := range r.mods {
		if mi.DisplayName == "" {
			out = append(out, Problem{ID: ModulePrefix + mi.ID, Error: mi.Builtin, Message: "module has no DisplayName"})
		}
	}
	return out
}

func (r *Registry) Check(strict bool) []Problem {
	out := r.checkPages(slices.Clone(r.problems))
	out = append(out, r.StyleProblems(strict)...)
	for i := range r.entries {
		e := &r.entries[i]
		for _, l := range e.Links {
			if _, ok := r.byID[l]; !ok {
				out = append(out, Problem{ID: e.ID, Error: true, Message: "broken link ref:" + l})
			}
		}
		if r.needsRun(i) && !r.runs(i, 0) {
			out = append(out, Problem{ID: e.ID, Error: true, Message: "has no runnable example (an ```aes run block)"})
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

func (r *Registry) needsRun(i int) bool {
	e := &r.entries[i]
	if e.Owner != scriptOwner && !r.builtin[e.Owner] {
		return false
	}
	switch e.Kind {
	case Module:
		return e.Group == GroupScriptAPI
	case Page:
		return e.Group == GroupScript && len(e.Keys) == 0
	}
	return false
}

func (r *Registry) runs(i, depth int) bool {
	e := &r.entries[i]
	for _, s := range e.Snippets {
		if (s.Run || s.Check && e.Kind == Module) && s.Lang == "aes" {
			return true
		}
	}
	if e.Kind != Page || depth > 32 {
		return false
	}
	for _, c := range r.children[e.ID] {
		if r.runs(c, depth+1) {
			return true
		}
	}
	return false
}

func (r *Registry) pageTry(src module.DocSource, rec record) {
	i, ok := r.byID[rec.id]
	if !ok {
		return
	}
	data, err := fs.ReadFile(src.FS, path.Clean(rec.try))
	if err != nil {
		r.problems = append(r.problems, Problem{ID: rec.id, Error: true, Message: "try scene " + rec.try + " cannot be read: " + err.Error()})
		return
	}
	r.entries[i].Try, r.entries[i].TrySelect = string(data), rec.trySelect
	r.tryExtras(&r.entries[i], src, rec.try)
}

var metaID = regexp.MustCompile(`\bid="([0-9a-f]{32})"`)

// tryExtras reads the script X.aes and the prefabs X.<name>.aeprefab, ids from their .meta, next to the try scene X.aescene.
func (r *Registry) tryExtras(e *Entry, src module.DocSource, scenePath string) {
	if r.trySeen == nil {
		r.trySeen = make(map[string]bool)
	}
	key := e.ID + "\x00" + path.Clean(scenePath)
	if r.trySeen[key] {
		return
	}
	r.trySeen[key] = true
	r.readDemoContract(e, src, scenePath)
	base := strings.TrimSuffix(path.Clean(scenePath), TryExt)
	if data, err := fs.ReadFile(src.FS, base+".aetry"); err == nil {
		if _, err := asset.DecodeTryArtifacts(data); err != nil {
			r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "try artifacts " + base + ".aetry: " + err.Error()})
		} else {
			e.TryArtifacts = data
		}
	} else if _, err := fs.Stat(src.FS, base+".assets"); err == nil {
		r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "try source assets need a prepared " + base + ".aetry payload; run gendocs -prepare-try"})
	}
	if data, err := fs.ReadFile(src.FS, base+TryScriptExt); err == nil {
		e.TryScript = string(data)
	}
	files, _ := fs.Glob(src.FS, base+".*.aeprefab")
	for _, f := range files {
		text, err := fs.ReadFile(src.FS, f)
		meta, merr := fs.ReadFile(src.FS, f+".meta")
		m := metaID.FindSubmatch(meta)
		if err != nil || merr != nil || m == nil {
			r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "try prefab " + f + " needs a .meta with its id"})
			continue
		}
		if e.TryPrefabs == nil {
			e.TryPrefabs = map[string]string{}
		}
		e.TryPrefabs[string(m[1])] = string(text)
	}
}

func (r *Registry) compileTries(compile func(string) (string, error)) {
	if compile == nil {
		return
	}
	for i := range r.entries {
		e := &r.entries[i]
		if e.TryScript == "" {
			continue
		}
		code, err := compile(e.TryScript)
		if err != nil {
			r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "try script does not compile: " + err.Error()})
			continue
		}
		e.TryCode = code
		e.Input = readsInput(e.TryScript)
		if e.Input {
			e.TryKeys = extraKeys(e.TryScript)
		}
	}
}

var keyUse = regexp.MustCompile(`\binput\.Key([A-Za-z0-9]+)(?:[^A-Za-z0-9(]|$)`)

func extraKeys(src string) []string {
	var keys []string
	for _, m := range keyUse.FindAllStringSubmatch(src, -1) {
		switch k := m[1]; k {
		case "W", "A", "S", "D", "Up", "Down", "Left", "Right":
		default:
			if !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

var importBlock = regexp.MustCompile(`(?s)\bimport\s*(\([^)]*\)|"[^"]*")`)

// readsInput reports whether a script imports aego/input.
func readsInput(src string) bool {
	for _, m := range importBlock.FindAllStringSubmatch(src, -1) {
		if strings.Contains(m[1], `"aego/input"`) {
			return true
		}
	}
	return false
}

func (r *Registry) componentNamed(short, owner string) (int, bool) {
	found := -1
	for i := range r.entries {
		e := &r.entries[i]
		if e.Component == nil || !strings.HasSuffix(e.ID, "."+short) {
			continue
		}
		if e.Owner == owner {
			return i, true
		}
		if found < 0 {
			found = i
		}
	}
	return found, found >= 0
}

func (r *Registry) tries(src module.DocSource) {
	files, err := fs.Glob(src.FS, TryDir+"/*"+TryExt)
	if err != nil {
		return
	}
	for _, f := range files {
		short := strings.TrimSuffix(path.Base(f), TryExt)
		i, ok := r.componentNamed(short, src.Owner)
		if !ok {
			r.problems = append(r.problems, Problem{ID: src.Owner + ":" + f, Error: true, Message: "try scene names no component " + short})
			continue
		}
		data, err := fs.ReadFile(src.FS, f)
		if err != nil {
			r.problems = append(r.problems, Problem{ID: src.Owner + ":" + f, Error: true, Message: err.Error()})
			continue
		}
		e := &r.entries[i]
		e.Try, e.TrySelect = string(data), e.ID
		r.tryExtras(e, src, f)
	}
}
