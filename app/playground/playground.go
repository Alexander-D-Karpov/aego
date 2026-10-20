package playground

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing/fstest"
	"unicode/utf8"

	"aego/app/assembly"
	playmods "aego/app/assembly/playground"
	"aego/core/log"
	"aego/engine"
	"aego/engine/docs"
	"aego/engine/module"
	"aego/modules/render2d"
	scriptsystems "aego/modules/scripting/systems"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/bytecode"
	"aego/script/diag"
	"aego/script/langsvc"
	"aego/script/lexer"
	"aego/script/runtime"
	"aego/script/source"
	"aego/script/workspace"
)

//go:embed console/donut.aes
var donutSource string

const (
	Module        = "game"
	ScriptsDir    = "scripts"
	DefaultFrames = 120
	MaxFrames     = 3600
)

type Diagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	EndLine  int    `json:"endLine"`
	EndCol   int    `json:"endCol"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type Line struct {
	Frame   uint64 `json:"frame"`
	Level   string `json:"level"`
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

type Fault struct {
	Script  string `json:"script"`
	Node    string `json:"node"`
	Code    string `json:"code"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

type Result struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Log         []Line       `json:"log"`
	Faults      []Fault      `json:"faults"`
	Frames      int          `json:"frames"`
}

type Token struct {
	Start int    `json:"s"`
	End   int    `json:"e"`
	Kind  string `json:"k"`
}

type Example struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Code  string `json:"code"`
	Scene bool   `json:"scene,omitempty"`
}

type Playground struct {
	types    *scene.Types
	reg      *bind.Registry
	man      *module.Manifest
	sections *module.Sections
	docs     *docs.Registry
	ws       *workspace.Workspace
	proj     *exampleProject
	gameCfg  GameConfig
	game     *Game

	simEngine *engine.Engine
}

func New() (*Playground, error) {
	return NewWith(playmods.Catalog())
}

// NewWith builds a playground over another module catalog, for checks that need every module's types.
func NewWith(cat assembly.Catalog) (*Playground, error) {
	proj, err := loadProject()
	if err != nil {
		return nil, err
	}
	types, err := assembly.NewTypes()
	if err != nil {
		return nil, err
	}
	reg := bind.New()
	man, err := cat.Load(types, reg, assembly.Options{Seed: 1, Testing: true})
	if err != nil {
		return nil, err
	}
	sections, err := man.LoadSections(proj.block, coreBlocks...)
	if err != nil {
		return nil, err
	}
	if err := types.Schema().Freeze(); err != nil {
		return nil, err
	}
	if err := scriptsystems.Freeze(reg, types.Schema()); err != nil {
		return nil, err
	}
	bm, err := manifest.FromRegistry(reg, types.Schema())
	if err != nil {
		return nil, err
	}
	d := docs.FromManifest(bm, types.Schema(), man)
	bm.Keywords = d.Keywords()
	ws, err := workspace.Open(workspace.Config{
		FS:       fstest.MapFS{},
		Module:   Module,
		Scripts:  ScriptsDir,
		Bindings: reg.Table(),
		Docs:     bm,
	})
	if err != nil {
		return nil, err
	}
	return &Playground{types: types, reg: reg, man: man, sections: sections, docs: d, ws: ws, proj: proj}, nil
}

func (p *Playground) Docs() *docs.Registry {
	return p.docs
}

func packageOf(src []byte) string {
	toks, _, _ := lexer.TokenizeAll(1, src, source.NewInterner())
	for i, t := range toks {
		if t.Kind.IsKeyword() && t.Text == "package" && i+1 < len(toks) && toks[i+1].Text != "" {
			return toks[i+1].Text
		}
	}
	return "main"
}

func place(files map[string]string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files))
	for name, src := range files {
		p := path.Clean("/" + name)[1:]
		if p == "" {
			p = "main" + workspace.Ext
		}
		if path.Ext(p) != workspace.Ext {
			p += workspace.Ext
		}
		if path.Dir(p) == "." {
			p = path.Join(packageOf([]byte(src)), p)
		}
		out[path.Join(ScriptsDir, p)] = []byte(src)
	}
	if len(out) == 0 {
		return nil, errors.New("no script files")
	}
	return out, nil
}

func severity(d diag.Diagnostic) string {
	switch d.Severity {
	case diag.Error:
		return "error"
	case diag.Info:
		return "info"
	}
	return "warning"
}

func diagnostics(sm *source.Manager, ds []diag.Diagnostic) []Diagnostic {
	out := make([]Diagnostic, 0, len(ds))
	for _, d := range ds {
		x := Diagnostic{Code: d.Code, Message: d.Message, Severity: severity(d)}
		if d.Primary.Valid() {
			x.File = strings.TrimPrefix(sm.Path(d.Primary.File), ScriptsDir+"/")
			x.Line, x.Col = sm.LineCol(d.Primary.File, d.Primary.Start)
			x.EndLine, x.EndCol = sm.LineCol(d.Primary.File, d.Primary.End)
		}
		out = append(out, x)
	}
	return out
}

func blocked(failed map[string][]diag.Diagnostic) []Diagnostic {
	pkgs := make([]string, 0, len(failed))
	for pkg := range failed {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	var out []Diagnostic
	for _, pkg := range pkgs {
		for _, d := range failed[pkg] {
			if d.Code == diag.CompileBlocked {
				out = append(out, Diagnostic{Code: d.Code, Message: d.Message, Severity: severity(d)})
			}
		}
	}
	return out
}

func failed(err error) []Diagnostic {
	return []Diagnostic{{Severity: "error", Message: err.Error()}}
}

func (p *Playground) Compile(files map[string]string) []Diagnostic {
	placed, err := place(files)
	if err != nil {
		return failed(err)
	}
	p.ws.Sync(placed)
	snap := p.ws.Snapshot()
	return diagnostics(snap.Sources, snap.Diagnostics)
}

func record(frame *uint64, r log.Record) Line {
	var b strings.Builder
	b.WriteString(r.Msg)
	for _, f := range r.Fields {
		if f.Key == "stack" || f.Key == "go" {
			continue
		}
		b.WriteString(" " + f.Key + "=" + fmt.Sprint(f.Val))
	}
	return Line{Frame: *frame, Level: r.Level.String(), Channel: string(r.Channel), Text: b.String()}
}

func shortName(typ string) string {
	return typ[strings.LastIndexByte(typ, '.')+1:]
}

func (p *Playground) spawn(rt *runtime.Runtime) *scene.Scene {
	s := scene.New(p.types, nil, scene.Options{})
	for _, typ := range rt.Linked() {
		h := s.Create(s.Root(), shortName(typ))
		*scene.Add[render2d.SpriteRenderer](s, h) = render2d.DefaultSpriteRenderer()
		sc := scene.Add[scriptcomp.ScriptComponent](s, h)
		sc.Attachments = append(sc.Attachments, scriptcomp.Attachment{TypeName: typ, Enabled: true})
	}
	s.Flush()
	s.UpdateTransforms()
	return s
}

func (p *Playground) Run(files map[string]string, frames int) Result {
	switch {
	case frames <= 0:
		frames = DefaultFrames
	case frames > MaxFrames:
		frames = MaxFrames
	}
	placed, err := place(files)
	if err != nil {
		return Result{Diagnostics: failed(err)}
	}
	p.ws.Sync(placed)
	gen, ds := p.ws.Build()
	res := Result{Diagnostics: diagnostics(gen.Sources, ds)}
	if len(gen.Failed) > 0 || diag.HasErrors(ds) {
		res.Diagnostics = append(res.Diagnostics, blocked(gen.Failed)...)
		return res
	}
	pkgs := make([]string, 0, len(gen.Modules))
	for pkg := range gen.Modules {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	mods := make([]*bytecode.Module, len(pkgs))
	for i, pkg := range pkgs {
		mods[i] = gen.Modules[pkg]
	}
	var frame uint64
	lg := log.New(log.FuncSink(func(r log.Record) {
		if strings.HasPrefix(r.Msg, "script stopped: ") {
			return
		}
		res.Log = append(res.Log, record(&frame, r))
	}))
	lg.SetLevel(log.Debug)
	rt := runtime.New(runtime.Options{Bindings: p.reg, Schema: p.types.Schema(), Log: lg, VM: playmods.SandboxVM()})
	defer rt.Close()
	if err := rt.Link(mods); err != nil {
		res.Diagnostics = append(res.Diagnostics, failed(err)...)
		return res
	}
	s := p.spawn(rt)
	if err := rt.Instantiate(s); err != nil {
		res.Diagnostics = append(res.Diagnostics, failed(err)...)
		return res
	}
	w := engine.NewWorld("playground")
	w.Input = &input.State{}
	w.SetService(scene.ServiceID, s)
	w.SetService(runtime.ServiceID, rt)
	e, detach, err := p.attach(w, s, lg)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, failed(err)...)
		return res
	}
	defer detach()
	for i := 0; i < frames; i++ {
		frame = uint64(i + 1)
		e.TickWorld(w, engine.ScopePlay, 1.0/60)
	}
	res.Frames = frames
	for _, f := range rt.Faults() {
		x := Fault{Script: f.Type, Node: f.Path, Code: f.Fault.Code, Message: f.Fault.Message}
		if len(f.Fault.Stack) > 0 {
			x.File = strings.TrimPrefix(f.Fault.Stack[0].File, ScriptsDir+"/")
			x.Line = int(f.Fault.Stack[0].Line)
		}
		res.Faults = append(res.Faults, x)
	}
	return res
}

func utf16Offsets(src []byte) []int {
	out := make([]int, len(src)+1)
	u := 0
	for i := 0; i < len(src); {
		r, n := utf8.DecodeRune(src[i:])
		for k := 0; k < n; k++ {
			out[i+k] = u
		}
		if r >= 0x10000 {
			u += 2
		} else {
			u++
		}
		i += n
	}
	out[len(src)] = u
	return out
}

func tokenKind(src []byte, start int, keyword bool) string {
	c := src[start]
	switch {
	case keyword:
		return "keyword"
	case c == '"' || c == '`':
		return "string"
	case c >= '0' && c <= '9':
		return "number"
	case c == '@':
		return "attr"
	case c == '_' || c >= 'a' && c <= 'z':
		return "ident"
	case c >= 'A' && c <= 'Z':
		return "type"
	}
	return "punct"
}

func Tokens(text string) []Token {
	src := []byte(text)
	toks, comments, _ := lexer.TokenizeAll(1, src, source.NewInterner())
	off := utf16Offsets(src)
	out := make([]Token, 0, len(toks)+len(comments))
	for _, t := range toks {
		s, e := int(t.Start), int(t.End)
		if e <= s || e > len(src) || t.Text == "" {
			continue
		}
		out = append(out, Token{Start: off[s], End: off[e], Kind: tokenKind(src, s, t.Kind.IsKeyword())})
	}
	for _, c := range comments {
		s, e := int(c.Start), min(int(c.End), len(src))
		out = append(out, Token{Start: off[s], End: off[e], Kind: "comment"})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	return out
}

func (p *Playground) Stubs() map[string]string {
	snap := p.ws.Snapshot()
	out := map[string]string{}
	for _, sp := range langsvc.StubPaths(snap) {
		if st, ok := langsvc.StubFor(snap, sp); ok {
			out[sp] = st.Text
		}
	}
	return out
}

func (p *Playground) Examples() []Example {
	out := make([]Example, 0, len(p.proj.scenes)+8)
	for _, s := range p.proj.scenes {
		out = append(out, Example{ID: ScenePrefix + s.name, Title: s.title, Code: s.files[s.main], Scene: true})
	}
	out = append(out, Example{ID: "console/donut", Title: "Software 3D donut (console)", Code: donutSource})
	for _, e := range p.docs.All() {
		n := 0
		for _, s := range e.Snippets {
			if !s.Run || s.Lang != "aes" {
				continue
			}
			n++
			title := e.Title
			if n > 1 {
				title += " " + strconv.Itoa(n)
			}
			out = append(out, Example{ID: e.ID + "/" + strconv.Itoa(n), Title: title, Code: s.Code})
		}
	}
	return out
}

type request struct {
	Files   map[string]string `json:"files"`
	Frames  int               `json:"frames"`
	Source  string            `json:"source"`
	Example string            `json:"example"`
	Count   int               `json:"count"`
	Animate bool              `json:"animate"`
	Lose    bool              `json:"lose"`
}

func (p *Playground) Call(method string, payload []byte) ([]byte, error) {
	var req request
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
	}
	var out any
	switch method {
	case "compile":
		out = p.Compile(req.Files)
	case "run":
		p.Stop()
		out = p.Run(req.Files, req.Frames)
	case "tokens":
		out = Tokens(req.Source)
	case "stubs":
		out = p.Stubs()
	case "examples":
		out = p.Examples()
	case "play":
		out = p.Play(req.Example, req.Files)
	case "stop":
		p.Stop()
		out = true
	case "poll":
		out = p.Poll()
	case "bench":
		out = p.Bench(req.Count, req.Animate)
	case "loseContext":
		out = p.LoseContext(req.Lose)
	default:
		return nil, errors.New("unknown method " + strconv.Quote(method))
	}
	return json.Marshal(out)
}

func (p *Playground) Reply(method, payload string) (reply string) {
	defer func() {
		if r := recover(); r != nil {
			reply = errorReply(fmt.Sprint("playground crashed: ", r))
		}
	}()
	out, err := p.Call(method, []byte(payload))
	if err != nil {
		return errorReply(err.Error())
	}
	return string(out)
}

func errorReply(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}

func Format(res Result) []string {
	var out []string
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			out = append(out, strconv.Itoa(d.Line)+":"+strconv.Itoa(d.Col)+" "+d.Code+" "+d.Message)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, l := range res.Log {
		out = append(out, "["+strconv.FormatUint(l.Frame, 10)+"] "+l.Text)
	}
	for _, f := range res.Faults {
		out = append(out, f.Script+" stopped with "+f.Code+": "+f.Message)
	}
	return append(out, strconv.Itoa(res.Frames)+" frames")
}

func (p *Playground) TryScenes() []docs.Entry {
	var out []docs.Entry
	for _, e := range p.docs.All() {
		if e.Try != "" {
			out = append(out, e)
		}
	}
	return out
}
