//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"context"
	"encoding/json"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"aego/app/playground"
	"aego/engine/docs/democheck"
	"aego/modules/savegame"
)

func TestNativePersistReadyRequiresLoadedWorldTicks(t *testing.T) {
	complete := persistCycleEvents{saved: true, loaded: true}
	if nativePersistReady(complete, true, 0) || nativePersistReady(complete, true, 1) ||
		!nativePersistReady(complete, true, 2) || nativePersistReady(complete, false, 2) ||
		nativePersistReady(persistCycleEvents{loaded: true}, true, 2) ||
		nativePersistReady(persistCycleEvents{saved: true}, true, 2) {
		t.Fatal("native completion must require both events, slot removal and two loaded-world ticks")
	}
}

func TestNativePersistComponentWaitsForDelayedWorker(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("save.Persist")
	if !ok || entry.Demo == nil || len(entry.Demo.Actions) != 1 || entry.Demo.Actions[0].Visual == nil ||
		len(entry.Demo.Actions[0].Expected) != 2 || entry.Demo.Actions[0].Expected[1].Observable != "save.Persist.PositionX" {
		t.Fatal("Persist component contract is absent")
	}
	ctx := context.Background()
	for _, target := range []string{"open", "initial reset", "action", "final reset"} {
		func() {
			var release func()
			held, waited, phase := false, false, "open"
			hold := func(current *nativeTryInstance) {
				jobs := current.app.e.Jobs()
				if jobs.Workers() < 1 {
					t.Fatal("no save worker available")
				}
				entered := make(chan struct{}, jobs.Workers())
				gate := make(chan struct{})
				var once sync.Once
				release = func() { once.Do(func() { close(gate) }) }
				t.Cleanup(release)
				for range jobs.Workers() {
					jobs.Submit(func() { entered <- struct{}{}; <-gate })
				}
				timer := time.NewTimer(10 * time.Second)
				defer timer.Stop()
				for range jobs.Workers() {
					select {
					case <-entered:
					case <-timer.C:
						t.Fatal("could not hold the save worker")
					}
				}
				held = true
			}
			onWait := func(current *nativeTryInstance) {
				if !held || waited {
					return
				}
				if phase != target {
					t.Fatalf("%s continued into %s before waiting", target, phase)
				}
				world := current.app.session.Controller().World()
				if world == nil {
					t.Fatal("Try world is not running")
				}
				service := savegame.Of(world)
				reply := current.app.Call("inspect", []byte(`{"component":"save.Persist","fields":["save.Persist.PositionX"]}`))
				if current.persist.saved || current.persist.loaded || service == nil || service.Exists("persist-demo") ||
					reply.Error != "" || reply.Inspection == nil || reply.Inspection.Observables["save.Persist.PositionX"] != float64(2) {
					t.Fatalf("%s did not hold the four-frame state: events=%+v inspection=%+v", target, current.persist, reply)
				}
				waited = true
				release()
			}
			runner := &nativeTryRunner{p: p, beforeWarmup: func(current *nativeTryInstance) func() {
				current.onPersistWait = onWait
				if target == "open" {
					hold(current)
					return release
				}
				return nil
			}}
			raw, err := runner.Open(ctx, *entry.Demo, "native")
			if err != nil {
				t.Fatal(err)
			}
			instance := raw.(*nativeTryInstance)
			defer func() {
				if release != nil {
					release()
				}
				if err := instance.Close(); err != nil {
					t.Error(err)
				}
			}()
			if target == "open" && !waited {
				t.Fatal("open did not wait for the delayed worker")
			}
			phase = "initial reset"
			if phase == target {
				hold(instance)
			}
			if err := instance.Reset(ctx, "reset"); err != nil {
				t.Fatal(err)
			}
			before, err := instance.Observe(ctx)
			if err != nil {
				t.Fatal(err)
			}
			phase = "action"
			if phase == target {
				hold(instance)
			}
			if err := instance.Action(ctx, entry.Demo.Actions[0]); err != nil {
				t.Fatal(err)
			}
			after, err := instance.Observe(ctx)
			if err != nil {
				t.Fatal(err)
			}
			phase = "final reset"
			if phase == target {
				hold(instance)
			}
			if err := instance.Reset(ctx, "reset"); err != nil {
				t.Fatal(err)
			}
			restored, err := instance.Observe(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !held || !waited {
				t.Fatalf("%s did not wait for the delayed worker: held=%v waited=%v", target, held, waited)
			}
			check := func(name string, observation democheck.Observation, position, noTransform string) {
				t.Helper()
				if !observation.RuntimeExecuting || string(observation.Observables["save.Persist.PositionX"]) != position ||
					string(observation.Observables["save.Persist.NoTransform"]) != noTransform {
					t.Fatalf("%s %s: unexpected runtime observation: %+v", target, name, observation.Observables)
				}
			}
			check("before", before, "2", "false")
			check("after", after, "-2", "true")
			check("reset", restored, "2", "false")
			compare := func(a, b democheck.Observation) int {
				t.Helper()
				if len(a.Frames) != 1 || len(b.Frames) != 1 {
					t.Fatal("native Game View frames are missing")
				}
				x, y := a.Frames[0], b.Frames[0]
				if x.Width <= 0 || x.Height <= 0 || x.Width != y.Width || x.Height != y.Height ||
					len(x.Pixels) != x.Width*x.Height*4 || len(y.Pixels) != len(x.Pixels) {
					t.Fatal("invalid native Game View frames")
				}
				changed := 0
				for i := 0; i < len(x.Pixels); i += 4 {
					if x.Pixels[i] != y.Pixels[i] || x.Pixels[i+1] != y.Pixels[i+1] ||
						x.Pixels[i+2] != y.Pixels[i+2] || x.Pixels[i+3] != y.Pixels[i+3] {
						changed++
					}
				}
				return changed
			}
			visual := entry.Demo.Actions[0].Visual
			if changed, resetDiff := compare(before, after), compare(before, restored); changed < visual.MinChangedPixels || resetDiff > visual.MaxResetDifferencePixels {
				t.Fatalf("%s: Game View pixels changed=%d reset difference=%d", target, changed, resetDiff)
			}
			t.Logf("%s: delayed save/load, observables and Game View passed", target)
		}()
	}
}

func TestNativePersistContractPreservesGate(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := p.Docs().Get("save.Persist")
	if !ok || entry.Demo == nil || len(entry.Demo.Actions) != 1 || entry.Demo.Actions[0].Visual == nil ||
		len(entry.Demo.Actions[0].Expected) != 2 || entry.Demo.Actions[0].Expected[1].Observable != "save.Persist.PositionX" {
		t.Fatal("Persist component contract is absent")
	}
	var component democheck.Component
	for _, candidate := range p.Docs().ComponentDemos() {
		if candidate.ID == entry.ID {
			component = candidate
			break
		}
	}
	if component.ID == "" {
		t.Fatal("Persist component is absent from the registry")
	}
	run := func(contract democheck.Contract) democheck.Result {
		t.Helper()
		report := democheck.Check(context.Background(), []democheck.Component{component}, []democheck.Contract{contract}, &nativeTryRunner{p: p})
		if report.Passed() || len(report.Results) != 1 {
			t.Fatalf("native alone must not satisfy the browser gates: %+v", report)
		}
		return report.Results[0]
	}
	actual := run(*entry.Demo)
	if !slices.Contains(actual.Environments, "native") {
		t.Fatalf("native Persist contract failed: %+v", actual)
	}
	wrong := *entry.Demo
	wrong.Actions = slices.Clone(wrong.Actions)
	wrong.Actions[0].Expected = slices.Clone(wrong.Actions[0].Expected)
	wrong.Actions[0].Expected[1].Equals = json.RawMessage("-999")
	badObservation := run(wrong)
	if slices.Contains(badObservation.Environments, "native") || !strings.Contains(strings.Join(badObservation.Errors, " "), "unexpected observable") {
		t.Fatalf("native gate accepted an incorrect position: %+v", badObservation)
	}
	wrong = *entry.Demo
	wrong.Actions = slices.Clone(wrong.Actions)
	visual := *wrong.Actions[0].Visual
	visual.MinChangedPixels = 1 << 30
	wrong.Actions[0].Visual = &visual
	badFrame := run(wrong)
	if slices.Contains(badFrame.Environments, "native") || !strings.Contains(strings.Join(badFrame.Errors, " "), "Game View changed") {
		t.Fatalf("native gate accepted an insufficient visual change: %+v", badFrame)
	}
}
