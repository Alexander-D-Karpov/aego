//go:build aego_editor

package docsview

import (
	"encoding/json"
	"runtime"
	"sync"
	"testing"
	"time"

	"aego/app/playground"
	"aego/contracts/persistence"
	"aego/engine"
	"aego/modules/savegame"
	"aego/save"
)

func loadPersistTry(t *testing.T) *App {
	t.Helper()
	p, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := p.Docs().Get("save.Persist")
	if !ok || e.Demo == nil {
		t.Fatal("Persist contract is absent")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	a := newHeadless(t)
	req, err := json.Marshal(map[string]any{"scene": e.Try, "select": e.TrySelect, "code": code, "prefabs": e.TryPrefabs, "artifacts": e.TryArtifacts})
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Call("load", req); r.Error != "" {
		t.Fatal(r.Error)
	}
	return a
}

// The save is encoded on a worker, while the load and OnLoad callback run on
// the main thread. A fixed number of headless frames is not a completion signal.
func checkPersistCycle(t *testing.T, a *App, want float64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var logs []Line
	saved, loaded, cleaned := false, false, false
	for time.Now().Before(deadline) {
		if err := a.Step(4); err != nil {
			t.Fatal(err)
		}
		for _, line := range a.Call("log", nil).Log {
			if len(logs) < 256 {
				logs = append(logs, line)
			}
			if line.Error {
				t.Fatalf("save Try error: %s", line.Text)
			}
			saved = saved || line.Text == "game saved"
			loaded = loaded || line.Text == "game loaded"
		}
		s := savegame.Of(a.session.Controller().World())
		cleaned = loaded && s != nil && !s.Exists("persist-demo")
		if saved && loaded && cleaned {
			break
		}
		runtime.Gosched()
	}
	if !saved || !loaded || !cleaned {
		t.Fatalf("real save/load did not complete: saved=%v loaded=%v slot removed=%v logs=%+v", saved, loaded, cleaned, logs)
	}
	sc := a.session.Controller().Scene()
	h := sc.Find(sc.Root(), "Saved target")
	if !sc.IsAlive(h) {
		t.Fatal("loaded target is missing")
	}
	if got := float64(sc.WorldPosition(h).X); got != want {
		t.Fatalf("loaded target position=%v, want %v", got, want)
	}
	r := a.Call("inspect", []byte(`{"component":"save.Persist","fields":["save.Persist.PositionX"]}`))
	if r.Error != "" || r.Inspection == nil || r.Inspection.Observables["save.Persist.PositionX"] != want {
		t.Fatalf("loaded position is not observable: %+v", r)
	}
}

func TestPersistTryActuallySavesLoadsAndResets(t *testing.T) {
	a := loadPersistTry(t)
	checkPersistCycle(t, a, 2)
	if r := a.Call("set", []byte(`{"component":"save.Persist","field":"NoTransform","value":1}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	checkPersistCycle(t, a, -2)
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	checkPersistCycle(t, a, 2)
}

func TestPersistTryWaitsForDelayedSaveWorker(t *testing.T) {
	a := loadPersistTry(t)
	checkPersistCycle(t, a, 2)
	if r := a.Call("set", []byte(`{"component":"save.Persist","field":"NoTransform","value":1}`)); r.Error != "" {
		t.Fatal(r.Error)
	}
	jobs := a.e.Jobs()
	entered := make(chan struct{}, jobs.Workers())
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release) // Release workers even when an assertion aborts the test.
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
	if err := a.Step(24); err != nil {
		t.Fatal(err)
	}
	sc := a.session.Controller().Scene()
	h := sc.Find(sc.Root(), "Saved target")
	s := savegame.Of(a.session.Controller().World())
	if !sc.IsAlive(h) || sc.WorldPosition(h).X != 2 || s == nil || s.Exists("persist-demo") {
		t.Fatal("fixture did not hold the save beyond the original 20-frame wait")
	}
	for _, line := range a.lines {
		if line.Text == "game saved" || line.Text == "game loaded" {
			t.Fatal("save/load finished while its worker was held")
		}
	}
	release()
	checkPersistCycle(t, a, -2)
}

// Reset must not let a pending load from the old play world apply its save to
// the freshly reset scene, even if the worker completed between two frames.
func TestPersistTryResetBeforeQueuedLoad(t *testing.T) {
	a := loadPersistTry(t)
	jobs := a.e.Jobs()
	entered := make(chan struct{}, jobs.Workers())
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	for range jobs.Workers() {
		jobs.Submit(func() { entered <- struct{}{}; <-gate })
	}
	for range jobs.Workers() {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("could not hold the save worker")
		}
	}
	if err := a.Step(4); err != nil {
		t.Fatal(err)
	}
	store := savegame.Of(a.session.Controller().World())
	if store == nil || store.Exists("persist-demo") {
		t.Fatal("save was not held")
	}
	release()
	deadline := time.Now().Add(10 * time.Second)
	for !store.Exists("persist-demo") && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if !store.Exists("persist-demo") {
		t.Fatal("save did not complete")
	}
	if err := a.Step(1); err != nil {
		t.Fatal(err)
	}
	if r := a.Call("reset", nil); r.Error != "" {
		t.Fatal(r.Error)
	}
	sc := a.session.Controller().Scene()
	h := sc.Find(sc.Root(), "Saved target")
	if !sc.IsAlive(h) || sc.WorldPosition(h).X != -2 {
		t.Fatalf("Reset restored stale save instead of the original scene: position=%v", sc.WorldPosition(h))
	}
	checkPersistCycle(t, a, 2)
}

func TestPersistTryDelayedSaveIsIsolatedAfterRestart(t *testing.T) {
	for _, tc := range []struct {
		name string
		call string
		args []byte
		want float64
	}{
		{name: "reset", call: "reset", want: 2},
		{name: "field change", call: "set", args: []byte(`{"component":"save.Persist","field":"NoTransform","value":1}`), want: -2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := loadPersistTry(t)
			jobs := a.e.Jobs()
			entered := make(chan struct{}, jobs.Workers())
			gate := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(gate) }) }
			t.Cleanup(release)
			for range jobs.Workers() {
				jobs.Submit(func() { entered <- struct{}{}; <-gate })
			}
			for range jobs.Workers() {
				select {
				case <-entered:
				case <-time.After(10 * time.Second):
					t.Fatal("could not hold the save worker")
				}
			}
			old, ok := engine.Service[*save.Store](a.e, persistence.StoreID)
			if !ok || old == nil {
				t.Fatal("old memory store is unavailable")
			}
			if err := a.Step(4); err != nil {
				t.Fatal(err)
			}
			if old.Exists("persist-demo") {
				t.Fatal("save was not held")
			}
			if tc.call == "set" {
				// Keep the replacement world paused until the old write completes,
				// so its own legitimate first save cannot obscure the assertion.
				if r := a.Call("play", nil); r.Error != "" {
					t.Fatal(r.Error)
				}
			}
			if r := a.Call(tc.call, tc.args); r.Error != "" {
				t.Fatal(r.Error)
			}
			if tc.call == "set" {
				// Field edits are applied at the next editor tick.
				if err := a.Step(1); err != nil {
					t.Fatal(err)
				}
			}
			current, ok := engine.Service[*save.Store](a.e, persistence.StoreID)
			if !ok || current == nil || current == old {
				t.Fatal("restart did not create a separate memory store")
			}
			release()
			deadline := time.Now().Add(10 * time.Second)
			for !old.Exists("persist-demo") && time.Now().Before(deadline) {
				runtime.Gosched()
			}
			if oldSaved, newSaved := old.Exists("persist-demo"), current.Exists("persist-demo"); !oldSaved || newSaved {
				t.Fatalf("old worker crossed stores: old=%v current=%v", oldSaved, newSaved)
			}
			if tc.call == "set" {
				if r := a.Call("play", nil); r.Error != "" {
					t.Fatal(r.Error)
				}
			}
			checkPersistCycle(t, a, tc.want)
		})
	}
}
