package material

import (
	"testing"

	"aego/gpu/null"
	"aego/render/shader"
)

func TestPendingTracksCandidateUntilCommitOrCancellation(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	c := NewCache(dev, false, spritePipe)
	defer c.Close()
	old := testShader("old", nil, []uint32{0}, pspec{"Value", shader.KindFloat, [4]float32{1}, false})
	next := testShader("next", nil, []uint32{0}, pspec{"Value", shader.KindFloat, [4]float32{2}, false})
	a, _ := New("a", old, 0, shader.BlendPremultiplied)
	b, _ := New("b", old, 0, shader.BlendPremultiplied)
	untracked, _ := New("untracked", old, 0, shader.BlendPremultiplied)
	c.Track(a)
	c.Track(b)
	if c.Pending(a) || c.Pending(nil) {
		t.Fatal("no candidate is pending")
	}
	c.QueueReload(old, next, []*Material{a}, nil)
	if !c.Pending(a) || !c.Pending(b) || c.Pending(untracked) || a.Shader() != old {
		t.Fatal("group candidate was not distinguished from committed definitions")
	}
	c.Begin()
	c.End()
	if c.Pending(a) || c.Pending(b) || a.Shader() != next || b.Shader() != next {
		t.Fatal("successful frame-boundary commit did not clear pending state")
	}
	candidate, _ := New("a draft", next, 0, shader.BlendPremultiplied)
	candidate.Set("Value", [4]float32{3})
	c.QueueMaterial(a, candidate, nil)
	if !c.Pending(a) || c.Pending(b) {
		t.Fatal("isolated material candidate affected its shared-shader neighbor")
	}
	c.CancelMaterial(a)
	if c.Pending(a) {
		t.Fatal("cancelled candidate is still reported pending")
	}
}
