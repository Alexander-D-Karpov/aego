package agentbridge

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"

	"aego/app/agent"
	"aego/editor/core/document"
	"aego/engine/i18n"
)

func documentToolSchema() map[string]any {
	return visualSchema([]string{"file"}, map[string]any{"file": map[string]any{"type": "string"}})
}

type documentTarget struct {
	path   string
	scene  *document.SceneDocument
	source *document.ScriptDocument
	asset  *document.AssetDocument
}

func (b *Bridge) openDocument(rel string) (documentTarget, error) {
	m := b.ctx.Docs
	if m == nil {
		return documentTarget{}, errors.New("document manager is unavailable")
	}
	t := documentTarget{path: rel}
	if t.scene = m.Find(rel); t.scene != nil {
		return t, nil
	}
	if t.source = m.Script(rel); t.source != nil {
		if t.source.ReadOnly || t.source.Conflict {
			return documentTarget{}, errors.New("document is read-only or has an unresolved disk conflict")
		}
		for _, d := range m.AssetDocuments() {
			if d.Metadata == t.source {
				t.asset = d
				break
			}
		}
		return t, nil
	}
	for _, d := range m.AssetDocuments() {
		if d.Path == rel {
			t.asset = d
			if d.Binding.Dirty != nil && d.Binding.Dirty() && d.Binding.PreviewRevision == nil {
				return documentTarget{}, errors.New("this asset editor cannot verify its revision; save or undo it manually")
			}
			return t, nil
		}
	}
	return documentTarget{}, fmt.Errorf("%s is not open in the editor", rel)
}

func (t documentTarget) revision() string {
	s := ""
	if t.scene != nil {
		s += fmt.Sprintf("scene:%d:%d:%t;", t.scene.ID, t.scene.History.Version(), t.scene.Dirty())
	}
	if t.source != nil {
		s += fmt.Sprintf("source:%d:%d:%t:%t;", t.source.ID, t.source.Code.Rev(), t.source.Dirty(), t.source.Conflict)
	}
	if t.asset != nil {
		s += fmt.Sprintf("asset:%d:%t;", t.asset.ID, t.asset.Dirty())
		for _, d := range []*document.ScriptDocument{t.asset.Source, t.asset.Metadata} {
			if d != nil {
				s += fmt.Sprintf("%d:%d:%t:%t;", d.ID, d.Code.Rev(), d.Dirty(), d.Conflict)
			}
		}
		if t.asset.Scene != nil {
			s += fmt.Sprintf("%d:%d;", t.asset.Scene.ID, t.asset.Scene.History.Version())
		}
		if t.asset.Binding.PreviewRevision != nil {
			s += fmt.Sprintf("binding:%d;", t.asset.Binding.PreviewRevision())
		}
	}
	return s
}

func (t documentTarget) available(op string) bool {
	if op == "save" {
		if t.scene != nil {
			return t.scene.Dirty()
		}
		if t.source != nil {
			return t.source.Dirty()
		}
		return t.asset != nil && t.asset.Dirty()
	}
	if t.scene != nil {
		if op == "undo" {
			return t.scene.History.CanUndo()
		}
		return t.scene.History.CanRedo()
	}
	if t.source != nil {
		if op == "undo" {
			return t.source.Code.CanUndo()
		}
		return t.source.Code.CanRedo()
	}
	if t.asset == nil {
		return false
	}
	if op == "undo" {
		return t.asset.CanUndo()
	}
	return t.asset.CanRedo()
}

func (t documentTarget) label(op string) string {
	if t.scene == nil || op == "save" {
		return ""
	}
	undo, redo := t.scene.History.Labels(1)
	if op == "undo" && len(undo) != 0 {
		return undo[0]
	}
	if op == "redo" && len(redo) != 0 {
		return redo[0]
	}
	return ""
}

func (b *Bridge) saveDocument(a map[string]any) (*agent.Output, error) {
	return b.documentOperation(a, "save")
}
func (b *Bridge) undoDocument(a map[string]any) (*agent.Output, error) {
	return b.documentOperation(a, "undo")
}
func (b *Bridge) redoDocument(a map[string]any) (*agent.Output, error) {
	return b.documentOperation(a, "redo")
}

func (b *Bridge) documentOperation(a map[string]any, op string) (*agent.Output, error) {
	name, _ := a["file"].(string)
	rel, err := b.Rel(name)
	if err != nil {
		return nil, err
	}
	gen := b.panelGeneration.Load()
	var target documentTarget
	var revision, label string
	var disk [32]byte
	var diskExists bool
	if err := b.main(func() {
		if b.cancelled(gen) {
			err = errCancelled
			return
		}
		if b.PlanOnly() {
			err = errPlanOnly
			return
		}
		target, err = b.openDocument(rel)
		if err != nil {
			return
		}
		if !target.available(op) {
			err = fmt.Errorf("nothing to %s in %s", op, rel)
			return
		}
		revision, label = target.revision(), target.label(op)
		if op == "save" && target.scene != nil {
			var data []byte
			data, err = os.ReadFile(b.ctx.Docs.Abs(rel))
			if err == nil {
				disk, diskExists = sha256.Sum256(data), true
			} else if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
	}); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	question := i18n.Tr("editor.agent.document_save", "Save the open document {file}?", i18n.Str("file", rel))
	if op == "undo" {
		question = i18n.Tr("editor.agent.document_undo", "Undo one step in {file}?", i18n.Str("file", rel))
	} else if op == "redo" {
		question = i18n.Tr("editor.agent.document_redo", "Redo one step in {file}?", i18n.Str("file", rel))
	}
	if label != "" {
		question += "\n" + label
	}
	if err := b.permit(question, gen); err != nil {
		return nil, err
	}
	if err := b.main(func() {
		if b.cancelled(gen) {
			err = errCancelled
			return
		}
		if b.PlanOnly() {
			err = errPlanOnly
			return
		}
		var current documentTarget
		current, err = b.openDocument(rel)
		if err != nil {
			return
		}
		if current != target || current.revision() != revision || !current.available(op) {
			err = errors.New("document changed while awaiting approval; inspect it and ask again")
			return
		}
		if op == "save" && target.scene != nil {
			var data []byte
			data, err = os.ReadFile(b.ctx.Docs.Abs(rel))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return
			}
			if (err == nil) != diskExists || (err == nil && sha256.Sum256(data) != disk) {
				err = errors.New("scene changed on disk while awaiting approval")
				return
			}
			err = nil
		}
		err = b.applyDocumentOperation(target, op)
	}); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return okValue(map[string]string{"file": rel, "operation": op}), nil
}

func (b *Bridge) applyDocumentOperation(t documentTarget, op string) error {
	if op == "save" {
		switch {
		case t.scene != nil:
			return b.ctx.Docs.Save(t.scene.ID)
		case t.source != nil:
			return b.ctx.Docs.SaveScript(t.source)
		default:
			return b.ctx.SaveAsset(t.asset)
		}
	}
	if t.scene != nil {
		var err error
		if op == "undo" {
			err = t.scene.History.Undo()
		} else {
			err = t.scene.History.Redo()
		}
		b.ctx.PruneSelectionOf(t.scene.ID)
		return err
	}
	if t.asset != nil {
		if op == "undo" {
			return b.ctx.UndoAsset(t.asset)
		}
		return b.ctx.RedoAsset(t.asset)
	}
	if op == "undo" {
		t.source.Code.Undo()
	} else {
		t.source.Code.Redo()
	}
	return nil
}
