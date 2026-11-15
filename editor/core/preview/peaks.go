package preview

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"aego/asset"
	"aego/audio/codec"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/tasks"
	"aego/engine/i18n"
)

const peaksExt = ".peaks"

type peakKey struct {
	id   ids.AssetID
	cols int
}

type peakItem struct {
	key  string
	data []float32
	done bool
	task *tasks.Task
}

// CachedPeaks returns a current in-memory waveform without scheduling work.
// Small folder previews can use this without decoding an unopened audio file.
func (s *Service) CachedPeaks(id ids.AssetID, cols int) []float32 {
	if cols <= 0 {
		return nil
	}
	e, ok := s.entry(id)
	if !ok || e.Kind != asset.ArtAudio {
		return nil
	}
	key, ok := s.Key(e)
	if !ok {
		return nil
	}
	it := s.peaks[peakKey{id, cols}]
	if it == nil || !it.done || it.key != key+"-"+strconv.Itoa(cols) {
		return nil
	}
	return it.data
}

func (s *Service) Peaks(id ids.AssetID, cols int) ([]float32, bool) {
	e, ok := s.entry(id)
	if !ok || e.Kind != asset.ArtAudio || s.d.Tasks == nil || cols <= 0 {
		return nil, true
	}
	key, ok := s.Key(e)
	if !ok {
		return nil, true
	}
	key += "-" + strconv.Itoa(cols)
	if s.peaks == nil {
		s.peaks = map[peakKey]*peakItem{}
	}
	pk := peakKey{id, cols}
	it := s.peaks[pk]
	if it != nil && it.key == key {
		return it.data, it.done
	}
	if it != nil && it.task != nil {
		it.task.Cancel()
	}
	it = &peakItem{key: key}
	s.peaks[pk] = it
	art, _ := e.Artifact(e.Kind)
	path := filepath.Join(s.dir, key+peaksExt)
	var out []float32
	spec := tasks.Spec{Title: i18n.Tr("editor.preview.waveform_name", "Waveform {name}", i18n.Str("name", filepath.Base(e.Name))), Kind: "preview", Priority: s.frame}
	it.task = s.d.Tasks.Work(spec, func(*tasks.Task) error {
		if v, err := readPeaks(path, cols); err == nil {
			out = v
			return nil
		}
		raw, err := s.d.Pipeline.Artifacts().ReadFile(art.Path)
		if err != nil {
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
		if out, err = codec.Peaks(d, cols); err != nil {
			return err
		}
		return writePeaks(path, out)
	}).Then(func(t *tasks.Task) {
		if s.peaks[pk] != it {
			return
		}
		it.task, it.done = nil, true
		if t.State() == tasks.Done {
			it.data = out
		}
	})
	return nil, false
}

func readPeaks(path string, cols int) ([]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) != 4*cols {
		return nil, errs.New(errs.AssetBadArtifact, "the cached waveform has a wrong size")
	}
	out := make([]float32, cols)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[4*i:]))
	}
	return out, nil
}

func writePeaks(path string, v []float32) error {
	data := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(data[4*i:], math.Float32bits(x))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
