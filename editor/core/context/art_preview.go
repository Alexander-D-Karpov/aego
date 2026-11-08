package context

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/document"
)

// workingPreviewAsset resolves a generated frame that exists only in the
// loader's published working generation. It never treats an unrelated or
// stale loader entry as an editable source.
func (e *Editor) workingPreviewAsset(id ids.AssetID) (document.AssetRequest, bool) {
	if e.Pipeline == nil || e.Loader == nil || e.Loader.Index() == nil || e.Docs == nil || e.metadata == nil || e.metadata.closed {
		return document.AssetRequest{}, false
	}
	entry, ok := e.Loader.Index().Lookup(id)
	if !ok || entry.Kind != asset.ArtSpriteRegion || entry.Source == "" || entry.Missing || entry.Error != "" {
		return document.AssetRequest{}, false
	}
	owner, ok := e.Pipeline.Lookup(entry.Source)
	if !ok || e.metadata.sourceWorking[owner] != entry.Source {
		return document.AssetRequest{}, false
	}
	doc := e.Docs.AssetDocument(owner)
	version, published := e.metadata.sourceVersions[owner]
	if doc == nil || doc.Path != entry.Source || doc.WorkingPreview.Applied == 0 ||
		!published || !e.WorkingSourceCurrent(doc, version.revision) {
		return document.AssetRequest{}, false
	}
	prefix := strings.TrimSuffix(entry.Source, path.Ext(entry.Source)) + "/"
	name, ok := strings.CutPrefix(entry.Name, prefix)
	if !ok || !strings.HasPrefix(name, "frame_") || strings.Contains(name, "/") {
		return document.AssetRequest{}, false
	}
	return document.AssetRequest{Asset: owner, Selected: id, Path: entry.Source, Subasset: name, Kind: entry.Kind}, true
}

// SourcePathForAsset resolves both saved asset frames and current working
// preview frames without activating a document or mutating the pipeline.
func (e *Editor) SourcePathForAsset(id ids.AssetID) (string, bool) {
	if e.Pipeline == nil {
		return "", false
	}
	if rel, ok := e.Pipeline.PathOf(id); ok {
		source, _, _ := strings.Cut(rel, pipeline.SubSep)
		return source, true
	}
	if req, ok := e.workingPreviewAsset(id); ok {
		return req.Path, true
	}
	return "", false
}

// WorkingSourceCurrent reports whether the loader contains this exact open
// document revision. A later edit invalidates the generation immediately, even
// before the coalesced preview job has been queued.
func (e *Editor) WorkingSourceCurrent(d *document.AssetDocument, revision uint64) bool {
	if e == nil || e.Docs == nil || e.Pipeline == nil || e.metadata == nil || e.metadata.closed || d == nil || revision == 0 || e.Docs.AssetDocument(d.Asset) != d {
		return false
	}
	q := e.metadata
	v, ok := q.sourceVersions[d.Asset]
	device, lost := e.metadataDevice()
	return ok && q.sourceWorking[d.Asset] == d.Path && v.doc == d && v.path == d.Path &&
		v.revision == revision && v.current != nil && v.current(revision) &&
		v.pipeline == e.Pipeline.Revision() && !lost && v.device == device
}

// sourcePreviewState extends the existing working-metadata generation. Source
// pixels use that same private VFS, dependency closure, loader transaction and
// last-good rollback; there is no separate sprite renderer or reload queue.
// Captures and publication checks run on the editor thread, not the import job.
type sourcePreviewState struct {
	versions map[ids.AssetID]sourcePreviewVersion
	working  map[ids.AssetID]string
}

type sourcePreviewVersion struct {
	doc      *document.AssetDocument
	path     string
	revision uint64
	current  func(uint64) bool
	pipeline uint64
	device   uint64
}

func (e *Editor) captureWorkingSources(q *metadataCoordinator, r *metadataRequest) error {
	r.sources = sourcePreviewState{
		versions: make(map[ids.AssetID]sourcePreviewVersion),
		working:  make(map[ids.AssetID]string),
	}
	total := 0
	for _, data := range r.overlay {
		total += len(data)
	}
	const maxSnapshots = 256 << 20
	if total > maxSnapshots {
		return fmt.Errorf("graphical source snapshot budget exceeded")
	}
	// A superseding request must include saved bytes for every discarded owner,
	// otherwise replacing the private VFS could retain a closed document's art.
	for id, path := range q.sourceWorking {
		current := e.Docs.AssetDocument(id)
		if current != nil && current.Binding.PreviewSnapshot != nil && !(r.restore && current == r.doc) {
			continue
		}
		data, err := e.Docs.ReadSource(path)
		if err != nil {
			return fmt.Errorf("restore graphical source %s: %w", path, err)
		}
		if len(data) > 64<<20 {
			return fmt.Errorf("saved graphical source %s exceeds 64 MiB", path)
		}
		total += len(data) - len(r.overlay[path])
		if total > maxSnapshots {
			return fmt.Errorf("graphical source snapshot budget exceeded")
		}
		r.overlay[path] = bytes.Clone(data)
	}
	for _, d := range e.Docs.AssetDocuments() {
		b := d.Binding
		if b.PreviewSnapshot == nil || (r.restore && d == r.doc) {
			continue
		}
		if b.PreviewCurrent == nil || b.Dirty == nil {
			return fmt.Errorf("graphical source %s has no publication guard", d.Path)
		}
		_, wasWorking := q.sourceWorking[d.Asset]
		dirty := b.Dirty()
		if !dirty && !wasWorking && d != r.doc {
			continue
		}
		data, revision, err := b.PreviewSnapshot()
		if err != nil {
			return fmt.Errorf("capture graphical source %s: %w", d.Path, err)
		}
		if len(data) == 0 || len(data) > 64<<20 || !b.PreviewCurrent(revision) {
			return fmt.Errorf("graphical source %s changed while capturing its preview", d.Path)
		}
		total += len(data) - len(r.overlay[d.Path])
		if total > maxSnapshots {
			return fmt.Errorf("graphical source preview exceeds the 256 MiB snapshot budget")
		}
		r.overlay[d.Path] = bytes.Clone(data)
		r.sources.versions[d.Asset] = sourcePreviewVersion{doc: d, path: d.Path, revision: revision, current: b.PreviewCurrent, pipeline: r.pipeline, device: r.device}
		if dirty {
			r.sources.working[d.Asset] = d.Path
			r.working[d.Asset] = d.Path
		}
	}
	return nil
}

func (e *Editor) workingSourcesCurrent(r *metadataRequest) bool {
	for id, v := range r.sources.versions {
		d := e.Docs.AssetDocument(id)
		if d != v.doc || d.Path != v.path || !v.current(v.revision) {
			return false
		}
	}
	return true
}

func (e *Editor) publishedWorkingSources(q *metadataCoordinator, r *metadataRequest) {
	q.sourceWorking = r.sources.working
	q.sourceVersions = r.sources.versions
	for _, v := range r.sources.versions {
		v.doc.WorkingPreview.Pending = false
		v.doc.WorkingPreview.Revision = v.revision
		v.doc.WorkingPreview.Applied = r.token
		v.doc.WorkingPreview.Error = ""
		q.tracked[v.doc.ID] = v.doc
	}
}

// Binding history takes precedence over a sidecar journal only for structured
// source handlers that explicitly advertise their history. Image Viewer and
// ordinary metadata/source documents keep their existing routing.
func structuredAssetHistory(d *document.AssetDocument, redo bool) (bool, error) {
	if d.Binding.CanUndo == nil && d.Binding.CanRedo == nil {
		return false, nil
	}
	if redo {
		if d.Binding.Redo != nil {
			return true, d.Binding.Redo()
		}
	} else if d.Binding.Undo != nil {
		return true, d.Binding.Undo()
	}
	return true, nil
}
