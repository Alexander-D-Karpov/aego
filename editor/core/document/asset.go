package document

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/engine/i18n"
)

// AssetRequest is resolved by the asset index: Selected identifies a frame, Asset its source sheet with shared metadata/history; generated artifacts may lack SourcePath.
type AssetRequest struct {
	Asset, Selected      ids.AssetID
	Path, Subasset, Role string
	Kind                 asset.ArtifactKind
	ReadOnly             bool
}

// AssetDocument is the identity shared by Inspector, search and asset views; the manager owns its lifetime, and View is presentation-only, never saved to source, metadata, material, scene or prefab.
type AssetDocument struct {
	ID                   ID
	Asset, Selected      ids.AssetID
	Path, Subasset, Role string
	Kind                 asset.ArtifactKind
	Handler              string
	Name                 i18n.Text
	Icon                 string
	Capabilities         Capabilities
	Source, Metadata     *ScriptDocument
	Scene                *SceneDocument
	View                 any
	Note                 string
	// Transient documents have no indexed source yet. Path is a display name,
	// not a filesystem path; their binding must provide an explicit first Save.
	Transient            bool
	WorkingPreview       MetadataPreviewState
	// Bindings adapt an already existing structured editor without duplicating
	// its mutable document model or introducing another tab manager.
	Binding AssetBinding
}

type MetadataPreviewState struct {
	Pending  bool
	Revision uint64
	Applied  uint64
	Error    string
}

// EditAction routes shared Edit commands to the focused structured document.
// A missing action must not fall through to a scene behind that document.
type EditAction uint8

const (
	ActionCopy EditAction = iota
	ActionCut
	ActionPaste
	ActionDelete
	ActionSelectAll
	ActionDuplicate
	ActionSaveAs
)

type AssetBinding struct {
	// Model owns structured source state; AssetDocument.View remains presentation-only.
	Model    any
	Actions  map[EditAction]func() error
	CanUndo  func() bool
	CanRedo  func() bool
	Close    func()
	Recovery func() ([]byte, error)
	// PendingRecovery is a previously saved recovery awaiting an explicit
	// user decision. Discarding the current document must not erase it.
	PendingRecovery func() bool
	PreviewRevision func() uint64
	PreviewSource   func() []byte
	// PreviewSnapshot captures immutable bytes; PreviewCurrent guards publication.
	PreviewSnapshot func() ([]byte, uint64, error)
	PreviewCurrent  func(uint64) bool
	Dirty           func() bool
	// BeforeSave must not write; it protects structured inputs before metadata
	// or source buffers are saved. SaveUnchanged supports an explicit first Save
	// for an in-memory source without falsely marking the document dirty.
	BeforeSave    func() error
	SaveUnchanged bool
	Save          func() error
	Undo          func() error
	Redo          func() error
	Discard       func()
}

// ErrSavePending means a user decision is required and no save has completed.
// In particular, Save All / Quit must not treat a queued path prompt as success.
var ErrSavePending = errors.New("save is waiting for a user decision")

func (d *AssetDocument) Dirty() bool {
	return (d.Source != nil && d.Source.Dirty()) || (d.Metadata != nil && d.Metadata.Dirty()) ||
		(d.Scene != nil && d.Scene.Dirty()) || (d.Binding.Dirty != nil && d.Binding.Dirty())
}

func (d *AssetDocument) Caption() string {
	label := path.Base(d.Path)
	if d.Dirty() {
		label += " " + DirtyMark
	}
	return label
}

func (m *Manager) Handlers() *HandlerRegistry       { return &m.handlers }
func (m *Manager) AssetDocuments() []*AssetDocument { return m.assets }

func (m *Manager) AssetDocument(id ids.AssetID) *AssetDocument {
	for _, d := range m.assets {
		if d.Asset == id {
			return d
		}
	}
	return nil
}

func validProjectPath(p string) bool {
	return p != "" && p != "." && !strings.HasPrefix(p, "/") && p != ".." && !strings.HasPrefix(p, "../") && !strings.Contains(p, ":")
}

// NewTransientAsset registers an in-memory document in the normal manager so
// dirty state, history, close and recovery are not bypassed. It creates no
// source or metadata file. The caller owns the binding and its Save workflow.
func (m *Manager) NewTransientAsset(id ids.AssetID, title, handler string, kind asset.ArtifactKind, capabilities Capabilities) (*AssetDocument, error) {
	if m == nil || id.IsZero() || title == "" || handler == "" || !capabilities.Has(View|EditSource) || m.AssetDocument(id) != nil {
		return nil, fmt.Errorf("invalid or already open in-memory asset document")
	}
	m.next++
	d := &AssetDocument{ID: m.next, Asset: id, Selected: id, Path: title, Handler: handler, Kind: kind,
		Capabilities: capabilities, Transient: true, Icon: "file.art"}
	m.assets = append(m.assets, d)
	return d, nil
}

// AcquireAsset loads the shared model without activating its presentation.
// Inspectors use it as well as OpenAsset; neither creates a private file cache.
func (m *Manager) AcquireAsset(req AssetRequest) (*AssetDocument, error) {
	if req.Asset.IsZero() {
		return nil, fmt.Errorf("asset document needs a stable AssetID")
	}
	if req.Selected.IsZero() {
		req.Selected = req.Asset
	}
	if req.Path != "" {
		req.Path = Clean(req.Path)
	}
	if !req.ReadOnly && !validProjectPath(req.Path) {
		return nil, fmt.Errorf("asset source path must stay inside the project")
	}
	h := m.handlers.Resolve(req.Kind, req.Path, req.Role)
	d := m.AssetDocument(req.Asset)
	fresh := d == nil
	if fresh {
		d = &AssetDocument{Asset: req.Asset}
	} else if d.Transient {
		return nil, fmt.Errorf("asset identity belongs to an open in-memory document")
	} else if d.Path != req.Path && !req.ReadOnly {
		// AssetID is authoritative after external moves/index refreshes: preserve dirty buffers and disk baselines at new paths without letting another open file steal their history.
		for _, move := range []struct {
			doc  *ScriptDocument
			path string
		}{{d.Source, req.Path}, {d.Metadata, req.Path + ".meta"}} {
			if move.doc == nil {
				continue
			}
			if other := m.Script(move.path); other != nil && other != move.doc {
				return nil, fmt.Errorf("asset moved to a path that already has an open document: %s", move.path)
			}
		}
		m.Renamed(d.Path, req.Path)
	}
	d.Path, d.Selected, d.Subasset, d.Role, d.Kind = req.Path, req.Selected, req.Subasset, req.Role, req.Kind
	d.Handler, d.Capabilities = h.ID, h.Capabilities
	d.Name, d.Icon = h.Name, h.Icon
	if req.ReadOnly {
		d.Capabilities &^= EditSource | EditMetadata
		if asset.IsBuiltin(req.Asset) {
			d.Capabilities &^= RevealSource
		}
	}
	if h.TextSource && !req.ReadOnly && d.Source == nil {
		s, err := m.OpenScript(req.Path)
		if err != nil {
			return nil, err
		}
		d.Source = s
		if !h.Capabilities.Has(EditSource) {
			s.ReadOnly = true
		}
	}
	if d.Capabilities.Has(EditMetadata) && d.Metadata == nil {
		s, err := m.OpenScript(req.Path + ".meta")
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			d.Capabilities &^= EditMetadata
			d.Note = "The metadata file is missing. Repair the asset metadata before editing it."
		} else {
			d.Metadata = s
			s.ExplicitSave = true
		}
	}
	if fresh {
		m.next++
		d.ID = m.next
		if d.Source != nil {
			d.ID = d.Source.ID
		}
		m.assets = append(m.assets, d)
	}
	return d, nil
}

func (d *AssetDocument) CanUndo() bool {
	if d.Binding.CanUndo != nil {
		return d.Binding.CanUndo()
	}
	if d.Metadata != nil {
		return d.Metadata.Code.CanUndo()
	}
	if d.Source != nil {
		return d.Source.Code.CanUndo()
	}
	if d.Scene != nil {
		return d.Scene.History.CanUndo()
	}
	return d.Binding.Undo != nil
}

func (d *AssetDocument) CanRedo() bool {
	if d.Binding.CanRedo != nil {
		return d.Binding.CanRedo()
	}
	if d.Metadata != nil {
		return d.Metadata.Code.CanRedo()
	}
	if d.Source != nil {
		return d.Source.Code.CanRedo()
	}
	if d.Scene != nil {
		return d.Scene.History.CanRedo()
	}
	return d.Binding.Redo != nil
}

func (m *Manager) OpenAsset(req AssetRequest) (*AssetDocument, error) {
	d, err := m.AcquireAsset(req)
	if err != nil {
		return nil, err
	}
	h := m.handlers.Resolve(req.Kind, req.Path, req.Role)
	if h.Open != nil {
		if err = h.Open(d); err != nil {
			return d, err
		}
	}
	return d, nil
}

func (m *Manager) SaveAsset(d *AssetDocument) error {
	if d == nil {
		return fmt.Errorf("no asset document")
	}
	if d.Binding.BeforeSave != nil {
		if err := d.Binding.BeforeSave(); err != nil {
			return err
		}
	}
	for _, s := range []*ScriptDocument{d.Source, d.Metadata} {
		if s != nil && s.Dirty() {
			if err := m.SaveScript(s); err != nil {
				return err
			}
		}
	}
	if d.Scene != nil && d.Scene.Dirty() {
		if err := m.Save(d.Scene.ID); err != nil {
			return err
		}
	}
	if d.Binding.Save != nil && (d.Binding.SaveUnchanged || d.Binding.Dirty != nil && d.Binding.Dirty()) {
		return d.Binding.Save()
	}
	return nil
}

func (m *Manager) CloseAsset(id ids.AssetID, force bool) error {
	for i, d := range m.assets {
		if d.Asset != id {
			continue
		}
		if d.Dirty() && !force {
			return fmt.Errorf("asset has unsaved changes: %s", d.Path)
		}
		if force {
			for _, source := range []*ScriptDocument{d.Source, d.Metadata} {
				if source != nil && source.views > 0 && source.Dirty() {
					if err := m.DiscardScript(source); err != nil {
						return err
					}
				}
			}
		}
		if force && d.Binding.Recovery != nil && (d.Binding.PendingRecovery == nil || !d.Binding.PendingRecovery()) {
			if err := m.ClearOwnedAssetRecovery(d); err != nil {
				return err
			}
		}
		if force && d.Binding.Discard != nil {
			d.Binding.Discard()
		}
		if d.Binding.Close != nil {
			d.Binding.Close()
		}
		m.assets = append(m.assets[:i], m.assets[i+1:]...)
		for _, s := range []*ScriptDocument{d.Source, d.Metadata} {
			if s != nil {
				m.CloseScript(s)
			}
		}
		m.emit(Event{Kind: EvClosed, Doc: d.ID, Path: d.Path})
		return nil
	}
	return fmt.Errorf("no such asset document")
}

func (m *Manager) scriptOwned(d *ScriptDocument) bool {
	for _, a := range m.assets {
		if a.Source == d || a.Metadata == d {
			return true
		}
	}
	return false
}
