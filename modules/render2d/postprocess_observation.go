package render2d

import (
	"aego/core/ids"
	"aego/engine"
	"aego/render/postfx"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
)

// ObservedPostEffect reads the plan retained by this presentation runner.
// A camera in another document, minimap or preview has no matching observation.
func ObservedPostEffect(w *engine.World, output view.ID, node ids.NodeID, instance string, preview frame.PostPreview) (postfx.Status, bool) {
	r, ok := engine.ServiceOf[*frame.Runner](w, frame.ServiceID)
	if !ok || r.Scene() == nil {
		return postfx.Status{}, false
	}
	camera, owners := -1, 0
	for i, v := range r.Views() {
		if v.OutputID != output {
			continue
		}
		if v.ID.Node == node {
			camera = i
		}
		if r.Bucket(i).PostProcessOwner {
			owners++
		}
	}
	if camera < 0 {
		return postfx.Status{}, false
	}
	c, ok := scene.Get[PostProcess](r.Scene(), r.Scene().Handle(node))
	if !ok {
		return postfx.Status{}, false
	}
	e, ok := c.Effect(instance)
	if !ok {
		return postfx.Status{}, false
	}
	st := postfx.Status{InstanceID: instance, TypeID: e.TypeID}
	result := func(state postfx.State, reason string) (postfx.Status, bool) {
		st.State, st.Reason = state, reason
		return st, true
	}
	if !c.Enabled {
		return result(postfx.Disabled, "component disabled")
	}
	if !r.Bucket(camera).PostProcessOwner {
		return result(postfx.Disabled, "post-processing setting disabled")
	}
	if owners > 1 {
		return result(postfx.Invalid, "conflicting output owners")
	}
	if plan := r.Bucket(camera).PostProcess; plan != nil {
		for _, state := range plan.States {
			if state.InstanceID != instance {
				continue
			}
			if state.State == postfx.Active && (preview.Bypass || preview.Camera == node && preview.Instance == instance) {
				return result(postfx.Disabled, "preview bypass")
			}
			return state, true
		}
	}
	if ds, _ := LightingDiagnostics(w); len(ds) > 0 {
		for _, d := range ds {
			if d.Subject.Node == node && d.Code == "PFX001" {
				return result(postfx.Invalid, d.Message)
			}
		}
	}
	return postfx.Status{}, false
}
