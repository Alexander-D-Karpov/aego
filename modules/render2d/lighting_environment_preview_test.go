package render2d

import (
	"bytes"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
)

func TestExplicitEnvironmentPreviewIsLiveAndRunnerLocal(t *testing.T) {
	f := lightingTestWorld(t)
	cam := f.camera("Game camera", ^uint64(0))
	h := f.s.Create(f.s.Root(), "Camera lighting")
	e := scene.Add[LightingEnvironment2D](f.s, h)
	*e = DefaultLightingEnvironment2D()
	e.Camera = scene.NodeRef{ID: f.s.ID(cam)}
	e.AmbientIntensity = .2
	f.s.Flush()
	f.s.UpdateTransforms()
	f.r.SetFixedView(view.View{Camera: view.Camera2D{Zoom: 32}, Viewport: mathx.Rect{W: 640, H: 360}})
	f.tick()
	if f.r.Bucket(0).Lighting.HasEnvironment {
		t.Fatal("a fixed Scene View pretended to be the game's camera")
	}
	before := sceneio.Save(f.s)
	play := frame.New()
	f.r.SetLightingEnvironmentPreview(f.s.ID(h))
	f.tick()
	if got := f.r.Bucket(0).Lighting; !got.HasEnvironment || got.Environment.AmbientIntensity != .2 {
		t.Fatal("explicit preview did not use the selected environment")
	}
	if !play.LightingEnvironmentPreview().IsZero() || !bytes.Equal(before, sceneio.Save(f.s)) {
		t.Fatal("preview selection changed Play or serialized scene data")
	}
	e, _ = scene.Get[LightingEnvironment2D](f.s, h)
	e.AmbientIntensity = .6
	f.tick()
	if f.r.Bucket(0).Lighting.Environment.AmbientIntensity != .6 {
		t.Fatal("Inspector changes were not reflected by the selected preview")
	}
	f.r.SetLightingEnvironmentPreview(ids.NodeID{})
	f.tick()
	if f.r.Bucket(0).Lighting.HasEnvironment {
		t.Fatal("reset did not restore fixed-view environment defaults")
	}
}

func TestMissingAndDisabledPreviewEnvironmentKeepsSelectionAndDiagnoses(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "deleted", true: "disabled"}[disabled], func(t *testing.T) {
			f := lightingTestWorld(t)
			f.camera("Camera", ^uint64(0))
			h := f.s.Create(f.s.Root(), "Preview lighting")
			*scene.Add[LightingEnvironment2D](f.s, h) = DefaultLightingEnvironment2D()
			f.s.Flush()
			id := f.s.ID(h)
			f.r.SetLightingEnvironmentPreview(id)
			if disabled {
				f.s.SetEnabled(h, false)
			} else {
				f.s.Destroy(h)
			}
			f.s.Flush()
			f.tick()
			if f.r.LightingEnvironmentPreview() != id || f.r.Bucket(0).Lighting.HasEnvironment {
				t.Fatal("invalid selection was discarded or used as a working environment")
			}
			diagnostics, _ := LightingDiagnostics(f.w)
			found := false
			for _, d := range diagnostics {
				found = found || d.Code == "LGT003" && d.Subject.Node == id
			}
			if !found {
				t.Fatal("missing preview environment had no diagnostic")
			}
		})
	}
}
