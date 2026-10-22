package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/edit"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/prefabs"
	"aego/project"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/bind"
)

const (
	crateRel    = "prefabs/crate.aeprefab"
	sandboxMain = "scenes/main.aescene"
	enemyGUID   = "7a1e0c5b3d9f4a2e8c6b1d0f9e8a7c02"
	fastGUID    = "7a1e0c5b3d9f4a2e8c6b1d0f9e8a7c03"
)

func copyProject(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == ".aego" || rel == "exports" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func openProject(t *testing.T, root string) *context.Editor {
	t.Helper()
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	return s.App.Context()
}

func mustHave(t *testing.T, text string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Fatalf("missing %q in:\n%s", w, text)
		}
	}
}

func spriteTint(t *testing.T, sc *scene.Scene, name string) (scene.NodeHandle, *schema.TypeDescriptor, *schema.FieldDescriptor) {
	t.Helper()
	h := sc.Find(sc.Root(), name)
	if h.IsZero() {
		t.Fatalf("%s is not in the scene", name)
	}
	td, ok := sc.Registry().ByName("engine.SpriteRenderer")
	if !ok {
		t.Fatal("sprite renderer is not registered")
	}
	fd, _ := td.Field("tint")
	return h, td, fd
}

func tintOf(t *testing.T, sc *scene.Scene, name string) uint32 {
	t.Helper()
	h, td, fd := spriteTint(t, sc, name)
	v, err := sc.GetProperty(h, td.ID, fd.ID)
	if err != nil {
		t.Fatal(err)
	}
	return prefab.ColorKey(v.C)
}

func setTint(t *testing.T, d *document.SceneDocument, name string, c mathx.Color) {
	t.Helper()
	sc := d.Scene()
	h, td, fd := spriteTint(t, sc, name)
	if err := d.Props.Set(nil, []ids.NodeID{sc.ID(h)}, td.ID, fd.ID, schema.Color(c)); err != nil {
		t.Fatal(err)
	}
}

func hasComponent(sc *scene.Scene, h scene.NodeHandle, name string) bool {
	td, ok := sc.Registry().ByName(name)
	if !ok {
		return false
	}
	_, has := sc.GetByType(h, td.ID)
	return has
}

func TestSandboxCratePrefab(t *testing.T) {
	root := copyProject(t, filepath.Join("..", "..", "examples", "physics-sandbox"))
	ctx := openProject(t, root)
	d, err := ctx.Docs.Open(sandboxMain)
	if err != nil {
		t.Fatal(err)
	}
	sc := d.Scene()
	disk, err := os.ReadFile(filepath.Join(root, sandboxMain))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(sceneio.Save(sc)); got != string(disk) {
		t.Fatalf("scene with prefab instances is not byte-stable:\n%s", got)
	}
	crateID, ok := ctx.Pipeline.Lookup(crateRel)
	if !ok {
		t.Fatal("crate prefab is not imported")
	}
	for _, n := range []string{"Crate0", "Crate1", "Crate2"} {
		h := sc.Find(sc.Root(), n)
		pi, ok := sceneio.InstanceOf(sc, h)
		if !ok || !pi.Owner || pi.Prefab != crateID || !hasComponent(sc, h, "physics.Body2D") {
			t.Fatalf("%s must be an expanded crate instance", n)
		}
	}
	readCrate := func() string {
		data, err := os.ReadFile(filepath.Join(root, crateRel))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	svc := prefabs.New(ctx)
	red, blue := mathx.Color{R: 1, A: 1}, mathx.Color{B: 1, A: 1}
	original := tintOf(t, sc, "Crate1")
	setTint(t, d, "Crate0", red)
	mustHave(t, string(sceneio.Save(sc)), `prop="engine.SpriteRenderer.tint"`)
	tintProp := prefab.PropKey{Owner: "engine.SpriteRenderer", Field: "tint"}
	c0 := sc.ID(sc.Find(sc.Root(), "Crate0"))
	if err := svc.Apply(d, prefabs.Change{Node: c0, Level: prefabs.LevelProperty, Prop: tintProp}); err != nil {
		t.Fatal(err)
	}
	if tintOf(t, sc, "Crate1") != prefab.ColorKey(red) || !strings.Contains(readCrate(), "tint=#ff0000ff") {
		t.Fatal("apply must reach the prefab file and the other crates")
	}
	if strings.Contains(string(sceneio.Save(sc)), `prop="engine.SpriteRenderer.tint"`) {
		t.Fatal("an applied value is no longer an override")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if tintOf(t, sc, "Crate1") != original || tintOf(t, sc, "Crate0") != prefab.ColorKey(red) || strings.Contains(readCrate(), "#ff0000ff") {
		t.Fatal("undo must restore the file and the other crates and keep the source override")
	}
	if err := d.History.Redo(); err != nil {
		t.Fatal(err)
	}
	setTint(t, d, "Crate1", blue)
	c1 := sc.ID(sc.Find(sc.Root(), "Crate1"))
	if err := svc.Revert(d, prefabs.Change{Node: c1, Level: prefabs.LevelProperty, Prop: tintProp}); err != nil {
		t.Fatal(err)
	}
	if tintOf(t, sc, "Crate1") != prefab.ColorKey(red) {
		t.Fatal("revert must return the prefab value")
	}
	tmpl, err := sceneio.NewLibrary(sc.Registry(), ctx.Loader).Template(crateID)
	if err != nil {
		t.Fatal(err)
	}
	spawned, err := sceneio.Instantiate(sc, scene.NodeHandle{}, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	sc.Flush()
	if !hasComponent(sc, spawned, "physics.Body2D") {
		t.Fatal("template copy must carry the physics body")
	}
	if r, ok := sceneio.PrefabRoot(sc, spawned); !ok || r != spawned {
		t.Fatal("template copy must know its prefab root")
	}
	sc.Destroy(spawned)
	sc.Flush()
	c2 := sc.ID(sc.Find(sc.Root(), "Crate2"))
	if err := svc.Unpack(d, c2, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := sceneio.InstanceOf(sc, sc.Handle(c2)); ok {
		t.Fatal("unpack must drop the link")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if pi, ok := sceneio.InstanceOf(sc, sc.Handle(c2)); !ok || !pi.Owner {
		t.Fatal("undo of unpack must restore the instance")
	}
	if err := svc.CreateVariant(crateID); err != nil {
		t.Fatal(err)
	}
	if _, ok := ctx.Pipeline.Lookup("prefabs/crate Variant.aeprefab"); !ok {
		t.Fatal("variant was not created")
	}
	if err := svc.Open(crateID); err != nil {
		t.Fatal(err)
	}
	pd := ctx.Docs.Find(crateRel)
	if pd == nil {
		t.Fatal("prefab document did not open")
	}
	green := mathx.Color{G: 1, A: 1}
	setTint(t, pd, "Crate", green)
	if err := svc.SaveDocument(pd); err != nil {
		t.Fatal(err)
	}
	if tintOf(t, sc, "Crate2") != prefab.ColorKey(green) || pd.Dirty() {
		t.Fatal("saving the prefab document must rebuild the instances")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if tintOf(t, sc, "Crate2") != prefab.ColorKey(red) {
		t.Fatal("undo in the scene must roll back the prefab document save")
	}
	if err := ctx.Docs.Save(d.ID); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Docs.Close(d.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Docs.Reload(pd.ID); err != nil {
		t.Fatal(err)
	}
	white := mathx.Color{R: 1, G: 1, B: 1, A: 1}
	setTint(t, pd, "Crate", white)
	if err := svc.SaveDocument(pd); err != nil {
		t.Fatal(err)
	}
	again, err := ctx.Docs.Open(sandboxMain)
	if err != nil {
		t.Fatal(err)
	}
	if tintOf(t, again.Scene(), "Crate2") != prefab.ColorKey(white) {
		t.Fatal("a closed scene must pick up the prefab change when it opens")
	}
}

func TestPlatformerEnemyPrefabs(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	reg := types.Schema()
	dir := filepath.Join("..", "..", "examples", "platformer")
	lib := sceneio.NewLibrary(reg, nil)
	for rel, guid := range map[string]string{"prefabs/enemy.aeprefab": enemyGUID, "prefabs/fast_enemy.aeprefab": fastGUID} {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		p, err := sceneio.ParsePrefab(data, rel, reg)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := ids.ParseAssetID(guid)
		lib.Set(id, p)
	}
	src, err := os.ReadFile(filepath.Join(dir, "scenes", "main.aescene"))
	if err != nil {
		t.Fatal(err)
	}
	s, rep, err := sceneio.Load(src, "main.aescene", types, nil, sceneio.LoadOptions{KeepIDs: true, Prefabs: lib})
	if err != nil || len(rep.Missing) != 0 {
		t.Fatalf("load: %v %+v", err, rep.Missing)
	}
	for name, x := range map[string]float32{"EnemyLeft": -8, "EnemyRight": 8} {
		h := s.Find(s.Root(), name)
		pi, ok := sceneio.InstanceOf(s, h)
		if !ok || !pi.Owner || s.Local(h).Position.X != x {
			t.Fatalf("%s must be an instance at x=%v", name, x)
		}
		if !hasComponent(s, h, "physics.Body2D") || !hasComponent(s, h, "gameplay.Hurtbox2D") || !hasComponent(s, h, "gameplay.Hitbox2D") {
			t.Fatalf("%s lost its body, hurtbox or hitbox", name)
		}
	}
	out := string(sceneio.SaveWith(s, lib))
	mustHave(t, out, `prop="script/mygame.enemy.EnemyAI.PointA" value=node("c0000000000000000000000000000008")`, `prop="node.name" value="EnemyRight"`)
	back, _, err := sceneio.Load([]byte(out), "main.aescene", types, nil, sceneio.LoadOptions{KeepIDs: true, Prefabs: lib})
	if err != nil {
		t.Fatal(err)
	}
	if again := string(sceneio.SaveWith(back, lib)); again != out {
		t.Fatalf("platformer save is not stable:\n%s\n---\n%s", out, again)
	}
	fastID, _ := ids.ParseAssetID(fastGUID)
	tmpl, err := lib.Template(fastID)
	if err != nil {
		t.Fatal(err)
	}
	fast, err := sceneio.Instantiate(s, scene.NodeHandle{}, tmpl)
	if err != nil {
		t.Fatal(err)
	}
	s.Flush()
	if s.Name(fast) != "FastEnemy" || !hasComponent(s, fast, "physics.Body2D") {
		t.Fatal("variant must expand through its base prefab")
	}
}
