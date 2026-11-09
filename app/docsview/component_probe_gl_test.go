//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/engine/docs/democheck"
	"aego/gpu"
)

// nativeTryRunner drives the actual docsview load/set/reset bridge and renderer.
// It is deliberately limited to native: a separate browser runner is required
// before a contract that declares chromium can count as verified.
type nativeTryRunner struct{ p *playground.Playground }

func (r *nativeTryRunner) Open(ctx context.Context, c democheck.Contract, env string) (democheck.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if env != "native" || c.Environments[env].Mode != "runtime" {
		return nil, fmt.Errorf("%s runner unavailable: NOT RUN", env)
	}
	entry, ok := r.p.Docs().Get(c.ComponentID)
	if !ok || entry.Try == "" || entry.TrySelect != c.ComponentID {
		return nil, fmt.Errorf("component Try is missing or selects a different component")
	}
	identity := ""
	for _, component := range r.p.Docs().ComponentDemos() {
		if component.ID == entry.ID {
			identity = component.Identity
			break
		}
	}
	if identity == "" || identity != c.Identity {
		return nil, fmt.Errorf("Try payload identity mismatch")
	}
	code := ""
	var err error
	if entry.TryScript != "" {
		code, err = r.p.TryCode(entry.TryScript)
		if err != nil {
			return nil, err
		}
	}
	a, err := New(Options{Width: 960, Height: 440})
	if err != nil {
		return nil, err
	}
	// Keep the real SDL/OpenGL renderer, but use the existing lockstep clock
	// so actual physics steps do not depend on wall time between test frames.
	a.e.SetLockstep(true)
	if reply := a.Call("control", []byte(`{"enabled":true}`)); reply.Error != "" {
		a.Close()
		return nil, fmt.Errorf("control: %s", reply.Error)
	}
	request, err := json.Marshal(map[string]any{"scene": entry.Try, "select": entry.TrySelect, "code": code, "prefabs": entry.TryPrefabs, "artifacts": entry.TryArtifacts})
	if err != nil {
		a.Close()
		return nil, err
	}
	if reply := a.Call("load", request); reply.Error != "" {
		a.Close()
		return nil, fmt.Errorf("load: %s", reply.Error)
	}
	instance := &nativeTryInstance{app: a, contract: c, identity: identity}
	// The gate resets and performs the contract warmup before observing.
	if err := instance.advance(ctx, 4, nil); err != nil {
		a.Close()
		return nil, err
	}
	return instance, nil
}

type nativeTryInstance struct {
	app          *App
	contract     democheck.Contract
	identity     string
	captures     int
	observations int
}

func warmupFrames(c democheck.Contract) int {
	if c.WarmupFrames > 0 {
		return c.WarmupFrames
	}
	return 4
}

func (p *nativeTryInstance) advance(ctx context.Context, frames int, keys []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	request, err := json.Marshal(map[string]any{"frames": frames, "keys": keys})
	if err != nil {
		return err
	}
	if reply := p.app.Call("advance", request); reply.Error != "" {
		return fmt.Errorf("advance: %s", reply.Error)
	}
	if err := p.app.Step(frames); err != nil {
		return err
	}
	if p.app.control.Remaining != 0 {
		return fmt.Errorf("controlled input sequence did not complete")
	}
	world := p.app.session.Controller().World()
	if world == nil || world.Time.Tick() < 2 {
		return fmt.Errorf("Try runtime did not advance at least two fixed simulation steps")
	}
	if messages := errorsOf(p.app); len(messages) != 0 {
		return fmt.Errorf("Try runtime reported: %s", strings.Join(messages, "; "))
	}
	return nil
}

func (p *nativeTryInstance) Observe(ctx context.Context) (democheck.Observation, error) {
	if err := ctx.Err(); err != nil {
		return democheck.Observation{}, err
	}
	var fields []string
	seen := map[string]bool{}
	visual := false
	for _, action := range p.contract.Actions {
		visual = visual || action.Visual != nil
		for _, expected := range action.Expected {
			if !seen[expected.Observable] {
				seen[expected.Observable] = true
				fields = append(fields, expected.Observable)
			}
		}
	}
	inspection, err := p.app.inspectTry(p.contract.ComponentID, fields, p.contract.Dependencies)
	if err != nil {
		return democheck.Observation{}, err
	}
	o := democheck.Observation{Identity: p.identity, Components: inspection.Components, Dependencies: inspection.Dependencies, Observables: map[string]json.RawMessage{}, RuntimeExecuting: inspection.RuntimeExecuting}
	for name, value := range inspection.Observables {
		o.Observables[name], err = json.Marshal(value)
		if err != nil {
			return o, err
		}
	}
	if visual {
		var shot *image.RGBA
		var shotErr error
		p.app.e.CaptureFrame(func(img *image.RGBA, err error) { shot, shotErr = img, err })
		if err := p.app.Step(1); err != nil {
			return o, err
		}
		if shotErr != nil || shot == nil {
			return o, fmt.Errorf("capture Game View: %v", shotErr)
		}
		area, err := p.app.viewport.pixelRect(shot.Bounds().Dx(), shot.Bounds().Dy())
		if err != nil {
			return o, err
		}
		frame := democheck.Frame{Width: area.Dx(), Height: area.Dy(), Pixels: make([]byte, area.Dx()*area.Dy()*4)}
		for y := 0; y < area.Dy(); y++ {
			copy(frame.Pixels[y*frame.Width*4:(y+1)*frame.Width*4], shot.Pix[shot.PixOffset(area.Min.X, area.Min.Y+y):][:frame.Width*4])
		}
		dir, _, err := tryEvidenceDir(p.contract, "native")
		if err != nil {
			return o, err
		}
		if dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return o, err
			}
			file, err := os.Create(filepath.Join(dir, fmt.Sprintf("game-%03d.png", p.captures)))
			if err != nil {
				return o, err
			}
			err = png.Encode(file, &image.RGBA{Pix: frame.Pixels, Stride: frame.Width * 4, Rect: image.Rect(0, 0, frame.Width, frame.Height)})
			closeErr := file.Close()
			if err != nil {
				return o, err
			}
			if closeErr != nil {
				return o, closeErr
			}
			p.captures++
		}
		o.Frames = append(o.Frames, frame)
	}
	if err := saveNativeTryObservation(p.contract, p.observations, o); err != nil {
		return o, err
	}
	p.observations++
	return o, nil
}

func (p *nativeTryInstance) Action(ctx context.Context, action democheck.Action) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if action.ScriptToggle != nil {
		if action.ScriptToggle.Enabled == nil {
			return fmt.Errorf("invalid script attachment action")
		}
		payload, err := json.Marshal(action.ScriptToggle)
		if err != nil {
			return err
		}
		if reply := p.app.Call("scriptEnabled", payload); reply.Error != "" {
			return fmt.Errorf("scriptEnabled: %s", reply.Error)
		}
	} else if action.Field != "" {
		field, ok := strings.CutPrefix(action.Field, p.contract.ComponentID+".")
		if !ok || field == "" {
			return fmt.Errorf("unsupported action field: %s", action.Field)
		}
		var value any
		if err := json.Unmarshal(action.Value, &value); err != nil {
			return fmt.Errorf("invalid action value: %w", err)
		}
		switch value := value.(type) {
		case float64, string:
		case []any:
			if len(value) > 64 {
				return fmt.Errorf("action string list is too long")
			}
			for _, item := range value {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("action list contains a non-string value")
				}
			}
		default:
			return fmt.Errorf("action value is not a number, string or string list")
		}
		request, err := json.Marshal(map[string]any{"component": p.contract.ComponentID, "field": field, "value": value})
		if err != nil {
			return err
		}
		if reply := p.app.Call("set", request); reply.Error != "" {
			return fmt.Errorf("set: %s", reply.Error)
		}
	}
	if (action.Field != "" || action.ScriptToggle != nil) && (len(action.Keys) != 0 || len(action.Inputs) != 0) {
		if err := p.advance(ctx, warmupFrames(p.contract), nil); err != nil {
			return err
		}
	}
	for _, step := range action.Inputs {
		if err := p.advance(ctx, step.Frames, step.Keys); err != nil {
			return err
		}
	}
	if len(action.Inputs) != 0 && action.Frames == 0 && len(action.Keys) == 0 {
		return nil
	}
	frames := action.Frames
	if frames == 0 {
		frames = warmupFrames(p.contract)
	}
	return p.advance(ctx, frames, action.Keys)
}

func (p *nativeTryInstance) Reset(ctx context.Context, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if action != "reset" {
		return fmt.Errorf("unsupported reset action: %s", action)
	}
	if reply := p.app.Call("reset", nil); reply.Error != "" {
		return fmt.Errorf("reset: %s", reply.Error)
	}
	return p.advance(ctx, warmupFrames(p.contract), nil)
}

func (p *nativeTryInstance) Close() error {
	reporter, reports := p.app.e.GPU().(gpu.ResourceReporter)
	p.app.Close()
	if reports {
		if remaining := reporter.ResourceStats(); remaining != (gpu.ResourceStats{}) {
			return fmt.Errorf("Try close retained GPU resources: %+v", remaining)
		}
	}
	return nil
}

// Physics-backed demos must survive multiple fresh native runs, including
// their real action, rendered change and Reset. Chromium is still required
// separately; this regression test does not count it as executed.
func TestPhysicsBackedComponentDemosNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	components := p.Docs().ComponentDemos()
	for _, id := range []string{"camera.CameraFollow2D", "physics.BoxCollider2D", "physics.CircleCollider2D", "physics.CapsuleCollider2D", "physics.PolygonCollider2D", "physics.EdgeCollider2D"} {
		// t.Run executes in another goroutine, so keep SDL creation, rendering
		// and destruction on this same locked OS thread instead of a subtest.
		entry, ok := p.Docs().Get(id)
		if !ok || entry.Demo == nil {
			t.Fatalf("%s: component demo contract is absent", id)
		}
		var component democheck.Component
		for _, candidate := range components {
			if candidate.ID == id {
				component = candidate
				break
			}
		}
		if component.ID == "" {
			t.Fatalf("%s: component is missing from the registry", id)
		}
		for run := range 3 {
			report := democheck.Check(context.Background(), []democheck.Component{component}, []democheck.Contract{*entry.Demo}, &nativeTryRunner{p: p})
			if len(report.Results) != 1 || !slices.Contains(report.Results[0].Environments, "native") {
				t.Errorf("%s: native run %d failed: %+v", id, run+1, report)
				break
			}
			if report.Passed() || slices.Contains(report.Results[0].Environments, "chromium") {
				t.Errorf("%s: native-only execution was counted as the browser gate", id)
				break
			}
		}
	}
}

func TestComponentContractProbeRunsNativeAndRejectsUnrunWeb(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("engine.Mesh2D")
	if !ok || entry.Demo == nil {
		t.Fatal("Mesh2D demo contract is absent")
	}
	var component democheck.Component
	for _, c := range p.Docs().ComponentDemos() {
		if c.ID == entry.ID {
			component = c
			break
		}
	}
	runner := &nativeTryRunner{p: p}
	run := func(c democheck.Contract) democheck.Report {
		return democheck.Check(context.Background(), []democheck.Component{component}, []democheck.Contract{c}, runner)
	}
	report := run(*entry.Demo)
	if report.Passed() || report.VerifiedComponents != 0 || len(report.Results) != 1 || !slices.Equal(report.Results[0].Environments, []string{"native"}) || !strings.Contains(strings.Join(report.Results[0].Errors, " "), "chromium runner unavailable") {
		t.Fatalf("native must run and missing chromium must fail closed: %+v", report)
	}
	t.Logf("native Mesh2D contract action and Reset passed; chromium NOT RUN, component remains unverified")
	full := p.Docs().CheckComponentDemos(context.Background(), runner)
	if full.Passed() || full.PublicComponents != len(p.Docs().ComponentDemos()) || full.VerifiedComponents >= full.PublicComponents {
		t.Fatalf("the full inventory must not pass with a browser runner missing: %+v", full)
	}
	native := map[string]bool{"engine.Camera2D": false, "engine.Camera3D": false, "engine.Light2D": false, "engine.LightingEnvironment2D": false, "engine.LightOccluder2D": false, "engine.Mesh2D": false, "engine.MeshRenderer": false, "engine.SpriteRenderer": false, "gameplay.Health": false, "particles.ParticleSystem2D": false, "physics.CircleCollider2D": false, "render2d.PostProcess": false}
	for _, result := range full.Results {
		if _, want := native[result.ComponentID]; want {
			native[result.ComponentID] = slices.Contains(result.Environments, "native") && strings.Contains(strings.Join(result.Errors, " "), "chromium runner unavailable")
			t.Logf("%s environments=%v errors=%v", result.ComponentID, result.Environments, result.Errors)
		}
	}
	for id, passed := range native {
		if !passed {
			t.Errorf("native registry probe failed for %s", id)
		}
	}
	if t.Failed() {
		t.Fatalf("native registry probe: %v", native)
	}
	t.Logf("full component inventory: %d/%d verified across every declared environment", full.VerifiedComponents, full.PublicComponents)
	bad := *entry.Demo
	bad.Dependencies = append(slices.Clone(bad.Dependencies), "ffffffffffffffffffffffffffffffff")
	report = run(bad)
	if report.Passed() || len(report.Results) != 1 || slices.Contains(report.Results[0].Environments, "native") || !strings.Contains(strings.Join(report.Results[0].Errors, " "), "unresolved dependency") {
		t.Fatalf("native probe accepted a missing asset: %+v", report)
	}
	wrong := *entry.Demo
	wrong.Actions = slices.Clone(wrong.Actions)
	wrong.Actions[0].Expected = slices.Clone(wrong.Actions[0].Expected)
	wrong.Actions[0].Expected[0].Equals = json.RawMessage("999")
	report = run(wrong)
	if report.Passed() || slices.Contains(report.Results[0].Environments, "native") || !strings.Contains(strings.Join(report.Results[0].Errors, " "), "unexpected observable") {
		t.Fatalf("native probe accepted a false expectation: %+v", report)
	}
}
