package rt

import (
	"testing"

	"aego/gpu"
	"aego/gpu/null"
)

func TestBucket(t *testing.T) {
	cases := map[int]int{0: 64, 1: 64, 64: 64, 65: 128, 300: 320}
	for in, want := range cases {
		if got := Bucket(in); got != want {
			t.Fatalf("Bucket(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestReuseWithinBucket(t *testing.T) {
	p := New(null.New(), gpu.TexSRGBA8, "test")
	defer p.Close()
	a, err := p.Acquire(100, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	tex := a.Texture
	p.Release(a)
	b, _ := p.Acquire(120, 110, false)
	if b.Texture != tex {
		t.Fatal("same bucket must reuse the texture")
	}
	if b.W != 120 || b.H != 110 || b.BucketW != 128 {
		t.Fatalf("target = %+v", b)
	}
	c, _ := p.Resize(b, 125, 70, false)
	if c != b {
		t.Fatal("resize within bucket must keep the target")
	}
	d, _ := p.Resize(c, 400, 400, false)
	if d == c || p.Free() != 1 {
		t.Fatal("resize across buckets must release the old target")
	}
	if p.Live() != 2 {
		t.Fatalf("live = %d", p.Live())
	}
	p.Release(d)
	p.Trim(0)
	if p.Live() != 0 {
		t.Fatal("trim did not destroy textures")
	}
	if _, err := p.Acquire(0, 5, false); err == nil {
		t.Fatal("zero size accepted")
	}
}

func TestDepthTargetsDoNotMix(t *testing.T) {
	p := New(null.New(), gpu.TexSRGBA8, "test")
	defer p.Close()
	a, err := p.Acquire(100, 100, true)
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasDepth() || a.Depth == a.Texture {
		t.Fatal("depth target has no separate depth texture")
	}
	p.Release(a)
	b, _ := p.Acquire(100, 100, false)
	if b == a || b.HasDepth() {
		t.Fatal("a color-only request reused a depth target")
	}
	c, _ := p.Acquire(90, 90, true)
	if c != a {
		t.Fatal("a depth request in the same bucket must reuse the depth target")
	}
	d, _ := p.Resize(c, 100, 100, false)
	if d == c || d.HasDepth() {
		t.Fatal("resize that drops depth must swap the target")
	}
	if p.Live() != 3 {
		t.Fatalf("live = %d", p.Live())
	}
	p.Release(b)
	p.Release(d)
	p.Trim(0)
	if p.Live() != 0 || p.Free() != 0 {
		t.Fatalf("trim left live=%d free=%d", p.Live(), p.Free())
	}
}
