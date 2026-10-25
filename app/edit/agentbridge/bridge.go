// Package agentbridge serves the agent tool catalog from inside the editor: tools run on the main thread over the open documents (D341).
package agentbridge

import (
	"encoding/json"
	"errors"
	"time"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/app/agentui/backend"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/animedit"
	"aego/editor/core/context"
	"aego/editor/core/history/commands"
	"aego/editor/core/projectedit"
	"aego/engine"
	"aego/engine/docs"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/project"
	"aego/scene"
	"aego/scene/mutate"
	"aego/script/bind"
)

const (
	wait     = 60 * time.Second
	playNote = "Play is running: the change went to the edited scene, not to the running game"
)

var errBusy = errors.New("the editor did not answer in time")

type Config struct {
	Ctx     *context.Editor
	Modules *module.Manifest
	OnMain  func(func())
	Command string
	System  string
	Docs    func() (*docs.Registry, error)
	// Bindings give schema_show the script methods of components.
	Bindings *bind.Registry
	// Timeout is how long a proposal waits for the user, DefaultTimeout when zero.
	Timeout    time.Duration
	OnProposal func(*Proposal)
	// ProjectChanged runs after project.set changed a section, so Project Settings and live settings follow.
	ProjectChanged func()
}

type Bridge struct {
	cfg  Config
	ctx  *context.Editor
	sess *agent.Session
	http *mcp.HTTP

	proposals []*Proposal
	next      int
	auto      bool
	last      *undoRef
}

// GuideExample is the transaction the Guide shows; a test applies it, so it stays valid.
const GuideExample = `tx version=1 { label="Add Ledge" op "add-node" { name="Ledge" } op "add-comp" { node="Ledge" comp="SpriteRenderer" } op "set" { node="Ledge" path="transform.position" value=[-13, 1] } }`

// Guide is the system prompt the editor adds for the model.
const Guide = "You work inside the Aego editor. Write tools wait until the user applies or rejects the change; a rejected call means the user said no, so ask before trying it again. Edits to open scenes become Undo steps in the editor and are saved by the user. " +
	"Each write call is one card the user has to apply, so send related edits of one scene as one scene_apply transaction: a new node with its components and fields is one call, not a chain of add-node, add-comp and set. Give the transaction a short label; it names the Undo step. Example tx: " + GuideExample + ". Use the single-step tools for one small change."

// Start opens the MCP server on loopback; tools run on the editor's main thread through OnMain.
func Start(cfg Config) (*Bridge, error) {
	c := cfg.Ctx
	b := &Bridge{cfg: cfg, ctx: c}
	b.sess = agent.Attach(c.Project, c.Types, cfg.Modules, c.Pipeline, cfg.Bindings, b)
	h, err := mcp.ListenHTTP(mcp.Options{Source: b, Docs: cfg.Docs, Extra: b.resources(), Version: engine.Version})
	if err != nil {
		return nil, err
	}
	b.http = h
	return b, nil
}

// Close rejects what still waits for the user and stops the server; call it on the main thread.
func (b *Bridge) Close() error {
	b.RejectAll()
	return b.http.Close()
}

// Backend is how a backend reaches this editor's MCP server.
func (b *Bridge) Backend() backend.Config {
	sys := b.cfg.System
	if sys == "" {
		sys = Guide
	}
	return backend.Config{Dir: b.ctx.Project.Root, Command: b.cfg.Command, MCPURL: b.http.URL, Token: b.http.Token, System: sys}
}

func (b *Bridge) main(fn func()) error {
	done := make(chan struct{})
	b.cfg.OnMain(func() {
		defer close(done)
		fn()
	})
	select {
	case <-done:
		return nil
	case <-time.After(wait):
		return errBusy
	}
}

// Run serves a tool call: reads run at once, writes become proposals the user applies or rejects (D342).
func (b *Bridge) Run(name string, a agent.Args) (*agent.Output, error) {
	c, ok := agent.Find(name)
	if ok && c.Write && !a.Bool("dry-run") {
		return b.propose(c, a)
	}
	var out *agent.Output
	var err error
	if e := b.main(func() { out, err = b.run(name, a) }); e != nil {
		return nil, e
	}
	return out, err
}

func (b *Bridge) run(name string, a agent.Args) (*agent.Output, error) {
	out, err := agent.Run(b.sess, name, a)
	if err != nil || out == nil {
		return out, err
	}
	if len(out.Files) > 0 {
		b.Wrote(out.Files)
	}
	if c, ok := agent.Find(name); ok && c.Write && b.ctx.Docs.InPlay() {
		out.Notes = append(out.Notes, playNote)
	}
	return out, nil
}

func (b *Bridge) Paths() ([]string, error) {
	var out []string
	err := b.main(func() {
		for _, a := range b.ctx.Pipeline.Assets() {
			out = append(out, a.Path)
		}
	})
	return out, err
}

func (b *Bridge) Project() *project.Project {
	return b.ctx.Project
}

func (b *Bridge) Rel(p string) (string, error) {
	return agent.RelIn(b.ctx.Project.Root, p)
}

func (b *Bridge) env(s *scene.Scene) mutate.Env {
	env := b.sess.Env()
	env.Types, env.Resolver = s.Types(), s.Resolver()
	return env
}

func (b *Bridge) OpenScene(rel string) (*mutate.Document, bool) {
	d := b.ctx.Docs.Find(rel)
	if d == nil {
		return nil, false
	}
	return mutate.Wrap(d.Scene(), rel, d.IsPrefab(), b.env(d.Scene())), true
}

func (b *Bridge) SceneDirty(rel string) bool {
	d := b.ctx.Docs.Find(rel)
	return d != nil && d.Dirty()
}

// ApplyScene runs the transaction as one Undo step of the open document.
func (b *Bridge) ApplyScene(rel, label string, tx mutate.Tx) (*mutate.Result, error) {
	d := b.ctx.Docs.Find(rel)
	if d == nil {
		return nil, errors.New(rel + " is not open in the editor")
	}
	cmd := commands.NewMutation("Agent: "+label, b.env(d.Scene()), tx.Ops...)
	if err := d.History.Exec(cmd); err != nil {
		return nil, err
	}
	b.note(d, cmd.Label())
	return cmd.Result(), nil
}

// ApplyFile changes a clip or controller as one Undo step of the edited scene, or directly when no scene is open.
func (b *Bridge) ApplyFile(rel string, ctrl bool, label string, tx mutate.Tx) (*mutate.Result, error) {
	e := &animedit.FileEdit{Name: "Agent: " + label, Rel: rel, Ctrl: ctrl, Tx: tx, Files: b.ctx.AnimFiles, Loaded: b.loaded}
	d := b.ctx.Docs.Edit()
	if d == nil {
		if err := e.Do(nil); err != nil {
			return nil, err
		}
		return e.Result, nil
	}
	if err := d.History.Exec(e); err != nil {
		return nil, err
	}
	b.note(d, e.Name)
	return e.Result, nil
}

func (b *Bridge) Sections() *module.Sections {
	return b.ctx.Sections
}

// ApplyProject replaces a module section of project.aego as one Undo step of the edited scene.
func (b *Bridge) ApplyProject(id string, n *aetext.Node, label string) error {
	e := &projectedit.Edit{Name: "Agent: " + label, Project: b.ctx.Project, Sections: b.ctx.Sections, ID: id, After: n, Changed: b.cfg.ProjectChanged}
	d := b.ctx.Docs.Edit()
	if d == nil {
		return e.Do(nil)
	}
	if err := d.History.Exec(e); err != nil {
		return err
	}
	b.note(d, e.Name)
	return nil
}

func (b *Bridge) loaded(rel string, data []byte) {
	if b.ctx.Anim != nil {
		b.ctx.Anim.Loaded(rel, data)
	}
}

// Wrote reimports files a tool wrote, so the loader sees them; open scenes reload through the usual external check.
func (b *Bridge) Wrote(rels []string) {
	pl := b.ctx.Pipeline
	if pl == nil {
		return
	}
	batch := &pipeline.Batch{Failed: map[ids.AssetID]error{}}
	seen := map[ids.AssetID]bool{}
	for _, rel := range rels {
		id, ok := pl.Lookup(rel)
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		if nb, err := pl.Reimport(id); err == nil && nb != nil {
			for fid, ferr := range nb.Failed {
				batch.Failed[fid] = ferr
			}
		}
		if e, ok := pl.Index().Lookup(id); ok && !e.Missing {
			batch.Imported = append(batch.Imported, *e)
		}
	}
	b.ctx.ApplyBatch(batch)
}

func jsonText(v any) (string, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	return string(out), err
}

func (b *Bridge) resources() []mcp.Extra {
	read := func(fn func() (any, error)) func() (string, error) {
		return func() (string, error) {
			var v any
			var err error
			if e := b.main(func() { v, err = fn() }); e != nil {
				return "", e
			}
			if err != nil {
				return "", err
			}
			return jsonText(v)
		}
	}
	return []mcp.Extra{
		{URI: "aego://editor/selection", Name: "selection", Description: "selected nodes of the active scene", MimeType: "application/json",
			Read: read(func() (any, error) { return b.selection(), nil })},
		{URI: "aego://editor/scene", Name: "active scene", Description: "node tree of the scene open in the editor, unsaved edits included", MimeType: "application/json",
			Read: read(b.activeTree)},
		{URI: "aego://editor/problems", Name: "problems", Description: "what project_validate reports", MimeType: "application/json",
			Read: read(func() (any, error) { return agent.Run(b.sess, "project.validate", agent.Args{}) })},
		{URI: "aego://editor/console", Name: "console", Description: "recent editor messages", MimeType: "application/json",
			Read: read(func() (any, error) { return b.console(), nil })},
		{URI: "aego://editor/play", Name: "play", Description: "whether the game runs in the editor", MimeType: "application/json",
			Read: read(func() (any, error) { return b.play(), nil })},
	}
}

type nodeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func (b *Bridge) selection() map[string]any {
	d := b.ctx.Docs.Edit()
	if d == nil {
		return map[string]any{"scene": "", "nodes": []nodeRef{}}
	}
	s := d.Scene()
	nodes := []nodeRef{}
	for _, id := range b.ctx.Selected() {
		h := s.Handle(id)
		if h.IsZero() || !s.IsAlive(h) {
			continue
		}
		nodes = append(nodes, nodeRef{ID: "#" + id.String(), Name: s.Name(h), Path: mutate.RootName + s.Path(h)})
	}
	return map[string]any{"scene": d.Path, "nodes": nodes}
}

func (b *Bridge) activeTree() (any, error) {
	d := b.ctx.Docs.Edit()
	if d == nil {
		return nil, errors.New("no scene is open")
	}
	out, err := agent.Run(b.sess, "scene.tree", agent.Args{"file": d.Path})
	if err != nil {
		return nil, err
	}
	return out.Value, nil
}

// console is the last 200 editor messages.
func (b *Bridge) console() []map[string]any {
	out := []map[string]any{}
	levels := [...]string{"info", "warning", "error"}
	ns := b.ctx.Notices()
	if len(ns) > 200 {
		ns = ns[len(ns)-200:]
	}
	for _, n := range ns {
		out = append(out, map[string]any{"level": levels[min(int(n.Level), len(levels)-1)], "text": n.Text})
	}
	return out
}

func (b *Bridge) play() map[string]any {
	m := map[string]any{"running": b.ctx.Docs.InPlay()}
	if d := b.ctx.Docs.Edit(); d != nil {
		m["scene"] = d.Path
	}
	return m
}
