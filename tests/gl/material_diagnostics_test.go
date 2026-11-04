//go:build aego_gltest

package gl

import (
	"runtime"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
)

func TestMaterialEffectiveGlobalsHaveNoUninitializedDriverDiagnostics(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	sink := &log.MemorySink{}
	logger := log.New(sink)
	logger.SetLevel(log.Debug)
	e, err := engine.NewBuilder(engine.Config{
		Title: "material-diagnostics", Width: 32, Height: 32, Hidden: true, Debug: true,
		ResRoot: t.TempDir(), UserRoot: t.TempDir(), AppID: "aego-gltest",
	}).WithLogger(logger).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Caps().Renderer == "null" || !e.GPU().Caps().DebugOutput {
		t.Skip("real GL debug output is unavailable; compiler diagnostic assertion not executed")
	}
	compile := func(name string, sources [4]string) {
		t.Helper()
		h, err := e.GPU().CreateShader(gpu.ShaderDesc{Label: name, Sources: []gpu.ShaderSource{{Stage: gpu.StageVertex, Source: sources[0]}, {Stage: gpu.StageFragment, Source: sources[1]}}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		e.GPU().DestroyShader(h)
	}
	for _, name := range asset.BuiltinMaterialNames() {
		id, _ := asset.BuiltinShaderNamed(name)
		data, _ := asset.BuiltinShader(id)
		a, err := asset.DecodeShader(data)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range a.Variants {
			compile(name, v.Sources)
		}
	}
	custom := runtimeShader(t, "typed-helper", `#pragma aego abi 1
#pragma material
uniform int Count; // @instance @default(2147483647)
uniform bool Enabled; // @instance @default(true)
uniform vec4 Tint; // @instance @default(1, 0.5, 0.25, 1)
uniform float Pulse; // @instance @default(0.5)
#pragma end
vec4 effectiveColor() { return Enabled && Count == 2147483647 ? Tint * Pulse : vec4(0.0); }
void vertex() { aego_world.x += effectiveColor().x; }
void fragment() { aego_out = effectiveColor(); }
`)
	compile("typed-helper", custom.Variants[0].Sources)
	for _, record := range sink.Records() {
		if record.Channel == log.GPU && strings.Contains(strings.ToLower(record.Msg), "uninitialized") {
			t.Errorf("generated effective parameter was not initialized: %s", record.Msg)
		}
	}
}

func TestMaterialCustomVertexPublishesAllEditedVaryings(t *testing.T) {
	r, dev, target := materialRig(t)
	sh := runtimeShader(t, "edited-varyings", `#pragma aego abi 1
void vertex() {
    aego_color = vec4(0.25, 0.5, 0.75, 1.0);
    aego_uv = vec2(0.125, 0.25);
    aego_local = vec2(0.5, 0.75);
    aego_region = vec4(0.125, 0.25, 0.75, 1.0);
    aego_local_rect = vec4(0.0, 0.125, 1.0, 0.875);
}
void fragment() {
    bool valid = distance(aego_uv, vec2(0.125, 0.25)) < 0.001
        && distance(aego_local, vec2(0.5, 0.75)) < 0.001
        && distance(aego_region, vec4(0.125, 0.25, 0.75, 1.0)) < 0.001
        && distance(aego_local_rect, vec4(0.0, 0.125, 1.0, 0.875)) < 0.001;
    aego_out = valid ? aego_color : vec4(1.0, 0.0, 0.0, 1.0);
}`)
	m, ok := material.New("edited-varyings", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("material creation")
	}
	px := drawMaterialFrame(t, r, dev, target, func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: 16, Y: 16}, Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m})
	})
	if px[0] < 63 || px[0] > 65 || px[1] < 127 || px[1] > 129 || px[2] < 190 || px[2] > 192 || px[3] != 255 {
		t.Fatalf("custom vertex edits were not published to fragment inputs: %v", px)
	}
}
