package audio

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aego/asset"
	"aego/asset/audiosidecar"
	audioimport "aego/asset/importers/audio"
	"aego/asset/pipeline"
	"aego/audio/codec"
	_ "aego/audio/codec/vorbis"
	"aego/audio/codec/wav"
	"aego/audio/edit"
	"aego/audio/mix"
	"aego/core/log"
	"aego/format/aetext"
	"aego/project"
)

type editedPreviewFixture struct {
	pipe                 *pipeline.Pipeline
	project              *project.Project
	path                 string
	source, meta, edited []byte
	doc                  audioedit.Document
	originalPCM          []float32
}

func newEditedPreviewFixture(t *testing.T, ext string, streamed bool) editedPreviewFixture {
	t.Helper()
	var source []byte
	if ext == ".wav" {
		pcm := make([]float32, 4096*2)
		for i := range pcm {
			pcm[i] = float32(i%61-30) / 64
		}
		source = wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})
	} else {
		var err error
		source, err = os.ReadFile(filepath.Join("..", "..", "audio", "codec", "testdata", "tone.ogg"))
		if err != nil {
			t.Fatal(err)
		}
	}
	d, err := codec.Open(bytes.NewReader(source), ext)
	if err != nil {
		t.Fatal(err)
	}
	info := d.Info()
	if info.Frames < 1024 {
		t.Fatalf("fixture is too short: %+v", info)
	}
	first := make([]float32, 1024*info.Channels)
	for n := 0; n < 1024; {
		got, err := d.Read(first[n*info.Channels:])
		if err != nil || got <= 0 {
			t.Fatalf("could not decode independent reference: %d %v", got, err)
		}
		n += got
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	pj, err := project.Create(t.TempDir(), "EditedPreview", false)
	if err != nil {
		t.Fatal(err)
	}
	path := "assets/tone" + ext
	file := filepath.Join(pj.Root, filepath.FromSlash(path))
	if err := os.WriteFile(file, source, 0o644); err != nil {
		t.Fatal(err)
	}
	imp := audioimport.New()
	meta := asset.NewSourceMeta(imp.ID(), imp.Version(), imp.Defaults())
	meta.Settings["stream"] = aetext.Bool(streamed)
	meta.Settings["gain"] = aetext.Float64(.75)
	meta.Settings["bus"] = aetext.Ident("ui")
	before := asset.MarshalSourceMeta(meta)
	if err := os.WriteFile(file+asset.MetaSuffix, before, 0o644); err != nil {
		t.Fatal(err)
	}
	reg := pipeline.NewImporterRegistry()
	if err := reg.Register(imp); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(pj, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Imported) != 1 || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v %v", batch, err)
	}
	doc, err := audioedit.New(audioedit.Source{AssetID: meta.ID.String(), Path: path, SHA256: fmt.Sprintf("%x", sha256.Sum256(source)),
		Codec: ext[1:], SampleRate: info.SampleRate, Channels: info.Channels, Frames: info.Frames})
	if err != nil {
		t.Fatal(err)
	}
	doc.Trim = audioedit.Range{Start: 16, End: 1000}
	doc.Loop = &audioedit.Range{Start: 40, End: 900}
	doc.Operations = []audioedit.Operation{
		{Kind: audioedit.Gain, Region: doc.Trim, Value: .5},
		{Kind: audioedit.Reverse, Region: audioedit.Range{Start: 200, End: 600}},
	}
	edited, err := audiosidecar.Encode(before, path, doc)
	if err != nil {
		t.Fatal(err)
	}
	return editedPreviewFixture{pipe: pl, project: pj, path: path, source: source, meta: before, edited: edited, doc: doc, originalPCM: first}
}

func TestEditedMetadataPreviewProducesRealPlaybackWithoutPublishing(t *testing.T) {
	for _, ext := range []string{".wav", ".ogg"} {
		for _, streamed := range []bool{false, true} {
			name := ext + "/memory"
			if streamed {
				name = ext + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				f := newEditedPreviewFixture(t, ext, streamed)
				id, ok := f.pipe.Lookup(f.path)
				if !ok {
					t.Fatal("source identity is missing")
				}
				prior, ok := f.pipe.PrimaryArtifact(id)
				if !ok {
					t.Fatal("saved artifact is missing")
				}
				priorBytes, err := f.pipe.Artifacts().ReadFile(prior.Path)
				if err != nil {
					t.Fatal(err)
				}
				plain, err := f.pipe.PreviewWithMetadata(f.path, f.source, f.meta, nil)
				if err != nil || plain == nil || len(plain.Artifacts) != 1 {
					t.Fatalf("ordinary metadata preview regressed: %+v %v", plain, err)
				}
				plainArt, err := asset.DecodeAudio(plain.Artifacts[0].Data)
				if err != nil || !bytes.Equal(plainArt.Data, f.source) {
					t.Fatal("ordinary metadata preview did not use the original import path", err)
				}
				result, err := f.pipe.PreviewWithMetadata(f.path, f.source, f.edited, nil)
				if err != nil || result == nil || result.Kind != asset.ArtAudio || len(result.Artifacts) != 1 {
					t.Fatalf("edited preview failed: %+v %v", result, err)
				}
				art, err := asset.DecodeAudio(result.Artifacts[0].Data)
				if err != nil {
					t.Fatal(err)
				}
				if art.Format != asset.AudioWAV || art.Frames != f.doc.Trim.Length() || art.Stream != streamed ||
					art.LoopStart != 24 || art.LoopEnd != 884 || art.Gain != .75 || art.Bus != "ui" {
					t.Fatalf("candidate lost edited layout/settings: %+v", art)
				}
				decoder, err := codec.Open(bytes.NewReader(art.Data), "preview.wav")
				if err != nil {
					t.Fatal(err)
				}
				mem, err := codec.DecodeAll(decoder, 0)
				if err != nil {
					decoder.Close()
					t.Fatal(err)
				}
				if err := decoder.Close(); err != nil {
					t.Fatal(err)
				}
				ch := f.doc.Source.Channels
				for i := int64(0); i < f.doc.Trim.Length(); i++ {
					src := f.doc.Trim.Start + i
					if src >= 200 && src < 600 {
						src = 799 - src
					}
					for c := range ch {
						want := f.originalPCM[int(src)*ch+c] * .5
						if math.Abs(float64(mem.PCM[int(i)*ch+c]-want)) > 1e-6 {
							t.Fatalf("frame %d channel %d: got %v, want %v", i, c, mem.PCM[int(i)*ch+c], want)
						}
					}
				}
				// Exercise the same streaming decoder and ring as runtime playback.
				decoder, err = codec.Open(bytes.NewReader(art.Data), "preview.wav")
				if err != nil {
					t.Fatal(err)
				}
				defer decoder.Close()
				st := mix.NewStream(art.Rate, art.Channels, int(art.Frames)+1)
				cancel := make(chan struct{})
				done := codec.Stream(decoder, st, codec.StreamOptions{Chunk: 127}, cancel)
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(10 * time.Second):
					close(cancel)
					<-done
					t.Fatal("edited stream did not complete")
				}
				got := make([]float32, len(mem.PCM))
				if n := st.Ring.Read(got); n != int(art.Frames) || !st.EOF() || !equalAudioSamples(got, mem.PCM) {
					t.Fatalf("edited stream differs from memory: frames=%d EOF=%v", n, st.EOF())
				}
				// Inject the private artifact into the actual audio module. No saved
				// asset or physical audio device is involved in this headless rig.
				r := newRig(t)
				resolver := &previewPauseResolver{id: id, raw: priorBytes}
				clip, err := r.mod.Clip(resolver, id)
				if err != nil || clip == nil || !bytes.Equal(clip.Art.Data, f.source) {
					t.Fatalf("runtime did not cache the original artifact: %+v %v", clip, err)
				}
				if err := r.mod.PreviewArtifact(id, result.Artifacts[0].Data, true); err != nil {
					t.Fatal("runtime rejected edited preview", err)
				}
				if cached, err := r.mod.Clip(resolver, id); err != nil || cached != clip || !bytes.Equal(cached.Art.Data, f.source) {
					t.Fatal("private preview changed the runtime asset cache", err)
				}
				started := false
				for range 100 {
					r.frames(1)
					if position, live := r.mod.PreviewPosition(); live && position > 0 {
						started = true
						break
					}
					time.Sleep(time.Millisecond)
				}
				if !started || r.mod.PreviewClip() != id {
					t.Fatal("runtime did not advance the edited preview voice")
				}
				if err := r.mod.StopPreview(); err != nil {
					t.Fatal("runtime could not stop the edited preview", err)
				}
				if got, err := f.pipe.Artifacts().ReadFile(prior.Path); err != nil || !bytes.Equal(got, priorBytes) {
					t.Fatal("private preview changed the saved artifact", err)
				}
				file := filepath.Join(f.project.Root, filepath.FromSlash(f.path))
				if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, f.source) {
					t.Fatal("private preview changed source bytes", err)
				}
				if got, err := os.ReadFile(file + asset.MetaSuffix); err != nil || !bytes.Equal(got, f.meta) {
					t.Fatal("private preview persisted the candidate metadata", err)
				}
				if err := f.pipe.VerifyCurrent(id); err != nil {
					t.Fatal("private preview invalidated the saved artifact", err)
				}
			})
		}
	}
}

func TestEditedMetadataPreviewRejectsConflictsWithoutWrites(t *testing.T) {
	for _, reason := range []string{"changed-source-argument", "changed-source-on-disk", "wrong-digest", "other-id", "layout-mismatch", "format-spoof", "loop-outside-trim", "preview-budget", "operation-budget", "bus-budget", "gain-overflow", "unsupported-version", "legacy-edit", "changed-disk-metadata", "pending-recovery"} {
		t.Run(reason, func(t *testing.T) {
			f := newEditedPreviewFixture(t, ".wav", false)
			file := filepath.Join(f.project.Root, filepath.FromSlash(f.path))
			source, meta := bytes.Clone(f.source), bytes.Clone(f.edited)
			wantSource, wantMeta := bytes.Clone(f.source), bytes.Clone(f.meta)
			switch reason {
			case "changed-source-argument":
				source[len(source)-1] ^= 1
			case "changed-source-on-disk":
				wantSource[len(wantSource)-1] ^= 1
				if err := os.WriteFile(file, wantSource, 0o644); err != nil {
					t.Fatal(err)
				}
			case "wrong-digest":
				candidate := f.doc.Clone()
				candidate.Source.SHA256 = fmt.Sprintf("%064x", 1)
				var err error
				meta, err = audiosidecar.Encode(f.meta, f.path, candidate)
				if err != nil {
					t.Fatal(err)
				}
			case "other-id":
				other := asset.NewSourceMeta("audio", 1, audioimport.New().Defaults())
				candidate := f.doc.Clone()
				candidate.Source.AssetID = other.ID.String()
				var err error
				meta, err = audiosidecar.Encode(asset.MarshalSourceMeta(other), f.path, candidate)
				if err != nil {
					t.Fatal(err)
				}
			case "layout-mismatch", "preview-budget", "operation-budget":
				candidate := f.doc.Clone()
				candidate.Source.Frames++
				if reason == "preview-budget" {
					candidate.Source.Frames = (64<<20)/8 + 1
					candidate.Trim = audioedit.Range{Start: 0, End: candidate.Source.Frames}
				}
				if reason == "operation-budget" {
					candidate.Source.Frames = 20000
					candidate.Trim = audioedit.Range{Start: 0, End: 20000}
					candidate.Operations = make([]audioedit.Operation, 4096)
					for i := range candidate.Operations {
						candidate.Operations[i] = audioedit.Operation{Kind: audioedit.Gain, Region: audioedit.Range{Start: 0, End: 1}, Value: .5}
					}
				}
				var err error
				meta, err = audiosidecar.Encode(f.meta, f.path, candidate)
				if err != nil {
					t.Fatal(err)
				}
			case "format-spoof":
				var err error
				source, err = os.ReadFile(filepath.Join("..", "..", "audio", "codec", "testdata", "tone.ogg"))
				if err != nil {
					t.Fatal(err)
				}
				wantSource = bytes.Clone(source)
				if err := os.WriteFile(file, source, 0o644); err != nil {
					t.Fatal(err)
				}
				candidate := f.doc.Clone()
				candidate.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(source))
				meta, err = audiosidecar.Encode(f.meta, f.path, candidate)
				if err != nil {
					t.Fatal(err)
				}
			case "loop-outside-trim", "bus-budget", "gain-overflow":
				candidate := f.doc.Clone()
				original, err := asset.ParseSourceMeta(f.meta, f.path+asset.MetaSuffix)
				if err != nil {
					t.Fatal(err)
				}
				switch reason {
				case "loop-outside-trim":
					candidate.Loop = nil
					original.Settings["loopStart"] = aetext.Int(0)
					original.Settings["loopEnd"] = aetext.Int(900)
				case "bus-budget":
					original.Settings["bus"] = aetext.String(strings.Repeat("b", 256))
				case "gain-overflow":
					original.Settings["gain"] = aetext.Float64(math.MaxFloat64)
				}
				meta, err = audiosidecar.Encode(asset.MarshalSourceMeta(original), f.path, candidate)
				if err != nil {
					t.Fatal(err)
				}
			case "unsupported-version":
				meta = bytes.Replace(meta, []byte("version=2"), []byte("version=3"), 1)
			case "legacy-edit":
				meta = bytes.Replace(meta, []byte("version=2"), []byte("version=1"), 1)
			case "changed-disk-metadata":
				wantMeta = append(wantMeta, '\n')
				if err := os.WriteFile(file+asset.MetaSuffix, wantMeta, 0o644); err != nil {
					t.Fatal(err)
				}
			case "pending-recovery":
				journal := filepath.Join(f.project.Root, ".aego", "source-transactions", "active")
				if err := os.MkdirAll(journal, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if result, err := f.pipe.PreviewWithMetadata(f.path, source, meta, nil); err == nil || result != nil {
				t.Fatalf("accepted %s: %+v %v", reason, result, err)
			}
			if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, wantSource) {
				t.Fatal("failed preview changed source bytes", err)
			}
			if got, err := os.ReadFile(file + asset.MetaSuffix); err != nil || !bytes.Equal(got, wantMeta) {
				t.Fatal("failed preview changed metadata", err)
			}
		})
	}
}

func TestLegacyMetadataCannotSilentlyImportAudioEdit(t *testing.T) {
	f := newEditedPreviewFixture(t, ".wav", false)
	id, ok := f.pipe.Lookup(f.path)
	if !ok {
		t.Fatal("source identity is missing")
	}
	prior, ok := f.pipe.PrimaryArtifact(id)
	if !ok {
		t.Fatal("saved artifact is missing")
	}
	before, err := f.pipe.Artifacts().ReadFile(prior.Path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := bytes.Replace(f.edited, []byte("version=2"), []byte("version=1"), 1)
	if bytes.Equal(legacy, f.edited) {
		t.Fatal("candidate has no version marker")
	}
	if _, err := asset.ParseSourceMeta(legacy, f.path+asset.MetaSuffix); err != nil {
		t.Fatal("fixture must be accepted by a legacy metadata parser", err)
	}
	file := filepath.Join(f.project.Root, filepath.FromSlash(f.path))
	if err := os.WriteFile(file+asset.MetaSuffix, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	batch, err := f.pipe.ReloadMetadata(id)
	if err != nil || batch == nil || batch.Failed[id] == nil || len(batch.Imported) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("legacy metadata silently imported unedited audio: %+v %v", batch, err)
	}
	if got, err := f.pipe.Artifacts().ReadFile(prior.Path); err != nil || !bytes.Equal(got, before) {
		t.Fatal("failed import changed the last artifact", err)
	}
	if err := f.pipe.VerifyCurrent(id); err == nil {
		t.Fatal("legacy edit was reported as a current artifact")
	}
	if got, err := os.ReadFile(file + asset.MetaSuffix); err != nil || !bytes.Equal(got, legacy) {
		t.Fatal("failed import changed external metadata", err)
	}
}

func equalAudioSamples(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > 1e-6 {
			return false
		}
	}
	return true
}
