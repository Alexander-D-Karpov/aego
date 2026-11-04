package postfx_test

import (
	"math"
	"strings"
	"testing"

	"aego/core/ids"
	"aego/render/postfx"
	"aego/render/shader"
)

func newInstance(t *testing.T, r *postfx.Registry, typ, id string) postfx.Instance {
	t.Helper()
	e, err := r.NewInstance(typ, id)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestPluginPlansInAuthoredOrder(t *testing.T) {
	r := postfx.NewRegistry()
	const id = "plugin.test-adjust"
	plugin := postfx.Descriptor{ID: id, Label: "Test Adjust", Version: 2, Phase: postfx.SceneLinear,
		Alpha: postfx.OpaqueOnly, Fragment: "vec4 aego_effect(vec4 c, vec2 uv, vec2 size, vec4 p[4]) { return c*p[0].x; }",
		Fields: []postfx.Field{{Name: "Amount", Kind: shader.KindFloat}},
	}
	if err := r.Register(plugin); err != nil {
		t.Fatal(err)
	}
	plugin.Fields[0].Name = "mutated by caller"
	a := newInstance(t, r, id, "a")
	a.Parameters[0].Value.X = .65
	b := newInstance(t, r, postfx.VignetteID, "b")
	c := newInstance(t, r, postfx.VignetteID, "c")
	c.Parameters[0].Value.X = .9
	items := []postfx.Instance{a, b, c}
	var p postfx.Plan
	if err := r.Build(&p, true, items, postfx.Profile{}); err != nil {
		t.Fatal(err)
	}
	if len(p.Passes) != 3 || p.Passes[0].Descriptor.ID != id || p.Passes[0].Params[0][0] != .65 {
		t.Fatalf("plugin pass: %+v", p.Passes)
	}
	desc := p.Passes[2].Descriptor
	items[1], items[2] = items[2], items[1]
	if err := r.Build(&p, true, items, postfx.Profile{}); err != nil {
		t.Fatal(err)
	}
	if p.Passes[1].InstanceID != "c" || p.Passes[1].Params[0][0] != .9 || p.Passes[1].Descriptor != desc {
		t.Fatalf("reorder/mutated uniform: %+v", p.Passes)
	}
	items[0], items[2] = items[2], items[0]
	if err := r.Build(&p, true, items, postfx.Profile{}); err == nil || len(p.Passes) != 0 || p.States[2].State != postfx.Invalid {
		t.Fatalf("accepted out-of-phase stack: %+v, %v", p, err)
	}
}

func TestTransparentAndLinearOutputPolicy(t *testing.T) {
	r := postfx.NewRegistry()
	a := newInstance(t, r, postfx.VignetteID, "safe")
	v, _ := r.Lookup(postfx.VignetteID)
	if v.Alpha != postfx.PreserveCoverage {
		t.Fatalf("vignette alpha: %v", v.Alpha)
	}
	requireOpaque := *v
	requireOpaque.ID, requireOpaque.Alpha = "plugin.opaque", postfx.OpaqueOnly
	if err := r.Register(requireOpaque); err != nil {
		t.Fatal(err)
	}
	items := []postfx.Instance{a, newInstance(t, r, requireOpaque.ID, "opaque")}
	var p postfx.Plan
	profile := postfx.Profile{Transparent: true}
	if err := r.Build(&p, true, items, profile); err != nil || len(p.Passes) != 1 || p.Passes[0].InstanceID != "safe" || p.States[1].State != postfx.Unsupported {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if err := r.ValidateForOutput(true, items, profile); err == nil || !strings.Contains(err.Error(), "opaque") {
		t.Fatalf("strict: %v", err)
	}
	profile.LinearOutput = true
	if err := r.Build(&p, true, items[:1], profile); err != nil || len(p.Passes) != 0 || p.States[0].State != postfx.Unsupported {
		t.Fatalf("display effect on linear output: %+v %v", p, err)
	}
}

func TestUnknownAndInvalidEffects(t *testing.T) {
	r := postfx.NewRegistry()
	known := newInstance(t, r, postfx.VignetteID, "valid")
	unknown := postfx.Instance{TypeID: "absent.module", InstanceID: "unknown", Version: 9,
		Parameters: []postfx.Parameter{{Name: "Map", Kind: shader.KindTexture, Asset: postfx.AssetRef{ID: ids.AssetID{Lo: 17}}}}}
	items := []postfx.Instance{known, unknown}
	var p postfx.Plan
	if err := r.Build(&p, true, items, postfx.Profile{}); err != nil || p.States[1].State != postfx.Unknown || len(p.Passes) != 1 {
		t.Fatalf("disabled unknown: %+v, %v", p, err)
	}
	items[1].Enabled = true
	if err := r.Build(&p, true, items, postfx.Profile{}); err == nil || len(p.Passes) != 0 || items[1].Parameters[0].Asset.ID.Lo != 17 {
		t.Fatalf("enabled unknown: %+v, %v", p, err)
	}
	if err := r.Validate([]postfx.Instance{known, known}); err == nil {
		t.Fatal("duplicate instance ID accepted")
	}
	for _, v := range []float32{-1, 1.1, float32(math.NaN()), float32(math.Inf(1))} {
		known.Parameters[0].Value.X = v
		if err := r.Validate([]postfx.Instance{known}); err == nil {
			t.Fatalf("accepted intensity %v", v)
		}
	}
	known.Parameters[0].Kind = shader.KindInt
	if err := r.Validate([]postfx.Instance{known}); err == nil {
		t.Fatal("mistyped intensity accepted")
	}
}

func TestWarmBypass(t *testing.T) {
	r := postfx.NewRegistry()
	e := newInstance(t, r, postfx.VignetteID, "v")
	items := []postfx.Instance{e}
	var p postfx.Plan
	for _, c := range []struct {
		enabled   bool
		intensity float32
	}{{false, .4}, {true, 0}, {true, .4}} {
		items[0].Parameters[0].Value.X = c.intensity
		build := func() {
			if err := r.Build(&p, c.enabled, items, postfx.Profile{}); err != nil {
				t.Fatal(err)
			}
		}
		build()
		if allocs := testing.AllocsPerRun(100, build); allocs != 0 {
			t.Errorf("warm build: %g allocations", allocs)
		}
		want := c.enabled && c.intensity != 0
		if (len(p.Passes) == 1) != want {
			t.Fatalf("pass count %d, enabled=%t intensity=%g", len(p.Passes), c.enabled, c.intensity)
		}
	}
	if err := r.Build(&p, true, nil, postfx.Profile{}); err != nil || len(p.States)+len(p.Passes) != 0 {
		t.Fatalf("stale state on empty plan: %+v", p)
	}
}
