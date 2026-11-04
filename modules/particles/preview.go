package particles

import (
	"errors"

	"aego/asset"
	"aego/core/mathx"
	"aego/scene"
	"aego/scene/frame"
)

// Preview owns only a simulator and its borrowed asset resolver. No gameplay component or callbacks are created.
type Preview struct {
	e               emitter
	res             asset.Resolver
	seed            uint64
	reduced, closed bool
	pose            mathx.Mat3
}

func NewPreview(_ *scene.Types, res asset.Resolver) *Preview {
	return &Preview{res: res, pose: mathx.Mat3Identity()}
}
func (p *Preview) Configure(c ParticleSystem2D, seed uint64) error {
	if p.closed {
		return errors.New("particle preview is closed")
	}
	reset, err := p.e.configure(&c, p.res)
	if err != nil {
		return err
	}
	if seed == 0 {
		seed = uint64(c.Seed)
		if seed == 0 {
			seed = 1
		}
	}
	if p.seed != seed || reset {
		p.seed = seed
		p.e.start(seed, p.reduced, p.pose, c.PlayOnStart)
	}
	return nil
}
func (p *Preview) SetReduced(on bool) { p.reduced = on; p.e.pool.SetReduced(&p.e.cfg, on) }
func (p *Preview) Restart() {
	if p.e.valid {
		p.e.start(p.seed, p.reduced, p.pose, true)
	}
}
func (p *Preview) Play() {
	if p.e.valid {
		p.e.play(p.pose)
	}
}
func (p *Preview) Stop()  { p.e.pool.Stop(false) }
func (p *Preview) Clear() { p.e.pool.Clear() }
func (p *Preview) Burst(n int) {
	if p.e.valid && n > 0 {
		p.e.pool.Burst(&p.e.cfg, n, p.pose)
	}
}
func (p *Preview) Step(dt float64, pose mathx.Mat3) {
	p.pose = pose
	if p.e.valid {
		p.e.pool.Step(&p.e.cfg, dt, pose)
	}
}
func (p *Preview) Extract(r *frame.Runner, pose mathx.Mat3) {
	p.ExtractLayers(r, pose, ^uint64(0))
}
func (p *Preview) ExtractLayers(r *frame.Runner, pose mathx.Mat3, layers uint64) {
	if p.e.valid {
		var stats Stats
		p.e.extract(r, p.res, pose, layers, nil, &stats)
	}
}
func (p *Preview) Playing() bool { return p.e.pool.Playing() }
func (p *Preview) Count() int    { return len(p.e.pool.Particles()) }
func (p *Preview) Close()        { *p = Preview{closed: true} }

func (p *Preview) Bounds(pose mathx.Mat3) (mathx.AABB2, bool) {
	var bounds mathx.AABB2
	a := p.e.pool.Particles()
	for i, v := range a {
		pos, b, _ := v.Surface(p.e.cfg.Space, pose)
		half := mathx.V2(.5*(mathx.Abs(b[0])+mathx.Abs(b[2])), .5*(mathx.Abs(b[1])+mathx.Abs(b[3])))
		lo, hi := pos.Sub(half), pos.Add(half)
		if i == 0 {
			bounds = mathx.AABB2{Min: lo, Max: hi}
		} else {
			bounds.Min.X = min(bounds.Min.X, lo.X)
			bounds.Min.Y = min(bounds.Min.Y, lo.Y)
			bounds.Max.X = max(bounds.Max.X, hi.X)
			bounds.Max.Y = max(bounds.Max.Y, hi.Y)
		}
	}
	return bounds, len(a) > 0
}
