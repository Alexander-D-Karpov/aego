package render

import (
	"aego/gpu"
	"aego/render/gpuprof"
	"aego/render/view"
)

// PassSample describes a submitted world/output pass. Its most recent available
// GPU sample can lag the inspected frame. Framebuffer sizes are physical pixels.
type PassSample struct {
	Output              view.ID
	Label               string
	InstanceID          string
	Width, Height       int
	Format              gpu.TextureFormat
	Window, KnownFormat bool
	GPUNS               uint64
	GPUValid            bool
}

type passIdentity struct {
	output          view.ID
	label, instance string
	target          gpu.TextureHandle
	subresource     gpu.TextureSubresource
	width, height   int
	format          gpu.TextureFormat
}

type passTimer struct {
	key  passIdentity
	prof *gpuprof.Profiler
}

// Pass inspection is opt-in. The normal renderer keeps its original gpu.Frame.
type passInspection struct {
	gpu.Frame
	renderer          *Renderer
	requested, active bool
	output            view.ID
	instance          string
	writing, previous []PassSample
	timers            []passTimer
}

func (r *Renderer) InspectOutputPasses() []PassSample {
	r.inspection.requested = true
	return r.inspection.previous
}

func (i *passInspection) closeTimers() {
	for _, t := range i.timers {
		t.prof.Close()
	}
	i.timers = nil
}

func (i *passInspection) begin(r *Renderer, f gpu.Frame) gpu.Frame {
	i.active = i.requested && f != nil
	i.requested = false
	if !i.active {
		if len(i.timers) != 0 {
			i.closeTimers()
		}
		i.previous = i.previous[:0]
		return f
	}
	i.Frame, i.renderer = f, r
	i.output = view.ID{}
	i.writing = i.writing[:0]
	for _, t := range i.timers {
		t.prof.Collect(f)
	}
	return i
}

// Query rings are owned by pass positions, with identity checked before reuse.
// Cap only the number of timers, not the list of physically submitted passes.
func (i *passInspection) BeginPass(desc gpu.PassDesc) gpu.Pass {
	p := i.Frame.BeginPass(desc)
	n := len(i.writing)
	s := PassSample{Output: i.output, Label: desc.Label, InstanceID: i.instance, Window: desc.Target.IsZero()}
	if s.Window {
		s.Width, s.Height = i.renderer.fbW, i.renderer.fbH
	} else if d, ok := i.renderer.dev.(gpu.TextureDescriber); ok {
		if t, found := d.TextureInfo(desc.Target); found {
			s.Format, s.KnownFormat = t.Format, true
			s.Width, s.Height, _ = t.SubresourceSize(desc.TargetSubresource)
		}
	}
	if n >= 128 || !i.renderer.caps.TimerQueries {
		i.writing = append(i.writing, s)
		return p
	}
	if n == len(i.timers) {
		i.timers = append(i.timers, passTimer{})
	}
	slot := &i.timers[n]
	key := passIdentity{output: s.Output, label: s.Label, instance: s.InstanceID, target: desc.Target,
		subresource: desc.TargetSubresource, width: s.Width, height: s.Height, format: s.Format}
	if slot.prof == nil || slot.key != key {
		if slot.prof != nil {
			slot.prof.Close()
		}
		slot.key = key
		slot.prof = gpuprof.New(i.renderer.dev, i.renderer.caps)
		slot.prof.Collect(i.Frame)
	}
	s.GPUNS, s.GPUValid = slot.prof.LastNS()
	i.writing = append(i.writing, s)
	slot.prof.Begin(p)
	return &inspectedPass{Pass: p, timer: slot.prof}
}

type inspectedPass struct {
	gpu.Pass
	timer *gpuprof.Profiler
}

func (p *inspectedPass) End() {
	p.timer.End(p.Pass)
	p.Pass.End()
}

func (i *passInspection) end() {
	if !i.active {
		return
	}
	i.previous, i.writing = i.writing, i.previous
	i.Frame = nil
	i.active = false
}
