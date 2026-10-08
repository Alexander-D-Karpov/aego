package context

import (
	"fmt"
	"time"

	"aego/asset"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/command"
	"aego/editor/document"
	"aego/editor/history"
	"aego/editor/props"
	"aego/editor/selection"
	"aego/engine"
	"aego/project"
	"aego/render"
	"aego/render/texture"
	"aego/scene"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/dnd"
	"aego/ui/widgets"
)

const (
	KindFiles = "files"
	KindAsset = "asset"
	KindNodes = "nodes"
)

type Level uint8

const (
	LevelInfo Level = iota
	LevelWarn
	LevelError
)

type Notice struct {
	Level Level
	Text  string
	Time  float64
}

type DataClipboard interface {
	core.Clipboard
	SetData(mime string, data []byte, text string) bool
	Data(mime string) ([]byte, bool)
}

type SceneSettings struct {
	Tool       uint8
	Grid       bool
	Snap       bool
	SnapMove   float32
	SnapRotate float32
	SnapScale  float32
}

func DefaultSettings() SceneSettings {
	return SceneSettings{Grid: true, SnapMove: 0.5, SnapRotate: 15, SnapScale: 0.1}
}

type PlayBridge interface {
	CanCopyToEdit(nodes []ids.NodeID, t schema.TypeID) bool
	CopyToEdit(nodes []ids.NodeID, t schema.TypeID) error
}

type ThumbSource interface {
	ByID(id ids.AssetID) (texture.Region, bool)
}

type ScriptProblem struct {
	Package string
	Path    string
	Line    int
	Col     int
	Code    string
	Error   bool
	Message string
}

type ScriptTypeStats struct {
	Name      string
	Instances int
	Calls     uint64
	Time      time.Duration
	Faults    int
}

type ScriptStats struct {
	Instances    int
	Update       int
	Fixed        int
	Instructions uint64
	HostCalls    uint64
	HeapObjects  int
	HeapBytes    int
	GCRuns       uint64
	GCTime       time.Duration
	Faults       int
	BudgetAborts uint64
	Types        []ScriptTypeStats
}

type ScriptHost interface {
	Errors() int
	Attach(w *engine.World, s *scene.Scene) (func(), error)
	Problems() ([]ScriptProblem, uint64)
	Stats() (ScriptStats, bool)
}

type Editor struct {
	Engine    *engine.Engine
	Renderer  *render.Renderer
	Project   *project.Project
	Pipeline  *pipeline.Pipeline
	Loader    *loader.Loader
	Types     *scene.Types
	Docs      *document.Manager
	Selection *selection.Service
	Commands  *command.Registry
	UI        *core.Context
	DnD       *dnd.State
	Clipboard DataClipboard
	Log       *log.ChannelLogger
	Settings  SceneSettings
	Thumbs    ThumbSource
	Scripts   ScriptHost

	ShowProblems func(filter string)
	RevealPath   func(rel string)

	FocusedPanel string
	AssetRev     uint64
	RenderStats  func() render.Stats
	Play         PlayBridge
	Collapsed    map[string]bool

	hooks     []func()
	questions []Question
	notices   []Notice
	scratch   []ids.NodeID
}

func (e *Editor) Now() float64 {
	if e.Engine == nil {
		return 0
	}
	return e.Engine.Platform().Now()
}

func (e *Editor) Doc() *document.SceneDocument {
	return e.Docs.Active()
}

func (e *Editor) Scene() *scene.Scene {
	if d := e.Doc(); d != nil {
		return d.Scene()
	}
	return nil
}

func (e *Editor) History() *history.History {
	if d := e.Doc(); d != nil {
		return d.History
	}
	return nil
}

func (e *Editor) Props() *props.Access {
	if d := e.Doc(); d != nil {
		return d.Props
	}
	return nil
}

func (e *Editor) Selected() []ids.NodeID {
	d := e.Doc()
	if d == nil {
		e.scratch = e.scratch[:0]
		return e.scratch
	}
	e.scratch = e.Selection.Nodes(d.ID, e.scratch)
	return e.scratch
}

func (e *Editor) items(nodes []ids.NodeID) []selection.Item {
	d := e.Doc()
	if d == nil {
		return nil
	}
	out := make([]selection.Item, len(nodes))
	for i, n := range nodes {
		out[i] = selection.Item{Doc: d.ID, Node: n}
	}
	return out
}

func (e *Editor) Select(nodes ...ids.NodeID) {
	e.Selection.Set(e.items(nodes)...)
}

func (e *Editor) AddToSelection(nodes ...ids.NodeID) {
	e.Selection.Add(e.items(nodes)...)
}

func (e *Editor) ToggleSelection(n ids.NodeID) {
	if d := e.Doc(); d != nil {
		e.Selection.Toggle(selection.Item{Doc: d.ID, Node: n})
	}
}

func (e *Editor) IsSelected(n ids.NodeID) bool {
	d := e.Doc()
	return d != nil && e.Selection.Contains(selection.Item{Doc: d.ID, Node: n})
}

func (e *Editor) PruneSelectionOf(doc document.ID) {
	d := e.Docs.Get(doc)
	if d == nil {
		e.Selection.DropDoc(doc)
		return
	}
	s := d.Scene()
	e.Selection.Prune(doc, func(id ids.NodeID) bool {
		h := s.Handle(id)
		return !h.IsZero() && s.IsAlive(h)
	})
}

func (e *Editor) Undo() error {
	h := e.History()
	if h == nil {
		return nil
	}
	err := h.Undo()
	e.PruneSelectionOf(e.Doc().ID)
	return err
}

func (e *Editor) Redo() error {
	h := e.History()
	if h == nil {
		return nil
	}
	err := h.Redo()
	e.PruneSelectionOf(e.Doc().ID)
	return err
}

func (e *Editor) Notify(l Level, text string) {
	if e.Log != nil {
		switch l {
		case LevelError:
			e.Log.Error(text)
		case LevelWarn:
			e.Log.Warn(text)
		default:
			e.Log.Info(text)
		}
	}
	e.notices = append(e.notices, Notice{Level: l, Text: text, Time: e.Now()})
	if len(e.notices) > 32 {
		e.notices = append(e.notices[:0], e.notices[len(e.notices)-32:]...)
	}
}

func (e *Editor) LastNotice() (Notice, bool) {
	if n := len(e.notices); n > 0 {
		return e.notices[n-1], true
	}
	return Notice{}, false
}

func (e *Editor) Notices() []Notice {
	return e.notices
}

type AssetPayload struct {
	ID   ids.AssetID
	Kind asset.ArtifactKind
	Name string
}

type NodesPayload struct {
	Doc   document.ID
	Nodes []ids.NodeID
}

func AssetMatches(fieldKind string, kind asset.ArtifactKind) bool {
	switch fieldKind {
	case "scene":
		return kind == asset.ArtScene
	case "prefab":
		return kind == asset.ArtPrefab
	case "mesh":
		return kind == asset.ArtMesh
	}
	return kind == asset.ArtTexture || kind == asset.ArtSpriteRegion
}

func (e *Editor) CommandItem(id, label string) bool {
	d, ok := e.Commands.Get(id)
	if !ok {
		return false
	}
	if label == "" {
		label = d.Name
	}
	if widgets.MenuItem(e.UI, label, d.Shortcut.String(), e.Commands.Enabled(id), false) {
		e.Commands.Run(id)
		return true
	}
	return false
}

type Question struct {
	Title   string
	Message string
	Buttons []string
	Then    func(choice int)
}

func (e *Editor) Ask(q Question) {
	e.questions = append(e.questions, q)
}

func (e *Editor) TakeQuestions() []Question {
	q := e.questions
	e.questions = nil
	return q
}

func (e *Editor) OnRender(fn func()) {
	e.hooks = append(e.hooks, fn)
}

func (e *Editor) RunRenderHooks() {
	for i, fn := range e.hooks {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					e.Notify(LevelError, fmt.Sprint("render hook crashed: ", rec))
				}
			}()
			fn()
		}()
		e.hooks[i] = nil
	}
	e.hooks = e.hooks[:0]
}

func (e *Editor) ApplyBatch(b *pipeline.Batch) {
	if b == nil {
		return
	}
	if e.Loader != nil && e.Pipeline != nil {
		if len(b.Imported) > 0 {
			if err := e.Loader.Swap(b.Imported, e.Pipeline.Artifacts()); err != nil {
				e.Notify(LevelError, "hot reload failed, keeping the previous assets: "+err.Error())
			}
		}
		for _, id := range b.Removed {
			e.Loader.Drop(id)
		}
	}
	for id, err := range b.Failed {
		e.Notify(LevelWarn, "import failed for "+id.String()[:8]+": "+err.Error())
	}
	e.AssetRev++
}
