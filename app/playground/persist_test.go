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
		name, scene string
		wantX       float32
	}{
		{"saved transform", e.Try, 2},
		{"scene transform", strings.Replace(e.Try, "noTransform=false", "noTransform=true", 1), -2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "scene transform" && tc.scene == e.Try {
				t.Fatal("fixture did not change the Persist flag")
			}
			src := []byte(tc.scene)
			if r := p.PlayScene(e.ID, src, code, e.TryPrefabs, e.TryArtifacts); !r.Started {
				t.Fatalf("could not start Persist Try: %+v", r)
			}
			// Reload must retain its own source, not the caller's mutable buffer.
			clear(src)
			loaded, saved := false, false
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) && !loaded {
				if err := p.Game().Step(4); err != nil {
					t.Fatal(err)
				}
				for _, line := range p.Poll().Log {
					if line.Level == "ERROR" {
						t.Fatalf("save/load runtime error: %s", line.Text)
					}
					saved = saved || strings.Contains(line.Text, "game saved")
					loaded = loaded || strings.Contains(line.Text, "game loaded")
				}
				runtime.Gosched()
			}
			if !saved || !loaded {
				t.Fatal("actual save and load did not finish")
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
