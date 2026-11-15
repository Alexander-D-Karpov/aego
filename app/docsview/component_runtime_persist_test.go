//go:build aego_editor

package docsview

import (
	"sync"
	"testing"
	"time"

	"aego/modules/savegame"
)

func TestPersistCycleEventsRequireBothCompletionsAndCleanup(t *testing.T) {
	for _, order := range [][]string{{"game saved", "game loaded"}, {"game loaded", "game saved"}} {
		t.Run(order[0], func(t *testing.T) {
			var events persistCycleEvents
			if events.complete(true) {
				t.Fatal("accepted no completion events")
			}
			events.record([]Line{{Text: order[0]}})
			if events.complete(true) {
				t.Fatal("accepted only one completion event")
			}
			events.record([]Line{{Text: order[1]}})
			if events.complete(false) || !events.complete(true) {
				t.Fatal("completion must also require deletion of the temporary slot")
			}
			events = persistCycleEvents{}
			if events.complete(true) {
				t.Fatal("completion events leaked into the next cycle")
			}
		})
	}
}

func TestComponentContractPersistWaitsForDelayedWorker(t *testing.T) {
	for _, target := range []string{"initial reset", "action", "final reset"} {
		t.Run(target, func(t *testing.T) {
			resets, held, waited := 0, false, false
			var release func()
			hooks := componentRuntimeHooks{only: "save.Persist"}
			hooks.beforeCycle = func(t *testing.T, a *App, phase string) {
				if phase == "reset" {
					resets++
				}
				selected := target == "action" && phase == "action" ||
					target == "initial reset" && phase == "reset" && resets == 1 ||
					target == "final reset" && phase == "reset" && resets == 2
				if !selected {
					return
				}
				jobs := a.e.Jobs()
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
			hooks.onWait = func(t *testing.T, a *App, phase string, events persistCycleEvents) {
				if !held || waited {
					return
				}
				if target == "action" && phase != "action" ||
					target == "initial reset" && (phase != "reset" || resets != 1) ||
					target == "final reset" && (phase != "reset" || resets != 2) {
					t.Fatalf("%s continued into %s before waiting", target, phase)
				}
				world := a.session.Controller().World()
				if world == nil {
					t.Fatal("Try world is not running")
				}
				service := savegame.Of(world)
				reply := a.Call("inspect", []byte(`{"component":"save.Persist","fields":["save.Persist.PositionX"]}`))
				if events.saved || events.loaded || service == nil || service.Exists("persist-demo") ||
					reply.Error != "" || reply.Inspection == nil || reply.Inspection.Observables["save.Persist.PositionX"] != float64(2) {
					t.Fatalf("worker delay did not hold the four-frame state: events=%+v inspection=%+v", events, reply)
				}
				waited = true
				release()
			}
			runComponentContractRuntimeObservables(t, hooks)
			if !held || !waited {
				t.Fatalf("%s did not wait for the delayed worker: held=%v waited=%v", target, held, waited)
			}
		})
	}
}
