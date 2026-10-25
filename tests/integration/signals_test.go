package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/sdk"
)

func patch(t *testing.T, path string, pairs ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for i := 0; i+1 < len(pairs); i += 2 {
		if !strings.Contains(text, pairs[i]) {
			t.Fatalf("%s does not contain %q", path, pairs[i])
		}
		text = strings.Replace(text, pairs[i], pairs[i+1], 1)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestComponentSignalReachesScriptAndGo(t *testing.T) {
	root := copyExample(t, "platformer")
	patch(t, filepath.Join(root, "scripts", "player", "player.aes"),
		"\tvar facing float = 1\n", "\tvar facing float = 1\n\tvar hitConn Connection\n\tvar signalHits int\n",
		"\t\twatch(EnemyB)\n", "\t\twatch(EnemyB)\n\t\thitConn = node.Get<Health>().Healed.Connect(OnHit)\n",
		"\tfunc OnEnemyDied() {", "\tfunc OnHit(hits int) {\n\t\tsignalHits = hits\n\t}\n\n\tfunc OnEnemyDied() {")
	pf := platformerAt(t, root)
	pf.step(1)
	var goHits int64
	ctx := &sdk.Context{World: pf.app.Engine.MainWorld(), Engine: pf.app.Engine}
	if _, err := sdk.Connect1[int64](ctx, pf.player, "Health.Healed", func(n int64) { goHits = n }); err != nil {
		t.Fatal(err)
	}
	pf.step(29)
	pf.hitPlayer(t)
	pf.health().Heal(1)
	pf.step(1)
	hits := int64(1)
	if got := pf.field("signalHits").Int(); got != hits {
		t.Fatalf("script handler saw %d healed, want %d", got, hits)
	}
	if goHits != hits {
		t.Fatalf("Go handler saw %d healed, want %d", goHits, hits)
	}
	if !pf.app.Scripts.SignalConnected(pf.field("hitConn")) {
		t.Fatal("the script connection must stay connected")
	}
	pf.noFaults(t)
}

func TestTagsAtRuntime(t *testing.T) {
	pf := openPlatformer(t)
	sc := pf.app.Scene
	if !sc.AddTag(pf.enemy, "Enemy.Walker") || !sc.AddTag(pf.player, "Player") {
		t.Fatal("the engine.Tags component is not registered")
	}
	pf.step(1)
	out := make([]sdk.NodeHandle, 4)
	if n := sc.WithTag("Enemy", out); n != 1 || out[0] != pf.enemy {
		t.Fatalf("WithTag Enemy: %d", n)
	}
	if n := testing.AllocsPerRun(50, func() { sc.WithTag("Enemy", out) }); n != 0 {
		t.Fatalf("WithTag allocates %v", n)
	}
	pf.noFaults(t)
}
