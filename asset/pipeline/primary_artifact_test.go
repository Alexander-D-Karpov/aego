package pipeline

import (
	"aego/asset"
	"aego/core/ids"
	"sync"
	"testing"
)

func TestPreviewAtlasArtifactDuringIndexRefresh(t *testing.T) {
	p := &Pipeline{index: asset.NewIndex()}
	id := ids.NewAssetID()
	set := func(path string) {
		p.index.Put(asset.Entry{ID: id, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: path, Hash: path}}})
	}
	set("A")
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 3000; i++ {
			p.mu.Lock()
			if i%2 == 0 {
				set("A")
			} else {
				set("B")
			}
			p.mu.Unlock()
		}
	}()
	close(start)
	for i := 0; i < 3000; i++ {
		a, ok := p.PrimaryArtifact(id)
		if !ok || (a.Path != "A" && a.Path != "B") || a.Hash != a.Path {
			t.Errorf("inconsistent atlas artifact: %+v", a)
			break
		}
	}
	workers.Wait()
}
