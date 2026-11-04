// Package agentbridge serves the agent tool catalog from inside the editor: tools run on the main thread over the open documents (D341).
package agentbridge

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/app/agentui/backend"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/agentui"
	"aego/editor/core/animedit"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/projectedit"
	"aego/editor/core/services"
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
	Console         func() []services.ConsoleMessage
	ConfirmPlay     bool
	Capture         func(string) ([]byte, uint64, string, error)
	Exporter        services.Exporter
	NewEmptyProject func(string) (string, error)
	SwitchProject   func(string)
	Ctx             *context.Editor
	Modules         *module.Manifest
	OnMain          func(func())
	Command         string
	System          string
	Docs            func() (*docs.Registry, error)
	// Bindings give schema_show the script methods of components.
	Bindings *bind.Registry
	// Timeout is how long a proposal waits for the user, DefaultTimeout when zero.
	Timeout    time.Duration
	OnProposal func(*Proposal)
	// ProjectChanged runs after project.set changed a section, so Project Settings and live settings follow.
	ProjectChanged func()
	// Run runs the game headless for scene.run; without it scene.run reports that runs are not available.
	Run agent.Runner
	// ProxyConfirm makes changes from the CLI and outside agents wait in the Agent panel like the panel's own; off, they apply at once.
	ProxyConfirm func() bool
	// OnExternal runs on the main thread when a call comes through the proxy, so follow mode shows it.
	OnExternal func(tool, args string)
	// Prefabs makes prefabs from open scenes as Create Prefab does, so prefab.create works before the scene is saved.
	Prefabs services.Prefabs
}

type Bridge struct {
	grants                   sync.Map
	playGrant                atomic.Uint64
	captures                 []visualReceipt
	variants                 variantSet
	exportPath, exportTarget string
	exportSeal               [32]byte
	exportGeneration         uint64
	binding                  string
	cfg                      Config
	ctx                      *context.Editor
	sess                     *agent.Session
	http                     *mcp.HTTP
	// docResults caps docs_search; the MCP server reads it off the main thread.
	docResults atomic.Int64
	// panelGeneration revokes queued work from a cancelled panel conversation.
	// External CLI/proxy calls keep their own lifetime and do not use this token.
	panelGeneration atomic.Uint64
	// via names the origin of the change being applied, for its Undo step; proxy serves the CLI and outside agents.
	via   string
	proxy *mcp.HTTP

	proposals []*Proposal
	questions []*Question
	next      int
	auto      bool
	planOnly  bool
	planMu    sync.Mutex
	last      *undoRef
}

// GuideExample is the transaction the Guide shows; a test applies it, so it stays valid.
const GuideExample = `tx version=1 { label="Add Ledge" op "add-node" { name="Ledge" } op "add-comp" { node="Ledge" comp="SpriteRenderer" } op "set" { node="Ledge" path="transform.position" value=[-13, 1] } }`

// Guide is the system prompt the editor adds for the model.
const Guide = "You work inside the Aego editor. Write tools wait until the user applies or rejects the change; a rejected call means the user said no, so ask before trying it again. Edits to open scenes become Undo steps in the editor and are saved by the user. " +
	"Each write call is one card the user has to apply, so send related edits of one scene as one scene_apply transaction: a new node with its components and fields is one call, not a chain of add-node, add-comp and set. Give the transaction a short label; it names the Undo step. Example tx: " + GuideExample + ". Use the single-step tools for one small change. " +
	"Read the selected Console entry as evidence. Read .aeshader and .aemat buffers with editor_material_source by AssetID or project path. Read scripts with script_read; change one with script_edit for a few lines or script_write for the whole text, and add one with script_create. A script attaches to a node like a component: add-comp with its type name (EnemyAI or the full mygame.enemy.EnemyAI), and its exported fields are read and set like component fields (EnemyAI.PointA). Node reference fields take a node path such as Root/PatrolA1. " +
	"Check behaviour with scene_run: it plays the scene as it is in the editor; input presses actions by frame (30 press Jump; 10-60 hold Right; 0-40 axis Move 1), assert checks values at a frame or at any frame (@120 Player.transform.position.y < 3; EnemyLeft.Health.hp == 0; exists Ledge) and fails the run when one does not hold, watch reports node positions, shot captures up to 3 frames as images. A sprite with no texture yet is drawn, not built from colored rectangles: asset_pixels takes a character grid and a palette (frames give an atlas and a flipbook), asset_svg smooth shapes for tiles, UI and icons; both answer with a preview to check. Draw in the style of project.aego's art section (project_get art): its palette, and without a palette argument 0-9 and a-z are its colors. When the user has to choose between ways to do something, call ask_user with the options instead of listing them in your answer."

// Start opens the MCP server on loopback; tools run on the editor's main thread through OnMain.
func Start(cfg Config) (*Bridge, error) {
	c := cfg.Ctx
	b := &Bridge{cfg: cfg, ctx: c, binding: rand.Text()}
	b.panelGeneration.Store(1)
	b.sess = agent.Attach(c.Project, c.Types, cfg.Modules, c.Pipeline, cfg.Bindings, b)
	h, err := mcp.ListenHTTP(mcp.Options{Source: b, Docs: cfg.Docs, Extra: b.resources(), Tools: append(b.tools(), b.visualTools()...), Version: engine.Version,
		DocResults: func() int { return int(b.docResults.Load()) }})
	if err != nil {
		return nil, err
	}
	b.http = h
	return b, nil
}

// Close rejects what still waits for the user and stops the server; call it on the main thread.
func (b *Bridge) Close() error {
	b.RejectAll()
	if b.proxy != nil {
		b.proxy.Close()
	}
	return b.http.Close()
}

// stepName is the Undo step of an applied change: "Agent: Add Ledge", or "via CLI: Add Ledge" for a change from outside.
func (b *Bridge) stepName(label string) string {
	if b.via != "" {
		return b.via + ": " + label
	}
	return "Agent: " + label
}

// ContextBase is what the model reads before the first message from the editor: the guide and the tool definitions.
func (b *Bridge) ContextBase() (string, string) {
	sys := b.cfg.System
	if sys == "" {
		sys = Guide
	}
	return sys, b.http.Definitions()
}

// Backend is how a backend reaches this editor's MCP server.
func (b *Bridge) Backend() backend.Config {
	sys := b.cfg.System
	if sys == "" {
		sys = Guide
	}
	return backend.Config{Binding: b.binding, Dir: b.ctx.Project.Root, Command: b.cfg.Command, MCPURL: b.http.URL, Token: b.http.Token(), System: sys}
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
	generation := b.panelGeneration.Load()
	if name == "scene.run" && b.cfg.Run != nil {
		return b.sceneRun(a)
	}
	c, ok := agent.Find(name)
	if ok && c.Write && !a.Bool("dry-run") {
		if b.PlanOnly() {
			return nil, errPlanOnly
		}
		return b.propose(c, a, generation)
	}
	var out *agent.Output
	var err error
	if e := b.main(func() {
		if b.cancelled(generation) {
			err = errCancelled
			return
		}
		out, err = b.run(name, a)
	}); e != nil {
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

// Editor is the editor the bridge serves, for the host that shows the agent's work.
func (b *Bridge) Editor() *context.Editor {
	return b.ctx
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

// scene is the document the agent works in: during Play the running game's copy of the scene, so its edits show in the game and in the Play changes like the user's own (11b5b).
func (b *Bridge) scene(rel string) *document.SceneDocument {
	if p := b.ctx.Docs.PlayDoc(); p != nil && p.Path == rel {
		return p
	}
	return b.ctx.Docs.Find(rel)
}

// CreatePrefab makes out from a node of the open scene rel and links the node, as one Undo step there.
func (b *Bridge) CreatePrefab(rel, node, out string) (ids.AssetID, error) {
	d := b.scene(rel)
	if d == nil || b.cfg.Prefabs == nil {
		return ids.AssetID{}, errors.New(rel + " is not open in the editor")
	}
	h, err := mutate.Wrap(d.Scene(), rel, d.IsPrefab(), b.env(d.Scene())).Node(node)
	if err != nil {
		return ids.AssetID{}, err
	}
	label := b.stepName("Create Prefab")
	id, err := b.cfg.Prefabs.CreateAt(d, d.Scene().ID(h), out, label)
	if err != nil {
		return ids.AssetID{}, err
	}
	b.note(d, label)
	return id, nil
}

func (b *Bridge) OpenScene(rel string) (*mutate.Document, bool) {
	d := b.scene(rel)
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
	d := b.scene(rel)
	if d == nil {
		return nil, errors.New(rel + " is not open in the editor")
	}
	cmd := commands.NewMutation(b.stepName(label), b.env(d.Scene()), tx.Ops...)
	if err := d.History.Exec(cmd); err != nil {
		return nil, err
	}
	b.note(d, cmd.Label())
	return cmd.Result(), nil
}

// ApplyFile changes a clip or controller as one Undo step of the edited scene, or directly when no scene is open.
func (b *Bridge) ApplyFile(rel string, ctrl bool, label string, tx mutate.Tx) (*mutate.Result, error) {
	e := &animedit.FileEdit{Name: b.stepName(label), Rel: rel, Ctrl: ctrl, Tx: tx, Files: b.ctx.AnimFiles, Loaded: b.loaded}
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
	e := &projectedit.Edit{Name: b.stepName(label), Project: b.ctx.Project, Sections: b.ctx.Sections, ID: id, After: n, Changed: b.cfg.ProjectChanged}
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

// console reads the same bounded log that backs the Console panel.
func (b *Bridge) console() []services.ConsoleMessage {
	if b.cfg.Console == nil {
		return []services.ConsoleMessage{}
	}
	return b.cfg.Console()
}

func (b *Bridge) play() map[string]any {
	m := map[string]any{"running": b.ctx.Docs.InPlay()}
	if d := b.ctx.Docs.Edit(); d != nil {
		m["scene"] = d.Path
	}
	return m
}

// SetBudgets caps what read tools return from now on; call it on the main thread.
func (b *Bridge) SetBudgets(bu agentui.Budgets) {
	b.sess.SetBudget(agent.Budget{TreeNodes: bu.TreeNodes, TreeDepth: bu.TreeDepth, ScriptLines: bu.ScriptLines})
	b.docResults.Store(int64(bu.DocResults))
}

// Rebind invalidates the previous panel conversation without rotating the independent CLI proxy credential.
func (b *Bridge) Rebind() {
	b.http.RotateToken()
	b.binding = rand.Text()
	b.RejectAll()
}
