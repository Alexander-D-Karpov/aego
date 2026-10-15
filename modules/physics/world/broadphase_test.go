package world

import (
	"math/rand"
	"testing"
)

func randBox(rng *rand.Rand, extent float32) AABB {
	c := Vec2{X: (rng.Float32()*2 - 1) * extent, Y: (rng.Float32()*2 - 1) * extent}
	h := Vec2{X: rng.Float32()*2 + 0.05, Y: rng.Float32()*2 + 0.05}
	return AABB{Min: c.Sub(h), Max: c.Add(h)}
}

func TestTreeMatchesBruteForce(t *testing.T) {
	scenes := 50
	if testing.Short() {
		scenes = 5
	}
	rng := rand.New(rand.NewSource(1))
	for s := 0; s < scenes; s++ {
		tr := newTree(1024)
		boxes := make([]AABB, 11000)
		ids := make([]int32, 11000)
		for i := range boxes {
			boxes[i] = randBox(rng, 200)
			ids[i] = tr.create(boxes[i], int32(i))
		}
		for i := 10000; i < 11000; i++ {
			d := Vec2{X: (rng.Float32()*2 - 1) * 5, Y: (rng.Float32()*2 - 1) * 5}
			boxes[i] = AABB{Min: boxes[i].Min.Add(d), Max: boxes[i].Max.Add(d)}
			tr.move(ids[i], boxes[i])
		}
		removed := make([]bool, len(boxes))
		for k := 0; k < 200; k++ {
			if i := rng.Intn(10000); !removed[i] {
				tr.destroy(ids[i])
				removed[i] = true
			}
		}
		if h := tr.height(); h > 40 {
			t.Fatalf("scene %d: tree height %d", s, h)
		}
		got := make([]bool, len(boxes))
		for i := 10000; i < 11000; i++ {
			clear(got)
			tr.query(boxes[i], func(id int32) bool {
				got[tr.data(id)] = true
				return true
			})
			for j := range boxes {
				if !removed[j] && boxOverlap(boxes[i], boxes[j]) && !got[j] {
					t.Fatalf("scene %d: query of %d missed %d", s, i, j)
				}
			}
		}
	}
}

func TestWorldPairsCoverOverlaps(t *testing.T) {
	w := newTestWorld(t, func(c *Config) { c.Gravity = Vec2{} })
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 300; i++ {
		b := randBox(rng, 30)
		addStatic(t, w, b.Center(), 0, Box{Half: b.Size().Scale(0.5)}, nil)
	}
	var dyn []ShapeHandle
	for i := 0; i < 200; i++ {
		b := randBox(rng, 30)
		_, s := addDynamic(t, w, b.Center(), rng.Float32(), Box{Half: b.Size().Scale(0.5)}, nil)
		dyn = append(dyn, s)
	}
	w.updatePairs()
	if w.stats.BroadphaseCandidates == 0 {
		t.Fatal("no broadphase candidates")
	}
	for i := range w.shapes.items {
		if !w.shapes.alive[i] {
			continue
		}
		other := &w.shapes.items[i]
		for _, sh := range dyn {
			if uint32(i) == sh.Index {
				continue
			}
			s := &w.shapes.items[sh.Index]
			pa, pb := s.proxies[0], other.proxies[0]
			if !boxOverlap(w.prox[pa].tight, w.prox[pb].tight) {
				continue
			}
			if _, ok := w.pairs.get(pairKey(pa, pb)); !ok {
				t.Fatalf("overlapping shapes %d and %d have no pair", sh.Index, i)
			}
		}
	}
}
