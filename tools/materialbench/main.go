// Command materialbench measures the same prepared render stream on any 12a
// checkpoint. gpu/null records CPU-side draw/transfer work, not driver timing.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"aego/core/log"
	"aego/core/mathx"
	"aego/engine/res"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
	"aego/vfs"
)

type counted struct {
	*null.Device
	bytes, uploads uint64
}

func (d *counted) UpdateBuffer(h gpu.BufferHandle, offset int, data []byte) {
	d.bytes += uint64(len(data))
	d.uploads++
	d.Device.UpdateBuffer(h, offset, data)
}

type result struct {
	Scenario      string  `json:"scenario"`
	Sprites       int     `json:"sprites"`
	Views         uint32  `json:"views"`
	DrawCalls     uint32  `json:"drawCalls"`
	Batches       uint32  `json:"batches"`
	InstanceBytes uint32  `json:"instanceBytes"`
	BufferBytes   uint64  `json:"bufferBytes"`
	BufferUploads uint64  `json:"bufferUploads"`
	GoAllocs      float64 `json:"goAllocsPerFrame"`
	CPUFrameNS    int64   `json:"cpuFrameNS"`
}

func main() {
	count := flag.Int("sprites", 3000, "fixed adjacent sprites")
	frames := flag.Int("frames", 1000, "measured warmed frames")
	runs := flag.Int("runs", 5, "timing samples per scenario")
	out := flag.String("out", "", "optional JSON output path")
	flag.Parse()
	if *count < 1 || *frames < 1 || *runs < 1 {
		panic("positive sprites, frames and runs required")
	}
	var results []result
	for _, name := range []string{"default", "one-material", "instance-overrides", "A-B-A", "two-views"} {
		for range *runs {
			results = append(results, measure(name, *count, *frames))
		}
	}
	data, err := json.MarshalIndent(struct {
		Backend, Go, Platform string
		Viewport              [2]int
		DPR                   int
		Results               []result
	}{"gpu/null CPU preparation; no driver timing", runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH, [2]int{1280, 720}, 1, results}, "", "  ")
	if err != nil {
		panic(err)
	}
	if *out != "" {
		if err = os.WriteFile(*out, append(data, '\n'), 0644); err != nil {
			panic(err)
		}
	} else {
		fmt.Println(string(data))
	}
}

func measure(name string, n, frames int) result {
	files, err := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
	if err != nil {
		panic(err)
	}
	dev := &counted{Device: null.New()}
	r, err := render.New(dev, dev.Caps(), files, log.Nop(), render.Options{MaxSprites: n + 16})
	if err != nil {
		panic(err)
	}
	defer dev.Close()
	defer r.Close()
	tex, err := r.Textures().Upload(texture.Solid(4, 4, mathx.Color{R: 1, G: 1, B: 1, A: 1}), texture.UploadOptions{})
	if err != nil {
		panic(err)
	}
	reg := texture.Whole(tex, 4, 4)
	plain := &material.Shader{Name: "shared", Variants: []material.Variant{{Sources: [4]string{"v", "f", "v", "f"}}}, Schema: material.NewSchema([]shader.Param{{Name: "Value", Kind: shader.KindFloat, Default: [4]float32{1}}}, 16, 0)}
	inst := &material.Shader{Name: "instance", Variants: []material.Variant{{Sources: [4]string{"v", "f", "v", "f"}}}, Schema: material.NewSchema([]shader.Param{{Name: "Value", Kind: shader.KindFloat, Instance: true}}, 0, 1)}
	a, _ := material.New("A", plain, 0, shader.BlendPremultiplied)
	b, _ := material.New("B", plain, 0, shader.BlendPremultiplied)
	o, _ := material.New("Overrides", inst, 0, shader.BlendPremultiplied)
	draws := make([]sprite.Draw, n)
	lanes := make([][16]float32, n)
	for i := range draws {
		draws[i] = sprite.Draw{Region: reg, Position: mathx.Vec2{X: float32(i%100)/10 - 5, Y: float32(i/100%30)/10 - 1.5}, Size: mathx.Vec2{X: 1, Y: 1}, Tint: mathx.Color{R: .5, G: .5, B: .5, A: .5}}
		switch name {
		case "one-material", "two-views":
			draws[i].Material = a
		case "instance-overrides":
			draws[i].Material = o
			lanes[i][0] = float32(i%7) / 7
			draws[i].Lanes = &lanes[i]
		case "A-B-A":
			draws[i].Material = a
			if i%3 == 1 {
				draws[i].Material = b
			}
		}
	}
	v := view.View{Viewport: mathx.Rect{W: 1280, H: 720}, TargetW: 1280, TargetH: 720, PixelScale: 1, Camera: view.Camera2D{Zoom: 16}, Label: "material-control"}
	var stat render.Stats
	frame := func() {
		dev.bytes, dev.uploads = 0, 0
		r.SetClock(render.Clock{Time: 1, Delta: 1.0 / 60, Unscaled: 1, Frame: 60})
		f := dev.BeginFrame(1280, 720)
		r.BeginFrame(f, 1280, 720, 1)
		vf := r.View(v)
		vf.Sprites(draws)
		vf.End()
		if name == "two-views" {
			second := v
			second.Viewport.W = 640
			second.Viewport.X = 640
			vf = r.View(second)
			vf.Sprites(draws)
			vf.End()
		}
		stat = r.EndFrame()
		dev.EndFrame()
	}
	for range 120 {
		frame()
	}
	allocs := testing.AllocsPerRun(200, frame)
	start := time.Now()
	for range frames {
		frame()
	}
	elapsed := time.Since(start)
	return result{name, n, stat.Views, stat.DrawCalls, stat.Batches, stat.InstanceBytes, dev.bytes, dev.uploads, allocs, elapsed.Nanoseconds() / int64(frames)}
}
