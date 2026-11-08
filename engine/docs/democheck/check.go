// Package democheck validates component Try contracts against the existing docs
// registry and observations from its real native/web Try runtime. It does not
// load scenes, simulate components or replace the .aetry execution pipeline.
package democheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type Text struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Input       string `json:"input"`
}
type Environment struct {
	// Mode is runtime or authoring. Authoring never counts as execution of a
	// native-only runtime feature; the page must display Reason in RU and EN.
	Mode   string            `json:"mode"`
	Reason map[string]string `json:"reason,omitempty"`
}
type Expectation struct {
	Observable string `json:"observable"`
	// Equals can be any JSON scalar/array/object; nil is a legitimate value.
	Equals json.RawMessage `json:"equals"`
	// Changes prevents a no-op action or a permanently painted preview from
	// satisfying the acceptance test merely by starting in its expected state.
	Changes bool `json:"changes"`
}
type Action struct {
	ID    string          `json:"id"`
	Field string          `json:"field,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
	// ScriptToggle is the Inspector's attachment Enabled switch on the selected
	// component's node, not a synthetic field on the demonstrated component.
	ScriptToggle *ScriptToggle `json:"scriptToggle,omitempty"`
	Expected     []Expectation `json:"expected"`
	// Visual is opt-in for actions whose result must be visible in Game View.
	// The runner supplies actual rendered frames, not a synthetic thumbnail.
	Visual *VisualExpectation `json:"visual,omitempty"`
}
type ScriptToggle struct {
	Script  string `json:"script"`
	Enabled *bool  `json:"enabled"`
}
type VisualExpectation struct {
	MinChangedPixels         int `json:"minChangedPixels"`
	MaxResetDifferencePixels int `json:"maxResetDifferencePixels"`
}
type Contract struct {
	Version     int    `json:"version"`
	ComponentID string `json:"componentID"`
	DemoID      string `json:"demoID"`
	DocumentID  string `json:"documentID"`
	// Scene is the existing registry entry's Try source identifier, not a second
	// scene implementation. Identity binds every probe result to its loaded Try.
	Scene          string                 `json:"scene"`
	Identity       string                 `json:"identity"`
	Dependencies   []string               `json:"dependencies"`
	EditableFields []string               `json:"editableFields"`
	Actions        []Action               `json:"actions"`
	Reset          string                 `json:"reset"`
	Languages      map[string]Text        `json:"languages"`
	Environments   map[string]Environment `json:"environments"`
}

// Component is derived from real Component Registry entries, never from all
// documentation items. Fields are derived from the current component schema.
type Component struct {
	ID, DocumentID, Scene, Identity string
	Fields                          []string
}
type Observation struct {
	Identity     string
	Components   []string
	Dependencies []string
	// Observables are deterministic, contract-selected values. Volatile clocks,
	// random IDs and whole-frame noise must not be used as reset assertions.
	Observables      map[string]json.RawMessage
	RuntimeExecuting bool
	// Frames are RGBA Game View captures. Web probes include one per language.
	Frames []Frame `json:"frames,omitempty"`
}
type Frame struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Pixels []byte `json:"pixels"`
}

// Runner adapts the existing Try runtime. Open must use the provided entry's
// real scene and prepared dependencies. Reset and Action must drive that same
// instance. No runner means NOT RUN, never a successful coverage report.
type Runner interface {
	Open(context.Context, Contract, string) (Instance, error)
}
type Instance interface {
	Observe(context.Context) (Observation, error)
	Action(context.Context, Action) error
	Reset(context.Context, string) error
	Close() error
}
type Result struct {
	ComponentID  string   `json:"componentID"`
	DemoID       string   `json:"demoID,omitempty"`
	Errors       []string `json:"errors,omitempty"`
	Environments []string `json:"verifiedEnvironments,omitempty"`
}
type Report struct {
	PublicComponents   int      `json:"publicComponents"`
	VerifiedComponents int      `json:"verifiedComponents"`
	Results            []Result `json:"results"`
	Errors             []string `json:"errors,omitempty"`
}

func (r Report) Passed() bool {
	return r.PublicComponents > 0 && r.VerifiedComponents == r.PublicComponents && len(r.Errors) == 0
}
func (r Report) Error() error {
	if r.Passed() {
		return nil
	}
	return fmt.Errorf("component Try coverage: %d/%d verified, %d registry errors", r.VerifiedComponents, r.PublicComponents, len(r.Errors))
}
func unique(values []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, v := range values {
		if strings.TrimSpace(v) == "" || out[v] {
			return nil, fmt.Errorf("empty or duplicate ID %q", v)
		}
		out[v] = true
	}
	return out, nil
}
func Check(ctx context.Context, components []Component, contracts []Contract, runner Runner) Report {
	r := Report{PublicComponents: len(components)}
	counts, ids, demoIDs := map[string]int{}, map[string]bool{}, map[string]int{}
	for _, c := range contracts {
		counts[c.ComponentID]++
		demoIDs[c.DemoID]++
	}
	for _, c := range components {
		if c.ID == "" || ids[c.ID] {
			r.Errors = append(r.Errors, "empty or duplicate component ID: "+c.ID)
		}
		ids[c.ID] = true
	}
	for _, c := range contracts {
		if !ids[c.ComponentID] {
			r.Errors = append(r.Errors, "contract for unknown component: "+c.ComponentID)
		}
	}
	if len(components) == 0 {
		r.Errors = append(r.Errors, "empty Component Registry cannot satisfy coverage")
	}
	ordered := append([]Component(nil), components...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, component := range ordered {
		result := Result{ComponentID: component.ID}
		if counts[component.ID] != 1 {
			result.Errors = append(result.Errors, fmt.Sprintf("expected one contract, found %d", counts[component.ID]))
			r.Results = append(r.Results, result)
			continue
		}
		var c Contract
		for _, v := range contracts {
			if v.ComponentID == component.ID {
				c = v
				break
			}
		}
		result.DemoID = c.DemoID
		if err := validate(component, c); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
		if demoIDs[c.DemoID] != 1 {
			result.Errors = append(result.Errors, "DemoID is not unique")
		}
		if len(result.Errors) == 0 {
			if runner == nil {
				result.Errors = append(result.Errors, "runtime probe is absent: NOT RUN")
			} else {
				envs := make([]string, 0, len(c.Environments))
				for name := range c.Environments {
					envs = append(envs, name)
				}
				sort.Strings(envs)
				for _, env := range envs {
					if err := probe(ctx, c, env, runner); err != nil {
						result.Errors = append(result.Errors, env+": "+err.Error())
					} else {
						result.Environments = append(result.Environments, env)
					}
				}
			}
		}
		if len(result.Errors) == 0 {
			r.VerifiedComponents++
		}
		r.Results = append(r.Results, result)
	}
	return r
}
func validate(component Component, c Contract) error {
	if c.Version != 1 || c.DemoID == "" || c.DocumentID != component.DocumentID || c.Scene == "" || c.Scene != component.Scene || c.Identity == "" || c.Identity != component.Identity {
		return errors.New("invalid contract identity or source binding")
	}
	fields, err := unique(component.Fields)
	if err != nil {
		return fmt.Errorf("component schema: %w", err)
	}
	edits, err := unique(c.EditableFields)
	if err != nil {
		return err
	}
	for field := range edits {
		if !fields[field] {
			return fmt.Errorf("editable field is absent in schema: %s", field)
		}
	}
	if _, err = unique(c.Dependencies); err != nil {
		return err
	}
	for _, lang := range []string{"en", "ru"} {
		t, ok := c.Languages[lang]
		if !ok || strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.Description) == "" || strings.TrimSpace(t.Input) == "" {
			return fmt.Errorf("missing %s name, description or input hint", lang)
		}
	}
	if c.Reset == "" || len(c.Actions) == 0 || len(c.Actions) > 256 {
		return errors.New("reset and 1..256 actions are required")
	}
	actionIDs := map[string]bool{}
	modified := map[string]bool{}
	for _, a := range c.Actions {
		if a.ID == "" || actionIDs[a.ID] {
			return errors.New("empty or duplicate action ID")
		}
		actionIDs[a.ID] = true
		if a.ScriptToggle != nil {
			if a.Field != "" || len(a.Value) != 0 || a.ScriptToggle.Enabled == nil || strings.TrimSpace(a.ScriptToggle.Script) != a.ScriptToggle.Script || a.ScriptToggle.Script == "" || len(a.ScriptToggle.Script) > 256 {
				return fmt.Errorf("invalid script attachment action: %s", a.ID)
			}
		} else if a.Field != "" {
			if !edits[a.Field] {
				return fmt.Errorf("action field is not editable: %s", a.Field)
			}
			if !json.Valid(a.Value) {
				return fmt.Errorf("invalid action value: %s", a.ID)
			}
			modified[a.Field] = true
		} else {
			return fmt.Errorf("action has no supported operation: %s", a.ID)
		}
		if len(a.Expected) == 0 {
			return fmt.Errorf("action has no observable result: %s", a.ID)
		}
		if a.Visual != nil && (a.Visual.MinChangedPixels <= 0 || a.Visual.MaxResetDifferencePixels < 0) {
			return fmt.Errorf("invalid visual expectation: %s", a.ID)
		}
		changed := false
		seen := map[string]bool{}
		for _, e := range a.Expected {
			if e.Observable == "" || seen[e.Observable] || !json.Valid(e.Equals) {
				return fmt.Errorf("invalid/duplicate expectation in %s", a.ID)
			}
			seen[e.Observable] = true
			changed = changed || e.Changes
		}
		if !changed {
			return fmt.Errorf("action must demonstrate a state change: %s", a.ID)
		}
	}
	for f := range edits {
		if !modified[f] {
			return fmt.Errorf("editable field has no tested action: %s", f)
		}
	}
	if len(c.Environments) == 0 {
		return errors.New("compatibility environments are absent")
	}
	// Both platforms must be accounted for. A truthful authoring scenario is
	// allowed for native-only features, not silently omitting their web entry.
	hasNative, hasWeb := false, false
	for env, compat := range c.Environments {
		if env == "native" {
			hasNative = true
		} else if env == "chromium" || env == "firefox" || env == "webkit" {
			hasWeb = true
		} else {
			return fmt.Errorf("unknown environment: %s", env)
		}
		if compat.Mode != "runtime" && compat.Mode != "authoring" {
			return fmt.Errorf("unknown compatibility mode: %s", env)
		}
		if compat.Mode == "authoring" && (strings.TrimSpace(compat.Reason["en"]) == "" || strings.TrimSpace(compat.Reason["ru"]) == "") {
			return fmt.Errorf("authoring-only limitation is not localized: %s", env)
		}
	}
	if !hasNative || !hasWeb {
		return errors.New("native and at least one web environment are required")
	}
	return nil
}
func checkObservation(c Contract, env string, o Observation) error {
	if o.Identity != c.Identity {
		return errors.New("probe loaded a different or stale Try payload")
	}
	components, err := unique(o.Components)
	if err != nil {
		return err
	}
	if !components[c.ComponentID] {
		return errors.New("loaded scene does not contain the demonstrated component")
	}
	deps, err := unique(o.Dependencies)
	if err != nil {
		return err
	}
	for _, id := range c.Dependencies {
		if !deps[id] {
			return fmt.Errorf("unresolved dependency: %s", id)
		}
	}
	if c.Environments[env].Mode == "authoring" && o.RuntimeExecuting {
		return errors.New("authoring-only demo falsely claims runtime execution")
	}
	if c.Environments[env].Mode == "runtime" && !o.RuntimeExecuting {
		return errors.New("runtime demo did not execute")
	}
	return nil
}
func equalJSON(a, b json.RawMessage) bool {
	var x, y any
	da, db := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	da.UseNumber()
	db.UseNumber()
	if da.Decode(&x) != nil || db.Decode(&y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func frameDifference(a, b Frame) (int, error) {
	if a.Width <= 0 || a.Height <= 0 || a.Width > 4096 || a.Height > 4096 || a.Width*a.Height > 4*1024*1024 || a.Width != b.Width || a.Height != b.Height || len(a.Pixels) != a.Width*a.Height*4 || len(b.Pixels) != len(a.Pixels) {
		return 0, errors.New("missing, invalid or differently sized Game View frame")
	}
	changed := 0
	for i := 0; i < len(a.Pixels); i += 4 {
		if a.Pixels[i] != b.Pixels[i] || a.Pixels[i+1] != b.Pixels[i+1] || a.Pixels[i+2] != b.Pixels[i+2] || a.Pixels[i+3] != b.Pixels[i+3] {
			changed++
		}
	}
	return changed, nil
}

func probe(ctx context.Context, c Contract, env string, r Runner) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	instance, err := r.Open(ctx, c, env)
	if err != nil {
		return err
	}
	if instance == nil {
		return errors.New("probe returned no instance")
	}
	defer func() { err = errors.Join(err, instance.Close()) }()
	// Each action starts from Reset, not from hidden state left by another test.
	for _, action := range c.Actions {
		if err = instance.Reset(ctx, c.Reset); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		before, e := instance.Observe(ctx)
		if e != nil {
			return e
		}
		if e = checkObservation(c, env, before); e != nil {
			return e
		}
		if err = instance.Action(ctx, action); err != nil {
			return fmt.Errorf("action %s: %w", action.ID, err)
		}
		after, e := instance.Observe(ctx)
		if e != nil {
			return e
		}
		if e = checkObservation(c, env, after); e != nil {
			return e
		}
		if action.Visual != nil {
			wantFrames := 1
			if env != "native" {
				wantFrames = 2 // EN and RU must each have an actual browser frame.
			}
			if len(before.Frames) != wantFrames || len(after.Frames) != wantFrames {
				return fmt.Errorf("action %s: missing Game View frames", action.ID)
			}
			for i := range before.Frames {
				diff, e := frameDifference(before.Frames[i], after.Frames[i])
				if e != nil {
					return fmt.Errorf("action %s: %w", action.ID, e)
				}
				if diff < action.Visual.MinChangedPixels {
					return fmt.Errorf("action %s: Game View changed %d pixels, need %d", action.ID, diff, action.Visual.MinChangedPixels)
				}
			}
		}
		for _, expect := range action.Expected {
			a, ok := after.Observables[expect.Observable]
			if !ok || !equalJSON(a, expect.Equals) {
				return fmt.Errorf("action %s: unexpected observable %s", action.ID, expect.Observable)
			}
			b, ok := before.Observables[expect.Observable]
			if !ok || !json.Valid(b) {
				return fmt.Errorf("action %s: baseline observable missing: %s", action.ID, expect.Observable)
			}
			if expect.Changes && equalJSON(a, b) {
				return fmt.Errorf("action %s: observable did not change: %s", action.ID, expect.Observable)
			}
		}
		if err = instance.Reset(ctx, c.Reset); err != nil {
			return fmt.Errorf("reset after action: %w", err)
		}
		reset, e := instance.Observe(ctx)
		if e != nil {
			return e
		}
		if e = checkObservation(c, env, reset); e != nil {
			return e
		}
		if action.Visual != nil {
			if len(reset.Frames) != len(before.Frames) {
				return fmt.Errorf("reset after %s: missing Game View frames", action.ID)
			}
			for i := range before.Frames {
				diff, e := frameDifference(before.Frames[i], reset.Frames[i])
				if e != nil {
					return fmt.Errorf("reset after %s: %w", action.ID, e)
				}
				if diff > action.Visual.MaxResetDifferencePixels {
					return fmt.Errorf("reset after %s: Game View differs in %d pixels, maximum %d", action.ID, diff, action.Visual.MaxResetDifferencePixels)
				}
			}
		}
		for _, expect := range action.Expected {
			if !equalJSON(before.Observables[expect.Observable], reset.Observables[expect.Observable]) {
				return fmt.Errorf("reset did not restore observable: %s", expect.Observable)
			}
		}
	}
	return nil
}
