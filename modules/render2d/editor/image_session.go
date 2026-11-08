package editor

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/png"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/code"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/gpu"
	"aego/render/texture"
	"aego/scene/mutate"
	"aego/ui/core"
	"aego/ui/widgets"
)

type imageState struct {
	content             imageContent
	source              *imagePixels
	imported            *imagePixels
	generated           *imagePixels
	generatedFrom       *imagePixels
	generatedRect       [4]int
	meta                *asset.SourceMeta
	role                asset.TextureRole
	metaError           error
	loaded              bool
	assetRevision       uint64
	metaRevision        uint64
	metaCode            *code.Doc
	inputRevision       uint64
	candidateSourceHash [32]byte
	selected            ids.AssetID
	viewport            imageViewport
	display             imageDisplay
	preview             imagePreview
	candidate           imageCandidate
	channel             imageChannel
	mode                int // source, imported, generated normal
	inspectorMode       imageInspectorMode
	background          int
	grid                bool
	showPreview         bool
	highlight           [4]int // used only by an explicit Open Atlas navigation
	gesture             string
	gestureRev          uint64
	scroll              [3]float32
	scrollHeight        [3]float32
	scrollBar           [3]widgets.ScrollState
	sections            [2]imageSections // separate state for the document and Inspector
	seen                bool
	viewSeen            bool
	closed              bool
	saved               bool
	pendingSave         bool
	closeAfterSave      bool
	message             string
	pickers             map[core.ID]*imageChannelPicker
}

func newImageState() *imageState {
	return &imageState{grid: true, showPreview: true, metaRevision: ^uint64(0)}
}

func (v *imageState) refresh(ed *context.Editor, d *document.AssetDocument) {
	metadataRev := uint64(0)
	var metadataCode *code.Doc
	if d.Metadata != nil {
		metadataRev = d.Metadata.Code.Rev()
		metadataCode = d.Metadata.Code
	}
	selectionChanged := v.selected != d.Selected
	contentChanged := !v.loaded || v.assetRevision != ed.AssetRev || selectionChanged
	metaChanged := v.metaRevision != metadataRev || v.metaCode != metadataCode
	sourceChanged := !v.loaded
	if contentChanged {
		content := loadImageContent(ed, d.Asset, d.Selected)
		sourceChanged = sourceChanged || content.SourceHash != v.content.SourceHash || content.Path != v.content.Path
		sizeChanged := content.Source != nil && (v.content.Source == nil || content.Source.data.Width != v.content.Source.data.Width || content.Source.data.Height != v.content.Source.data.Height)
		if selectionChanged || !v.loaded || sizeChanged {
			v.viewport.Ready = false
		}
		v.content = content
		v.assetRevision, v.selected = ed.AssetRev, d.Selected
		v.display.close(ed)
	}
	if metaChanged || contentChanged {
		v.meta, v.metaError = nil, nil
		if d.Metadata != nil {
			v.meta, v.metaError = asset.ParseSourceMeta(d.Metadata.Code.Bytes(), d.Metadata.Path)
		} else if ed.Pipeline != nil {
			if meta, ok := ed.Pipeline.MetaOf(d.Asset); ok {
				v.meta = &meta
			}
		}
		if v.meta != nil {
			v.role, v.metaError = asset.TextureRoleOf(d.Path, v.meta.Settings)
			v.content.Frames = v.meta.Subs
			if v.content.Source != nil {
				v.content.SourceRect = [4]int{0, 0, v.content.Source.data.Width, v.content.Source.data.Height}
			}
			if f, found := v.meta.Frame(d.Selected); found {
				v.content.SourceRect = f.Rect
			}
		}
		if v.meta == nil && v.content.Imported != nil {
			switch v.content.Imported.semantic {
			case asset.TextureNormal:
				v.role = asset.RoleNormal
			case asset.TextureHeight:
				v.role = asset.RoleHeight
			case asset.TextureEmissionRGB:
				v.role = asset.RoleEmissionRGB
			case asset.TextureEmissionMask:
				v.role = asset.RoleEmissionMask
			default:
				v.role = asset.RoleColor
			}
		}
		if !v.loaded {
			switch v.role {
			case asset.RoleNormal:
				v.channel = imageRGB
			case asset.RoleHeight, asset.RoleMask, asset.RoleCookieMask, asset.RoleEmissionMask:
				v.channel = imageR
			}
		}
		v.source, _ = v.content.sourcePixels()
		v.imported, _ = v.content.importedPixels()
		if v.source == nil && v.imported != nil && !v.loaded {
			v.mode = 1
		}
		v.metaRevision = metadataRev
		v.metaCode = metadataCode
	}
	inputsChanged := metaChanged || selectionChanged || sourceChanged
	if inputsChanged {
		v.inputRevision++
		v.candidate.cancel()
		v.pendingSave = false
	}
	v.loaded, v.seen = true, true
	if inputsChanged && v.role == asset.RoleHeight && v.meta != nil && v.meta.Settings.Bool("generate_normal", false) {
		if err := v.prepare(ed, d); err != nil {
			v.message = err.Error()
		}
	}
}

func (v *imageState) pixels() *imagePixels {
	switch v.mode {
	case 1:
		return v.imported
	case 2:
		if v.candidate.good != nil {
			if v.generatedFrom != v.candidate.good || v.generatedRect != v.content.SourceRect {
				v.generated, _ = croppedPixels(v.candidate.good, v.content.SourceRect, [4]int{})
				v.generatedFrom, v.generatedRect = v.candidate.good, v.content.SourceRect
			}
			return v.generated
		}
	}
	return v.source
}

func imageDevice(ed *context.Editor) (uint64, bool) {
	if ed.Renderer != nil {
		if state, ok := ed.Renderer.Device().(gpu.ContextState); ok {
			return state.ContextGeneration(), state.ContextLost()
		}
	}
	return 0, false
}

func (v *imageState) prepare(ed *context.Editor, d *document.AssetDocument) error {
	if ed.Pipeline == nil || ed.Docs == nil || d.Metadata == nil {
		return fmt.Errorf("image preparation requires source metadata and the project importer")
	}
	if d.Metadata.Conflict {
		return fmt.Errorf("resolve the metadata conflict before preparing a candidate")
	}
	if v.meta == nil || v.role != asset.RoleHeight || !v.meta.Settings.Bool("generate_normal", false) {
		return fmt.Errorf("enable Generate normal on this height source before preparing a normal candidate")
	}
	source, err := readImageSource(ed.Docs, d.Path)
	if err != nil {
		return err
	}
	if len(source) > maxViewerSource {
		return fmt.Errorf("height source exceeds the viewer source budget")
	}
	config, err := png.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		return err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxViewerPixels/config.Height {
		return fmt.Errorf("height source exceeds the viewer pixel budget")
	}
	metadata := bytes.Clone(d.Metadata.Code.Bytes())
	v.candidateSourceHash = sha256.Sum256(source)
	overlay := map[string][]byte{}
	for _, doc := range ed.Docs.Scripts() {
		if doc.Dirty() {
			overlay[doc.Path] = bytes.Clone(doc.Code.Bytes())
		}
	}
	pipeline, name := ed.Pipeline, d.Path
	device, _ := imageDevice(ed)
	v.candidate.start(v.inputRevision, device, func() (*imagePixels, error) {
		result, err := pipeline.PreviewWithMetadata(name, source, metadata, func(path string) ([]byte, bool) {
			value, ok := overlay[path]
			return value, ok
		})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fmt.Errorf("texture importer returned no candidate")
		}
		if result.Error != nil {
			return nil, result.Error
		}
		if result.Raw != nil && result.RawSemantic == asset.TextureNormal {
			return &imagePixels{data: *result.Raw, semantic: result.RawSemantic}, nil
		}
		for _, a := range result.Artifacts {
			if a.Kind == asset.ArtTexture {
				p, err := decodeImageArtifact(a.Data)
				if err != nil {
					return nil, err
				}
				if p.semantic != asset.TextureNormal {
					return nil, fmt.Errorf("height import did not produce a normal texture")
				}
				return p, nil
			}
		}
		return nil, fmt.Errorf("height import produced no normal pixels")
	})
	return nil
}

func (v *imageState) poll(ed *context.Editor, d *document.AssetDocument) {
	if d.Metadata == nil || v.closed {
		return
	}
	if d.Metadata.Code != v.metaCode || d.Metadata.Code.Rev() != v.metaRevision || d.Selected != v.selected {
		v.candidate.cancel()
		v.pendingSave = false
		return
	}
	device, lost := imageDevice(ed)
	wasPending := v.candidate.pending
	previous, previousRevision := v.candidate.good, v.candidate.goodRevision
	if v.candidate.poll(v.inputRevision, device, lost) {
		// A file may change before the watcher reports its asset revision.
		source, err := readImageSource(ed.Docs, d.Path)
		if err != nil || sha256.Sum256(source) != v.candidateSourceHash {
			v.candidate.good, v.candidate.goodRevision = previous, previousRevision
			v.candidate.err = fmt.Errorf("image source changed during candidate preparation; the previous valid normal is retained")
			v.pendingSave, v.loaded = false, false
			return
		}
		v.message = ""
		if v.pendingSave {
			v.pendingSave = false
			v.saveNow(ed, d)
		}
	}
	if !v.candidate.pending && v.candidate.err != nil {
		v.pendingSave = false
	}
	if wasPending && !v.candidate.pending && v.pendingSave {
		v.pendingSave = false
		v.message = i18n.Tr("editor.image.candidate_stale", "The document or graphics context changed during preparation. Prepare the current settings again before saving.")
	}
}

func (v *imageState) save(ed *context.Editor, d *document.AssetDocument) {
	if d.Metadata == nil {
		return
	}
	if v.role == asset.RoleHeight && v.meta != nil && v.meta.Settings.Bool("generate_normal", false) {
		if err := v.prepare(ed, d); err != nil {
			v.message = err.Error()
			return
		}
		v.pendingSave = true
		return
	}
	v.saveNow(ed, d)
}

func (v *imageState) saveNow(ed *context.Editor, d *document.AssetDocument) {
	if err := ed.SaveAsset(d); err != nil {
		v.message = err.Error()
		return
	}
	v.saved, v.message = true, ""
}

func (v *imageState) reload(ed *context.Editor, d *document.AssetDocument) {
	v.candidate.cancel()
	v.pendingSave, v.closeAfterSave = false, false
	v.gesture, v.message = "", ""
	if err := ed.ReloadMetadata(d); err != nil {
		v.message = err.Error()
	}
	v.refresh(ed, d)
}

func (v *imageState) mutate(ed *context.Editor, d *document.AssetDocument, key string, edit widgets.Edit, ops ...mutate.Op) {
	if !edit.Changed {
		if edit.End {
			v.gesture = ""
		}
		return
	}
	env, err := ed.TextureAuthoringEnv(d.Asset)
	if err == nil {
		continuing := v.gesture == key && !edit.Begin && d.Metadata.Code.Rev() == v.gestureRev
		_, err = document.ApplyTextureMutation(d.Metadata, env, mutate.Tx{Label: "Texture " + key, Ops: ops}, continuing)
	}
	if err != nil {
		v.message = err.Error()
		return
	}
	v.message = ""
	v.gestureRev, v.gesture = d.Metadata.Code.Rev(), key
	if edit.End {
		v.gesture = ""
	}
	// Only private preview data is invalidated. Saving is explicit.
	v.candidate.cancel()
	v.pendingSave = false
}

func (v *imageState) surface(ed *context.Editor, d *document.AssetDocument) imageSurface {
	w, h := v.content.SourceRect[2], v.content.SourceRect[3]
	if w <= 0 || h <= 0 {
		w, h = v.content.Region.W, v.content.Region.H
	}
	out := imageSurface{Size: mathx.Vec2{X: 2, Y: 2}}
	if w > 0 && h > 0 {
		out.Size = mathx.Vec2{X: float32(w), Y: float32(h)}.Scale(2 / float32(max(w, h)))
	}
	if ed.Loader == nil || ed.Renderer == nil {
		out.Reason = i18n.Tr("editor.image.preview_no_loader", "The surface preview needs a runtime asset loader and a graphics device.")
		return out
	}
	id := d.Selected
	if id.IsZero() {
		id = d.Asset
	}
	region, imported := ed.Loader.Texture(id)
	switch v.role {
	case asset.RoleNormal, asset.RoleHeight:
		if info, ok := ed.Loader.TextureMetadata(id); ok && info.Semantic == asset.TextureNormal && imported {
			out.Normal = region
		} else if v.candidate.good == nil {
			out.Reason = i18n.Tr("editor.image.no_prepared_normal", "No prepared normal is available. Enable generation for a height source, then prepare a candidate or save its settings.")
		}
	case asset.RoleEmissionRGB, asset.RoleEmissionMask:
		if imported {
			mode := texture.EmissionRGB
			if v.role == asset.RoleEmissionMask {
				mode = texture.EmissionMask
			}
			out.Emission = texture.Emission{Region: region, Mode: mode}
		}
	default:
		if !imported {
			out.Reason = i18n.Tr("editor.image.no_prepared_albedo", "No committed albedo is available. Save valid import settings and fix the import diagnostic first.")
			return out
		}
		out.Albedo = region
		_, out.Normal, _ = ed.Loader.NormalFor(id)
		_, out.Emission, _ = ed.Loader.EmissionFor(id)
		if v.meta != nil {
			for _, channel := range []string{"normal", "emission"} {
				ref, explicit, _ := asset.TextureChannel(v.meta, id, channel)
				if channel == "normal" && !explicit {
					// A frame's independently mapped height source may supply its
					// normal. Whole-sheet UVs never substitute for a frame mapping.
					ref, explicit, _ = asset.TextureChannel(v.meta, id, "height")
				}
				if !explicit {
					continue
				}
				t, ok := ed.Loader.MaterialParameterTexture(ref)
				if channel == "normal" {
					out.Normal = texture.Region{}
					if ok && t.NormalMap {
						out.Normal = t.Region
					}
				} else {
					out.Emission = texture.Emission{}
					if ok {
						out.Emission = texture.Emission{Region: t.Region, Mode: t.EmissionMode}
					}
				}
			}
		}
	}
	return out
}

func (v *imageState) close(ed *context.Editor) {
	v.closed = true
	v.pendingSave = false
	v.candidate.close()
	v.preview.close(ed)
	v.display.close(ed)
	if ed.UI != nil {
		for _, p := range v.pickers {
			p.palette.Hide(ed.UI)
		}
	}
	clear(v.pickers)
}
