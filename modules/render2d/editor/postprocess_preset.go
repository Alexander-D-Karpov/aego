package editor

import (
	"errors"
	"fmt"

	"aego/asset"
	"aego/asset/importers/preset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/preview"
	"aego/editor/core/services"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/gpu"
	"aego/modules/render2d"
	"aego/render/postfx"
	"aego/render/rt"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/ui/widgets"
)

func postPresetNode(data []byte, path string) (*aetext.Node, error) {
	n, err := aetext.Parse(data, path)
	if err != nil {
		return nil, err
	}
	if typeName, _, _ := preset.Type(n); typeName != render2d.NamePostProcess {
		return nil, preview.ErrUnsupportedContent
	}
	return asset.PresetComponent(n, render2d.NamePostProcess)
}

func (w *postInspector) readPresetPlan(ed *context.Editor, data []byte, path string, plan *postfx.Plan, gpuRequired bool) error {
	node, err := postPresetNode(data, path)
	if err != nil {
		return err
	}
	if err = sceneio.ValidatePresetFields(ed.Types.Schema(), w.td, node); err != nil {
		return err
	}
	sc, report, err := sceneio.Load(data, path, ed.Types, ed.Loader, sceneio.LoadOptions{})
	if err != nil {
		return err
	}
	if len(report.Unknown)+len(report.Missing)+len(report.Warnings) != 0 {
		return fmt.Errorf("invalid camera effects preset: %+v", report)
	}
	var c *render2d.PostProcess
	scene.Store[render2d.PostProcess](sc).EachAll(func(_ scene.NodeHandle, fx *render2d.PostProcess) { c = fx })
	if c == nil {
		return errors.New("missing camera effects")
	}
	return render2d.PreparePostProcess(w.reg, plan, c, ed.Loader, gpuRequired)
}

func (w *postInspector) registerPresetPreview(r *ext.Registry, ed *context.Editor) error {
	open := r.DocumentView(func(d *document.AssetDocument) func(*services.Frame, mathx.Rect) {
		if d.Source != nil {
			d.Source.ExplicitSave = true
			d.Source.ValidateSave = func(data []byte) error { return w.readPresetPlan(ed, data, d.Path, &postfx.Plan{}, false) }
		}
		return func(f *services.Frame, area mathx.Rect) {
			ui := f.UI
			if ed.OpenScript != nil && widgets.Button(ui, i18n.Tr("editor.postfx.edit_source", "Edit preset source")) {
				ed.OpenScript(d.Path, 1)
			}
			if ed.Thumbs == nil {
				return
			}
			if region, ok := ed.Thumbs.ByID(d.Asset); ok {
				size := min(float32(256), area.W, max(float32(0), area.H-ui.Theme.RowHeight))
				r := ui.Cursor().Next(size)
				r.W = size
				ui.List().Texture(region, r, mathx.White)
			} else if issues, ok := ed.Thumbs.(interface{ Reason(ids.AssetID) string }); ok {
				widgets.LabelWrapped(ui, issues.Reason(d.Asset), true)
			}
		}
	})
	err := r.DocumentHandler(document.Handler{ID: "render2d.postfx-preset", Name: i18n.T("editor.postfx.preset_document", "Camera effects preset"), Icon: "file.preset", Kinds: []asset.ArtifactKind{asset.ArtPreset}, Roles: []string{render2d.NamePostProcess}, TextSource: true, Capabilities: document.View | document.EditSource | document.Preview | document.RevealSource, Open: open})
	if err != nil {
		return err
	}
	return r.Thumbnail(asset.ArtPreset, preview.Handler{
		Component: render2d.NamePostProcess, ReadSource: true, Version: "render2d.postfx.v1",
		Render: func(e asset.Entry, size int) (preview.Surface, error) {
			var data []byte
			if doc := ed.Docs.Script(e.Source); doc != nil {
				data = doc.Code.Bytes()
			} else {
				a, ok := e.Artifact(asset.ArtPreset)
				if !ok {
					return preview.Surface{}, errors.New("missing preset artifact")
				}
				var err error
				data, err = ed.Pipeline.Artifacts().ReadFile(a.Path)
				if err != nil {
					return preview.Surface{}, err
				}
			}
			var plan postfx.Plan
			if err := w.readPresetPlan(ed, data, e.Source, &plan, true); err != nil {
				return preview.Surface{}, err
			}
			return renderPostPreset(ed, &plan, size)
		}, Build: func(card *preview.Card) (bool, error) {
			node, err := postPresetNode(card.Source, card.Entry.Source)
			if errors.Is(err, preview.ErrUnsupportedContent) {
				return false, nil
			}
			if err != nil {
				return true, err
			}
			card.Label(4, 3, "PostFX", true)
			if v, ok := node.Field("effects"); ok {
				for i, it := range v.List {
					if i == 4 {
						break
					}
					for _, p := range it.Obj {
						if p.Name == "typeID" {
							card.Label(4, 19+i*14, p.Value.Str, false)
							break
						}
					}
				}
			}
			return true, nil
		},
	})
}

func renderPostPreset(ed *context.Editor, plan *postfx.Plan, size int) (preview.Surface, error) {
	r := ed.Renderer
	pool := rt.New(r.Device(), gpu.TexSRGBA8, "postfx.preset")
	t, err := pool.Acquire(size, size, false)
	if err != nil {
		pool.Close()
		return preview.Surface{}, err
	}
	id := frame.New().OutputID()
	closeSurface := func() { _ = r.ReleaseLightingView(id, id); pool.Release(t); pool.Close() }
	if err = drawPostScene(r, id, t, size, plan, "postfx.preset"); err != nil {
		closeSurface()
		return preview.Surface{}, err
	}
	return preview.Surface{Target: t.Texture, Close: closeSurface}, nil
}

// Factory choices replace the stack in one history transaction. Existing bindings
// keep their instance when the same effect type survives a change of preset.
func (w *postInspector) applyFactoryPreset(c *render2d.PostProcess, choice int, label string) error {
	w.finish()
	draft := render2d.PostProcess{Enabled: choice != 0}
	switch choice {
	case 0:
		draft.Effects = c.Effects // Original bypasses the stack without discarding the author's settings.
	case 1, 2, 3, 4:
		types := [][]string{{postfx.BloomID}, {postfx.ColorGradeID}, {postfx.PixelateID, postfx.CRTID}, {postfx.CustomID}}
		for _, typ := range types[choice-1] {
			id := ""
			for _, old := range c.Effects {
				if old.TypeID == typ {
					id = old.InstanceID
					break
				}
			}
			if id == "" {
				id = ids.NewNodeID().String()
			}
			e, err := w.reg.NewInstance(typ, id)
			if err != nil {
				return err
			}
			e.Enabled = true
			// Use the shared field schema to validate factory values as for user edits.
			var field string
			var value schema.Value
			switch typ {
			case postfx.BloomID:
				field, value = "Threshold", schema.Float32(.75)
			case postfx.ColorGradeID:
				field, value = "Saturation", schema.Float32(.8)
			case postfx.PixelateID:
				field, value = "Size", schema.Int32(3)
			case postfx.CustomID:
				e.Material.ID = asset.BuiltinMaterialPostTint
			}
			if field != "" {
				desc, _ := w.reg.Lookup(typ)
				for i := range desc.Fields {
					if f := &desc.Fields[i]; f.Name == field {
						if err := render2d.SetEffectValue(&e, f, value); err != nil {
							return err
						}
						break
					}
				}
			}
			draft.Effects = append(draft.Effects, e)
		}
	default:
		return errors.New("unknown camera effects preset")
	}
	if err := w.reg.Validate(draft.Effects); draft.Enabled && err != nil {
		return err
	}
	tx := w.doc.History.Begin(label)
	enabled, _ := w.td.Field("enabled")
	if err := w.doc.Props.Set(tx, []ids.NodeID{w.node}, w.td.ID, enabled.ID, schema.Bool(draft.Enabled)); err != nil {
		tx.Abort()
		return err
	}
	if err := w.write(&draft, tx); err != nil {
		tx.Abort()
		return err
	}
	tx.Commit()
	return nil
}
