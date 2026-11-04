package render2d

import (
	"testing"

	"aego/engine"
	"aego/render/postfx"
	"aego/scene"
)

// Exercise the cached BuildWithMaterials call counter for the exact
// Bloom lane animated by the Inspector, animation and tween paths.
func TestBloomIntensityUpdatesDoNotRebuildPlan(t *testing.T) {
	f := lightingTestWorld(t)
	h := f.camera("camera", ^uint64(0))
	reg := postfx.NewRegistry()
	e, err := reg.NewInstance(postfx.BloomID, "lamp")
	if err != nil {
		t.Fatal(err)
	}
	*scene.Add[PostProcess](f.s, h) = PostProcess{Enabled: true, Effects: []postfx.Instance{e}}
	f.tick()
	st, _ := engine.ServiceOf[*lightingState](f.w, lightingServiceID)
	c, _ := scene.Get[PostProcess](f.s, h)
	if len(c.Effects[0].Parameters) < 2 || c.Effects[0].Parameters[1].Name != "Intensity" {
		t.Fatal("Bloom schema lost its Intensity lane")
	}
	st.preparePostProcess(f.r, reg)
	cached := st.postProcess[f.r.Views()[0].ID]
	if cached == nil || len(cached.plan.Passes) != 1 {
		t.Fatal("active Bloom has no cached pass")
	}
	builds := cached.builds
	for i := 0; i < 60; i++ {
		want := float32(.1)
		if i%2 != 0 {
			want = .9
		}
		c.Effects[0].Parameters[1].Value.X = want
		st.preparePostProcess(f.r, reg)
		if cached.builds != builds || len(cached.plan.Passes) != 1 || cached.plan.Passes[0].Params[1][0] != want {
			t.Fatalf("numeric edit %d: builds %d -> %d, passes %+v", i, builds, cached.builds, cached.plan.Passes)
		}
	}
	c.Effects[0].Parameters[1].Value.X = 0
	st.preparePostProcess(f.r, reg)
	if cached.builds != builds+1 || len(cached.plan.Passes) != 0 {
		t.Fatalf("zero intensity: builds=%d passes=%d", cached.builds, len(cached.plan.Passes))
	}
	c.Effects[0].Parameters[1].Value.X = .22
	st.preparePostProcess(f.r, reg)
	if cached.builds != builds+2 || len(cached.plan.Passes) != 1 || cached.plan.Passes[0].Params[1][0] != .22 {
		t.Fatalf("restored intensity: builds=%d passes=%+v", cached.builds, cached.plan.Passes)
	}
	t.Logf("Bloom structural builds: %d after warmup and 60 numeric updates; %d after zero and restore", builds, cached.builds)
}
