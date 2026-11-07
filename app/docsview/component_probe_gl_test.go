//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"aego/app/playground"
	"aego/asset"
	"aego/core/ids"
	"aego/engine/docs/democheck"
	"aego/schema"
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
	if err := instance.step(ctx); err != nil {
		a.Close()
		return nil, err
	}
	return instance, nil
}

type nativeTryInstance struct {
	app      *App
	contract democheck.Contract
	identity string
}

func (p *nativeTryInstance) step(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.app.Step(4); err != nil {
		return err
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
	sc := p.app.session.Controller().Scene()
	if sc == nil {
		return democheck.Observation{}, fmt.Errorf("Try scene is not running")
	}
	state := p.app.session.State()
	o := democheck.Observation{Identity: p.identity, Observables: map[string]json.RawMessage{}, RuntimeExecuting: state.Running && !state.Paused && state.Frames > 0}
	td, ok := sc.Registry().ByName(p.contract.ComponentID)
	if !ok {
		return o, fmt.Errorf("component is not registered in the Try runtime: %s", p.contract.ComponentID)
	}
	var found bool
	for _, h := range sc.PreOrder() {
		if _, found = sc.GetByType(h, td.ID); found {
			o.Components = append(o.Components, td.FullName)
			break
		}
	}
	if !found {
		return o, fmt.Errorf("runtime scene does not contain %s", p.contract.ComponentID)
	}
	for _, dep := range p.contract.Dependencies {
		id, err := ids.ParseAssetID(dep)
		if err != nil {
			return o, fmt.Errorf("invalid dependency %s: %w", dep, err)
		}
		kind := asset.ArtifactKind("")
		if builtin, ok := asset.LookupBuiltin(id); ok {
			kind = builtin.Kind
		} else if entry, ok := p.app.loader.Index().Lookup(id); ok && !entry.Missing {
			kind = entry.Kind
		}
		resolved := false
		switch kind {
		case asset.ArtMesh:
			_, resolved = p.app.loader.Mesh(id)
		case asset.ArtTexture, asset.ArtSpriteRegion, asset.ArtAtlasPage:
			_, resolved = p.app.loader.Texture(id)
		case "":
			return o, fmt.Errorf("unresolved dependency %s", dep)
		default:
			return o, fmt.Errorf("dependency kind %s is not supported by this probe", kind)
		}
		if !resolved {
			return o, fmt.Errorf("unresolved dependency %s", dep)
		}
		o.Dependencies = append(o.Dependencies, dep)
	}
	for _, action := range p.contract.Actions {
		for _, expected := range action.Expected {
			name, field, ok := strings.Cut(expected.Observable, p.contract.ComponentID+".")
			if !ok || name != "" || field == "" {
				return o, fmt.Errorf("unsupported runtime observable %s", expected.Observable)
			}
			var fd *schema.FieldDescriptor
			for i := range td.Fields {
				if strings.EqualFold(td.Fields[i].Name, field) {
					fd = &td.Fields[i]
					break
				}
			}
			if fd == nil {
				return o, fmt.Errorf("observable field is missing: %s", expected.Observable)
			}
			for _, h := range sc.PreOrder() {
				if _, ok := sc.GetByType(h, td.ID); !ok {
					continue
				}
				v, err := sc.GetProperty(h, td.ID, fd.ID)
				if err != nil {
					return o, err
				}
				var raw any
				switch v.Kind {
				case schema.KindBool:
					raw = v.I != 0
				case schema.KindInt32, schema.KindInt64, schema.KindEnum:
					raw = v.I
				case schema.KindFloat32, schema.KindFloat64:
					raw = v.F
				default:
					return o, fmt.Errorf("observable kind %v is not supported", v.Kind)
				}
				o.Observables[expected.Observable], err = json.Marshal(raw)
				if err != nil {
					return o, err
				}
				break
			}
		}
	}
	return o, nil
}

func (p *nativeTryInstance) Action(ctx context.Context, action democheck.Action) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	field, ok := strings.CutPrefix(action.Field, p.contract.ComponentID+".")
	if !ok || field == "" {
		return fmt.Errorf("unsupported action field: %s", action.Field)
	}
	var number float64
	if err := json.Unmarshal(action.Value, &number); err != nil {
		return fmt.Errorf("action value is not a number: %w", err)
	}
	request, err := json.Marshal(map[string]any{"component": p.contract.ComponentID, "field": field, "value": number})
	if err != nil {
		return err
	}
	if reply := p.app.Call("set", request); reply.Error != "" {
		return fmt.Errorf("set: %s", reply.Error)
	}
	return p.step(ctx)
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
	return p.step(ctx)
}

func (p *nativeTryInstance) Close() error {
	p.app.Close()
	return nil
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
	meshNative, spriteNative := false, false
	for _, result := range full.Results {
		if result.ComponentID == entry.ID {
			meshNative = slices.Contains(result.Environments, "native") && strings.Contains(strings.Join(result.Errors, " "), "chromium runner unavailable")
		} else if result.ComponentID == "engine.SpriteRenderer" {
			spriteNative = slices.Contains(result.Environments, "native") && strings.Contains(strings.Join(result.Errors, " "), "chromium runner unavailable")
		}
	}
	if !meshNative || !spriteNative {
		t.Fatalf("native registry probe: mesh=%v sprite=%v", meshNative, spriteNative)
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
