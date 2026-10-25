package services

import (
	"image"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/prefabs"
	"aego/editor/core/selection"
	"aego/editor/core/shape"
	"aego/editor/core/templates"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	"aego/ui/core"
	"aego/ui/dnd"
)

type Frame struct {
	UI      *core.Context
	DnD     *dnd.State
	Now     float64
	Focused bool
}

type Notices interface {
	Notify(l context.Level, text string)
	Ask(q context.Question)
}

type Navigation interface {
	Open(l context.Location)
}

type Problems interface {
	ScriptProblems(dst []context.ScriptProblem) ([]context.ScriptProblem, uint64)
	FirstError(pkg string) (context.ScriptProblem, int, bool)
}

type AssetOps interface {
	AttachScriptType(nodes []ids.NodeID, typeName string) error
	ScriptDir(typeName string) string
	CopyAssets(list []ids.AssetID) bool
	ImportImage(dir, stem string, img image.Image) (ids.AssetID, string, error)
}

type Selection interface {
	Select(nodes ...ids.NodeID)
	AddToSelection(nodes ...ids.NodeID)
	ToggleSelection(n ids.NodeID)
	IsSelected(n ids.NodeID) bool
	Selected() []ids.NodeID
	SelectionState() *selection.Service
}

type Documents interface {
	Doc() *document.SceneDocument
	Documents() *document.Manager
}

type SceneOps interface {
	Create(parent ids.NodeID, pl templates.Place, at *mathx.Vec2, onCreate func([]ids.NodeID))
	Instantiate(t templates.Template, parent ids.NodeID, index int) error
	InstantiateAt(t templates.Template, at *mathx.Vec2) error
	InstantiateAt3(t templates.Template, at mathx.Vec3) error
	AttachScript(nodes []ids.NodeID, script ids.AssetID)
	RenameScene(d *document.SceneDocument)
	SaveSceneAs(d *document.SceneDocument)
}

type Prefabs interface {
	Info(d *document.SceneDocument, node ids.NodeID) (*prefabs.Info, bool)
	Row(d *document.SceneDocument, node ids.NodeID) prefabs.Row
	Locked(d *document.SceneDocument, node ids.NodeID) bool
	Targets(d *document.SceneDocument, node ids.NodeID) []prefabs.Target
	Apply(d *document.SceneDocument, c prefabs.Change) error
	Revert(d *document.SceneDocument, c prefabs.Change) error
	Unpack(d *document.SceneDocument, node ids.NodeID, deep bool) error
	CreateFrom(d *document.SceneDocument, nodes []ids.NodeID, dir string) error
	CreateVariant(id ids.AssetID) error
	Open(id ids.AssetID) error
	SelectAsset(id ids.AssetID)
	FindInstances(id ids.AssetID)
}

type Viewport struct {
	UI          *core.Context
	GPU         gpu.Device
	Renderer    *render.Renderer
	Shapes      *shape.Registry
	Overlay     *context.Overlays
	OnRender    func(func())
	RenderWorld func(w *engine.World)
	GameHUD     []func(ui *core.Context, r mathx.Rect)
	Bare        bool
}

var (
	_ Notices    = (*context.Editor)(nil)
	_ Navigation = (*context.Editor)(nil)
	_ Problems   = (*context.Editor)(nil)
	_ Selection  = (*context.Editor)(nil)
	_ Documents  = (*context.Editor)(nil)
)
