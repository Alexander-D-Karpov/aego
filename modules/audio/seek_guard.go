package audio

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"hash"

	"aego/asset"
	"aego/audio/mix"
	"aego/core/ids"
)

// Hashing an entire large artifact on a browser's owner thread can block the
// UI for seconds. Copies are made while the caller owns the input; only the
// private copies are then hashed, in bounded pieces during subsequent frames.
const previewSeekHashChunk = 128 << 10

// ErrPreviewSeekSourceChanged means the displayed source cannot be verified
// against the active preview. The editor can present it in the user's language.
var ErrPreviewSeekSourceChanged = errors.New("audio: the preview source is different or cannot be verified")

type previewSeekGuard struct {
	source         *Clip
	sourceArtifact *asset.AudioArtifact
	sourceData     []byte // owner-thread access only; never read by a worker
	preview        *asset.AudioArtifact
	previewData    []byte // owner-thread access only; never read by a worker
	artifact       asset.AudioArtifact
	displayed      []byte
	hash           hash.Hash
	digest         [sha256.Size]byte
	offset         int
	mixer          *mix.Mixer
	voice          mix.VoiceID
	id             ids.AssetID
	epoch          uint64
	rate           int
	paused         bool
	loop           bool
	frame          int64
	stream         bool
}

func samePreviewBytes(a, b []byte) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// RequestPreviewMemorySeek checks the displayed clip against the active preview
// before queueing a seek in decoded memory frames. A nil error means the request
// was accepted, not that verification or mixer execution has completed. Large
// source hashes are spread over frames; check PreviewStreamSeekPending and
// TakePreviewStreamSeekError for the outcome. Until the request settles, the
// caller must access and mutate the clip and its artifact only on the module
// owner thread. Source checks are best-effort, not an atomic snapshot or CAS.
func (m *Module) RequestPreviewMemorySeek(c *Clip, frame int64) error {
	return m.requestPreviewSeek(c, frame, false)
}

// RequestPreviewStreamSeek is the source-frame equivalent for a streaming
// preview. It copies the source before returning and performs the remaining
// verification incrementally. The feeder uses only that private snapshot.
// The same owner-thread and best-effort constraints apply as for memory seek.
func (m *Module) RequestPreviewStreamSeek(c *Clip, frame int64) error {
	return m.requestPreviewSeek(c, frame, true)
}

func (m *Module) requestPreviewSeek(c *Clip, frame int64, stream bool) error {
	if m == nil || m.mixer == nil || !m.mixer.Live(m.preview) || c == nil || c.ID != m.previewID ||
		m.previewArtifact == nil || !m.previewGuard || c.Art == nil {
		return ErrPreviewSeekSourceChanged
	}
	q, a, b := &m.streamSeek, m.previewArtifact, c.Art
	if q.busy() {
		return errors.New("audio: the previous preview seek is still finishing")
	}
	// A pending guard and a subsequent mixer command each advance the epoch.
	if m.PreviewEpoch() >= ^uint64(0)-2 {
		return errors.New("audio: preview generation is exhausted")
	}
	if a.Format != b.Format || a.Rate != b.Rate || a.Channels != b.Channels || a.Frames != b.Frames ||
		len(a.Data) == 0 || len(a.Data) != len(b.Data) || len(a.Data) > MaxPrivatePreviewPayloadBytes {
		return ErrPreviewSeekSourceChanged
	}
	if stream {
		if m.previewRate > 0 || !a.Stream || a.Frames <= 0 || a.Frames > 1<<53 || frame < 0 || frame >= a.Frames ||
			m.previewLoop && a.LoopStart >= 0 && a.LoopEnd > a.LoopStart && a.LoopEnd <= a.Frames && frame >= a.LoopEnd {
			return errors.New("audio: stream seek frame or source is invalid")
		}
	} else if frames, _, ok := m.PreviewMemorySeekRange(); !ok || frame < 0 || frame >= frames {
		return errors.New("audio: preview seek frame is outside the playable range")
	}

	// Clone before leaving the owner thread. Even if a public Clip is changed
	// later, no background feeder or incremental hash reads its mutable data.
	displayed := bytes.Clone(b.Data)
	artifact := *a
	artifact.Data = nil
	if stream {
		if samePreviewBytes(a.Data, b.Data) {
			artifact.Data = displayed
		} else {
			artifact.Data = bytes.Clone(a.Data)
		}
	}
	m.bumpPreviewEpoch()
	g := &previewSeekGuard{source: c, sourceArtifact: b, sourceData: b.Data,
		preview: a, previewData: a.Data, artifact: artifact, displayed: displayed,
		hash: sha256.New(), digest: m.previewDigest, mixer: m.mixer, voice: m.preview,
		id: m.previewID, epoch: m.PreviewEpoch(), rate: m.mixer.SampleRate(),
		paused: m.previewPaused, loop: m.previewLoop, frame: frame, stream: stream}
	q.guard, q.err = g, nil
	m.pollPreviewSeekGuard()
	if q.guard == nil && q.err != nil {
		err := q.err
		q.err = nil
		return err
	}
	return nil
}

func (m *Module) pollPreviewSeekGuard() {
	q := &m.streamSeek
	g := q.guard
	if g == nil {
		return
	}
	if m.mixer != g.mixer || m.mixer == nil || m.preview != g.voice || m.previewID != g.id ||
		m.PreviewEpoch() != g.epoch || m.mixer.SampleRate() != g.rate || m.previewPaused != g.paused ||
		m.previewLoop != g.loop || !m.mixer.Live(g.voice) {
		q.guard = nil
		return
	}
	a, b := m.previewArtifact, g.source.Art
	if a != g.preview || g.source.ID != g.id || b != g.sourceArtifact ||
		!samePreviewBytes(a.Data, g.previewData) || !samePreviewBytes(b.Data, g.sourceData) ||
		a.Format != b.Format || a.Rate != b.Rate || a.Channels != b.Channels || a.Frames != b.Frames {
		q.guard = nil
		q.err = ErrPreviewSeekSourceChanged
		return
	}
	end := min(g.offset+previewSeekHashChunk, len(g.displayed))
	part := g.displayed[g.offset:end]
	if !bytes.Equal(part, b.Data[g.offset:end]) ||
		g.stream && (!bytes.Equal(part, g.artifact.Data[g.offset:end]) ||
			!bytes.Equal(g.artifact.Data[g.offset:end], a.Data[g.offset:end])) {
		q.guard = nil
		q.err = ErrPreviewSeekSourceChanged
		return
	}
	_, _ = g.hash.Write(part)
	g.offset = end
	if end < len(g.displayed) {
		return
	}
	q.guard = nil
	if !bytes.Equal(g.hash.Sum(nil), g.digest[:]) {
		q.err = ErrPreviewSeekSourceChanged
		return
	}
	if g.stream {
		m.startStreamSeek(g.frame, g.artifact)
	} else {
		q.err = m.SeekPreviewMemory(g.frame)
	}
}
