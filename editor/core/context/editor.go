package context

import (
	"fmt"
	"image"
	"strings"
	"time"

	"aego/asset"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/data"
	"aego/editor/core/animedit"
	"aego/editor/core/command"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/props"
	"aego/editor/core/selection"
	"aego/editor/core/shape"
	"aego/engine"
	"aego/engine/i18n"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/project"
	"aego/render"
	"aego/render/texture"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/dnd"
	"aego/vfs"
)

// Scene view tools stored in Settings.Tool.
const (
	ToolSelect uint8 = iota
	ToolMove
	ToolRotate
	ToolScale
)

const (
	KindFiles  = "files"
	KindAsset  = "asset"
	KindAssets = "assets"
	KindNodes  = "nodes"
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
	World() *engine.World
}

type ThumbSource interface {
	ByID(id ids.AssetID) (texture.Region, bool)
}

const (
	ProblemScript = "script"
	ProblemScene  = "scene"
)

type ScriptProblem struct {
	Category string
	Package  string
	Path     string
	Line     int
	Col      int
	Node     ids.NodeID
	Code     string
	Error    bool
	Block    bool
	Message  string
	Help     string
}

func (p ScriptProblem) Script() bool {
	return p.Category == "" || p.Category == ProblemScript
}

type ProblemSource func(dst []ScriptProblem) ([]ScriptProblem, uint64)

type ScriptTypeStats struct {
	Name      string
	Instances int
	Calls     uint64
	Time      time.Duration
	Faults    int
}

type ScriptCoroutineStats struct {
	Active          int
	Sleeping        int
	ResumesPerFrame uint64
	Canceled        uint64
}

type ScriptSignalStats struct {
	Connections      int
	EmitsPerFrame    uint64
	HandlersPerFrame uint64
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
	Coroutines   ScriptCoroutineStats
	Signals      ScriptSignalStats
	Types        []ScriptTypeStats
}

type ScriptHost interface {
	Errors() int
	Attach(w *engine.World, s *scene.Scene) (func(), error)
	Problems() ([]ScriptProblem, uint64)
	Stats() (ScriptStats, bool)
	Status() (generation uint64, latestFailed bool)
	Watch(w *engine.World, node scene.NodeHandle, script, field string, lane int) (text string, num float64, isNum bool, ok bool)
}

type Editor struct {
	ScenePreviews  []func(*frame.Runner)
	CameraPreview  func() *scene.Scene
	EffectPreview  func() (ids.NodeID, string)
	GamePostOutput func(*document.SceneDocument) (*engine.World, view.ID, frame.PostPreview)
	ImageGenerator ImageGenerator
	BeforeSwap     func(*pipeline.Batch) func()
	Engine         *engine.Engine
	Renderer       *render.Renderer
	Project        *project.Project
	Pipeline       *pipeline.Pipeline
	Loader         *loader.Loader
	Types          *scene.Types
	Docs           *document.Manager
	Selection      *selection.Service
	Commands       *command.Registry
	UI             *core.Context
	DnD            *dnd.State
	Clipboard      DataClipboard
	Log            *log.ChannelLogger
	Settings       SceneSettings
	ImageContent   func(ids.AssetID, string) (image.Image, string, error)
	Thumbs         ThumbSource
	Scripts        ScriptHost
	Code           ScriptNav
	Shapes         *shape.Registry
	Problems       *problems.Service
	Sections       *module.Sections
	Anim           *animedit.Registry
	AnimFiles      animedit.Files

	ShowProblems func(filter string)
	RevealPath   func(rel string)
	OpenScript   func(rel string, line int)
	ShowDocs     func(ref, query string)
	ShowEvents   func(frame uint64)
	Crash        func(where string, rec any, stack []byte)
	NewScene     func(rel string) error
	NewPrefab    func(rel string) error
	// CreateAsset uses the shared Assets history for source creation.
	CreateAsset      func(rel string, source []byte) (ids.AssetID, error)
	AssetTransaction func(fn func() error) error
	InspectAsset     func(ids.AssetID)
	FallbackAsset    func(*document.AssetDocument) error

	FocusedPanel  string
	SceneLighting LightingViewBridge
	AssetRev      uint64
	RenderStats   func() render.Stats
	Play          PlayBridge
	Collapsed     map[string]bool
	Pages         []SettingsPage
	Vectors       []VectorLayer
	Profiler      []ProfilerSection
	CreateActions []CreateAction
	AssetViews    map[string]AssetView
	// DrawSpriteSlicer draws the existing grid/alpha tools using the image
	// document's current source metadata, including uncommitted frame edits.
	DrawSpriteSlicer func(ids.AssetID, *asset.SourceMeta)
	AssetPlayers     map[string]AssetPlayer
	Overlay          Overlays
	ShowSettings     func(page string)

	hooks          []func()
	afterViewHooks []func()
	questions      []Question
	notices        []Notice
	scratch        []ids.NodeID
	metadata       *metadataCoordinator
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

func (e *Editor) SelectionState() *selection.Service {
	return e.Selection
}

func (e *Editor) Documents() *document.Manager {
	return e.Docs
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
		en := i18n.EnglishOf(text)
		switch l {
		case LevelError:
			e.Log.Error(en)
		case LevelWarn:
			e.Log.Warn(en)
		default:
			e.Log.Info(en)
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

type AssetsPayload struct {
	Items []AssetPayload
	Dirs  []string
}

type NodesPayload struct {
	Doc   document.ID
	Nodes []ids.NodeID
}

func AssetMatches(fieldKind string, kind asset.ArtifactKind) bool {
	switch {
	case fieldKind == "scene":
		return kind == asset.ArtScene
	case fieldKind == "prefab":
		return kind == asset.ArtPrefab
	case fieldKind == "mesh":
		return kind == asset.ArtMesh
	case fieldKind == "material":
		return kind == asset.ArtMaterial
	case fieldKind == "shader":
		return kind == asset.ArtShader
	case fieldKind == data.KindData || strings.HasPrefix(fieldKind, data.KindData+":"):
		return kind == asset.ArtData
	case fieldKind == data.KindTable || strings.HasPrefix(fieldKind, data.KindTable+":"):
		return kind == asset.ArtTable
	}
	return kind == asset.ArtTexture || kind == asset.ArtSpriteRegion
}

func (e *Editor) CommandItem(id, label string) bool {
	return command.MenuItem(e.UI, e.Commands, id, label)
}

type Question struct {
	Title   string
	Message string
	Buttons []string
	Fields  []string
	Values  []string
	Then    func(choice int)
	Submit  func(values []string)
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

// OnRenderAfterViews schedules a GPU resource consumer after the ordinary view
// render hooks, but before the editor UI is submitted. This avoids depending on
// the order in which workspace panels built their UI that frame.
func (e *Editor) OnRenderAfterViews(fn func()) {
	e.afterViewHooks = append(e.afterViewHooks, fn)
}

func (e *Editor) RunRenderHooks() {
	e.runRenderHooks(&e.hooks)
	e.runRenderHooks(&e.afterViewHooks)
}

func (e *Editor) runRenderHooks(queue *[]func()) {
	current := *queue
	*queue = nil
	for i, fn := range current {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					e.Notify(LevelError, fmt.Sprint("render hook crashed: ", rec))
				}
			}()
			fn()
		}()
		current[i] = nil
	}
	// A callback may enqueue work for the next drain. Keep it, while releasing
	// this frame's captured documents/textures and reusing the queue storage.
	*queue = append(current[:0], (*queue)...)
}

func (e *Editor) ApplyBatch(b *pipeline.Batch) {
	var files vfs.FS
	if e.Pipeline != nil {
		files = e.Pipeline.Artifacts()
	}
	// Working artifact paths use content hashes and retain a disk fallback.
	// A saved import must not discard another open document's working bytes,
	// especially if the device is restored before the next candidate finishes.
	if e.metadata != nil && e.metadata.files != nil {
		files = e.metadata.files
	}
	_ = e.applyBatch(b, files)
}

// applyBatch is shared by saved imports and validated working metadata. The
// latter supplies a private artifact VFS but uses the same loader transaction,
// prefab callbacks, notifications and consumer invalidation.
func (e *Editor) applyBatch(b *pipeline.Batch, files vfs.FS) error {
	if b == nil {
		return nil
	}
	var failure error
	var after func()
	if e.BeforeSwap != nil {
		after = e.BeforeSwap(b)
	}
	if e.Loader != nil && e.Pipeline != nil {
		for id := range b.Failed {
			e.Loader.RejectSource(id)
		}
		swapped := true
		if len(b.Imported) > 0 {
			if err := e.Loader.Swap(b.Imported, files); err != nil {
				swapped = false
				failure = err
				e.Notify(LevelError, i18n.Note("editor.context.hot_reload_failed_keeping_the", "hot reload failed, keeping the previous assets: {error}", i18n.Str("error", err.Error())))
			}
		}
		if swapped {
			for _, id := range b.Removed {
				e.Loader.Drop(id)
			}
		}
	}
	if after != nil {
		after()
	}
	for id, err := range b.Failed {
		e.Notify(LevelWarn, i18n.Note("editor.context.import_failed_for_v_error", "import failed for {v}: {error}", i18n.Str("v", id.String()[:8]), i18n.Str("error", err.Error())))
	}
	e.AssetRev++
	return failure
}

func (e *Editor) PlayWorld() *engine.World {
	if e.Play == nil {
		return nil
	}
	return e.Play.World()
}
