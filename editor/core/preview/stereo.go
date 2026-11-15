package preview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"path/filepath"
	"strconv"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/edit"
	"aego/audio/edit/codecsource"
	"aego/core/ids"
	"aego/editor/core/tasks"
	"aego/engine/i18n"
)

const (
	waveArtifactLimit = 16 << 20
	wavePeakBuckets   = 16000
)

type wavePeakItem struct {
	key       string
	data      []float32 // one peak per mono column or interleaved L/R peaks per stereo column
	pyramid   audioedit.Pyramid
	view      []float32
	viewStart int64
	viewEnd   int64
	done      bool
	task      *tasks.Task
}

// StereoPeaks builds the imported stereo artifact's waveform on the existing
// preview queue. It never reads the source file or writes metadata. Results
// are keyed to the imported artifact and discarded on reimport or cancellation.
// A failed or unsupported request returns nil with done=true so the caller can
// retain its existing mono fallback.
func (s *Service) StereoPeaks(id ids.AssetID, cols int) ([]float32, bool) {
	return s.channelPeaks(id, cols, 2)
}

func (s *Service) channelPeaks(id ids.AssetID, cols, channels int) ([]float32, bool) {
	if channels != 1 && channels != 2 {
		return nil, true
	}
	e, ok := s.entry(id)
	if !ok || e.Kind != asset.ArtAudio || s.d.Tasks == nil || s.d.Pipeline == nil || cols <= 0 || cols > 4096 {
		return nil, true
	}
	key, ok := s.Key(e)
	if !ok {
		return nil, true
	}
	art, ok := e.Artifact(e.Kind)
	if !ok {
		return nil, true
	}
	cache, suffix := &s.stereoPeaks, "-stereo-"
	if channels == 1 {
		cache, suffix = &s.monoWavePeaks, "-mono-"
	}
	key += suffix + strconv.Itoa(cols)
	pk := peakKey{id, cols}
	if *cache == nil {
		*cache = make(map[peakKey]*wavePeakItem)
	}
	items := *cache
	old := items[pk]
	if old != nil && old.key == key {
		return old.data, old.done
	}
	if old != nil && old.task != nil {
		old.task.Cancel()
	}
	it := &wavePeakItem{key: key}
	items[pk] = it
	files := s.d.Pipeline.Artifacts()
	var out []float32
	var pyramid audioedit.Pyramid
	spec := tasks.Spec{Title: i18n.Tr("editor.preview.waveform_name", "Waveform {name}", i18n.Str("name", filepath.Base(e.Name))), Kind: "preview", Priority: s.frame}
	it.task = s.d.Tasks.Work(spec, func(t *tasks.Task) error {
		f, err := files.Open(art.Path)
		if err != nil {
			return err
		}
		raw, err := readWaveArtifact(f, art)
		if err != nil || t.Canceled() {
			return err
		}
		a, err := asset.DecodeAudio(raw)
		if err != nil {
			return err
		}
		d, err := codec.Open(bytes.NewReader(a.Data), "")
		if err != nil {
			return err
		}
		defer d.Close()
		info := d.Info()
		if info.Channels != channels || info.Channels != a.Channels || info.SampleRate != a.Rate || info.Frames != a.Frames {
			return errors.New("waveform does not match its imported artifact")
		}
		reader, err := codecsource.New(d)
		if err != nil {
			return err
		}
		pyramid, err = buildWavePyramid(taskPeakReader{reader: reader, task: t}, info.Frames, channels)
		if err == nil {
			out, err = wavePeakColumns(pyramid, 0, info.Frames, cols, channels)
		}
		return err
	}).Then(func(t *tasks.Task) {
		if items[pk] != it {
			return
		}
		it.task, it.done = nil, true
		current, ok := s.entry(id)
		if !ok {
			delete(items, pk)
			return
		}
		currentKey, ok := s.Key(current)
		if !ok || currentKey+suffix+strconv.Itoa(cols) != it.key {
			delete(items, pk)
			return
		}
		if t.State() == tasks.Done {
			it.data, it.pyramid = out, pyramid
		}
	})
	return nil, false
}

// StereoPeaksWindow returns a read-only view of the same imported artifact's
// bounded pyramid. The initial request schedules the same background work as
// StereoPeaks; changing a viewport never decodes or writes the source again.
// Edge buckets can include samples outside the exact range; these amplitudes
// are a display approximation, not a sample-accurate editing selection.
func (s *Service) StereoPeaksWindow(id ids.AssetID, start, end int64, cols int) ([]float32, bool) {
	return s.channelPeaksWindow(id, start, end, cols, 2)
}

// MonoPeaksWindow is the equivalent bounded imported-artifact view for a mono
// document. It does not change the compact Inspector's existing peak cache.
func (s *Service) MonoPeaksWindow(id ids.AssetID, start, end int64, cols int) ([]float32, bool) {
	return s.channelPeaksWindow(id, start, end, cols, 1)
}

func (s *Service) channelPeaksWindow(id ids.AssetID, start, end int64, cols, channels int) ([]float32, bool) {
	if start < 0 || end <= start {
		return nil, true
	}
	full, done := s.channelPeaks(id, cols, channels)
	if !done || len(full) == 0 {
		return nil, done
	}
	items := s.stereoPeaks
	if channels == 1 {
		items = s.monoWavePeaks
	}
	it := items[peakKey{id, cols}]
	if it == nil || len(it.pyramid.Levels) == 0 || end > it.pyramid.TotalFrames {
		return nil, true
	}
	if start == 0 && end == it.pyramid.TotalFrames {
		return full, true
	}
	if it.view != nil && it.viewStart == start && it.viewEnd == end {
		return it.view, true
	}
	data, err := wavePeakColumns(it.pyramid, start, end, cols, channels)
	if err != nil {
		return nil, true
	}
	it.viewStart, it.viewEnd, it.view = start, end, data
	return it.view, true
}

func readWaveArtifact(f interface {
	io.Reader
	io.Closer
	Size() int64
}, art asset.Artifact) ([]byte, error) {
	defer f.Close()
	if f.Size() < 0 || f.Size() > waveArtifactLimit || art.Size > waveArtifactLimit {
		return nil, errors.New("waveform artifact exceeds its memory limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, waveArtifactLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > waveArtifactLimit || int64(len(raw)) != f.Size() || (art.Size > 0 && int64(len(raw)) != art.Size) {
		return nil, errors.New("waveform artifact changed during read")
	}
	sum := sha256.Sum256(raw)
	if art.Hash == "" || hex.EncodeToString(sum[:8]) != art.Hash {
		return nil, errors.New("waveform artifact hash changed")
	}
	return raw, nil
}

type taskPeakReader struct {
	reader audioedit.Reader
	task   *tasks.Task
}

func (r taskPeakReader) ReadFrames(ctx context.Context, start int64, frames int) ([]float32, error) {
	if r.task.Canceled() {
		return nil, context.Canceled
	}
	data, err := r.reader.ReadFrames(ctx, start, frames)
	if r.task.Canceled() {
		return nil, context.Canceled
	}
	return data, err
}

func buildWavePyramid(reader audioedit.Reader, frames int64, channels int) (audioedit.Pyramid, error) {
	if reader == nil || frames <= 0 || (channels != 1 && channels != 2) {
		return audioedit.Pyramid{}, errors.New("invalid waveform request")
	}
	base := 512
	for (frames-1)/int64(base)+1 > wavePeakBuckets && base < 65536 {
		base *= 2
	}
	if (frames-1)/int64(base)+1 > wavePeakBuckets {
		return audioedit.Pyramid{}, errors.New("waveform is too long")
	}
	return audioedit.BuildPyramid(context.Background(), reader, frames, channels, base)
}

func wavePeakColumns(pyramid audioedit.Pyramid, start, end int64, cols, channels int) ([]float32, error) {
	if cols <= 0 || cols > 4096 || (channels != 1 && channels != 2) || pyramid.Channels != channels {
		return nil, errors.New("invalid waveform viewport")
	}
	level, _, _, err := pyramid.Visible(start, end, cols)
	if err != nil {
		return nil, err
	}
	out := make([]float32, cols*channels)
	span := end - start
	width := int64(cols)
	whole, remainder := span/width, span%width
	// Calculate pixel boundaries without multiplying the entire frame span by
	// the column number. That also preserves a partial first/last bucket in
	// its actual position, rather than stretching both buckets equally.
	offset := func(col int64) int64 {
		return whole*col + remainder*col/width
	}
	for col := range cols {
		left := start + offset(int64(col))
		right := start + offset(int64(col+1))
		if right == left {
			right++ // More columns than frames: repeat the visible frame.
		}
		lo := int(left / level.FramesPerBucket)
		hi := int((right-1)/level.FramesPerBucket) + 1
		for i := lo; i < hi; i++ {
			for ch := range channels {
				p := level.Samples[i*channels+ch]
				v := float32(math.Max(math.Abs(float64(p.Min)), math.Abs(float64(p.Max))))
				out[col*channels+ch] = max(out[col*channels+ch], v)
			}
		}
	}
	return out, nil
}
