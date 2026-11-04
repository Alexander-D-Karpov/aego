package loader

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/vfs"
	"errors"
	"testing"
)

type countedTextureFiles struct {
	vfs.FS
	reads    int
	failNext bool
}

func (f *countedTextureFiles) ReadFile(path string) ([]byte, error) {
	f.reads++
	if f.failNext {
		f.failNext = false
		return nil, errors.New("transient read failure")
	}
	return f.FS.ReadFile(path)
}

type changingContext struct {
	gpu.Device
	generation uint64
}

func (d *changingContext) ContextGeneration() uint64 { return d.generation }
func (*changingContext) ContextLost() bool           { return false }

func TestTextureMetadataReusesSuccessfulPreparedRead(t *testing.T) {
	l, dev := newLoader(t)
	c := &changingContext{dev, 1}
	l.dev = c
	id := ids.NewAssetID()
	f := &countedTextureFiles{FS: newFS(t, map[string][]byte{"one": textureBytes(3, 5, mathx.White), "two": textureBytes(9, 7, mathx.White)})}
	idx := asset.NewIndex()
	entry := asset.Entry{ID: id, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://one"}}}
	idx.Put(entry)
	if err := l.Mount(idx, f); err != nil {
		t.Fatal(err)
	}
	check := func(width int, reads int) {
		t.Helper()
		start := f.reads
		for range 8 {
			info, ok := l.TextureMetadata(id)
			if !ok || info.Width != width {
				t.Fatal(info, ok)
			}
		}
		if f.reads-start != reads {
			t.Fatalf("read %d times, expected %d", f.reads-start, reads)
		}
	}
	check(3, 1)
	entry.Artifacts = []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://two"}}
	if err := l.Swap([]asset.Entry{entry}, f); err != nil {
		t.Fatal(err)
	}
	check(9, 1)
	c.generation++
	check(9, 1)
	c.generation++
	f.failNext = true
	if _, ok := l.TextureMetadata(id); ok {
		t.Fatal("resource read failure was lost")
	}
	check(9, 1)
	l.Drop(id)
	if _, ok := l.TextureMetadata(id); ok {
		t.Fatal("dropped texture remained cached")
	}
}
