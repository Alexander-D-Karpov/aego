package frame

import (
	"encoding/binary"
	"math"
	"testing"

	"aego/core/clock"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/res"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/view"
	"aego/vfs"
)

type materialWorldDevice struct {
	*null.Device
	frame, lanes gpu.BufferHandle
	frameValue   [4]float32
	laneValue    float32
}

func (d *materialWorldDevice) CreateBuffer(desc gpu.BufferDesc) (gpu.BufferHandle, error) {
	h, err := d.Device.CreateBuffer(desc)
	switch desc.Label {
	case "render.frame":
		d.frame = h
	case "sprite.lanes":
		d.lanes = h
	}
	return h, err
}

func (d *materialWorldDevice) UpdateBuffer(h gpu.BufferHandle, offset int, data []byte) {
	if h == d.frame && offset == 0 && len(data) >= shader.FrameSize {
		for i := range d.frameValue {
			d.frameValue[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
		}
	}
	if h == d.lanes && offset == 0 && len(data) >= shader.LaneStride {
		d.laneValue = math.Float32frombits(binary.LittleEndian.Uint32(data))
	}
	d.Device.UpdateBuffer(h, offset, data)
}

func TestTwoWorldsKeepIndependentMaterialTimeAndInstanceState(t *testing.T) {
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	dev := &materialWorldDevice{Device: null.New()}
	defer dev.Close()
	renderer, err := render.New(dev, dev.Caps(), files, log.Nop(), render.Options{MaxSprites: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	sh := &material.Shader{Name: "shared", Variants: []material.Variant{{Sources: [4]string{"v", "f", "v", "f"}}},
		Schema: material.NewSchema([]shader.Param{{Name: "Amount", Kind: shader.KindFloat, Instance: true, Default: [4]float32{.5}}}, 0, 1)}
	shared, ok := material.New("shared", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("material")
	}
	a, b := engine.NewWorld("edit-preview"), engine.NewWorld("play")
	a.Time.Configure(clock.Config{MaxFrameDelta: 1})
	b.Time.Configure(clock.Config{MaxFrameDelta: 1})
	a.Time.SetScale(.5)
	a.Time.Begin(.5)
	b.Time.SetScale(2)
	b.Time.Begin(.2)
	makeRunner := func(w *engine.World, amount float32) {
		r := New()
		r.Bind(nil, renderer, nil)
		r.AddView(view.ScreenView(32, 32, 1), 0)
		lanes := *shared.DefaultLanes()
		lanes[0] = amount
		r.Bucket(0).Sprites = append(r.Bucket(0).Sprites, sprite.Draw{
			Region: renderer.White(), Size: mathx.Vec2{X: 16, Y: 16}, Tint: mathx.White, Material: shared, Lanes: &lanes,
		})
		w.SetService(ServiceID, r)
	}
	makeRunner(a, .2)
	makeRunner(b, .8)
	f := dev.BeginFrame(32, 32)
	if err = renderer.BeginFrame(f, 32, 32, 1); err != nil {
		t.Fatal(err)
	}
	check := func(w *engine.World, expected [4]float32, amount float32) {
		t.Helper()
		submit{}.Run(&engine.Context{World: w, Time: w.Time, Dt: w.Time.Span(clock.Game).Delta, Frame: w.Time.Frame()})
		for i, value := range dev.frameValue {
			if math.Abs(float64(value-expected[i])) > 1e-6 {
				t.Fatalf("%s frame block=%v, want %v", w.Name(), dev.frameValue, expected)
			}
		}
		if dev.laneValue != amount {
			t.Fatalf("%s received another world's instance value: %v, want %v", w.Name(), dev.laneValue, amount)
		}
	}
	check(a, [4]float32{.25, .25, .5, 1}, .2)
	check(b, [4]float32{.4, .4, .2, 1}, .8)
	b.Time.SetPaused(true)
	b.Time.Begin(.1)
	check(b, [4]float32{.4, 0, .3, 2}, .8)
	check(a, [4]float32{.25, .25, .5, 1}, .2)
	renderer.EndFrame()
	dev.EndFrame()
	if shared.Explicit("Amount") || shared.DefaultLanes()[0] != .5 {
		t.Fatal("world instance state changed the common material definition")
	}
}
