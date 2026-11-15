package editor

import (
	"bytes"
	stdcontext "context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"aego/asset"
	"aego/asset/audiosidecar"
	"aego/asset/pipeline"
	"aego/audio/edit"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/modules/audio"
)

// workingAudioQueue has at most one CPU worker and one latest queued snapshot.
// Only its owner thread touches the queue; workers see immutable snapshots.
type workingAudioQueue struct {
	sequence uint64
	running  *workingAudioRequest
	waiting  *workingAudioRequest
	result   <-chan workingAudioResult
	// Deterministic checkpoints for exercising changes during worker execution.
	beforeVerify, afterVerify func()
}

type workingAudioRequest struct {
	ctx      stdcontext.Context
	cancel   stdcontext.CancelFunc
	sequence uint64
	id       ids.AssetID
	docID    document.ID
	docs     *document.Manager
	path     string
	pipeline *pipeline.Pipeline
	source   []byte
	metadata []byte
	edit     []byte
	module   *audio.Module
	rate     int
	loop     bool
	current  func() bool // owner thread only
}

type workingAudioResult struct {
	request    *workingAudioRequest
	prepared   *audio.PreparedPreview
	verifiedAt time.Time
	err        error
}

func (p *plugin) audioPreviewOwner() *plugin {
	if p.auditionOwner != nil {
		return p.auditionOwner
	}
	return p
}

func (p *plugin) cancelWorkingAudio() {
	q := &p.workingAudio
	if q.sequence < ^uint64(0) {
		q.sequence++
	}
	if q.running != nil {
		q.running.cancel()
	}
	if q.waiting != nil {
		q.waiting.cancel()
		q.waiting = nil
	}
	if q.result != nil {
		select {
		case <-q.result:
			q.running, q.result = nil, nil
		default:
		}
	}
}

func (p *plugin) workingAudioPending(id ids.AssetID) bool {
	q := &p.workingAudio
	if q.waiting != nil {
		return q.waiting.sequence == q.sequence && q.waiting.id == id
	}
	return q.running != nil && q.running.sequence == q.sequence && q.running.id == id
}

func (p *plugin) prepareWorkingAudio(ed *context.Editor, mod *audio.Module, d *document.AssetDocument, loop bool) error {
	p.cancelWorkingAudio()
	if ed == nil || ed.Docs == nil || ed.Pipeline == nil || ed.Loader == nil || ed.Engine == nil ||
		mod == nil || mod.Mixer() == nil || d == nil || ed.Docs.AssetDocument(d.Asset) != d || d.Metadata == nil ||
		d.Metadata.Code == nil || d.Metadata.ReadOnly || d.Metadata.Conflict || !d.Metadata.Dirty() {
		return errors.New("audio: working preview needs an open, editable metadata document")
	}
	if audio.ModuleOf(ed.Engine) != mod {
		return errors.New("audio: preview output changed")
	}
	switch strings.ToLower(filepath.Ext(d.Path)) {
	case ".wav", ".ogg":
	default:
		return errors.New(i18n.Tr("editor.audio.preview_save_required", "Save this audio's import settings before previewing it."))
	}
	if p.workingAudio.sequence == ^uint64(0) || mod.PreviewEpoch() == ^uint64(0) {
		return errors.New("audio: private preview generation is exhausted")
	}
	clip, err := mod.Clip(ed.Loader, d.Asset)
	if err != nil {
		return err
	}
	if clip == nil || clip.Art == nil || len(clip.Art.Data) > audio.MaxPrivatePreviewPayloadBytes || d.Metadata.Code.Len() > 16<<20 {
		return errors.New("audio: working preview exceeds its source or metadata budget")
	}
	pipe, docs, loader, eng, project := ed.Pipeline, ed.Docs, ed.Loader, ed.Engine, ed.Project
	metadata, journal := d.Metadata, d.Metadata.Code
	id, path := d.Asset, d.Path
	revision, saved, assetRevision, pipelineRevision := journal.Rev(), metadata.SavedEdits, ed.AssetRev, pipe.Revision()
	epoch, rate := mod.PreviewEpoch(), mod.Mixer().SampleRate()
	edit, _ := journal.Volatile()
	ctx, cancel := stdcontext.WithCancel(stdcontext.Background())
	r := &workingAudioRequest{ctx: ctx, cancel: cancel, sequence: p.workingAudio.sequence, id: id, docID: d.ID, docs: docs, path: path,
		pipeline: pipe, source: bytes.Clone(clip.Art.Data), metadata: bytes.Clone(journal.Bytes()), edit: edit, module: mod, rate: rate, loop: loop}
	r.current = func() bool {
		return ed.Pipeline == pipe && ed.Docs == docs && ed.Loader == loader && ed.Engine == eng && ed.Project == project &&
			ed.AssetRev == assetRevision && pipe.Revision() == pipelineRevision && docs.AssetDocument(id) == d &&
			d.Asset == id && d.Path == path && d.Metadata == metadata && metadata.Code == journal &&
			journal.Rev() == revision && metadata.SavedEdits == saved && metadata.Dirty() && !metadata.ReadOnly && !metadata.Conflict &&
			audio.ModuleOf(eng) == mod && mod.Mixer() != nil && mod.Mixer().SampleRate() == rate && mod.PreviewEpoch() == epoch
	}
	q := &p.workingAudio
	if q.running != nil {
		q.waiting = r
	} else {
		q.launch(r)
	}
	return nil
}

func (p *plugin) closeWorkingAudio(docs *document.Manager, id document.ID) {
	q := &p.workingAudio
	for _, r := range []*workingAudioRequest{q.running, q.waiting} {
		if r != nil && r.sequence == q.sequence && r.docs == docs && r.docID == id {
			p.cancelWorkingAudio()
			return
		}
	}
}

func (q *workingAudioQueue) launch(r *workingAudioRequest) {
	out := make(chan workingAudioResult, 1)
	q.running, q.result = r, out
	beforeVerify, afterVerify := q.beforeVerify, q.afterVerify
	go func() {
		result := workingAudioResult{request: r}
		defer func() {
			if value := recover(); value != nil {
				result.err = fmt.Errorf("audio preview worker panicked: %v", value)
			}
			if err := r.ctx.Err(); err != nil {
				result.prepared = nil
				if result.err == nil {
					result.err = err
				}
			}
			r.source, r.metadata, r.edit = nil, nil, nil
			r.cancel()
			out <- result
		}()
		metadata := r.metadata
		if len(r.edit) != 0 {
			doc, err := audioedit.Decode(r.edit)
			if err != nil {
				result.err = fmt.Errorf("audio: invalid in-memory edit: %w", err)
				return
			}
			if err := r.ctx.Err(); err != nil {
				result.err = err
				return
			}
			_, present, err := audiosidecar.Decode(metadata, r.path)
			if err != nil {
				result.err = err
				return
			}
			if present {
				result.err = errors.New("audio: working metadata already contains an audio edit")
				return
			}
			metadata, err = audiosidecar.Encode(metadata, r.path, doc)
			if err != nil {
				result.err = err
				return
			}
		}
		candidate, err := r.pipeline.PreviewGuardedWithMetadataContext(r.ctx, r.path, r.source, metadata)
		if err != nil {
			result.err = err
			return
		}
		if candidate == nil || candidate.Error != nil || candidate.Kind != asset.ArtAudio || len(candidate.Artifacts) != 1 || candidate.Artifacts[0].Kind != asset.ArtAudio {
			result.err = errors.New("audio importer returned an invalid private preview")
			return
		}
		result.prepared, result.err = audio.PreparePreviewArtifact(r.ctx, r.id, candidate.Artifacts[0].Data, r.rate)
		if result.err != nil {
			return
		}
		if beforeVerify != nil {
			beforeVerify()
		}
		fingerprint := pipeline.SourceFingerprint{Exists: true, Size: int64(len(r.source)), SHA256: sha256.Sum256(r.source)}
		if result.err = r.pipeline.VerifyGuardedPreviewSource(r.ctx, r.id, r.path, fingerprint); result.err != nil {
			result.prepared = nil
			return
		}
		result.verifiedAt = time.Now()
		if afterVerify != nil {
			afterVerify()
		}
	}()
}

func (p *plugin) pollWorkingAudio(mod *audio.Module) {
	q := &p.workingAudio
	if q.result == nil {
		return
	}
	current := q.waiting
	if current == nil && q.running != nil && q.running.sequence == q.sequence {
		current = q.running
	}
	if current != nil && !current.current() {
		p.cancelWorkingAudio()
		p.notify(errors.New(i18n.Tr("editor.audio.preview_stale", "The audio or preview changed while preparing. Play again to use the current version.")))
		return
	}
	select {
	case result := <-q.result:
		q.running, q.result = nil, nil
		if next := q.waiting; next != nil {
			q.waiting = nil
			if next.sequence == q.sequence && next.current() {
				q.launch(next)
			} else {
				next.cancel()
			}
		}
		if result.request.sequence != q.sequence {
			return
		}
		if result.err != nil {
			if !errors.Is(result.err, stdcontext.Canceled) {
				p.notify(result.err)
			}
			return
		}
		if result.prepared == nil || mod != result.request.module || !result.request.current() || time.Since(result.verifiedAt) > time.Second {
			p.notify(errors.New(i18n.Tr("editor.audio.preview_stale", "The audio or preview changed while preparing. Play again to use the current version.")))
			return
		}
		if err := mod.PreviewPrepared(result.prepared, result.request.loop); err != nil {
			p.notify(err)
			return
		}
		p.aud = audition{}
	default:
	}
}
