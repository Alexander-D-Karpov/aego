package context

import (
	"bytes"
	"fmt"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/code"
	"aego/editor/core/document"
	"aego/gpu"
	"aego/vfs"
)

// A candidate contains the complete working metadata snapshot, so only the
// latest pending snapshot is useful. There is one worker plus one pending job.
const (
	maxMetadataRequests       = 1
	metadataRestoreRetryDelay = 1.0
)

type metadataRequest struct {
	doc                                      *document.AssetDocument
	metadata                                 *document.ScriptDocument
	asset                                    ids.AssetID
	path                                     string
	revision, saved, device, pipeline, token uint64
	source                                   []byte
	overlay                                  map[string][]byte
	versions                                 map[string]metadataVersion
	working                                  map[ids.AssetID]string
	sources                                  sourcePreviewState
	restore                                  bool
}

type metadataVersion struct {
	doc             *document.ScriptDocument
	code            *code.Doc
	revision, saved uint64
}

type metadataCoordinator struct {
	queue          []*metadataRequest
	running        bool
	closed         bool
	next           uint64
	tracked        map[document.ID]*document.AssetDocument
	working        map[ids.AssetID]string
	sourceWorking  map[ids.AssetID]string
	sourceVersions map[ids.AssetID]sourcePreviewVersion
	published      []ids.AssetID
	retry          *metadataRequest
	rollback       error
	rollbackDevice uint64
	rollbackAfter  float64
	unsubscribe    func()
	files          vfs.FS
	// Scheduling and CPU preparation are independent of GPU publication. Tests
	// drive these seams to finish old jobs after newer edits/context loss.
	submit, onMain func(func())
	build          func(*metadataRequest) (*pipeline.Batch, vfs.FS, error)
	device         func() (uint64, bool)
}

type metadataPipeline interface {
	PreviewMetadata(ids.AssetID, []byte, func(string) ([]byte, bool)) (*pipeline.Batch, vfs.FS, error)
}

func (e *Editor) metadataDevice() (uint64, bool) {
	if e.metadata != nil && e.metadata.device != nil {
		return e.metadata.device()
	}
	if e.Engine != nil {
		if d, ok := e.Engine.GPU().(gpu.ContextState); ok {
			return d.ContextGeneration(), d.ContextLost()
		}
	}
	return 0, false
}

func (e *Editor) metadataService() *metadataCoordinator {
	if e.metadata != nil {
		return e.metadata
	}
	q := &metadataCoordinator{tracked: make(map[document.ID]*document.AssetDocument), working: make(map[ids.AssetID]string)}
	if e.Engine != nil {
		q.submit, q.onMain = e.Engine.Jobs().Submit, e.Engine.Jobs().OnMain
	}
	q.build = func(r *metadataRequest) (*pipeline.Batch, vfs.FS, error) {
		p, ok := any(e.Pipeline).(metadataPipeline)
		if !ok {
			return nil, nil, fmt.Errorf("the pipeline cannot prepare working metadata")
		}
		return p.PreviewMetadata(r.asset, r.source, func(name string) ([]byte, bool) { b, ok := r.overlay[name]; return b, ok })
	}
	e.metadata = q
	if e.Docs != nil {
		q.unsubscribe = e.Docs.Subscribe(func(ev document.Event) {
			if ev.Kind != document.EvClosed || q.closed {
				return
			}
			d := q.tracked[ev.Doc]
			if d == nil {
				return
			}
			delete(q.tracked, ev.Doc)
			// Closing/discarding a working document restores saved artifacts;
			// an old pending edit may never publish after its document closes.
			if err := e.queueMetadata(d, true); err != nil {
				q.next++
				q.retry = &metadataRequest{doc: d, asset: d.Asset, restore: true}
				if _, lost := e.metadataDevice(); !lost {
					err = e.restoreSavedMetadata(q, err)
					q.retry = nil
				}
				e.Notify(LevelError, err.Error())
			}
		})
	}
	return q
}

// ApplyMetadataPreview imports a working buffer without saving: one CPU job runs at a time, newer requests replace the queued snapshot, and publication uses the engine-thread asset reload route.
func (e *Editor) ApplyMetadataPreview(id ids.AssetID) error {
	if e.Docs == nil {
		return fmt.Errorf("document manager is unavailable")
	}
	d := e.Docs.AssetDocument(id)
	if d == nil {
		return fmt.Errorf("metadata document is no longer open")
	}
	return e.queueMetadata(d, false)
}

func (e *Editor) queueMetadata(d *document.AssetDocument, restore bool) error {
	if e.Pipeline == nil || e.Loader == nil || d == nil || d.Metadata == nil {
		return fmt.Errorf("working metadata requires an asset loader and metadata document")
	}
	q := e.metadataService()
	if q.closed || q.submit == nil || q.onMain == nil {
		return fmt.Errorf("working metadata jobs are unavailable")
	}
	if !restore && (d.Metadata.ReadOnly || d.Metadata.Conflict) {
		return fmt.Errorf("resolve the metadata conflict before previewing it")
	}
	device, _ := e.metadataDevice()
	r := &metadataRequest{doc: d, metadata: d.Metadata, asset: d.Asset, path: d.Path, revision: d.Metadata.Code.Rev(), saved: d.Metadata.SavedEdits, device: device, pipeline: e.Pipeline.Revision(), restore: restore, overlay: make(map[string][]byte), versions: make(map[string]metadataVersion), working: make(map[ids.AssetID]string)}
	// Snapshot every owner from the last published working generation, using saved metadata for closed ones, so a later superseding close cannot erase an earlier restore.
	for id, path := range q.working {
		if current := e.Docs.AssetDocument(id); current != nil && current.Metadata != nil {
			path = current.Path
			if current.Metadata.Conflict {
				return fmt.Errorf("resolve the metadata conflict for %s before updating working assets", path)
			}
			s := current.Metadata
			r.overlay[path+".meta"] = bytes.Clone(s.Code.Bytes())
			r.versions[s.Path] = metadataVersion{s, s.Code, s.Code.Rev(), s.SavedEdits}
			if s.Dirty() {
				r.working[id] = path
			}
		} else {
			raw, err := e.Docs.ReadSource(path + ".meta")
			if err != nil {
				return fmt.Errorf("restore metadata for %s: %w", path, err)
			}
			r.overlay[path+".meta"] = raw
		}
	}
	if restore {
		data, err := e.Docs.ReadSource(d.Path + ".meta")
		if err != nil {
			return err
		}
		r.source = data
	} else {
		r.source = bytes.Clone(d.Metadata.Code.Bytes())
	}
	for _, other := range e.Docs.Scripts() {
		if other.ExplicitSave && other.SavedEdits > 0 && other.Dirty() && !other.Conflict && strings.HasSuffix(other.Path, ".meta") {
			path := strings.TrimSuffix(other.Path, ".meta")
			id, ok := e.Pipeline.Lookup(path)
			if !ok {
				continue
			}
			meta, ok := e.Pipeline.MetaOf(id)
			structured := e.Docs.AssetDocument(id)
			if !ok || (meta.Importer != "texture" && (structured == nil || structured.Binding.PreviewSnapshot == nil)) {
				continue
			}
			r.overlay[other.Path] = bytes.Clone(other.Code.Bytes())
			r.versions[other.Path] = metadataVersion{other, other.Code, other.Code.Rev(), other.SavedEdits}
			r.working[id] = path
		}
	}
	r.overlay[d.Path+".meta"] = r.source
	if !restore {
		r.versions[d.Metadata.Path] = metadataVersion{d.Metadata, d.Metadata.Code, r.revision, r.saved}
		if d.Metadata.Dirty() {
			r.working[d.Asset] = d.Path
		}
	}
	if err := e.captureWorkingSources(q, r); err != nil {
		return err
	}
	q.next++
	r.token = q.next
	q.retry = nil
	for _, previous := range q.queue {
		if !previous.restore && previous.doc != d {
			previous.doc.WorkingPreview.Pending = false
		}
	}
	q.queue = append(q.queue[:0], r)
	if !restore {
		q.tracked[d.ID] = d
		d.WorkingPreview.Pending = true
		d.WorkingPreview.Revision = r.revision
		d.WorkingPreview.Error = ""
	}
	e.pumpMetadata(q)
	return nil
}

func (e *Editor) metadataCurrent(q *metadataCoordinator, r *metadataRequest) bool {
	if q.closed || q.next != r.token || e.Pipeline.Revision() != r.pipeline {
		return false
	}
	if !e.workingSourcesCurrent(r) {
		return false
	}
	for path, v := range r.versions {
		if e.Docs.Script(path) != v.doc || v.doc.Code != v.code || v.doc.Conflict || v.doc.Code.Rev() != v.revision || v.doc.SavedEdits != v.saved {
			return false
		}
	}
	device, lost := e.metadataDevice()
	if lost || device != r.device {
		return false
	}
	d := e.Docs.AssetDocument(r.asset)
	if r.restore {
		// A newly reopened clean view can observe the restored saved generation.
		// A changed/saved replacement document must prepare its own candidate.
		return d == nil || (d.Metadata != nil && !d.Metadata.Conflict && d.Metadata.SavedEdits == 0)
	}
	return d == r.doc && d.Path == r.path && d.Metadata == r.metadata && !r.metadata.Conflict &&
		r.metadata.Code.Rev() == r.revision && r.metadata.SavedEdits == r.saved
}

func (e *Editor) pumpMetadata(q *metadataCoordinator) {
	if q.running || q.closed || len(q.queue) == 0 {
		return
	}
	r := q.queue[0]
	q.queue[0] = nil
	q.queue = q.queue[:0]
	q.running = true
	q.submit(func() {
		batch, files, err := q.build(r)
		q.onMain(func() {
			q.running = false
			current := e.metadataCurrent(q, r)
			if current {
				if err == nil {
					if batch == nil || files == nil {
						err = fmt.Errorf("metadata importer returned no prepared working generation")
					} else if len(batch.Failed) != 0 {
						for _, failed := range batch.Failed {
							err = failed
							break
						}
					} else {
						e.pruneWorkingMetadata(q, batch)
						err = e.applyBatch(batch, files)
					}
				}
				if err == nil {
					q.files = files
					q.rollback = nil
					q.working = r.working
					e.publishedWorkingSources(q, r)
					q.published = q.published[:0]
					for _, entry := range batch.Imported {
						q.published = append(q.published, entry.ID)
					}
					for _, d := range e.Docs.AssetDocuments() {
						if d.Metadata == nil {
							continue
						}
						if v, ok := r.versions[d.Metadata.Path]; ok && v.doc == d.Metadata {
							d.WorkingPreview.Pending = false
							d.WorkingPreview.Revision = v.revision
							d.WorkingPreview.Applied = r.token
							d.WorkingPreview.Error = ""
							q.tracked[d.ID] = d
						}
					}
				}
				if !r.restore {
					r.doc.WorkingPreview.Pending = false
					if err == nil {
						r.doc.WorkingPreview.Applied = r.token
						r.doc.WorkingPreview.Error = ""
					} else {
						r.doc.WorkingPreview.Error = err.Error()
					}
				}
				if err != nil {
					if r.restore {
						err = e.restoreSavedMetadata(q, err)
					}
					for _, d := range e.Docs.AssetDocuments() {
						if d.Metadata != nil {
							if v, ok := r.versions[d.Metadata.Path]; ok && v.doc == d.Metadata {
								d.WorkingPreview.Pending = false
								d.WorkingPreview.Error = err.Error()
							}
						}
					}
					e.Notify(LevelError, err.Error())
				}
			} else if !q.closed && q.next == r.token {
				if !r.restore {
					r.doc.WorkingPreview.Pending = false
				}
				// If the device generation or a saved import changed during CPU work, retry with fresh source/document snapshots on a later frame; never publish for the old device.
				q.retry = r
			}
			e.pumpMetadata(q)
		})
	})
}

func (e *Editor) pruneWorkingMetadata(q *metadataCoordinator, batch *pipeline.Batch) {
	retained := make(map[ids.AssetID]bool, len(batch.Imported)+len(batch.Removed))
	for _, entry := range batch.Imported {
		retained[entry.ID] = true
	}
	for _, id := range batch.Removed {
		retained[id] = true
	}
	// Compare the prior published generation before replacing the private VFS: Undo's importer cannot remove private frame/atlas IDs the persistent index never saw.
	for _, id := range q.published {
		if !retained[id] {
			if _, saved := e.Pipeline.Entry(id); !saved {
				batch.Removed = append(batch.Removed, id)
			}
		}
	}
}

// PollAssetPreviews resumes superseded device/import work when the context recovers; failed imports remain diagnostic rather than retrying every frame, but edit/Undo/Save can retry.
func (e *Editor) PollAssetPreviews() {
	q := e.metadata
	if q == nil || q.closed || q.running || len(q.queue) != 0 {
		return
	}
	device, lost := e.metadataDevice()
	if lost {
		return
	}
	if q.rollback != nil {
		// Keep one rollback after a failed GPU upload; retry promptly on a fresh context, at most once per second on the same device, and never while the device is lost.
		if device == q.rollbackDevice && e.Now() < q.rollbackAfter {
			return
		}
		err := e.restoreSavedMetadata(q, q.rollback)
		if q.rollback == nil {
			e.Notify(LevelError, err.Error())
		}
		return
	}
	if q.retry == nil {
		return
	}
	r := q.retry
	q.retry = nil
	d, restore := e.Docs.AssetDocument(r.asset), false
	if d == nil {
		d, restore = r.doc, true
	}
	if err := e.queueMetadata(d, restore); err != nil {
		if restore {
			err = e.restoreSavedMetadata(q, err)
		}
		e.Notify(LevelError, err.Error())
	}
}

// If reimporting a discarded document fails, release the entire private texture generation and restore the pipeline's last validated artifacts.
// Never change source or dirty journals; partial rollback loses private VFS dependencies.
func (e *Editor) restoreSavedMetadata(q *metadataCoordinator, cause error) error {
	device, lost := e.metadataDevice()
	deferRetry := func() {
		q.rollback = cause
		q.rollbackDevice = device
		q.rollbackAfter = e.Now() + metadataRestoreRetryDelay
	}
	if lost {
		deferRetry()
		return fmt.Errorf("discarded metadata will restore saved artifacts when the graphics context is available: %w", cause)
	}
	batch := &pipeline.Batch{}
	idx := e.Pipeline.Index()
	idx.Each(func(entry *asset.Entry) {
		if entry.Missing {
			return
		}
		switch entry.Kind {
		case asset.ArtTexture, asset.ArtSpriteRegion, asset.ArtAtlasPage, asset.ArtAnim:
			batch.Imported = append(batch.Imported, *entry)
		}
	})
	for _, id := range q.published {
		if _, exists := idx.Lookup(id); !exists {
			batch.Removed = append(batch.Removed, id)
		}
	}
	if err := e.applyBatch(batch, e.Pipeline.Artifacts()); err != nil {
		deferRetry()
		return fmt.Errorf("discarded metadata could not restore saved artifacts: %v; %w", cause, err)
	}
	q.rollback = nil
	q.retry = nil
	q.files = nil
	clear(q.working)
	clear(q.sourceWorking)
	clear(q.sourceVersions)
	q.published = q.published[:0]
	message := fmt.Sprintf("working texture previews returned to the last saved artifacts after a discarded document failed to restore: %v; unsaved document edits are preserved", cause)
	for _, d := range e.Docs.AssetDocuments() {
		if d.Metadata != nil {
			d.WorkingPreview.Pending = false
			d.WorkingPreview.Applied = 0
			d.WorkingPreview.Error = message
		}
	}
	return fmt.Errorf("%s", message)
}

// CloseAssetPreviews cancels publication before engine shutdown drains jobs.
// Immutable CPU work may finish, but it cannot touch a closed device/document.
func (e *Editor) CloseAssetPreviews() {
	if q := e.metadata; q != nil {
		q.closed = true
		clear(q.queue)
		q.queue = nil
		clear(q.tracked)
		clear(q.working)
		clear(q.sourceWorking)
		clear(q.sourceVersions)
		q.published = nil
		q.retry = nil
		q.rollback = nil
		q.files = nil
		if q.unsubscribe != nil {
			q.unsubscribe()
			q.unsubscribe = nil
		}
	}
}

func (e *Editor) SaveAsset(d *document.AssetDocument) error {
	if e.Docs == nil {
		return fmt.Errorf("document manager is unavailable")
	}
	return e.Docs.SaveAsset(d)
}

// ReloadMetadata discards the shared source journal, restores saved runtime textures after Undo/Redo previews, and invalidates candidates captured before Reload.
func (e *Editor) ReloadMetadata(d *document.AssetDocument) error {
	if e.Docs == nil || d == nil || e.Docs.AssetDocument(d.Asset) != d || d.Metadata == nil {
		return fmt.Errorf("metadata document is no longer open")
	}
	wasWorking := d.Metadata.SavedEdits > 0 || d.WorkingPreview.Applied != 0 || d.WorkingPreview.Pending
	if err := e.Docs.ReloadScript(d.Metadata); err != nil {
		return err
	}
	if wasWorking && (d.Kind == asset.ArtTexture || d.Kind == asset.ArtSpriteRegion) {
		return e.ApplyMetadataPreview(d.Asset)
	}
	return nil
}

func (e *Editor) UndoAsset(d *document.AssetDocument) error { return e.assetHistory(d, false) }
func (e *Editor) RedoAsset(d *document.AssetDocument) error { return e.assetHistory(d, true) }

func (e *Editor) assetHistory(d *document.AssetDocument, redo bool) error {
	if d == nil {
		return nil
	}
	if handled, err := structuredAssetHistory(d, redo); handled {
		return err
	}
	var source *document.ScriptDocument
	if d.Metadata != nil {
		source = d.Metadata
	} else {
		source = d.Source
	}
	if source != nil {
		changed := false
		if redo {
			changed = source.Code.Redo()
		} else {
			changed = source.Code.Undo()
		}
		if changed && source == d.Metadata && source.SavedEdits > 0 && (d.Kind == asset.ArtTexture || d.Kind == asset.ArtSpriteRegion) {
			return e.ApplyMetadataPreview(d.Asset)
		}
		return nil
	}
	if d.Scene != nil {
		if redo {
			return d.Scene.History.Redo()
		}
		return d.Scene.History.Undo()
	}
	if redo && d.Binding.Redo != nil {
		return d.Binding.Redo()
	}
	if !redo && d.Binding.Undo != nil {
		return d.Binding.Undo()
	}
	return nil
}
