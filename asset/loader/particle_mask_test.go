package loader

import (
	"aego/asset"
	"testing"
)

func TestMaskCacheIsReleasedOnCloseAndRemount(t *testing.T) {
	l, _ := newLoader(t)
	check := func() {
		for _, slot := range l.masks.slots {
			if slot.points != nil {
				t.Fatal("retained the previous project's mask table")
			}
		}
	}
	for _, last := range []bool{false, true} {
		points, ok := l.ParticleMask(asset.BuiltinWhite, 0)
		if !ok || len(points) == 0 {
			t.Fatal("missing test mask")
		}
		if last {
			l.Close()
		} else if err := l.Mount(asset.NewIndex(), newFS(t, map[string][]byte{})); err != nil {
			t.Fatal(err)
		}
		check()
	}
}
