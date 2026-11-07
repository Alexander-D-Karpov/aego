//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"aego/app/playground"
	"aego/engine/docs/democheck"
)

type browserProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	encode *json.Encoder
	decode *json.Decoder
	next   int
}

func startBrowserProcess(ctx context.Context, site, script string) (*browserProcess, error) {
	cmd := exec.CommandContext(ctx, "node", script, "--site="+site)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		return nil, err
	}
	return &browserProcess{cmd: cmd, stdin: in, encode: json.NewEncoder(in), decode: json.NewDecoder(out)}, nil
}

func (p *browserProcess) call(ctx context.Context, method string, values map[string]any, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if values == nil {
		values = map[string]any{}
	}
	p.next++
	values["id"], values["method"] = p.next, method
	if err := p.encode.Encode(values); err != nil {
		return fmt.Errorf("browser %s request: %w", method, err)
	}
	var reply struct {
		ID     int             `json:"id"`
		Error  string          `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := p.decode.Decode(&reply); err != nil {
		return fmt.Errorf("browser %s reply: %w", method, err)
	}
	if reply.ID != p.next {
		return fmt.Errorf("browser protocol response ID mismatch: %d != %d", reply.ID, p.next)
	}
	if reply.Error != "" {
		return fmt.Errorf("browser %s: %s", method, reply.Error)
	}
	if result != nil {
		return json.Unmarshal(reply.Result, result)
	}
	return nil
}

func (p *browserProcess) close() error {
	err := p.stdin.Close()
	return errors.Join(err, p.cmd.Wait())
}

type browserTryRunner struct {
	p       *playground.Playground
	process *browserProcess
}

func (r *browserTryRunner) payload(c democheck.Contract) (map[string]any, error) {
	e, ok := r.p.Docs().Get(c.ComponentID)
	if !ok || e.Try == "" || e.TrySelect != c.ComponentID {
		return nil, fmt.Errorf("component Try is missing or selects a different component")
	}
	matched := false
	for _, component := range r.p.Docs().ComponentDemos() {
		if component.ID == c.ComponentID && component.Identity == c.Identity {
			matched = true
			break
		}
	}
	if !matched {
		return nil, fmt.Errorf("Try payload identity mismatch")
	}
	code := ""
	var err error
	if e.TryScript != "" {
		code, err = r.p.TryCode(e.TryScript)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts}, nil
}

func (r *browserTryRunner) Open(ctx context.Context, c democheck.Contract, env string) (democheck.Instance, error) {
	if env != "chromium" || c.Environments[env].Mode != "runtime" {
		return nil, fmt.Errorf("%s runner unavailable: NOT RUN", env)
	}
	payload, err := r.payload(c)
	if err != nil {
		return nil, err
	}
	if err := r.process.call(ctx, "open", map[string]any{"contract": c, "payload": payload}, nil); err != nil {
		return nil, err
	}
	return &browserTryInstance{process: r.process, identity: c.Identity}, nil
}

type browserTryInstance struct {
	process  *browserProcess
	identity string
}

func (p *browserTryInstance) Observe(ctx context.Context) (democheck.Observation, error) {
	var observation democheck.Observation
	if err := p.process.call(ctx, "observe", nil, &observation); err != nil {
		return observation, err
	}
	// Open verified the actual payload on both localized pages against the exact
	// registry entry before the runtime loaded it. Never trust an HTML label.
	observation.Identity = p.identity
	return observation, nil
}

func (p *browserTryInstance) Action(ctx context.Context, action democheck.Action) error {
	return p.process.call(ctx, "action", map[string]any{"action": action}, nil)
}

func (p *browserTryInstance) Reset(ctx context.Context, action string) error {
	return p.process.call(ctx, "reset", map[string]any{"action": action}, nil)
}

func (p *browserTryInstance) Close() error {
	return p.process.call(context.Background(), "close", nil, nil)
}

type combinedTryRunner struct {
	native  *nativeTryRunner
	browser *browserTryRunner
}

func (r combinedTryRunner) Open(ctx context.Context, c democheck.Contract, env string) (democheck.Instance, error) {
	if env == "native" {
		return r.native.Open(ctx, c, env)
	}
	return r.browser.Open(ctx, c, env)
}

// AEGO_TRY_SITE must name a freshly built docs site. Without it this test does
// not run and does not claim coverage. Set AEGO_DEMOCHECK_ENFORCE=1 to fail on
// every missing contract or failed environment, not only on the implemented
// browser/native probe regression cases.
func TestComponentContractsAcrossNativeAndChromium(t *testing.T) {
	site := os.Getenv("AEGO_TRY_SITE")
	if site == "" {
		if os.Getenv("AEGO_DEMOCHECK_ENFORCE") == "1" {
			t.Fatal("AEGO_TRY_SITE is required for the enforced native+browser component gate")
		}
		t.Skip("AEGO_TRY_SITE is unset: native+browser component gate NOT RUN")
	}
	site, err := filepath.Abs(site)
	if err != nil {
		t.Fatal(err)
	}
	script := os.Getenv("AEGO_TRY_BROWSER_RUNNER")
	if script == "" {
		script = filepath.Join("..", "..", "tools", "websmoke", "try-contract.mjs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	process, err := startBrowserProcess(ctx, site, script)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := process.close(); err != nil {
			t.Errorf("browser runner shutdown: %v", err)
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	browser := &browserTryRunner{p: p, process: process}
	runner := combinedTryRunner{native: &nativeTryRunner{p: p}, browser: browser}
	report := p.Docs().CheckComponentDemos(ctx, runner)
	if name := os.Getenv("AEGO_DEMOCHECK_REPORT"); name != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	verified := map[string]bool{}
	for _, result := range report.Results {
		if len(result.Errors) == 0 && slices.Contains(result.Environments, "native") && slices.Contains(result.Environments, "chromium") {
			verified[result.ComponentID] = true
		}
		if result.DemoID != "" {
			t.Logf("%s environments=%v errors=%v", result.ComponentID, result.Environments, result.Errors)
		}
	}
	for _, id := range []string{"engine.Camera2D", "engine.Camera3D", "engine.Light2D", "engine.LightingEnvironment2D", "engine.LightOccluder2D", "engine.Mesh2D", "engine.MeshRenderer", "engine.SpriteRenderer", "particles.ParticleSystem2D", "render2d.PostProcess"} {
		if !verified[id] {
			t.Errorf("%s did not pass native and Chromium", id)
		}
	}
	if t.Failed() {
		t.Fatalf("implemented contracts did not pass native and Chromium: %v", verified)
	}
	t.Logf("actual component Try coverage: %d/%d", report.VerifiedComponents, report.PublicComponents)
	entry, _ := p.Docs().Get("engine.Mesh2D")
	var component democheck.Component
	for _, c := range p.Docs().ComponentDemos() {
		if c.ID == entry.ID {
			component = c
		}
	}
	wrong := *entry.Demo
	wrong.Actions = slices.Clone(wrong.Actions)
	wrong.Actions[0].Expected = slices.Clone(wrong.Actions[0].Expected)
	wrong.Actions[0].Expected[0].Equals = json.RawMessage("999")
	bad := democheck.Check(ctx, []democheck.Component{component}, []democheck.Contract{wrong}, runner)
	if bad.Passed() || len(bad.Results) != 1 || len(bad.Results[0].Environments) != 0 || !strings.Contains(strings.Join(bad.Results[0].Errors, " "), "unexpected observable") {
		t.Fatalf("false expectations were accepted: %+v", bad)
	}
	badPayload, err := browser.payload(*entry.Demo)
	if err != nil {
		t.Fatal(err)
	}
	badPayload["scene"] = "different scene"
	if err := process.call(ctx, "open", map[string]any{"contract": *entry.Demo, "payload": badPayload}, nil); err == nil || !strings.Contains(err.Error(), "payload does not match") {
		t.Fatalf("stale published Try payload was accepted: %v", err)
	}
	if os.Getenv("AEGO_DEMOCHECK_ENFORCE") == "1" && !report.Passed() {
		t.Fatalf("full component gate failed: %v", report.Error())
	}
}
