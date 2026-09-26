package stats

import "runtime/metrics"

type Frame struct {
	FrameTime   float64
	UpdateTime  float64
	RenderTime  float64
	FPS         float64
	DrawCalls   uint32
	Vertices    uint32
	Triangles   uint32
	TexSwitches uint32
	Allocs      uint64
	AllocBytes  uint64
	GCPauses    uint32
}

func (f *Frame) ResetGPU() {
	f.DrawCalls = 0
	f.Vertices = 0
	f.Triangles = 0
	f.TexSwitches = 0
}

const (
	metricAllocObjects = "/gc/heap/allocs:objects"
	metricAllocBytes   = "/gc/heap/allocs:bytes"
	metricGCCycles     = "/gc/cycles/total:gc-cycles"
	defaultFPSWindow   = 0.5
)

type Counter struct {
	samples  [3]metrics.Sample
	prev     [3]uint64
	primed   bool
	fpsTime  float64
	fpsCount int
	fps      float64
	Window   float64
}

func NewCounter() *Counter {
	c := &Counter{Window: defaultFPSWindow}
	c.samples[0].Name = metricAllocObjects
	c.samples[1].Name = metricAllocBytes
	c.samples[2].Name = metricGCCycles
	return c
}

func (c *Counter) Sample(f *Frame) {
	metrics.Read(c.samples[:])
	var cur [3]uint64
	for i := range c.samples {
		if c.samples[i].Value.Kind() == metrics.KindUint64 {
			cur[i] = c.samples[i].Value.Uint64()
		}
	}
	if c.primed {
		f.Allocs = cur[0] - c.prev[0]
		f.AllocBytes = cur[1] - c.prev[1]
		f.GCPauses = uint32(cur[2] - c.prev[2])
	}
	c.prev = cur
	c.primed = true

	c.fpsTime += f.FrameTime
	c.fpsCount++
	if c.fpsTime >= c.Window && c.fpsTime > 0 {
		c.fps = float64(c.fpsCount) / c.fpsTime
		c.fpsTime = 0
		c.fpsCount = 0
	}
	f.FPS = c.fps
}
