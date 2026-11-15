package playground

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/contracts/persistence"
	"aego/core/ids"
	"aego/engine"
	"aego/modules/savegame"
	"aego/save"
)

func TestPlaygroundPersistTryReloadsItsOwnScene(t *testing.T) {
	p := newHeadless(t)
	e, ok := p.Docs().Get("save.Persist")
	if !ok || e.Try == "" || e.TryScript == "" {
		t.Fatal("Persist Try is absent")
	}
	code, err := p.TryCode(e.TryScript)
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.host()
	if err != nil {
		t.Fatal(err)
	}
	host, ok := engine.ServiceOf[persistence.Host](g.Engine().MainWorld(), persistence.HostID)
	if !ok || !host.SceneID().IsZero() || host.LoadScene(ids.AssetID{}) == nil {
		t.Fatal("playground host must reject reloading when no scene is running")
	}
	for _, tc := range []struct {
		name, scene   string
		wantX         float32
		delaySaveDone bool
	}{
		{"saved transform", e.Try, 2, false},
		{"scene transform", strings.Replace(e.Try, "noTransform=false", "noTransform=true", 1), -2, false},
		{"save callback after load", e.Try, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "scene transform" && tc.scene == e.Try {
				t.Fatal("fixture did not change the Persist flag")
			}
			var delayed *delayedSaveFS
			if tc.delaySaveDone {
				delayed = newDelayedSaveFS()
				t.Cleanup(delayed.release)
				g.Engine().SetService(persistence.StoreID, save.NewStoreFS("saves", delayed))
			}
			src := []byte(tc.scene)
			if r := p.PlayScene(e.ID, src, code, e.TryPrefabs, e.TryArtifacts); !r.Started {
				t.Fatalf("could not start Persist Try: %+v", r)
			}
			// Reload must retain its own source, not the caller's mutable buffer.
			clear(src)
			loaded, saved := false, false
			deadline := time.Now().Add(10 * time.Second)
			// The save file can become visible before the worker queues its
			// completion callback. Loading may therefore finish first; require
			// both real completion notices instead of stopping on either one.
			for time.Now().Before(deadline) && (!saved || !loaded) {
				if err := p.Game().Step(4); err != nil {
					t.Fatal(err)
				}
				poll := p.Poll()
				if len(poll.Faults) != 0 {
					t.Fatalf("save/load script faults: %+v", poll.Faults)
				}
				for _, line := range poll.Log {
					if line.Level == "ERROR" {
						t.Fatalf("save/load runtime error: %s", line.Text)
					}
					saved = saved || strings.Contains(line.Text, "game saved")
					loaded = loaded || strings.Contains(line.Text, "game loaded")
				}
				if loaded && delayed != nil {
					if saved {
						t.Fatal("the save callback ran before the deliberately delayed load")
					}
					select {
					case <-delayed.entered:
					default:
						t.Fatal("save worker did not reach the delay")
					}
					delayed.release()
					delayed = nil
				}
				runtime.Gosched()
			}
			if !saved || !loaded {
				t.Fatalf("actual save and load did not finish: saved=%v loaded=%v", saved, loaded)
			}
			sc := p.Game().Scene()
			h := sc.Find(sc.Root(), "Saved target")
			if !sc.IsAlive(h) || sc.WorldPosition(h).X != tc.wantX {
				t.Fatalf("loaded position=%v want %v", sc.WorldPosition(h), tc.wantX)
			}
			service := savegame.Of(p.Game().Engine().MainWorld())
			if service == nil || service.Exists("persist-demo") {
				t.Fatal("temporary in-memory slot was not removed on load")
			}
			before := p.Game().Scene()
			if host.LoadScene(ids.AssetID{Lo: 1}) == nil || p.Game().Scene() != before {
				t.Fatal("playground must reject a different scene ID without changing the running scene")
			}
		})
	}
}
