package render2d

import (
	"aego/engine"
	"aego/render/lighting"
	"aego/scene/frame"
	"aego/script/bind"
)

func lightingRunner(c *bind.Ctx) *frame.Runner {
	r, ok := engine.ServiceOf[*frame.Runner](c.World, frame.ServiceID)
	if !ok || r == nil {
		c.Fail("lighting view is unavailable in this world")
		return nil
	}
	return r
}

func setLightingEnabled(c *bind.Ctx, enabled bool) {
	if r := lightingRunner(c); r != nil {
		_, debug := r.LightingView()
		r.SetLightingView(enabled, debug)
	}
}

func lightingEnabled(c *bind.Ctx) bool {
	if r := lightingRunner(c); r != nil {
		enabled, _ := r.LightingView()
		return enabled
	}
	return false
}

func setLightingDebug(c *bind.Ctx, name string) {
	var mode lighting.DebugMode
	switch name {
	case "lighting":
		mode = lighting.DebugLighting
	case "normals":
		mode = lighting.DebugNormals
	case "shadows":
		mode = lighting.DebugShadows
	default:
		c.Fail("unknown lighting debug mode; use lighting, normals or shadows")
		return
	}
	if r := lightingRunner(c); r != nil {
		enabled, _ := r.LightingView()
		r.SetLightingView(enabled, mode)
	}
}

func lightingDebug(c *bind.Ctx) string {
	if r := lightingRunner(c); r != nil {
		_, mode := r.LightingView()
		switch mode {
		case lighting.DebugNormals:
			return "normals"
		case lighting.DebugShadows:
			return "shadows"
		}
	}
	return "lighting"
}

func installLightingScript(m *bind.Module) {
	bind.Proc1(m, "SetEnabled", setLightingEnabled, bind.Params("enabled"), bind.Doc("Enables or bypasses lighting in this world's current view without changing scene materials or simulation."))
	bind.Func0(m, "Enabled", lightingEnabled, bind.Doc("Returns whether this world's current view applies lighting."))
	bind.Proc1(m, "SetDebug", setLightingDebug, bind.Params("mode"), bind.Doc("Selects the view debug display: lighting, normals or shadows. This does not mutate the scene."))
	bind.Func0(m, "Debug", lightingDebug, bind.Doc("Returns this world's view debug display: lighting, normals or shadows."))
}
