package settings

import (
	"strings"
	"testing"

	"aego/format/aetext"
)

func TestRoundTrip(t *testing.T) {
	src := "physics {\n  gravity=[0, 20]\n  substeps=8\n  layers=[Default, Player, Ground]\n  collide {\n    Player=[Ground]\n  }\n}\n"
	n, err := aetext.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}
	p := Default()
	if err := Decode(n, &p); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	out := &aetext.Node{Head: ID}
	Encode(&p, out)
	if got := string(aetext.Write(out)); got != src {
		t.Fatalf("got\n%s\nwant\n%s", got, src)
	}
}

func TestRenameKeepsIndex(t *testing.T) {
	p := Project{LayerNames: []string{"Default", "Player"}}
	i, _ := p.LayerIndex("Player")
	p.LayerNames[1] = "Hero"
	if j, ok := p.LayerIndex("Hero"); !ok || j != i {
		t.Fatalf("index moved from %d to %d", i, j)
	}
}

const layered = "physics {\n  layers=[Default, Ground, Player, Enemy]\n  collide {\n    Player=[Ground, Enemy]\n    Enemy=[Ground, Player]\n  }\n}\n"

func decode(t *testing.T, src string) (Project, error) {
	t.Helper()
	n, err := aetext.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}
	p := Default()
	if err := Decode(n, &p); err != nil {
		return p, err
	}
	return p, p.Validate()
}

func TestMatrixSymmetric(t *testing.T) {
	p, err := decode(t, layered)
	if err != nil {
		t.Fatal(err)
	}
	m := p.Matrix()
	for i := 0; i < MaxLayers; i++ {
		for j := 0; j < MaxLayers; j++ {
			if (m[i]>>j)&1 != (m[j]>>i)&1 {
				t.Fatalf("not symmetric at %d,%d", i, j)
			}
		}
	}
	idx := func(n string) int {
		i, _ := p.LayerIndex(n)
		return i
	}
	on := func(a, b string) bool { return (m[idx(a)]>>idx(b))&1 == 1 }
	switch {
	case !on("Player", "Ground"), !on("Player", "Enemy"), !on("Default", "Default"):
		t.Fatal("expected pairs are off")
	case on("Player", "Default"), on("Enemy", "Enemy"), on("Player", "Player"):
		t.Fatal("restricted pairs are on")
	}
	c := Project{LayerNames: p.LayerNames}
	c.SetMatrix(m)
	if c.Matrix() != m {
		t.Fatal("SetMatrix does not round trip")
	}
}

func TestUnknownLayer(t *testing.T) {
	src := strings.Replace(layered, "Player=[Ground, Enemy]", "Player=[Ground, Lava]", 1)
	if _, err := decode(t, src); err == nil || !strings.Contains(err.Error(), "Lava") {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaults(t *testing.T) {
	p := Default()
	if p.Layers()[0] != DefaultLayer || p.Matrix()[0] != ^uint32(0) {
		t.Fatal("default layer table")
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}
