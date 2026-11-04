package preview

import (
	"image"
	"sync"
	"testing"

	"aego/asset"
	"aego/core/ids"
)

// Card handlers run on workers while the editor can replace its source list.
func TestCardBlitDoesNotReadMutableSources(t *testing.T) {
	s := New(Deps{})
	c := s.input(asset.Entry{}, nil)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 1000; i++ {
			s.sources = map[ids.AssetID]sourceFile{{Lo: uint64(i + 1)}: {}}
		}
	}()
	close(start)
	for i := 0; i < 1000; i++ {
		if _, err := c.Blit(asset.BuiltinWhite, image.Rect(0, 0, 16, 16)); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}
