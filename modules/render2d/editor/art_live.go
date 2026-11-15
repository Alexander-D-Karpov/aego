package editor

import (
	"aego/editor/core/artedit"
	"aego/editor/core/context"
	"aego/editor/core/document"
)

type artPreviewStamp struct {
	revision  uint64
	attempted uint64
	pipeline  uint64
	changed   float64
}

// Only completed authoring changes reach the game. A live stroke remains a
// private canvas draft. Coalesce rapid Undo/Redo into the engine's existing
// latest-candidate queue; no files, colliders or scene logic are modified here.
func pollArtSourcePreviews(ed *context.Editor, stamps map[document.ID]artPreviewStamp) {
	if ed == nil || ed.Docs == nil || ed.Pipeline == nil {
		return
	}
	pipelineRevision := ed.Pipeline.Revision()
	live := make(map[document.ID]bool)
	for _, d := range ed.Docs.AssetDocuments() {
		b, ok := d.Binding.Model.(*artedit.Document)
		if !ok {
			continue
		}
		// A PNG draft is not an .aeart source. Publishing its ZIP bytes through
		// the texture importer would be incorrect; wait for explicit linking.
		if b.Draft {
			continue
		}
		live[d.ID] = true
		revision := b.PreviewRevision()
		if revision == 0 {
			continue
		}
		stamp, seen := stamps[d.ID]
		if !seen && !b.Session.Info().Dirty {
			stamps[d.ID] = artPreviewStamp{revision: revision, attempted: revision, pipeline: pipelineRevision}
			continue
		}
		// A persistent import (including saving another open document) may
		// replace runtime resources. Rebuild the complete dirty source snapshot
		// through the same queue, even when this drawing's pixels did not change.
		if stamp.pipeline != pipelineRevision {
			stamp.pipeline = pipelineRevision
			if b.Session.Info().Dirty || d.WorkingPreview.Applied != 0 {
				stamp.attempted, stamp.changed = 0, ed.Now()
			}
		}
		if stamp.revision != revision {
			stamp.revision, stamp.changed = revision, ed.Now()
		}
		if stamp.attempted != revision && !b.Session.Info().Gesture && ed.Now()-stamp.changed >= .05 {
			stamp.attempted = revision
			if err := ed.ApplyMetadataPreview(d.Asset); err != nil {
				d.WorkingPreview.Error = err.Error()
				ed.Notify(context.LevelError, err.Error())
			}
		}
		stamps[d.ID] = stamp
	}
	for id := range stamps {
		if !live[id] {
			delete(stamps, id)
		}
	}
}
