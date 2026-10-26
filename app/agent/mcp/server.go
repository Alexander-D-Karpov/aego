package mcp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aego/app/agent"
	"aego/app/docstool"
	"aego/asset/pipeline"
	"aego/engine"
	"aego/engine/docs"
	"aego/project"
	"aego/sdk"
)

const (
	latest         = "2025-06-18"
	codeParse      = -32700
	codeInvalid    = -32600
	codeMethod     = -32601
	codeParams     = -32602
	codeInternal   = -32603
	codeNoResource = -32002
)

var versions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

var skipDirs = []string{".aego", ".git", "exports", "native"}

const instructions = `Tools for one Aego project. Paths are relative to the project root; a node is a path from the root (Player/Sprite) or #<id> from scene_tree; values use scene syntax: 2.5, [1, 2], #ff8800ff, Kinematic, "text", asset("<id>") or a project path. Read schema_show before setting fields of an unfamiliar component and docs_search / docs_read for how the engine works. Every write tool takes dry-run; scene_apply applies a whole transaction at once. Check results with scene_validate, project_validate and scene_run. A sprite with no texture yet is drawn, not built from colored rectangles: asset_pixels takes a character grid and a palette (frames give an atlas and a flipbook), asset_svg smooth shapes for tiles, UI and icons; both answer with a preview to check. Draw in the style of project.aego's art section (project_get art): its palette, and without a palette argument 0-9 and a-z are its colors.`

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type note struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

type resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type Options struct {
	Kit      *agent.Kit
	Docs     func() (*docs.Registry, error)
	Watch    bool
	Interval time.Duration
	Log      io.Writer
	Version  string
	Source   Source
	Extra    []Extra
	Tools    []Tool
	// DocResults caps docs_search for the agent in the editor; zero leaves the caller's limit.
	DocResults func() int
}

type Server struct {
	opt      Options
	enc      *json.Encoder
	wmu      sync.Mutex
	smu      sync.Mutex
	subs     map[string]bool
	known    map[string]bool
	ready    atomic.Bool
	docsOnce sync.Once
	docs     *docs.Registry
	docsErr  error
}

func Main(p *project.Project, args []string, stdin io.Reader, stdout, stderr io.Writer, opt agent.Options) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	watch := fs.Bool("watch", false, "send resources/updated when project files change")
	interval := fs.Duration("interval", 500*time.Millisecond, "how often --watch polls the project")
	fs.String("project", "", "project directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if addr, token, ok := agent.EditorProxy(p.Root); ok {
		if err := Relay(stdin, stdout, addr, token); err != nil {
			fmt.Fprintln(stderr, "aego mcp:", err)
			return 1
		}
		return 0
	}
	kit, err := agent.NewKit(p, opt)
	if err != nil {
		fmt.Fprintln(stderr, "aego:", err)
		return 1
	}
	err = Serve(stdin, stdout, Options{
		Kit:      kit,
		Docs:     func() (*docs.Registry, error) { return docstool.Registry(p, sdk.AdaptAll(opt.Mods)) },
		Watch:    *watch,
		Interval: *interval,
		Log:      stderr,
		Version:  engine.Version,
	})
	if err != nil {
		fmt.Fprintln(stderr, "aego mcp:", err)
		return 1
	}
	return 0
}

func newServer(opt Options) *Server {
	if opt.Source == nil && opt.Kit != nil {
		opt.Source = kitSource{kit: opt.Kit}
	}
	return &Server{opt: opt, subs: map[string]bool{}, known: map[string]bool{}}
}

func Serve(in io.Reader, out io.Writer, opt Options) error {
	s := newServer(opt)
	s.enc = json.NewEncoder(out)
	s.enc.SetEscapeHTML(false)
	if opt.Watch {
		stop, err := s.watch()
		if err != nil {
			return err
		}
		defer stop()
	}
	dec := json.NewDecoder(in)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			s.send(reply{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: codeParse, Message: err.Error()}})
			return err
		}
		s.handle(raw)
	}
}

func (s *Server) send(v any) {
	if s.enc == nil {
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.enc.Encode(v); err != nil && s.opt.Log != nil {
		fmt.Fprintln(s.opt.Log, "aego mcp:", err)
	}
}

// respond answers one JSON-RPC message; a notification has no reply.
func (s *Server) respond(raw json.RawMessage) (reply, bool) {
	var r request
	if err := json.Unmarshal(raw, &r); err != nil || r.Method == "" {
		id := r.ID
		if len(id) == 0 {
			id = json.RawMessage("null")
		}
		return reply{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: codeInvalid, Message: "invalid request"}}, true
	}
	res, rerr := s.dispatch(r)
	if len(r.ID) == 0 {
		return reply{}, false
	}
	if rerr != nil {
		return reply{JSONRPC: "2.0", ID: r.ID, Error: rerr}, true
	}
	return reply{JSONRPC: "2.0", ID: r.ID, Result: res}, true
}

func (s *Server) handle(raw json.RawMessage) {
	if rep, ok := s.respond(raw); ok {
		s.send(rep)
	}
}

func (s *Server) dispatch(r request) (any, *rpcError) {
	switch r.Method {
	case "initialize":
		return s.initialize(r.Params), nil
	case "notifications/initialized":
		s.ready.Store(true)
		return nil, nil
	case "notifications/cancelled":
		return nil, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.tools()}, nil
	case "tools/call":
		return s.call(r.Params)
	case "resources/list":
		return s.resources()
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": templates()}, nil
	case "resources/read":
		return s.readResource(r.Params)
	case "resources/subscribe", "resources/unsubscribe":
		return s.subscribe(r.Method, r.Params)
	}
	return nil, &rpcError{Code: codeMethod, Message: "unknown method " + r.Method}
}

func (s *Server) initialize(params json.RawMessage) any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	json.Unmarshal(params, &p)
	v := latest
	if slices.Contains(versions, p.ProtocolVersion) {
		v = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": v,
		"capabilities": map[string]any{
			"tools":     map[string]any{"listChanged": false},
			"resources": map[string]any{"subscribe": true, "listChanged": true},
		},
		"serverInfo":   map[string]any{"name": "aego", "version": s.opt.Version},
		"instructions": instructions,
	}
}

func object(required []string, props map[string]any) map[string]any {
	sch := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		sch["required"] = required
	}
	return sch
}

func prop(typ, doc string) map[string]any {
	return map[string]any{"type": typ, "description": doc}
}

func (s *Server) tools() []tool {
	var list []tool
	for _, c := range agent.Commands() {
		list = append(list, tool{
			Name:        c.Tool(),
			Description: c.Doc + "\nCLI: aego " + c.Usage(),
			InputSchema: c.Schema(),
			Annotations: map[string]any{"readOnlyHint": !c.Write, "destructiveHint": c.Destroys, "openWorldHint": false},
		})
	}
	ro := map[string]any{"readOnlyHint": true, "openWorldHint": false}
	list = append(list,
		tool{Name: "docs_search", Description: "search the engine and project documentation", Annotations: ro,
			InputSchema: object([]string{"query"}, map[string]any{"query": prop("string", "words to look for"), "limit": prop("integer", "results, 20 by default")})},
		tool{Name: "docs_read", Description: "read a documentation page by id, from docs_search or a ref: link", Annotations: ro,
			InputSchema: object([]string{"id"}, map[string]any{"id": prop("string", "page id")})},
	)
	for _, t := range s.opt.Tools {
		list = append(list, tool{Name: t.Name, Description: t.Description, InputSchema: t.Schema,
			Annotations: map[string]any{"readOnlyHint": t.ReadOnly, "openWorldHint": false}})
	}
	return list
}

func command(name string) *agent.Command {
	for _, c := range agent.Commands() {
		if c.Tool() == name {
			return c
		}
	}
	return nil
}

func result(v any, failed bool) map[string]any {
	text, ok := v.(string)
	if !ok {
		b, _ := json.Marshal(v)
		text = string(b)
	}
	content := []any{map[string]any{"type": "text", "text": text}}
	if o, ok := v.(*agent.Output); ok {
		for _, img := range o.Images {
			content = append(content, map[string]any{"type": "image", "data": base64.StdEncoding.EncodeToString(img.Data), "mimeType": img.Mime})
		}
	}
	out := map[string]any{"content": content, "isError": failed}
	if !ok && !failed {
		out["structuredContent"] = v
	}
	return out
}

func (s *Server) call(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeParams, Message: err.Error()}
	}
	for _, t := range s.opt.Tools {
		if t.Name == p.Name {
			text, failed := t.Run(p.Arguments)
			return result(text, failed), nil
		}
	}
	switch p.Name {
	case "docs_search":
		return s.docsSearch(agent.Args(p.Arguments)), nil
	case "docs_read":
		reg, err := s.registry()
		if err != nil {
			return result(err.Error(), true), nil
		}
		text, _, err := page(reg, agent.Args(p.Arguments).Str("id"))
		if err != nil {
			return result(err.Error(), true), nil
		}
		return result(text, false), nil
	}
	c := command(p.Name)
	if c == nil {
		return nil, &rpcError{Code: codeParams, Message: "unknown tool " + p.Name}
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	out, err := s.opt.Source.Run(c.Name, agent.Args(p.Arguments))
	if err != nil {
		return result(err.Error(), true), nil
	}
	return result(out, false), nil
}

func (s *Server) registry() (*docs.Registry, error) {
	s.docsOnce.Do(func() {
		if s.opt.Docs == nil {
			s.docsErr = errors.New("documentation is not available")
			return
		}
		s.docs, s.docsErr = s.opt.Docs()
	})
	return s.docs, s.docsErr
}

func (s *Server) docsSearch(a agent.Args) any {
	reg, err := s.registry()
	if err != nil {
		return result(err.Error(), true)
	}
	limit, err := a.Int("limit", 20)
	if err != nil {
		return result(err.Error(), true)
	}
	if s.opt.DocResults != nil {
		if n := s.opt.DocResults(); n > 0 {
			limit = min(limit, n)
		}
	}
	type hit struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Kind    string `json:"kind"`
		Parent  string `json:"parent,omitempty"`
		Summary string `json:"summary,omitempty"`
	}
	hits := []hit{}
	for _, e := range reg.Search(a.Str("query"), limit) {
		kind := e.Kind.String()
		if e.Parent == docs.EventsID {
			kind = "event"
		}
		hits = append(hits, hit{ID: e.ID, Title: e.Title, Kind: kind, Parent: e.Parent, Summary: e.Summary})
	}
	return result(map[string]any{"results": hits}, false)
}

func page(reg *docs.Registry, id string) (string, string, error) {
	e, ok := reg.Resolve(id)
	if !ok {
		e, ok = reg.Get(id)
	}
	if !ok {
		return "", "", errors.New("no documentation page " + id)
	}
	if text := reg.Compose(e.ID); text != "" {
		return text, "text/markdown", nil
	}
	return "# " + e.Title + "\n\n" + e.Body, "text/markdown", nil
}

func docsIndex(reg *docs.Registry) string {
	var b strings.Builder
	b.WriteString("# Aego documentation\n\nRead a page as aego://docs/<id> or with docs_read; search with docs_search.\n")
	for _, g := range reg.Groups() {
		roots := reg.GroupRoots(g.ID)
		if len(roots) == 0 {
			continue
		}
		b.WriteString("\n## " + g.Title + "\n\n")
		for _, e := range roots {
			b.WriteString("- `" + e.ID + "` " + e.Title)
			if e.Summary != "" {
				b.WriteString(": " + e.Summary)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func templates() []map[string]any {
	return []map[string]any{
		{"uriTemplate": "aego://docs/{id}", "name": "documentation page", "mimeType": "text/markdown"},
		{"uriTemplate": "aego://schema/{type}", "name": "component or data type", "mimeType": "application/json"},
		{"uriTemplate": "aego://file/{path}", "name": "project document", "mimeType": "text/plain"},
	}
}

func fileURI(rel string) string {
	return "aego://file/" + rel
}

func document(rel string) bool {
	switch filepath.Ext(rel) {
	case agent.SceneExt, agent.PrefabExt, ".aedata", ".aetable":
		return true
	}
	return false
}

func (s *Server) resources() (any, *rpcError) {
	list := []resource{
		{URI: "aego://project/manifest", Name: project.FileName, Description: "the project file", MimeType: "text/plain"},
		{URI: "aego://project/problems", Name: "problems", Description: "what project_validate reports", MimeType: "application/json"},
		{URI: "aego://docs/index", Name: "documentation", Description: "guide and reference roots", MimeType: "text/markdown"},
		{URI: "aego://schema/index", Name: "types", Description: "component and data types", MimeType: "application/json"},
	}
	for _, x := range s.opt.Extra {
		list = append(list, resource{URI: x.URI, Name: x.Name, Description: x.Description, MimeType: x.MimeType})
	}
	paths, err := s.opt.Source.Paths()
	if err != nil {
		return nil, &rpcError{Code: codeInternal, Message: err.Error()}
	}
	for _, p := range paths {
		if document(p) {
			list = append(list, resource{URI: fileURI(p), Name: p, MimeType: "text/plain"})
		}
	}
	return map[string]any{"resources": list}, nil
}

func (s *Server) runJSON(name string, a agent.Args) (string, string, error) {
	out, err := s.opt.Source.Run(name, a)
	if err != nil {
		return "", "", err
	}
	b, err := json.MarshalIndent(out, "", "  ")
	return string(b), "application/json", err
}

func (s *Server) read(uri string) (string, string, error) {
	for _, x := range s.opt.Extra {
		if x.URI == uri {
			text, err := x.Read()
			return text, x.MimeType, err
		}
	}
	rest, ok := strings.CutPrefix(uri, "aego://")
	if !ok {
		return "", "", errors.New("not an aego resource")
	}
	kind, arg, _ := strings.Cut(rest, "/")
	arg, err := url.PathUnescape(arg)
	if err != nil {
		return "", "", err
	}
	switch kind {
	case "project":
		switch arg {
		case "manifest":
			b, err := os.ReadFile(s.opt.Source.Project().FilePath())
			return string(b), "text/plain", err
		case "problems":
			return s.runJSON("project.validate", agent.Args{})
		}
	case "docs":
		reg, err := s.registry()
		if err != nil {
			return "", "", err
		}
		if arg == "index" {
			return docsIndex(reg), "text/markdown", nil
		}
		return page(reg, arg)
	case "schema":
		if arg == "index" {
			return s.runJSON("schema.list", agent.Args{})
		}
		return s.runJSON("schema.show", agent.Args{"type": arg})
	case "file":
		rel, err := s.opt.Source.Rel(arg)
		if err != nil {
			return "", "", err
		}
		b, err := os.ReadFile(filepath.Join(s.opt.Source.Project().Root, filepath.FromSlash(rel)))
		return string(b), "text/plain", err
	}
	return "", "", errors.New("unknown resource " + uri)
}

func (s *Server) readResource(params json.RawMessage) (any, *rpcError) {
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeParams, Message: err.Error()}
	}
	text, mime, err := s.read(p.URI)
	if err != nil {
		return nil, &rpcError{Code: codeNoResource, Message: err.Error()}
	}
	return map[string]any{"contents": []any{map[string]any{"uri": p.URI, "mimeType": mime, "text": text}}}, nil
}

func (s *Server) subscribe(method string, params json.RawMessage) (any, *rpcError) {
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.URI == "" {
		return nil, &rpcError{Code: codeParams, Message: "uri is needed"}
	}
	s.smu.Lock()
	if method == "resources/subscribe" {
		s.subs[p.URI] = true
	} else {
		delete(s.subs, p.URI)
	}
	s.smu.Unlock()
	return map[string]any{}, nil
}

func (s *Server) watch() (func(), error) {
	root := s.opt.Source.Project().Root
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && slices.Contains(skipDirs, d.Name()) {
			return filepath.SkipDir
		}
		if rel, err := filepath.Rel(root, p); err == nil && document(rel) {
			s.known[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	interval := s.opt.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return pipeline.PollWatcher{}.Start(root, skipDirs, interval, s.changed)
}

func (s *Server) changed(list []pipeline.Change) {
	if !s.ready.Load() {
		return
	}
	root := s.opt.Source.Project().Root
	touched := map[string]bool{"aego://project/problems": true}
	listChanged := false
	s.smu.Lock()
	for _, c := range list {
		p := c.Path
		if filepath.IsAbs(p) {
			if r, err := filepath.Rel(root, p); err == nil {
				p = r
			}
		}
		rel := strings.TrimSuffix(filepath.ToSlash(p), pipeline.MetaSuffix)
		if rel == project.FileName {
			touched["aego://project/manifest"] = true
			continue
		}
		touched[fileURI(rel)] = true
		if !document(rel) {
			continue
		}
		gone := c.Kind == pipeline.ChangeRemove
		if gone == s.known[rel] {
			listChanged = true
		}
		s.known[rel] = !gone
	}
	var uris []string
	for u := range touched {
		if s.subs[u] {
			uris = append(uris, u)
		}
	}
	s.smu.Unlock()
	sort.Strings(uris)
	for _, u := range uris {
		s.send(note{JSONRPC: "2.0", Method: "notifications/resources/updated", Params: map[string]any{"uri": u}})
	}
	if listChanged {
		s.send(note{JSONRPC: "2.0", Method: "notifications/resources/list_changed"})
	}
}

// Definitions is what the model reads about this server before the first message: its instructions and the tool definitions.
func (s *Server) Definitions() string {
	b, _ := json.Marshal(s.tools())
	return instructions + "\n" + string(b)
}
