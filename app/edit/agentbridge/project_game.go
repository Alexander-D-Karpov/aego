package agentbridge

import (
	"errors"

	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/history"
	"aego/project"
)

type mainSceneEdit struct {
	name          string
	p             *project.Project
	before, after ids.AssetID
}

func (e *mainSceneEdit) Label() string              { return e.name }
func (e *mainSceneEdit) Merge(history.Command) bool { return false }
func (e *mainSceneEdit) Do(history.Target) error {
	return project.ChangeMainScene(e.p, e.before, e.after)
}
func (e *mainSceneEdit) Undo(history.Target) error {
	return project.ChangeMainScene(e.p, e.after, e.before)
}

func (b *Bridge) SetMainScene(rel string, from, to ids.AssetID) error {
	d := b.ctx.Docs.Edit()
	if d == nil {
		var err error
		d, err = b.ctx.Docs.Open(rel)
		if err != nil {
			return err
		}
	}
	cmd := &mainSceneEdit{b.stepName("Set main scene"), b.ctx.Project, from, to}
	if err := d.History.Exec(cmd); err != nil {
		return err
	}
	b.note(d, cmd.Label())
	return nil
}

type newSceneEdit struct {
	name, rel string
	ctx       *context.Editor
	data      []byte
	id        ids.AssetID
	files     assetops.Ops
}

func (e *newSceneEdit) Label() string              { return e.name }
func (e *newSceneEdit) Merge(history.Command) bool { return false }
func (e *newSceneEdit) Do(history.Target) error {
	if !e.id.IsZero() {
		return e.files.Redo(e.ctx)
	}
	var err error
	e.id, err = e.files.CreateFile(e.ctx, e.rel, e.data)
	return err
}
func (e *newSceneEdit) Undo(history.Target) error { return e.files.Undo(e.ctx) }
func (b *Bridge) CreateSceneAsset(rel string, src []byte) (ids.AssetID, error) {
	if d := b.ctx.Docs.Edit(); d != nil {
		c := &newSceneEdit{name: b.stepName("Create scene"), rel: rel, ctx: b.ctx, data: src}
		if err := d.History.Exec(c); err != nil {
			return ids.AssetID{}, err
		}
		b.note(d, c.Label())
		return c.id, nil
	}
	if b.ctx.CreateAsset == nil {
		return ids.AssetID{}, errors.New("asset history is unavailable")
	}
	return b.ctx.CreateAsset(rel, src)
}
