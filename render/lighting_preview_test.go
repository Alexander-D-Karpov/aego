package render

import (
	"testing"

	"aego/gpu"
	"aego/gpu/null"
	"aego/render/view"
)

func TestShadowPreviewRejectsInvalidResourcesAndReusesOwnedState(t *testing.T) {
	dev := null.New()
	t.Cleanup(dev.Close)
	id := view.ID{Scope: 23}
	array, err := dev.CreateTexture(gpu.TextureDesc{Width: 16, Height: 16, Format: gpu.TexR8, Dimension: gpu.Texture2DArray, Layers: 2})
	if err != nil {
		t.Fatal(err)
	}
	target, err := dev.CreateTexture(gpu.TextureDesc{Width: 32, Height: 32, Format: gpu.TexSRGBA8})
	if err != nil {
		t.Fatal(err)
	}
	r := &Renderer{dev: dev, caps: dev.Caps()}
	r.lightState.views = map[view.ID]*lightingView{id: {shadows: array, resolution: 16, shadowLayers: 2}}
	for i := 0; i < 2; i++ {
		r.lightState.views[id].shadow[i].valid = true
		r.lightState.views[id].shadow[i].active = true
	}
	if err = r.PreviewShadowLayer(id, 0, target, 32, 32); err == nil {
		t.Fatal("preview outside a frame was accepted")
	}
	r.frame = dev.BeginFrame(32, 32)
	for _, tc := range []struct {
		name   string
		id     view.ID
		layer  int
		target gpu.TextureHandle
		w, h   int
	}{
		{"unknown view", view.ID{Scope: 24}, 0, target, 32, 32},
		{"negative layer", id, -1, target, 32, 32},
		{"past last layer", id, 2, target, 32, 32},
		{"window target", id, 0, gpu.TextureHandle{}, 32, 32},
		{"array target", id, 0, array, 16, 16},
		{"wrong size", id, 0, target, 31, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := r.frame.Stats()
			if err := r.PreviewShadowLayer(tc.id, tc.layer, tc.target, tc.w, tc.h); err == nil {
				t.Fatal("invalid preview was accepted")
			}
			if r.frame.Stats() != before || r.shadowPreview != nil {
				t.Fatal("invalid preview submitted work or allocated resources")
			}
		})
	}
	r.lightState.views[id].shadow[1].active = false
	if err = r.PreviewShadowLayer(id, 1, target, 32, 32); err == nil {
		t.Fatal("cached but inactive layer was exposed as current")
	}
	r.lightState.views[id].shadow[1].active = true
	if err = r.PreviewShadowLayer(id, 1, target, 32, 32); err != nil {
		t.Fatal(err)
	}
	state, first := r.shadowPreview, dev.ResourceStats()
	for i := 0; i < 20; i++ {
		if err = r.PreviewShadowLayer(id, i%2, target, 32, 32); err != nil {
			t.Fatal(err)
		}
	}
	if state != r.shadowPreview || dev.ResourceStats() != first {
		t.Fatal("preview leaked or recreated warmed resources")
	}
	dev.EndFrame()
	r.frame = nil
	r.closeShadowPreview()
	after := dev.ResourceStats()
	if after.Shaders != 0 || after.Pipelines != 0 || after.Buffers != 0 || after.Samplers != 0 || after.Textures != 2 {
		t.Fatalf("preview cleanup changed borrowed resources: %+v", after)
	}
}

func TestReleaseLightingViewDropsOnlyItsOwnResourcesAndRejectsPendingComposite(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	makeTex := func(array bool) gpu.TextureHandle {
		d := gpu.TextureDesc{Width: 8, Height: 8, Format: gpu.TexRGBA8}
		if array {
			d.Dimension = gpu.Texture2DArray
			d.Layers = 2
		}
		h, err := dev.CreateTexture(d)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	id, other := view.ID{Scope: 44}, view.ID{Scope: 45}
	r := &Renderer{dev: dev}
	r.lightState.views = map[view.ID]*lightingView{id: {shadows: makeTex(true), cookies: makeTex(true)}, other: {shadows: makeTex(true)}}
	r.lightState.outputs = map[view.ID]*lightingOutput{id: {color: makeTex(false), pending: true}, other: {color: makeTex(false)}}
	before := dev.ResourceStats()
	if err := r.ReleaseLightingView(id, id); err == nil || dev.ResourceStats() != before {
		t.Fatal("pending output released before composition")
	}
	r.lightState.outputs[id].pending = false
	if err := r.ReleaseLightingView(id, id); err != nil {
		t.Fatal(err)
	}
	if dev.ResourceStats().Textures != 2 || r.lightState.views[id] != nil || r.lightState.outputs[id] != nil || r.lightState.views[other] == nil || r.lightState.outputs[other] == nil {
		t.Fatal("release leaked private resources or destroyed another view")
	}
	if err := r.ReleaseLightingView(id, id); err != nil || dev.ResourceStats().Textures != 2 {
		t.Fatal("release is not idempotent")
	}
}
