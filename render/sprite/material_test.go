package sprite

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/material"
	"aego/render/shader"
)

func testMaterial(t *testing.T, name string, lanes bool) *material.Material {
	t.Helper()
	var params []shader.Param
	n := 0
	if lanes {
		params = []shader.Param{{Name: "Flash", Kind: shader.KindFloat, Instance: true, Default: [4]float32{0.25}}}
		n = 1
	}
	sh := &material.Shader{Name: name, Variants: []material.Variant{{}}, Schema: material.NewSchema(params, 0, n)}
	m, ok := material.New(name, sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("no variant")
	}
	return m
}

func TestMaterialsOnLayersMakeOneBatchEach(t *testing.T) {
	b, dev := newTestBatcher(t, 512)
	defer b.Close()
	reg := fakeRegion(t, dev, "a")
	mats := []*material.Material{testMaterial(t, "a", false), testMaterial(t, "b", true), testMaterial(t, "c", false)}
	for i := 0; i < 300; i++ {
		d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}, Layer: int16(i / 100), Material: mats[i/100]}
		b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	}
	b.Sort()
	batches := b.Build()
	if len(batches) != 3 {
		t.Fatalf("%d batches, want 3", len(batches))
	}
	for i, bt := range batches {
		if bt.Material != mats[i] || bt.Count != 100 {
			t.Errorf("batch %d: %s × %d", i, bt.Material.Name, bt.Count)
		}
	}
	if batches[0].LaneFirst != -1 || batches[1].LaneFirst != 0 || batches[2].LaneFirst != -1 {
		t.Errorf("lane starts %d %d %d", batches[0].LaneFirst, batches[1].LaneFirst, batches[2].LaneFirst)
	}
}

// Alternating materials in one layer and order break batches in submission order (D19): the key never reorders them.
func TestAlternatingMaterialsKeepSubmissionOrder(t *testing.T) {
	b, dev := newTestBatcher(t, 64)
	defer b.Close()
	reg := fakeRegion(t, dev, "a")
	x, y := testMaterial(t, "x", false), testMaterial(t, "y", false)
	order := []*material.Material{x, y, x, y, y, x}
	for _, m := range order {
		d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}, Material: m}
		b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	}
	b.Sort()
	got := b.Build()
	want := []*material.Material{x, y, x, y, x}
	if len(got) != len(want) {
		t.Fatalf("%d batches, want %d", len(got), len(want))
	}
	if b.Breaks().Pipeline != 4 {
		t.Fatalf("wrong shader batch break count: %+v", b.Breaks())
	}
	for i := range want {
		if got[i].Material != want[i] {
			t.Errorf("batch %d is %s, want %s", i, got[i].Material.Name, want[i].Name)
		}
	}
}

func TestLaneValuesDoNotBreakBatches(t *testing.T) {
	b, dev := newTestBatcher(t, 64)
	defer b.Close()
	reg := fakeRegion(t, dev, "a")
	m := testMaterial(t, "flash", true)
	for i := 0; i < 8; i++ {
		lanes := [16]float32{float32(i)}
		d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}, Material: m, Lanes: &lanes}
		b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	}
	d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}, Material: m}
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	b.Sort()
	batches := b.Build()
	if len(batches) != 1 || batches[0].Count != 9 {
		t.Fatalf("%d batches", len(batches))
	}
	if b.Breaks() != (BatchBreaks{}) || b.LaneBytes() != 9*shader.LaneStride {
		t.Fatalf("override batch accounting %+v, %d lane bytes", b.Breaks(), b.LaneBytes())
	}
	for i := 0; i < 8; i++ {
		if b.laneOut[i][0] != float32(i) {
			t.Fatalf("lane %d = %v", i, b.laneOut[i][0])
		}
	}
	if b.laneOut[8][0] != 0.25 {
		t.Fatalf("a sprite without lanes got %v, not the material default", b.laneOut[8][0])
	}
}

type countingDevice struct {
	*null.Device
	creates, updates int
}

func (d *countingDevice) CreateBuffer(desc gpu.BufferDesc) (gpu.BufferHandle, error) {
	d.creates++
	return d.Device.CreateBuffer(desc)
}

func (d *countingDevice) UpdateBuffer(h gpu.BufferHandle, off int, data []byte) {
	d.updates++
	d.Device.UpdateBuffer(h, off, data)
}

func TestNoLaneMaterialsLeaveTheDeviceAlone(t *testing.T) {
	dev := &countingDevice{Device: null.New()}
	b, err := NewBatcher(dev, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	b.SetView(0, 1, 1)
	reg := fakeRegion(t, dev, "a")
	m := testMaterial(t, "plain", false)
	for i := 0; i < 10; i++ {
		d := Draw{Region: reg, Size: mathx.Vec2{X: 1, Y: 1}, Material: m}
		if i%2 == 0 {
			d.Material = nil
		}
		b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	}
	b.Sort()
	b.Build()
	creates, updates := dev.creates, dev.updates
	if !b.UploadLanes() || dev.creates != creates || dev.updates != updates || !b.LaneBuffer().IsZero() {
		t.Fatalf("UploadLanes touched the device: %d creates, %d updates", dev.creates-creates, dev.updates-updates)
	}
}

func TestTrimAndUntrimShareMaterialBatchAndKeepFastPath(t *testing.T) {
	b, dev := newTestBatcher(t, 16)
	defer b.Close()
	reg := fakeRegion(t, dev, "trim")
	reg.W, reg.H = 3, 2
	reg.SourceW, reg.SourceH, reg.TrimX, reg.TrimY = 8, 6, 2, 1
	m := testMaterial(t, "trim-material", true)
	d := Draw{Region: reg, Size: mathx.Vec2{X: 16, Y: 12}, Origin: mathx.Vec2{X: .5, Y: .5}, Material: m}
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	d.Region.SourceW, d.Region.SourceH, d.Region.TrimX, d.Region.TrimY = 0, 0, 0, 0
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	b.Sort()
	bs := b.Build()
	if len(bs) != 1 || bs[0].TrimFirst != 0 || b.TrimBytes() != 2*shader.TrimStride {
		t.Fatalf("trim split material or lost stream %+v", bs)
	}
	if b.sorted[0].Size != (mathx.Vec2{X: 6, Y: 4}) || b.sorted[0].Origin != (mathx.Vec2{X: 2.0 / 3, Y: 1}) {
		t.Fatalf("trim geometry %+v", b.sorted[0])
	}
	if b.trimOut[1] != ([4]float32{}) {
		t.Fatal("untrimmed sprite lacks default ABI sentinel")
	}
	b.Reset()
	b.Push(&d, gpu.SamplerHandle{}, mathx.Rect{}, false)
	b.Sort()
	b.Build()
	if b.TrimBytes() != 0 || b.batches[0].TrimFirst != -1 {
		t.Fatal("untrimmed path uploads trim data")
	}
}
