package sprite

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/material"
	"aego/render/shader"
)

func TestVariantAndMaterialResourcesCreateOrderedBatchBoundaries(t *testing.T) {
	b, dev := newTestBatcher(t, 8)
	defer dev.Close()
	defer b.Close()
	sh := &material.Shader{Name: "variants", Keywords: []string{"ALT"}, Variants: []material.Variant{{Mask: 0}, {Mask: 1}}, Schema: material.NewSchema(nil, 0, 0)}
	a, _ := material.New("a", sh, 0, shader.BlendPremultiplied)
	variant, _ := material.New("variant", sh, 1, shader.BlendPremultiplied)
	resource, _ := material.New("distinct-resource", sh, 0, shader.BlendPremultiplied)
	region := fakeRegion(t, dev, "atlas")
	order := []*material.Material{a, variant, a, resource, a}
	for _, m := range order {
		d := Draw{Region: region, Size: mathx.Vec2{X: 1, Y: 1}, Tint: mathx.White, Material: m}
		b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	}
	b.Sort()
	batches := b.Build()
	if len(batches) != len(order) {
		t.Fatalf("variant/resource state was merged: %d batches", len(batches))
	}
	for i, batch := range batches {
		if batch.Material != order[i] || batch.First != i || batch.Count != 1 {
			t.Fatalf("batch %d changed painter order: %+v", i, batch)
		}
	}
	if got := b.Breaks(); got.Pipeline != 2 || got.Material != 2 || got.Clip != 0 || got.Texture != 0 {
		t.Fatalf("wrong break reasons: %+v", got)
	}
}
