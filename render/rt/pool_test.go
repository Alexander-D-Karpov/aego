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
	a, err := p.Acquire(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	tex := a.Texture
	p.Release(a)
	b, _ := p.Acquire(120, 110)
	if b.Texture != tex {
		t.Fatal("same bucket must reuse the texture")
	}
	if b.W != 120 || b.H != 110 || b.BucketW != 128 {
		t.Fatalf("target = %+v", b)
	}
	c, _ := p.Resize(b, 125, 70)
	if c != b {
		t.Fatal("resize within bucket must keep the target")
	}
	d, _ := p.Resize(c, 400, 400)
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
	if _, err := p.Acquire(0, 5); err == nil {
		t.Fatal("zero size accepted")
	}
}
