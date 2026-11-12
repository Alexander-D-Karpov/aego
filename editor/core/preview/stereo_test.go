package preview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	audioimport "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/audio/codec/wav"
	"aego/editor/core/tasks"
	"aego/project"
)

func TestStereoPeaksFollowImportedGenerationAndPreserveSource(t *testing.T) {
	pj, err := project.Create(t.TempDir(), "Stereo", false)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(pj.AssetsDir(), "tone.wav")
	write := func(left, right float32) []byte {
		t.Helper()
		pcm := make([]float32, 4096*2)
		for i := 0; i < len(pcm); i += 2 {
			pcm[i], pcm[i+1] = left, right
		}
		raw := wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})
		if err := os.WriteFile(source, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	write(.75, -.25)
	reg := pipeline.NewImporterRegistry()
	if err := reg.Register(audioimport.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(pj, reg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	importAll := func() {
		t.Helper()
		batch, err := pl.ImportAll()
		if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("import: batch=%+v error=%v", batch, err)
		}
	}
	importAll()
	id, ok := pl.Lookup("assets/tone.wav")
	if !ok {
		t.Fatal("fixture did not import")
	}
	var workers []func()
	q := tasks.New(tasks.Deps{Submit: func(fn func()) { workers = append(workers, fn) }, OnMain: func(fn func()) { fn() }})
	s := New(Deps{Pipeline: pl, Tasks: q})
	t.Cleanup(s.Close)
	if data, done := s.StereoPeaks(id, 32); done || data != nil {
		t.Fatal("waveform did not schedule asynchronously")
	}
	q.Tick()
	if len(workers) != 1 {
		t.Fatal("waveform job was not submitted")
	}
	// Replace the imported generation while its old job is still pending.
	latest := write(.125, -.5)
	importAll()
	workers[0]()
	if data, done := s.StereoPeaks(id, 32); done || data != nil {
		t.Fatalf("stale waveform escaped after reimport: done=%v data=%v", done, data)
	}
	q.Tick()
	if len(workers) != 2 {
		t.Fatal("new artifact did not get a fresh waveform job")
	}
	workers[1]()
	data, done := s.StereoPeaks(id, 32)
	if !done || len(data) != 64 {
		t.Fatalf("stereo waveform = %v, done %v", data, done)
	}
	for col := 0; col < 32; col++ {
		if data[col*2] != .125 || data[col*2+1] != .5 {
			t.Fatalf("channels mixed at column %d: %v", col, data[col*2:col*2+2])
		}
	}
	actual, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(actual, latest) {
		t.Fatal("waveform analysis changed its source", err)
	}
	// A corrupt replacement at the same artifact path must not be published
	// under the previous artifact hash.
	e, ok := pl.Entry(id)
	if !ok {
		t.Fatal("entry disappeared")
	}
	art, ok := e.Artifact(asset.ArtAudio)
	if !ok {
		t.Fatal("audio artifact disappeared")
	}
	raw, err := pl.Artifacts().ReadFile(art.Path)
	if err != nil || len(raw) < 8 {
		t.Fatal("cannot read test artifact", err)
	}
	raw[len(raw)-1] ^= 1
	if err := pl.Artifacts().WriteFile(art.Path, raw); err != nil {
		t.Fatal(err)
	}
	s.Forget(id)
	if data, done := s.StereoPeaks(id, 32); done || data != nil {
		t.Fatal("corrupted artifact was not checked asynchronously")
	}
	q.Tick()
	workers[2]()
	if data, done := s.StereoPeaks(id, 32); !done || data != nil {
		t.Fatal("corrupt artifact produced a trusted waveform")
	}
	var pending []func()
	q2 := tasks.New(tasks.Deps{Submit: func(fn func()) { pending = append(pending, fn) }, OnMain: func(fn func()) { fn() }})
	closing := New(Deps{Pipeline: pl, Tasks: q2})
	closing.StereoPeaks(id, 32)
	q2.Tick()
	if len(pending) != 1 {
		t.Fatal("closing fixture did not schedule a worker")
	}
	closing.Close()
	pending[0]()
	if len(closing.stereoPeaks) != 0 {
		t.Fatal("closed preview service published a pending waveform")
	}
	if current, err := os.ReadFile(source); err != nil || sha256.Sum256(current) != sha256.Sum256(latest) {
		t.Fatal("artifact failure changed the source", err)
	}
}

type unexpectedPeakReader struct{}

func (unexpectedPeakReader) ReadFrames(context.Context, int64, int) ([]float32, error) {
	panic("invalid request called the decoder")
}

func TestStereoPeaksRejectInvalidOrExcessiveRequests(t *testing.T) {
	for _, tc := range []struct {
		frames int64
		cols   int
	}{
		{0, 32}, {100, 0}, {100, 4097}, {int64(stereoPeakBuckets)*65536 + 1, 32},
	} {
		if _, err := buildStereoPeaks(unexpectedPeakReader{}, tc.frames, tc.cols); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
}
