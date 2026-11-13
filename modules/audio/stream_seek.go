package audio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"aego/asset"
	"aego/audio/mix"
	"aego/core/ids"
)

type streamSeekTimeline struct {
	origin             int64
	rate               int
	loopStart, loopEnd int64
	loop               bool
}

type streamSeekRequest struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mixer    *mix.Mixer
	voice    mix.VoiceID
	id       ids.AssetID
	epoch    uint64
	rate     int
	paused   bool
	loop     bool
	frame    int64
	gain     float32
	artifact asset.AudioArtifact
}

type streamSeekResult struct {
	request  *streamSeekRequest
	prepared *preparedStreamSeek
	detach   func() bool
	err      error
}

type pendingStreamReplacement struct {
	old, next mix.VoiceID
	prepared  *preparedStreamSeek
}

// Only the module owner thread touches this state. At most one preparation,
// one queued replacement and two retiring feeders can exist at a time.
type previewStreamSeek struct {
	guard    *previewSeekGuard
	request  *streamSeekRequest
	result   <-chan streamSeekResult
	pending  *pendingStreamReplacement
	draining [2]<-chan struct{}
	blocked  bool // fail closed if an invariant would exceed the cleanup bound
	err      error
}

func (q *previewStreamSeek) reap() {
	for i, done := range q.draining {
		if done == nil {
			continue
		}
		select {
		case <-done:
			q.draining[i] = nil
		default:
		}
	}
}

func (q *previewStreamSeek) track(done <-chan struct{}) {
	if done == nil {
		return
	}
	q.reap()
	select {
	case <-done:
		return
	default:
	}
	for _, old := range q.draining {
		if old == done {
			return
		}
	}
	for i, old := range q.draining {
		if old == nil {
			q.draining[i] = done
			return
		}
	}
	q.blocked = true
}

func (q *previewStreamSeek) busy() bool {
	q.reap()
	return q.blocked || q.guard != nil || q.request != nil || q.pending != nil || q.draining[0] != nil || q.draining[1] != nil
}

// SeekPreviewStream prepares a bounded, private stream in the background for a
// source-relative frame of the current streaming preview. One request at a time
// is allowed; callers can retry after PreviewStreamSeekPending becomes false.
// A nil error means preparation was requested, not that the mixer or output has
// moved. Already buffered output cannot be removed without affecting other voices.
// All preview methods are called on the module owner thread.
func (m *Module) SeekPreviewStream(frame int64) error {
	if m == nil || m.mixer == nil || !m.mixer.Live(m.preview) || m.previewArtifact == nil ||
		m.previewRate > 0 || !m.previewArtifact.Stream {
		return errors.New("audio: there is no active streaming preview to seek")
	}
	q, a := &m.streamSeek, m.previewArtifact
	if q.busy() {
		return errors.New("audio: the previous stream seek is still finishing")
	}
	// Starting the request advances the epoch once. Never capture the
	// saturated value: it cannot distinguish later preview actions.
	if m.PreviewEpoch() >= ^uint64(0)-1 {
		return errors.New("audio: preview generation is exhausted")
	}
	if !m.previewGuard || a.Frames <= 0 || a.Frames > 1<<53 || frame < 0 || frame >= a.Frames || len(a.Data) > MaxPrivatePreviewPayloadBytes {
		return errors.New("audio: stream seek frame or source size is invalid")
	}
	if m.previewLoop && a.LoopStart >= 0 && a.LoopEnd > a.LoopStart && a.LoopEnd <= a.Frames && frame >= a.LoopEnd {
		return errors.New("audio: stream seek is beyond the loop end")
	}
	copyArtifact := *a
	copyArtifact.Data = bytes.Clone(a.Data)
	if sha256.Sum256(copyArtifact.Data) != m.previewDigest {
		return errors.New("audio: the preview source changed before seeking")
	}
	m.startStreamSeek(frame, copyArtifact)
	return nil
}

// startStreamSeek takes ownership of an already checked private snapshot. The
// caller must validate the preview generation and make sure no seek is active.
func (m *Module) startStreamSeek(frame int64, copyArtifact asset.AudioArtifact) {
	q := &m.streamSeek
	ctx, cancel := context.WithCancel(context.Background())
	m.bumpPreviewEpoch()
	r := &streamSeekRequest{ctx: ctx, cancel: cancel, mixer: m.mixer, voice: m.preview, id: m.previewID,
		epoch: m.PreviewEpoch(), rate: m.mixer.SampleRate(), paused: m.previewPaused,
		loop: m.previewLoop, frame: frame, gain: m.previewGain, artifact: copyArtifact}
	out := make(chan streamSeekResult, 1)
	q.request, q.result, q.err = r, out, nil
	go func() {
		result := streamSeekResult{request: r}
		defer func() {
			if value := recover(); value != nil {
				result.err = fmt.Errorf("audio: stream seek worker panicked: %v", value)
			}
			if result.prepared != nil {
				// A cancelled request must stop its feeder even if the owner never
				// polls another frame (for example after shutdown).
				result.detach = context.AfterFunc(r.ctx, result.prepared.cancel)
			}
			r.artifact.Data = nil
			out <- result
		}()
		result.prepared, result.err = prepareStreamSeek(r.ctx, &r.artifact, r.frame, r.loop, r.rate)
	}()
}

// PreviewStreamSeekPending remains true during a guarded preview seek,
// preparation, mixer acknowledgement and bounded feeder cleanup. Guarded
// memory seeks may also be pending while their source snapshot is verified.
// It does not mean that audio output has drained.
func (m *Module) PreviewStreamSeekPending() bool {
	return m != nil && m.streamSeek.busy()
}

// TakePreviewStreamSeekError returns and clears a background preparation or
// replacement error. Cancellation due to another accepted preview action is not
// reported as an error.
func (m *Module) TakePreviewStreamSeekError() error {
	if m == nil {
		return nil
	}
	err := m.streamSeek.err
	m.streamSeek.err = nil
	return err
}

func (m *Module) cancelStreamSeekPreparation() {
	m.streamSeek.guard = nil
	if r := m.streamSeek.request; r != nil {
		r.cancel()
	}
}

func (m *Module) discardStreamSeek(result streamSeekResult) {
	if result.detach != nil {
		result.detach()
	}
	result.request.cancel()
	if result.prepared != nil {
		result.prepared.cancel()
		m.streamSeek.track(result.prepared.done)
	}
}

func (m *Module) pollStreamSeek() {
	q := &m.streamSeek
	q.reap()
	m.reconcileStreamSeek()
	m.pollPreviewSeekGuard()
	if q.result == nil {
		return
	}
	select {
	case result := <-q.result:
		q.request, q.result = nil, nil
		r, p := result.request, result.prepared
		current := r.ctx.Err() == nil && m.mixer == r.mixer && m.mixer != nil && m.preview == r.voice &&
			m.previewID == r.id && m.previewPaused == r.paused && m.PreviewEpoch() == r.epoch &&
			m.mixer.SampleRate() == r.rate && m.mixer.Live(r.voice)
		if !current || result.err != nil || p == nil {
			m.discardStreamSeek(result)
			if current && result.err != nil && !errors.Is(result.err, context.Canceled) {
				q.err = result.err
			} else if current && result.err == nil && p == nil {
				q.err = errors.New("audio: stream seek preparation returned no result")
			}
			return
		}
		select {
		case <-p.done:
			if p.err != nil {
				m.discardStreamSeek(result)
				q.err = p.err
				return
			}
		default:
		}
		// Detach cancellation before transferring ownership. A callback that
		// has already started makes the result stale instead of guessing.
		if result.detach == nil || !result.detach() {
			m.discardStreamSeek(result)
			return
		}
		r.cancel()
		params := mix.PlayParams{Stream: p.stream, Bus: int(BusUI), Gain: r.gain,
			Pitch: 1, Loop: r.loop, Priority: musicPriority}
		next := m.mixer.ReplaceStream(r.voice, params, p.initial, r.paused)
		if !next.Valid() {
			p.cancel()
			q.track(p.done)
			q.err = errors.New("audio: could not queue stream seek playback")
			return
		}
		q.pending = &pendingStreamReplacement{old: r.voice, next: next, prepared: p}
	default:
	}
}

func (m *Module) reconcileStreamSeek() {
	q := &m.streamSeek
	p := q.pending
	if p == nil || m.mixer == nil {
		return
	}
	if m.mixer.Started(p.next) && !m.mixer.Live(p.old) {
		m.closePreviewFeeder()
		m.preview = p.next
		m.previewCancel, m.previewClose, m.previewDone = p.prepared.stop, p.prepared.cancel, p.prepared.done
		m.previewTime = &streamSeekTimeline{origin: p.prepared.origin, rate: p.prepared.stream.Rate,
			loop: p.prepared.loop, loopStart: p.prepared.loopStart, loopEnd: p.prepared.loopEnd}
		q.pending = nil
		return
	}
	if !m.mixer.Live(p.next) {
		p.prepared.cancel()
		q.track(p.prepared.done)
		q.pending = nil
		q.err = errors.New("audio: the mixer could not apply the stream seek")
		if !m.mixer.Live(p.old) {
			m.endPreview()
		}
	}
}
