package integration

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"aego/app/run"
	"aego/contracts/physics2d"
	"aego/engine"
	"aego/modules/gameplay"
	"aego/platform"
	"aego/platform/input"
	"aego/scene"
)

// TestTryScenesNeverTrapThePlayer mashes keys in every docs try scene with a Motion 2D player and checks it never ends up inside a collider or unable to move.
func TestTryScenesNeverTrapThePlayer(t *testing.T) {
	scenes, _ := filepath.Glob(filepath.Join("..", "..", "modules", "gameplay", "res", "docs", "try", "*.aescene"))
	for _, path := range scenes {
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), `"gameplay.Motion2D"`) {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), ".aescene")
		t.Run(name, func(t *testing.T) {
			for seed := int64(1); seed <= 12; seed++ {
				mash(t, openAt(t, tryProject(t, path)), "Player", platformerKeys, seed)
			}
		})
	}
}

// TestGuideScenesNeverTrapThePlayer does the same for every scene of the platformer guide and the recipes, with the scripts written above it.
func TestGuideScenesNeverTrapThePlayer(t *testing.T) {
	for _, page := range []string{"platformer", "recipes"} {
		md, err := os.ReadFile(filepath.Join("..", "..", "engine", "res", "docs", page+".md"))
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{}
		for i, b := range fences(string(md)) {
			switch b[0] {
			case "aes check":
				m := scriptName.FindStringSubmatch(b[1])
				files[m[1]] = b[1]
			case "aescene check":
				if len(files) == 0 {
					continue
				}
				t.Run(fmt.Sprintf("%s/%d", page, i), func(t *testing.T) {
					root := t.TempDir()
					writeProject(t, root, b[1], files)
					for seed := int64(1); seed <= 4; seed++ {
						mash(t, openAt(t, root), "Player", platformerKeys, seed)
					}
				})
			}
		}
	}
}

var scriptName = regexp.MustCompile(`(?m)^script (\w+)`)

// fences are the fenced blocks of a page as info string and code.
func fences(md string) [][2]string {
	var out [][2]string
	parts := strings.Split(md, "```")
	for i := 1; i+1 < len(parts); i += 2 {
		info, code, _ := strings.Cut(parts[i], "\n")
		out = append(out, [2]string{strings.TrimSpace(info), code})
	}
	return out
}

func writeProject(t *testing.T, root, scene string, scripts map[string]string) {
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "project.aego"), []byte("project version=1 {\n  module=\"game\"\n}\n"), 0o644))
	must(os.MkdirAll(filepath.Join(root, "scenes"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "scripts", "game"), 0o755))
	must(os.WriteFile(filepath.Join(root, "scenes", "main.aescene"), []byte(scene), 0o644))
	for name, src := range scripts {
		must(os.WriteFile(filepath.Join(root, "scripts", "game", name+".aes"), []byte(src), 0o644))
	}
}

func tryProject(t *testing.T, scenePath string) string {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "project.aego"), []byte("project version=1 {\n  module=\"game\"\n}\n"), 0o644))
	must(os.MkdirAll(filepath.Join(root, "scenes"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "scripts", "demo"), 0o755))
	sc, err := os.ReadFile(scenePath)
	must(err)
	must(os.WriteFile(filepath.Join(root, "scenes", "main.aescene"), sc, 0o644))
	src, err := os.ReadFile(strings.TrimSuffix(scenePath, ".aescene") + ".aes")
	must(err)
	must(os.WriteFile(filepath.Join(root, "scripts", "demo", "try.aes"), src, 0o644))
	return root
}

// TestExamplesNeverTrapThePlayer does the same in the platformer and top-down examples, enemies, pits and checkpoints included.
func TestExamplesNeverTrapThePlayer(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		pf := openPlatformer(t)
		mash(t, pf.app, "Player", append(platformerKeys, input.KeyJ), seed)
		app, _ := openExample(t, "topdown")
		mash(t, app, "Hero", append(topDownKeys, input.KeyJ), seed)
	}
}

var (
	platformerKeys = []input.Scancode{input.KeyLeft, input.KeyRight, input.KeySpace, input.KeySpace, input.KeyUp}
	topDownKeys    = []input.Scancode{input.KeyLeft, input.KeyRight, input.KeyUp, input.KeyDown}
)

func mash(t *testing.T, app *run.App, name string, keys []input.Scancode, seed int64) {
	t.Helper()
	sc := app.Scene
	player := sc.Find(sc.Root(), name)
	if player.IsZero() {
		t.Fatal("no Player node")
	}
	q, _ := engine.ServiceOf[physics2d.Queries](app.Engine.MainWorld(), physics2d.ServiceID)
	d := app.Driver()
	key := func(k input.Scancode, down bool) {
		kind := platform.EvKeyUp
		if down {
			kind = platform.EvKeyDown
		}
		platform.ApplyInput(app.Engine.Input(), platform.Event{Kind: kind, Key: input.KeyEvent{Scan: k}})
	}
	step := func(n int) {
		for i := 0; i < n; i++ {
			app.Engine.Step(d)
		}
	}
	m, _ := scene.Get[gameplay.Motion2D](sc, player)
	f := physics2d.QueryFilter{LayerMask: uint32(m.CollisionMask), ExcludeBody: player}
	hits := make([]physics2d.CastHit, 8)
	rng := rand.New(rand.NewSource(seed))
	held := map[input.Scancode]bool{}
	inside, prevInside := 0, 0
	for frame := 0; frame < 1800; frame++ {
		if frame%6 == 0 {
			k := keys[rng.Intn(len(keys))]
			held[k] = !held[k]
			key(k, held[k])
		}
		step(1)
		deep := false
		for _, h := range hits[:q.Penetration(player, f, hits)] {
			// a kinematic platform may press the player into the floor; walking out is checked below
			deep = deep || h.Depth > 0.05 && sc.Name(h.Node) != "Lift"
		}
		if inside = 0; deep {
			inside = prevInside + 1
		}
		if prevInside = inside; inside > 10 {
			t.Fatalf("seed %d frame %d: player %v stays inside a collider", seed, frame, sc.WorldPosition(player))
		}
		if frame%300 != 299 {
			continue
		}
		for k := range held {
			key(k, false)
			held[k] = false
		}
		if hp, ok := scene.Get[gameplay.Health](sc, player); ok && hp.Dead {
			step(120)
		}
		if !canMove(sc, player, key, step, slices.Contains(keys, input.KeyDown)) {
			t.Fatalf("seed %d frame %d: player stuck at %v", seed, frame, sc.WorldPosition(player))
		}
	}
}

func canMove(sc *scene.Scene, player scene.NodeHandle, key func(input.Scancode, bool), step func(int), vertical bool) bool {
	dirs := []input.Scancode{input.KeyLeft, input.KeyRight}
	if vertical {
		dirs = append(dirs, input.KeyUp, input.KeyDown)
	}
	for _, k := range dirs {
		p := sc.WorldPosition(player)
		key(k, true)
		step(20)
		key(k, false)
		if q := sc.WorldPosition(player); q.Sub(p).Len() > 0.3 {
			return true
		}
	}
	return false
}
