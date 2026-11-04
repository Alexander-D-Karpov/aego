package refactor_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"aego/app/projects"
	"aego/app/run"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/project"
	"aego/project/refactor"
	"aego/project/refs"
	"aego/project/rename"
)

const (
	builtinSprite = "ae900000000000000000000000000010"
	mainScene     = "scenes/main.aescene"
	enemyPrefab   = "prefabs/enemy.aeprefab"
	otherScene    = "scenes/other.aescene"
	brokenScene   = "scenes/broken.aescene"
	texA          = "assets/tex/a.png"
	texB          = "assets/tex/b.png"
	missingTarget = "c00000000000000000000000000000ff"
)

type fixture struct {
	t    *testing.T
	dir  string
	p    *project.Project
	pl   *pipeline.Pipeline
	a, b ids.AssetID
}

func copyExample(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", name)
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
			switch rel {
			case ".aego", "native", "exports":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if rel == "go.mod" || rel == "go.sum" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == "project.aego" {
			data = []byte(strings.Replace(string(data), "native=true", "native=false", 1))
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func pngBytes(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (f *fixture) abs(rel string) string {
	return filepath.Join(f.dir, filepath.FromSlash(rel))
}

func (f *fixture) read(rel string) []byte {
	f.t.Helper()
	data, err := os.ReadFile(f.abs(rel))
	if err != nil {
		f.t.Fatal(err)
	}
	return data
}

func (f *fixture) write(rel string, data []byte) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.abs(rel)), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.abs(rel), data, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) sync() {
	f.t.Helper()
	if _, _, err := f.pl.Scan(); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.pl.ImportAll(); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) id(rel string) ids.AssetID {
	f.t.Helper()
	id, ok := f.pl.Lookup(rel)
	if !ok {
		f.t.Fatalf("%s is not imported", rel)
	}
	return id
}

func (f *fixture) reimport(rels ...string) {
	f.t.Helper()
	if _, err := refactor.Reimport(f.pl, rels); err != nil {
		f.t.Fatal(err)
	}
}

func open(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, dir: copyExample(t, "platformer")}
	p, err := project.Load(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := projects.OpenWorkspace(p, "", false)
	if err != nil {
		t.Fatal(err)
	}
	imps, err := run.Importers(ws)
	if err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, imps, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	f.p, f.pl = p, pl
	f.write(texA, pngBytes(t, color.NRGBA{R: 255, A: 255}))
	f.write(texB, pngBytes(t, color.NRGBA{B: 255, A: 255}))
	f.write(brokenScene, []byte(`scene version=1 {
  node "c00000000000000000000000000000f1" name="Hook" enabled=true layers=1 {
    transform {
      position=[0, 0]
      rotation=0
      scale=[1, 1]
    }
    component "physics.DistanceJoint2D" version=1 {
      target=node("`+missingTarget+`")
    }
  }
}
`))
	f.sync()
	f.a, f.b = f.id(texA), f.id(texB)
	sprite, err := ids.ParseAssetID(builtinSprite)
	if err != nil {
		t.Fatal(err)
	}
	builtin := string(refs.Token(sprite))
	main := string(f.read(mainScene))
	f.write(otherScene, []byte(strings.ReplaceAll(main, builtin, string(refs.Token(f.b)))))
	f.write(mainScene, []byte(strings.ReplaceAll(main, builtin, string(refs.Token(f.a)))))
	// This fixture needs a prefab sprite reference, regardless of which art
	// the canonical platformer currently assigns to the enemy.
	prefab, err := aetext.Parse(f.read(enemyPrefab), enemyPrefab)
	if err != nil {
		t.Fatal(err)
	}
	node := prefab.Block("node", "")
	if node == nil || node.Block("component", "engine.SpriteRenderer") == nil {
		t.Fatal("enemy prefab root must have a SpriteRenderer")
	}
	node.Block("component", "engine.SpriteRenderer").SetField("sprite", aetext.AssetRef(f.a.String()))
	f.write(enemyPrefab, aetext.Write(prefab))
	f.sync()
	f.reimport(mainScene, enemyPrefab)
	return f
}

func paths(list []refs.Ref) []string {
	var out []string
	for _, r := range list {
		out = append(out, r.Path)
	}
	slices.Sort(out)
	return out
}

func TestReferrers(t *testing.T) {
	f := open(t)
	got := paths(refs.Referrers(f.p, f.pl, f.a))
	if !slices.Contains(got, mainScene) || !slices.Contains(got, enemyPrefab) {
		t.Fatalf("referrers of the texture: %v", got)
	}
	var deps []string
	for _, id := range f.pl.Dependents(f.a) {
		if rel, ok := f.pl.PathOf(id); ok {
			deps = append(deps, rel)
		}
	}
	slices.Sort(deps)
	if !slices.Equal(got, deps) {
		t.Fatalf("referrers %v, pipeline dependents %v", got, deps)
	}
	hits := refs.Find(f.p, f.pl, f.a)
	foundEmbers := false
	for _, h := range hits {
		if h.Err != nil || len(h.Places) == 0 {
			t.Fatalf("%s: %v, %d places", h.Ref.Path, h.Err, len(h.Places))
		}
		for _, pl := range h.Places {
			if pl.Field == "sprite" && pl.Owner == "engine.SpriteRenderer" {
				continue
			}
			if h.Ref.Path == mainScene && pl.Path == "StartLantern/Embers" && pl.Owner == "particles.ParticleSystem2D" && pl.Field == "sprite" {
				foundEmbers = true
				continue
			}
			t.Fatalf("%s: place %s", h.Ref.Path, pl)
		}
	}
	if !foundEmbers {
		t.Fatal("missing particle sprite reference at StartLantern/Embers")
	}
}

func snapshot(f *fixture, rels []string) map[string][]byte {
	out := map[string][]byte{}
	for _, rel := range rels {
		out[rel] = f.read(rel)
	}
	return out
}

func TestReplaceRoundTrip(t *testing.T) {
	f := open(t)
	plan, err := refactor.PlanReplace(f.p, f.pl, f.a, f.b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) < 2 {
		t.Fatalf("plan %+v", plan)
	}
	before := snapshot(f, plan.Files)
	backup := refactor.BackupDir(time.Unix(0, 0))
	changed, err := plan.Write(f.dir, f.pl.Abs, backup)
	if err != nil {
		t.Fatal(err)
	}
	f.reimport(changed...)
	if hits := refs.Find(f.p, f.pl, f.a); len(hits) != 0 {
		t.Fatalf("%d files still use the old texture", len(hits))
	}
	now := paths(refs.Referrers(f.p, f.pl, f.b))
	for _, rel := range plan.Files {
		if !slices.Contains(now, rel) {
			t.Fatalf("%s does not use the new texture: %v", rel, now)
		}
		bak, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(backup), filepath.FromSlash(rel)))
		if err != nil || !bytes.Equal(bak, before[rel]) {
			t.Fatalf("backup of %s: %v", rel, err)
		}
	}
	if !strings.HasPrefix(backup, refactor.RecoveryDir+"/refs-") {
		t.Fatalf("backup dir %s", backup)
	}
	back, err := refactor.PlanReplace(f.p, f.pl, f.b, f.a, func(rel string) bool { return !slices.Contains(plan.Files, rel) })
	if err != nil {
		t.Fatal(err)
	}
	changed, err = back.Write(f.dir, f.pl.Abs, "")
	if err != nil {
		t.Fatal(err)
	}
	f.reimport(changed...)
	for rel, data := range before {
		if !bytes.Equal(f.read(rel), data) {
			t.Fatalf("%s differs after the reverse replacement", rel)
		}
	}
}

func TestEditsWriteNothing(t *testing.T) {
	f := open(t)
	plan, err := refactor.PlanReplace(f.p, f.pl, f.a, f.b, nil)
	if err != nil {
		t.Fatal(err)
	}
	stamp := map[string]time.Time{}
	for _, rel := range plan.Files {
		st, err := os.Stat(f.abs(rel))
		if err != nil {
			t.Fatal(err)
		}
		stamp[rel] = st.ModTime()
	}
	edits, err := plan.Edits(f.pl.Abs)
	if err != nil || len(edits) != len(plan.Files) {
		t.Fatalf("%d edits for %d files: %v", len(edits), len(plan.Files), err)
	}
	for _, e := range edits {
		if bytes.Equal(e.Before, e.After) || !bytes.Contains(e.After, refs.Token(f.b)) {
			t.Fatalf("%s: the edit does not replace the token", e.Path)
		}
		st, _ := os.Stat(f.abs(e.Path))
		if !st.ModTime().Equal(stamp[e.Path]) {
			t.Fatalf("%s was written by Edits", e.Path)
		}
	}
}

func TestCheckReplace(t *testing.T) {
	f := open(t)
	if err := refactor.CheckReplace(f.pl, f.a, f.id(mainScene)); err == nil {
		t.Fatal("a texture was replaced with a scene")
	}
	if err := refactor.CheckReplace(f.pl, f.a, f.a); err == nil {
		t.Fatal("an asset replaced itself")
	}
	if err := refactor.CheckReplace(f.pl, f.a, f.b); err != nil {
		t.Fatal(err)
	}
}

func TestMissingAssets(t *testing.T) {
	f := open(t)
	if got := refs.MissingAssets(f.p, f.pl); len(got) != 0 {
		t.Fatalf("the fixture already misses %v", got)
	}
	data := f.read(texA)
	if _, err := refactor.Delete(f.pl, []ids.AssetID{f.a}); err != nil {
		t.Fatal(err)
	}
	f.sync()
	got := refs.MissingAssets(f.p, f.pl)
	if len(got) != 1 || got[0].ID != f.a || !got[0].Buried || got[0].Grave.Path != texA {
		t.Fatalf("missing %+v", got)
	}
	f.write(texA, data)
	f.sync()
	fresh := f.id(texA)
	got = refs.MissingAssets(f.p, f.pl)
	if len(got) != 1 || got[0].Suggest != fresh || got[0].Path != texA || fresh == f.a {
		t.Fatalf("suggestion %+v, new id %s", got, fresh)
	}
	plan, err := refactor.PlanReplace(f.p, f.pl, f.a, got[0].Suggest, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := plan.Write(f.dir, f.pl.Abs, "")
	if err != nil {
		t.Fatal(err)
	}
	f.reimport(changed...)
	if got := refs.MissingAssets(f.p, f.pl); len(got) != 0 {
		t.Fatalf("still missing %+v", got)
	}
}

func TestUsers(t *testing.T) {
	f := open(t)
	if got := refactor.Users(f.p, f.pl, []ids.AssetID{f.b}); !slices.Equal(paths(got), []string{otherScene}) {
		t.Fatalf("users of b: %v", paths(got))
	}
	if got := refactor.Users(f.p, f.pl, []ids.AssetID{f.b, f.id(otherScene)}); len(got) != 0 {
		t.Fatalf("deleting b with its only user leaves users %v", paths(got))
	}
}

func TestMissingNodes(t *testing.T) {
	f := open(t)
	var found []refs.MissingNode
	for _, m := range refs.MissingNodes(f.pl) {
		if m.Scene == brokenScene {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("missing nodes %+v", found)
	}
	m := found[0]
	if m.Path != "Hook" || m.Owner != "physics.DistanceJoint2D" || m.Field != "target" || m.Target.String() != missingTarget {
		t.Fatalf("missing node %+v", m)
	}
}

func exists(f *fixture, rel string) bool {
	_, err := os.Stat(f.abs(rel))
	return err == nil
}

func TestRename(t *testing.T) {
	f := open(t)
	rows, ops, err := refactor.PlanRename(f.pl, rename.Rule{Pattern: "{name}_x"}, []ids.AssetID{f.a, f.b})
	if err != nil {
		t.Fatalf("%v %+v", err, rows)
	}
	if _, err := refactor.ApplyOps(f.p, f.pl, ops); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[ids.AssetID]string{f.a: "assets/tex/a_x.png", f.b: "assets/tex/b_x.png"} {
		if rel, _ := f.pl.PathOf(id); rel != want || !exists(f, want+".meta") {
			t.Fatalf("%s is at %s", want, rel)
		}
	}
	if exists(f, texA) || exists(f, texA+".meta") {
		t.Fatal("the old file or meta is still there")
	}
	swap := refactor.RenameOps([]string{"assets/tex/a_x.png", "assets/tex/b_x.png"}, []ids.AssetID{f.a, f.b},
		[]rename.Row{{From: "a_x", To: "b_x"}, {From: "b_x", To: "a_x"}})
	if len(swap) != 4 || !strings.Contains(swap[0].To, "__rename_") {
		t.Fatalf("swap ops %+v", swap)
	}
	if _, err := refactor.ApplyOps(f.p, f.pl, swap); err != nil {
		t.Fatal(err)
	}
	if ra, _ := f.pl.PathOf(f.a); ra != "assets/tex/b_x.png" {
		t.Fatalf("a is at %s after the swap", ra)
	}
	if rb, _ := f.pl.PathOf(f.b); rb != "assets/tex/a_x.png" {
		t.Fatalf("b is at %s after the swap", rb)
	}
	rows, _, err = refactor.PlanRename(f.pl, rename.Rule{Pattern: "a_x"}, []ids.AssetID{f.a})
	if err == nil || !rename.Blocked(rows) {
		t.Fatalf("a name taken by another file was accepted: %+v", rows)
	}
}

func TestMoveScript(t *testing.T) {
	f := open(t)
	const from, to = "scripts/enemy/enemy.aes", "scripts/shared/enemy.aes"
	id := f.id(from)
	before := f.read(from)
	note, err := refactor.Move(f.p, f.pl, id, to)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, "package shared") || !strings.Contains(note, "was enemy") {
		t.Fatalf("note %q", note)
	}
	if !bytes.HasPrefix(f.read(to), []byte("package shared")) {
		t.Fatalf("package line not rewritten:\n%s", f.read(to))
	}
	if _, err := refactor.Move(f.p, f.pl, id, from); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.read(from), before) {
		t.Fatal("moving back did not restore the script")
	}
}
