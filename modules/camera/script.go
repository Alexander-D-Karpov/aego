package camera

import (
	"aego/core/mathx"
	"aego/scene"
	"aego/script/abi"
	"aego/script/bind"
)

type Blend uint64

func blendOf(k uint64) Blend {
	return Blend(k)
}

func (b Blend) key() uint64 {
	return uint64(b)
}

func live(c *bind.Ctx) *Service {
	if c.World == nil {
		return nil
	}
	return Of(c.World)
}

func need(c *bind.Ctx) *Service {
	s := live(c)
	if s == nil {
		c.Fail("cameras run only during Play")
	}
	return s
}

func installScript(m *bind.Module) {
	bind.Struct[Pose](m, "Pose", bind.Doc("Pose is a camera position in world units, a rotation in radians and a zoom in pixels per unit."))
	bind.Handle(m, "Blend", true, blendOf, Blend.key,
		bind.Doc("Blend is a running blend between virtual cameras. yield b waits until it ends; a nil blend is done."))

	bind.Func0(m, "Active", func(c *bind.Ctx) scene.NodeHandle {
		if s := live(c); s != nil {
			return s.Active("")
		}
		return scene.NodeHandle{}
	}, bind.Doc("Active returns the virtual camera that drives the main output, or nil."))
	bind.Func1(m, "ActiveOn", func(c *bind.Ctx, output string) scene.NodeHandle {
		if s := live(c); s != nil {
			return s.Active(output)
		}
		return scene.NodeHandle{}
	}, bind.Doc("ActiveOn returns the virtual camera that drives the named output."), bind.Params("output"))
	bind.Func2(m, "BlendTo", func(c *bind.Ctx, vc scene.NodeHandle, sec float32) Blend {
		s := need(c)
		if s == nil {
			return 0
		}
		k, err := s.BlendTo(vc, sec, false)
		if err != nil {
			c.Fail(err.Error())
		}
		return Blend(k)
	}, bind.Doc("BlendTo makes vc active over priorities and blends to it; seconds 0 uses its Blend Time. Release hands control back to priorities."),
		bind.Params("camera", "seconds"), bind.Example("yield camera.BlendTo(BossCam, 1.5)"))
	bind.Proc1(m, "Cut", func(c *bind.Ctx, vc scene.NodeHandle) {
		if s := need(c); s != nil {
			if _, err := s.BlendTo(vc, 0, true); err != nil {
				c.Fail(err.Error())
			}
		}
	}, bind.Doc("Cut makes vc active at once, without a blend."), bind.Params("camera"))
	bind.Func1(m, "Release", func(c *bind.Ctx, sec float32) Blend {
		if s := live(c); s != nil {
			return Blend(s.Release("", sec))
		}
		return 0
	}, bind.Doc("Release ends BlendTo or Cut on the main output; priorities decide again, blending over seconds or the camera's Blend Time."),
		bind.Params("seconds"))
	bind.Proc1(m, "Shake", func(c *bind.Ctx, t float32) {
		if s := live(c); s != nil {
			s.Shake("", t)
		}
	}, bind.Doc("Shake adds trauma to the main output; shake grows with trauma, which is capped at 1 and fades over time."),
		bind.Params("trauma"), bind.Example("camera.Shake(0.4)"))
	bind.Proc2(m, "ShakeOn", func(c *bind.Ctx, output string, t float32) {
		if s := live(c); s != nil {
			s.Shake(output, t)
		}
	}, bind.Doc("ShakeOn adds trauma to the named output."), bind.Params("output", "trauma"))
	bind.Func0(m, "Trauma", func(c *bind.Ctx) float32 {
		if s := live(c); s != nil {
			return s.Trauma("")
		}
		return 0
	}, bind.Doc("Trauma returns the current trauma of the main output, 0 to 1."))
	bind.Func0(m, "LogicalPose", func(c *bind.Ctx) Pose {
		if s := live(c); s != nil {
			return s.Logical("")
		}
		return Pose{}
	}, bind.Doc("LogicalPose returns the pose of the main output without shake and pixel snapping."))
	bind.Func0(m, "RenderPose", func(c *bind.Ctx) Pose {
		if s := live(c); s != nil {
			return s.Render("")
		}
		return Pose{}
	}, bind.Doc("RenderPose returns the pose the main output was drawn with in the last frame."))
	bind.Func1(m, "ScreenToWorld", func(c *bind.Ctx, p mathx.Vec2) mathx.Vec2 {
		if s := live(c); s != nil {
			if v, ok := s.View(""); ok {
				return v.ScreenToWorld(p)
			}
		}
		return p
	}, bind.Doc("ScreenToWorld converts output pixels to world units with the last drawn view of the main output."), bind.Params("point"))
	bind.Func1(m, "WorldToScreen", func(c *bind.Ctx, p mathx.Vec2) mathx.Vec2 {
		if s := live(c); s != nil {
			if v, ok := s.View(""); ok {
				return v.WorldToScreen(p)
			}
		}
		return p
	}, bind.Doc("WorldToScreen converts world units to output pixels with the last drawn view of the main output."), bind.Params("point"))

	hm := m.Registry().HandleMethods(KindBlend)
	bind.Func1(hm, "Done", func(c *bind.Ctx, b Blend) bool {
		s := live(c)
		return s == nil || s.BlendDone(uint64(b))
	}, bind.Doc("Done reports whether the blend ended, was replaced or is nil."))
	bind.Func1(hm, "Running", func(c *bind.Ctx, b Blend) bool {
		s := live(c)
		return s != nil && !s.BlendDone(uint64(b))
	}, bind.Doc("Running is the opposite of Done."))
	bind.Proc1(hm, "Stop", func(c *bind.Ctx, b Blend) {
		if s := live(c); s != nil {
			s.StopBlend(uint64(b))
		}
	}, bind.Doc("Stop ends the blend at the target camera."))
	bind.Signal(hm, "Completed", []abi.TypeRef{abi.T(abi.Bool)}, bind.Params("finished"),
		bind.Doc("Completed runs once when the blend ends: finished is false after Stop or when another camera became active first."))
}
