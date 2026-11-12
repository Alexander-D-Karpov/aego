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
	"aego/core/ids"
	"aego/editor/core/audioedit"
	"aego/editor/core/audioedit/codecsource"
	"aego/editor/core/tasks"
	"aego/engine/i18n"
)

const (
	stereoArtifactLimit = 16 << 20
	stereoPeakBuckets   = 16000
)

type stereoPeakItem struct {
	key  string
	data []float32 // interleaved left/right peak amplitudes, one pair per column
	done bool
	task *tasks.Task
}

// StereoPeaks builds the imported stereo artifact's waveform on the existing
// preview queue. It never reads the source file or writes metadata. Results
// are keyed to the imported artifact and discarded on reimport or cancellation.
// A failed or unsupported request returns nil with done=true so the caller can
// retain its existing mono fallback.
func (s *Service) StereoPeaks(id ids.AssetID, cols int) ([]float32, bool) {
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
	key += "-stereo-" + strconv.Itoa(cols)
	pk := peakKey{id, cols}
	if s.stereoPeaks == nil {
		s.stereoPeaks = make(map[peakKey]*stereoPeakItem)
	}
	old := s.stereoPeaks[pk]
	if old != nil && old.key == key {
		return old.data, old.done
	}
	if old != nil && old.task != nil {
		old.task.Cancel()
	}
	it := &stereoPeakItem{key: key}
	s.stereoPeaks[pk] = it
	files := s.d.Pipeline.Artifacts()
	var out []float32
	spec := tasks.Spec{Title: i18n.Tr("editor.preview.waveform_name", "Waveform {name}", i18n.Str("name", filepath.Base(e.Name))), Kind: "preview", Priority: s.frame}
	it.task = s.d.Tasks.Work(spec, func(t *tasks.Task) error {
		f, err := files.Open(art.Path)
		if err != nil {
			return err
		}
		raw, err := readStereoArtifact(f, art)
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
		if info.Channels != 2 || info.Channels != a.Channels || info.SampleRate != a.Rate || info.Frames != a.Frames {
			return errors.New("stereo waveform does not match its imported artifact")
		}
		reader, err := codecsource.New(d)
		if err != nil {
			return err
		}
		out, err = buildStereoPeaks(taskPeakReader{reader: reader, task: t}, info.Frames, cols)
		return err
	}).Then(func(t *tasks.Task) {
		if s.stereoPeaks[pk] != it {
			return
		}
		it.task, it.done = nil, true
		current, ok := s.entry(id)
		if !ok {
			delete(s.stereoPeaks, pk)
			return
		}
		currentKey, ok := s.Key(current)
		if !ok || currentKey+"-stereo-"+strconv.Itoa(cols) != it.key {
			delete(s.stereoPeaks, pk)
			return
		}
		if t.State() == tasks.Done {
			it.data = out
		}
	})
	return nil, false
}

func readStereoArtifact(f interface {
	io.Reader
	io.Closer
	Size() int64
}, art asset.Artifact) ([]byte, error) {
	defer f.Close()
	if f.Size() < 0 || f.Size() > stereoArtifactLimit || art.Size > stereoArtifactLimit {
		return nil, errors.New("stereo waveform artifact exceeds its memory limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, stereoArtifactLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > stereoArtifactLimit || int64(len(raw)) != f.Size() || (art.Size > 0 && int64(len(raw)) != art.Size) {
		return nil, errors.New("stereo waveform artifact changed during read")
	}
	sum := sha256.Sum256(raw)
	if art.Hash == "" || hex.EncodeToString(sum[:8]) != art.Hash {
		return nil, errors.New("stereo waveform artifact hash changed")
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

func buildStereoPeaks(reader audioedit.Reader, frames int64, cols int) ([]float32, error) {
	if reader == nil || frames <= 0 || cols <= 0 || cols > 4096 {
		return nil, errors.New("invalid stereo waveform request")
	}
	base := 512
	for (frames-1)/int64(base)+1 > stereoPeakBuckets && base < 65536 {
		base *= 2
	}
	if (frames-1)/int64(base)+1 > stereoPeakBuckets {
		return nil, errors.New("stereo waveform is too long")
	}
	pyramid, err := audioedit.BuildPyramid(context.Background(), reader, frames, 2, base)
	if err != nil {
		return nil, err
	}
	level, first, last, err := pyramid.Visible(0, frames, cols)
	if err != nil {
		return nil, err
	}
	n := last - first
	out := make([]float32, cols*2)
	for col := range cols {
		lo := int(int64(col) * int64(n) / int64(cols))
		hi := max(lo+1, int(int64(col+1)*int64(n)/int64(cols)))
		for i := lo; i < hi && i < n; i++ {
			for ch := range 2 {
				p := level.Samples[(first+i)*2+ch]
				v := float32(math.Max(math.Abs(float64(p.Min)), math.Abs(float64(p.Max))))
				out[col*2+ch] = max(out[col*2+ch], v)
			}
		}
	}
	return out, nil
}
