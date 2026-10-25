package sceneops

import (
	"image"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/templates"
)

var (
	_ services.SceneOps = (*Ops)(nil)
	_ services.AssetOps = (*Ops)(nil)
)

type Ops struct {
	ctx    *context.Editor
	files  *assetops.Ops
	create func(parent ids.NodeID, pl templates.Place, at *mathx.Vec2, onCreate func([]ids.NodeID))
}

func New(ctx *context.Editor, files *assetops.Ops) *Ops {
	return &Ops{ctx: ctx, files: files}
}

// SetCreate routes Create to the palette (D344).
func (o *Ops) SetCreate(fn func(parent ids.NodeID, pl templates.Place, at *mathx.Vec2, onCreate func([]ids.NodeID))) {
	o.create = fn
}

func (o *Ops) Create(parent ids.NodeID, pl templates.Place, at *mathx.Vec2, onCreate func([]ids.NodeID)) {
	if o.create != nil {
		o.create(parent, pl, at, onCreate)
	}
}

func (o *Ops) InstantiateAt(t templates.Template, at *mathx.Vec2) error {
	_, err := templates.Instantiate(o.ctx, t, ids.NodeID{}, -1, at)
	return err
}

func (o *Ops) InstantiateAt3(t templates.Template, at mathx.Vec3) error {
	_, err := templates.InstantiateAt3(o.ctx, t, ids.NodeID{}, at)
	return err
}

func (o *Ops) Instantiate(t templates.Template, parent ids.NodeID, index int) error {
	_, err := templates.Instantiate(o.ctx, t, parent, index, nil)
	return err
}

func (o *Ops) AttachScript(nodes []ids.NodeID, script ids.AssetID) {
	assetops.AttachScriptAsset(o.ctx, nodes, script)
}

func (o *Ops) RenameScene(d *document.SceneDocument) {
	assetops.PromptRenameScene(o.ctx, o.files, d)
}

func (o *Ops) SaveSceneAs(d *document.SceneDocument) {
	assetops.PromptSaveAs(o.ctx, d)
}

func (o *Ops) AttachScriptType(nodes []ids.NodeID, typeName string) error {
	return assetops.AttachScript(o.ctx, nodes, typeName)
}

func (o *Ops) ScriptDir(typeName string) string {
	return assetops.ScriptDir(o.ctx, typeName)
}

func (o *Ops) CopyAssets(list []ids.AssetID) bool {
	return len(assetops.CopyAssets(o.ctx, list)) > 0
}

func (o *Ops) ImportImage(dir, stem string, img image.Image) (ids.AssetID, string, error) {
	if err := assetops.EnsureDir(o.ctx, dir); err != nil {
		return ids.AssetID{}, "", err
	}
	rel := assetops.UniqueFile(o.ctx, dir, stem, ".png")
	id, err := assetops.CreatePNG(o.ctx, rel, img)
	return id, rel, err
}
