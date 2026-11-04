package editor

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/preview"
	"aego/format/aetext"
	"aego/gpu"
	"aego/modules/particles"
	"aego/render/rt"
	"aego/render/view"
	"aego/scene/frame"
	"errors"
	"fmt"
	"math"
	"strings"
)

type surface struct {
	pool       *rt.Pool
	target     *rt.Target
	runner     *frame.Runner
	generation int
}

func (s *surface) close() {
	if s.pool != nil {
		if s.target != nil {
			s.pool.Release(s.target)
		}
		s.pool.Close()
	}
	if s.runner != nil {
		s.runner.Close()
	}
	*s = surface{}
}
func (s *surface) prepare(ed *context.Editor, size int) error {
	if ed.Renderer == nil || strings.Contains(strings.ToLower(ed.Renderer.Device().Caps().Renderer), "null") {
		return errors.New("GPU preview unavailable")
	}
	dev := ed.Renderer.Device()
	g := 0
	if d, ok := dev.(interface {
		Lost() bool
		Restores() int
	}); ok {
		if d.Lost() {
			return errors.New("graphics context lost")
		}
		g = d.Restores()
	}
	if s.generation != g {
		s.close()
	}
	if s.target != nil {
		return nil
	}
	s.generation = g
	s.pool = rt.New(dev, gpu.TexSRGBA8, "editor.particles")
	var err error
	s.target, err = s.pool.Acquire(size, size, false)
	if err != nil {
		s.close()
		return err
	}
	s.runner = frame.New()
	s.runner.Bind(nil, ed.Renderer, resolver(ed))
	return nil
}
func (s *surface) render(v *transport, fixed bool) {
	t := s.target
	pose := mathx.Mat3Identity()
	extent := float32(2)
	center := mathx.V2(0, 0)
	if b, ok := v.sim.Bounds(pose); ok {
		sz := b.Size()
		if fixed {
			extent = max(.25, sz.X, sz.Y) * 1.22
			center = b.Center()
		} else {
			extent = max(2, sz.X, sz.Y) * 1.3
			center = b.Center().Scale(.7)
		}
	}
	size := float32(t.W)
	out := view.View{Target: t.Texture, TargetW: t.BucketW, TargetH: t.BucketH, Viewport: mathx.Rect{W: size, H: size}, PixelsPerUnit: 1, PixelScale: 1, Layers: view.AllLayers, Camera: view.Camera2D{Zoom: size / extent, Position: center}, ZMax: 1, Clear: mathx.Color{R: .075, G: .085, B: .10, A: 1}, DoClear: true, Label: "particles.preview"}
	s.runner.Prepare(t.W, t.H, 1)
	s.runner.AddView(out, 0)
	v.sim.Extract(s.runner, pose)
	s.runner.Submit(s.runner.Renderer())
}
func (p *adapter) presetSource(e asset.Entry) ([]byte, error) {
	if d := p.ed.Docs.Script(e.Source); d != nil {
		return d.Code.Bytes(), nil
	}
	a, ok := e.Artifact(asset.ArtPreset)
	if !ok {
		return nil, errors.New("preset artifact missing")
	}
	return p.ed.Pipeline.Artifacts().ReadFile(a.Path)
}
func particleNode(data []byte, name string) (*aetext.Node, *aetext.Node, error) {
	n, err := aetext.Parse(data, name)
	if err != nil {
		return nil, nil, err
	}
	for _, node := range n.Blocks {
		if node.Head != "node" {
			continue
		}
		for _, c := range node.Blocks {
			if c.Head == "component" && c.Label == particles.NameSystem {
				return n, c, nil
			}
		}
	}
	return nil, nil, preview.ErrUnsupportedContent
}
func (p *adapter) thumbnail(e asset.Entry, size int) (preview.Surface, error) {
	data, err := p.presetSource(e)
	if err != nil {
		return preview.Surface{}, err
	}
	if _, _, err = particleNode(data, e.Source); err != nil {
		return preview.Surface{}, err
	}
	c, err := particles.ReadPreset(p.ed.Types, resolver(p.ed), data)
	if err != nil {
		return preview.Surface{}, err
	}
	var v transport
	defer v.close()
	v.configure(p.ed, c, true)
	if v.bad != "" {
		return preview.Surface{}, errors.New(v.bad)
	}
	v.sampleThumbnail()
	var s surface
	if err = s.prepare(p.ed, size); err != nil {
		return preview.Surface{}, err
	}
	s.render(&v, true)
	return preview.Surface{Target: s.target.Texture, Close: s.close}, nil
}
func (p *adapter) card(card *preview.Card) (bool, error) {
	_, node, err := particleNode(card.Source, card.Entry.Source)
	if errors.Is(err, preview.ErrUnsupportedContent) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	card.Label(4, 3, "PARTICLES", true)
	d := particles.DefaultParticleSystem2D()
	shape, rate, capacity := "point", d.Rate, d.MaxParticles
	if a, ok := node.Field("shape"); ok && a.Ident != "" {
		shape = a.Ident
	}
	if a, ok := node.Field("rate"); ok {
		rate = float32(a.AsFloat())
	}
	if a, ok := node.Field("maxParticles"); ok {
		capacity = int32(a.AsInt())
	}
	card.Label(4, 19, fmt.Sprintf("%s  %.1f/s", shape, rate), false)
	card.Label(4, 33, fmt.Sprintf("Pool: %d", capacity), false)
	if value, ok := node.Field("colorOverLife"); ok {
		card.Label(4, 47, fmt.Sprintf("Gradient: %d keys", len(value.List)), false)
	}
	card.Label(4, 63, "GPU unavailable", false)
	return true, nil
}

func (v *transport) sampleThumbnail() {
	c := &v.cfg
	pose := mathx.Mat3Identity()
	steps := min(24, max(1, int(c.Lifetime.X*18)))
	if c.Loop && c.Rate > 0 {
		steps = min(150, max(60, int(c.Lifetime.Y*75)))
	}
	work := 0
	for range steps {
		v.sim.Step(1./60, pose)
		work += max(1, v.sim.Count())
		if work >= 131072 {
			break
		}
	}

	if v.sim.Count() != 0 {
		return
	}

	// A short burst may be gone by the standard sample, or may be scheduled
	// much later. Fast-forward the same schedule to its next birth.
	v.restart()
	if v.sim.Count() != 0 {
		return
	}
	warm := float64(0)
	if c.Loop {
		warm = float64(c.Prewarm)
	}
	next := math.Inf(1)
	duration := float64(c.Duration)
	if c.Rate > 0 {
		rate := float64(c.Rate)
		next = (1 - math.Mod(warm*rate, 1)) / rate
		if !c.Loop && next > duration {
			next = math.Inf(1)
		}
	}
	local := math.Mod(warm, duration)
	for _, key := range c.Bursts {
		if key.Y <= 0 {
			continue
		}
		delay := float64(key.X) - local
		if delay < 0 {
			if !c.Loop {
				continue
			}
			delay += duration
		}
		next = min(next, delay)
	}
	if math.IsInf(next, 1) {
		return
	}
	// Splitting just before the birth keeps the bounded catch-up policy from
	// dropping a very sparse event along with whole empty cycles.
	before := math.Nextafter(next, 0)
	if before > 0 {
		v.sim.Step(before, pose)
	}
	v.sim.Step(next-before+min(1./60, float64(c.Lifetime.X)*.25), pose)
}
