package editor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"aego/asset/pipeline"
	"aego/audio/codec"
	"aego/audio/edit"
	"aego/audio/edit/codecsource"
	"aego/core/ids"
	editorcontext "aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/modules/audio"
	"aego/ui/core"
	"aego/ui/widgets"
)

// A single worker and one latest request are shared by document views. Only
// the owner thread accesses this queue; the worker receives private bytes.
type sourceAnalysisQueue struct {
	sequence uint64
	running  *sourceAnalysisRequest
	waiting  *sourceAnalysisRequest
	result   <-chan sourceAnalysisResult
	ready    *sourceAnalysisResult
	// Deterministic checkpoint for cancellation and stale-generation tests.
	beforeVerify func()
}

type sourceAnalysisRequest struct {
	ctx      context.Context
	cancel   context.CancelFunc
	sequence uint64
	docs     *document.Manager
	docID    document.ID
	id       ids.AssetID
	path     string
	pipe     *pipeline.Pipeline
	source   []byte
	rate     int
	channels int
	frames   int64
	region   audioedit.Range
	view     *plugin
	current  func() bool // owner thread only
}

type sourceAnalysisResult struct {
	request *sourceAnalysisRequest
	value   audioedit.SignalAnalysis
	err     error
}

func (p *plugin) cancelSourceAnalysis() {
	q := &p.analysis
	if q.sequence < ^uint64(0) {
		q.sequence++
	}
	if q.running != nil {
		q.running.cancel()
		// A closed view can own a large loaded source. The worker never reads
		// these owner-only fields and keeps only its private source snapshot.
		q.running.view, q.running.current = nil, nil
	}
	if q.waiting != nil {
		q.waiting.cancel()
		q.waiting = nil
	}
	q.ready = nil
}

func (p *plugin) closeSourceAnalysis(docs *document.Manager, id document.ID) {
	q := &p.analysis
	if q.waiting != nil && q.waiting.docs == docs && q.waiting.docID == id {
		p.cancelSourceAnalysis()
		return
	}
	if q.ready != nil && q.ready.request.docs == docs && q.ready.request.docID == id {
		q.ready = nil
	}
	if q.running != nil && q.running.docs == docs && q.running.docID == id {
		if q.running.sequence == q.sequence {
			p.cancelSourceAnalysis()
		} else {
			// The canceled worker belongs to an older document. It must not
			// cancel a newer queued request from a different document.
			q.running.cancel()
			q.running.view, q.running.current = nil, nil
		}
	}
}

// startSourceAnalysis analyzes the loaded WAV/OGG source, not import gain,
// mixer effects or a future edited document. Copying the bounded payload is
// done on the owner thread because Clip data is not worker-owned.
func (p *plugin) startSourceAnalysis(ed *editorcontext.Editor, v ClipView) error {
	owner := p.audioPreviewOwner()
	owner.cancelSourceAnalysis()
	if ed == nil || ed.Docs == nil || ed.Pipeline == nil || ed.Loader == nil || v.source == nil || v.source.Art == nil {
		return errors.New(i18n.Tr("i18n.audio.analysis_unavailable", "Analysis requires a selected WAV or OGG source of at most 64 MiB."))
	}
	d := ed.Docs.AssetDocument(v.ID)
	start, end, selected := p.clip.waveSelection(v.Frames)
	ext := strings.ToLower(filepath.Ext(v.Path))
	a := v.source.Art
	if d == nil || d.Metadata == nil || d.Metadata.Code == nil || d.Metadata.Conflict || !selected || (ext != ".wav" && ext != ".ogg") ||
		v.source.ID != v.ID || d.Path != v.Path || !p.clip.waveBound || !p.clip.waveSource.present ||
		a.Rate != v.Rate || a.Channels != v.Channels || a.Frames != v.Frames ||
		len(a.Data) == 0 || len(a.Data) > audio.MaxPrivatePreviewPayloadBytes || owner.analysis.sequence == ^uint64(0) ||
		p.clip.waveSource.docs != ed.Docs || p.clip.waveSource.id != v.ID || p.clip.waveSource.first != &a.Data[0] ||
		p.clip.waveSource.length != len(a.Data) || p.clip.waveSource.format != a.Format || p.clip.waveSource.rate != a.Rate ||
		p.clip.waveSource.channels != a.Channels || p.clip.waveSource.frames != a.Frames {
		return errors.New(i18n.Tr("i18n.audio.analysis_unavailable", "Analysis requires a selected WAV or OGG source of at most 64 MiB."))
	}
	docs, pipe, loader, project, metadata, journal := ed.Docs, ed.Pipeline, ed.Loader, ed.Project, d.Metadata, d.Metadata.Code
	id, path, wave, frames := v.ID, v.Path, p.clip.waveSource, v.Frames
	revision, saved, assetRevision, pipelineRevision := journal.Rev(), metadata.SavedEdits, ed.AssetRev, pipe.Revision()
	ctx, cancel := context.WithCancel(context.Background())
	r := &sourceAnalysisRequest{ctx: ctx, cancel: cancel, sequence: owner.analysis.sequence, docs: docs, docID: d.ID,
		id: id, path: path, pipe: pipe, source: bytes.Clone(a.Data), rate: v.Rate, channels: v.Channels,
		frames: frames, region: audioedit.Range{Start: start, End: end}, view: p}
	r.current = func() bool {
		x, y, ok := p.clip.waveSelection(frames)
		return ed.Docs == docs && ed.Pipeline == pipe && ed.Loader == loader && ed.Project == project &&
			ed.AssetRev == assetRevision && pipe.Revision() == pipelineRevision && docs.AssetDocument(id) == d &&
			d.Asset == id && d.Path == path && d.Metadata == metadata && metadata.Code == journal &&
			journal.Rev() == revision && metadata.SavedEdits == saved && !metadata.Conflict &&
			p.clip.waveBound && p.clip.waveSource == wave && ok && x == start && y == end
	}
	q := &owner.analysis
	if q.running != nil {
		q.waiting = r
	} else {
		q.launch(r)
	}
	return nil
}

func (q *sourceAnalysisQueue) launch(r *sourceAnalysisRequest) {
	out := make(chan sourceAnalysisResult, 1)
	q.running, q.result = r, out
	beforeVerify := q.beforeVerify
	go func() {
		result := sourceAnalysisResult{request: r}
		defer func() {
			if value := recover(); value != nil {
				result.err = fmt.Errorf("audio analysis worker panicked: %v", value)
			}
			if err := r.ctx.Err(); err != nil {
				result.value = audioedit.SignalAnalysis{}
				if result.err == nil {
					result.err = err
				}
			}
			r.source = nil
			r.cancel()
			out <- result
		}()
		if err := r.ctx.Err(); err != nil {
			result.err = err
			return
		}
		sum := sha256.Sum256(r.source)
		decoder, err := codec.Open(bytes.NewReader(r.source), r.path)
		if err != nil {
			result.err = err
			return
		}
		defer decoder.Close()
		info := decoder.Info()
		if info.SampleRate != r.rate || info.Channels != r.channels || info.Frames != r.frames {
			result.err = errors.New("audio source layout changed")
			return
		}
		reader, err := codecsource.New(decoder)
		if err != nil {
			result.err = err
			return
		}
		doc, err := audioedit.New(audioedit.Source{AssetID: r.id.String(), Path: r.path, SHA256: hex.EncodeToString(sum[:]),
			Codec: strings.TrimPrefix(strings.ToLower(filepath.Ext(r.path)), "."), SampleRate: r.rate, Channels: r.channels, Frames: r.frames})
		if err != nil {
			result.err = err
			return
		}
		result.value, result.err = audioedit.Analyze(r.ctx, reader, doc, r.region)
		if result.err != nil {
			return
		}
		if beforeVerify != nil {
			beforeVerify()
		}
		fingerprint := pipeline.SourceFingerprint{Exists: true, Size: int64(len(r.source)), SHA256: sum}
		result.err = r.pipe.VerifyGuardedPreviewSource(r.ctx, r.id, r.path, fingerprint)
	}()
}

func (p *plugin) pollSourceAnalysis() {
	q := &p.analysis
	current := q.waiting
	if current == nil && q.running != nil && q.running.sequence == q.sequence {
		current = q.running
	}
	if current != nil && (current.current == nil || !current.current()) {
		p.cancelSourceAnalysis()
	}
	if q.ready != nil && !q.ready.request.current() {
		q.ready = nil
	}
	if q.result == nil {
		return
	}
	select {
	case result := <-q.result:
		q.running, q.result = nil, nil
		if next := q.waiting; next != nil {
			q.waiting = nil
			if next.sequence == q.sequence && next.current != nil && next.current() {
				q.launch(next)
			} else {
				next.cancel()
			}
		}
		if result.request.sequence != q.sequence || result.request.current == nil || !result.request.current() {
			return
		}
		if result.err != nil {
			if !errors.Is(result.err, context.Canceled) {
				p.notify(fmt.Errorf("%s: %w", i18n.Tr("i18n.audio.analysis_failed", "Source analysis failed"), result.err))
			}
			return
		}
		q.ready = &result
	default:
	}
}

func (p *plugin) sourceAnalysisUI(ed *editorcontext.Editor, v ClipView) {
	owner := p.audioPreviewOwner()
	owner.pollSourceAnalysis()
	if _, _, ok := p.clip.waveSelection(v.Frames); !ok || v.source == nil {
		return
	}
	q := &owner.analysis
	current := q.waiting
	if current == nil && q.running != nil && q.running.sequence == q.sequence {
		current = q.running
	}
	if current != nil && current.view == p {
		if sourceAnalysisButton(ed.UI, i18n.Tr("i18n.audio.analysis_cancel", "Cancel analysis")) {
			owner.cancelSourceAnalysis()
		}
		widgets.LabelWrapped(ed.UI, i18n.Tr("i18n.audio.analysis_pending", "Analyzing loaded source..."), true)
	} else if sourceAnalysisButton(ed.UI, i18n.Tr("i18n.audio.analysis_start", "Analyze source selection")) {
		if err := p.startSourceAnalysis(ed, v); err != nil {
			p.notify(err)
		}
	}
	if q.ready == nil || q.ready.request.view != p {
		return
	}
	widgets.LabelWrapped(ed.UI, i18n.Tr("i18n.audio.analysis_scope", "Loaded source before import and playback gain; full-scale samples may indicate clipping."), true)
	for i, channel := range q.ready.value.Channels {
		widgets.LabelWrapped(ed.UI, i18n.Tr("i18n.audio.analysis_channel", "Channel {channel}: peak {peak}, RMS {rms}, full-scale frames {count}",
			i18n.Str("channel", strconv.Itoa(i+1)), i18n.Str("peak", strconv.FormatFloat(channel.Peak, 'g', 4, 64)),
			i18n.Str("rms", strconv.FormatFloat(channel.RMS, 'g', 4, 64)), i18n.Str("count", strconv.FormatInt(channel.ClippedFrames, 10))), true)
	}
}

func sourceAnalysisButton(ui *core.Context, label string) bool {
	id := ui.ID("source-analysis")
	r := ui.Cursor().Next(0)
	clicked := widgets.ButtonRect(ui, id, r, widgets.Elide(ui, label, max(0, r.W-12)))
	widgets.Tooltip(ui, id, label)
	return clicked
}
