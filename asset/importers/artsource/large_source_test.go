package artsource

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/project"
)

// Build a valid, uncompressed 2048x2048 .aeart from an actual encoded
// document. Repeated tiles keep the decoded working set bounded while the ZIP
// itself crosses the pipeline's former 16 MiB limit.
func largeArtSource(t *testing.T) []byte {
	t.Helper()
	const size = 2048
	doc, err := art.New(size, size, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	session, err := art.NewSession(doc, art.NewJournal(art.DefaultLimits()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.Execute(context.Background(), art.Operation{
		Kind: "stroke", Points: []image.Point{{X: 0, Y: 0}},
		Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 127, G: 33, B: 12, A: 255}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := doc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(seed), int64(len(seed)))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte)
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(r)
		if err := r.Close(); readErr == nil {
			readErr = err
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[f.Name] = data
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	var images []map[string]json.RawMessage
	if err := json.Unmarshal(manifest["images"], &images); err != nil || len(images) != 1 {
		t.Fatalf("seed image records: %d, %v", len(images), err)
	}
	var imageFile string
	if err := json.Unmarshal(images[0]["file"], &imageFile); err != nil || imageFile == "" {
		t.Fatalf("seed image path: %q, %v", imageFile, err)
	}
	across := size / art.TileSize
	tileBytes := art.TileSize * art.TileSize * 4
	tileCount := across * across
	pixels := make([]byte, 4+tileCount*(8+tileBytes))
	binary.LittleEndian.PutUint32(pixels, uint32(tileCount))
	tile := make([]byte, tileBytes)
	for i := 0; i < len(tile); i += 4 {
		copy(tile[i:i+4], []byte{127, 33, 12, 255})
	}
	for i := range tileCount {
		offset := 4 + i*(8+tileBytes)
		binary.LittleEndian.PutUint32(pixels[offset:], uint32(i%across))
		binary.LittleEndian.PutUint32(pixels[offset+4:], uint32(i/across))
		copy(pixels[offset+8:offset+8+tileBytes], tile)
	}
	sum := sha256.Sum256(pixels)
	images[0]["size"], _ = json.Marshal(len(pixels))
	images[0]["sha256"], _ = json.Marshal(hex.EncodeToString(sum[:]))
	manifest["images"], err = json.Marshal(images)
	if err != nil {
		t.Fatal(err)
	}
	files["manifest.json"], err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files[imageFile] = pixels
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, name := range []string{"manifest.json", imageFile} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if out.Len() <= 16<<20 || int64(out.Len()) > art.DefaultLimits().MaxFileBytes {
		t.Fatalf("fixture is not in the intended size range: %d", out.Len())
	}
	return out.Bytes()
}

func TestLargeArtSourceTransactionImportsRealDocument(t *testing.T) {
	if got, want := New().SourceEditLimit(), art.DefaultLimits().MaxFileBytes; got != want {
		t.Fatalf("importer limit = %d, Art Core limit = %d", got, want)
	}
	data := largeArtSource(t)
	t.Logf("valid .aeart source: %d bytes; SHA-256 %x", len(data), sha256.Sum256(data))
	p, err := project.Create(t.TempDir(), "LargeArt", false)
	if err != nil {
		t.Fatal(err)
	}
	registry := pipeline.NewImporterRegistry()
	if err := registry.Register(New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, registry, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	const rel = "assets/large.aeart"
	meta, err := pl.NewSourceMeta(rel)
	if err != nil {
		t.Fatal(err)
	}
	edits := []pipeline.SourceEdit{
		{Path: rel, After: data},
		{Path: rel + pipeline.MetaSuffix, After: pipeline.WriteMeta(meta)},
	}
	batch, err := pl.ApplySources(edits, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("large art import: %+v, %v", batch, err)
	}
	if id, ok := pl.Lookup(rel); !ok || id != meta.ID {
		t.Fatalf("large art source identity = %s, exists=%v; want %s", id, ok, meta.ID)
	}
	got, err := pl.SourceBytes(rel)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("guarded source read: size=%d, %v", len(got), err)
	}
	if disk, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel))); err != nil || !bytes.Equal(disk, data) {
		t.Fatalf("source bytes on disk differ: size=%d, %v", len(disk), err)
	}
}
