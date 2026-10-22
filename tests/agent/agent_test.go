package agent_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aego/app/agent"
	"aego/app/run"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/project"
	"aego/scene/mutate"
	"aego/sdk"
	"aego/tests/fixture"
)

var update = flag.Bool("update", false, "rewrite the golden files")

type env struct {
	t   *testing.T
	dir string
	p   *project.Project
	opt agent.Options
}

func open(t *testing.T) *env {
	t.Helper()
	dir := fixture.Copy(t, "platformer")
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	mods := []sdk.Module{fixture.Platformer{}}
	return &env{t: t, dir: dir, p: p, opt: agent.Options{Mods: mods, Run: run.AgentRunner(mods...)}}
}

func (e *env) run(name string, a agent.Args) (*agent.Output, error) {
	e.t.Helper()
	s, err := agent.Open(e.p, e.opt)
	if err != nil {
		e.t.Fatal(err)
	}
	defer s.Close()
	return agent.Run(s, name, a)
}

func (e *env) must(name string, a agent.Args) *agent.Output {
	e.t.Helper()
	out, err := e.run(name, a)
	if err != nil {
		e.t.Fatalf("%s %v: %v", name, a, err)
	}
	return out
}

func (e *env) read(rel string) []byte {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.dir, filepath.FromSlash(rel)))
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e *env) write(rel string, b []byte) {
	e.t.Helper()
	abs := filepath.Join(e.dir, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(abs), 0o755)
	if err := os.WriteFile(abs, b, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

const level = "scenes/main.aescene"

func golden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name+".json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from %s (run with -update):\n%s", name, path, got)
	}
}

func TestReadCommandsGolden(t *testing.T) {
	e := open(t)
	cases := []struct {
		name string
		cmd  string
		args agent.Args
	}{
		{"scene_tree", "scene.tree", agent.Args{"file": level}},
		{"scene_get_player", "scene.get", agent.Args{"file": level, "node": "Player"}},
		{"schema_list", "schema.list", agent.Args{}},
		{"schema_show_body2d", "schema.show", agent.Args{"type": "Body2D"}},
		{"refs_missing", "refs.missing", agent.Args{}},
		{"project_validate", "project.validate", agent.Args{}},
		{"asset_list_prefab", "asset.list", agent.Args{"kind": "prefab"}},
	}
	for _, c := range cases {
		golden(t, c.name, e.must(c.cmd, c.args))
	}
}

func TestSetGetAndDryRun(t *testing.T) {
	e := open(t)
	writes := []struct {
		cmd  string
		args agent.Args
	}{
		{"scene.set", agent.Args{"file": level, "node": "Player", "path": "Body2D.gravityScale", "value": "2"}},
		{"scene.add-node", agent.Args{"file": level, "name": "Extra"}},
		{"scene.rename", agent.Args{"file": level, "node": "Player", "name": "Hero"}},
		{"scene.add-comp", agent.Args{"file": level, "node": "Camera", "comp": "Tags"}},
		{"data.set", agent.Args{"file": "assets/data/enemy_stats.aedata", "path": "speed", "value": "3"}},
		{"asset.rename", agent.Args{"asset": "prefabs/fast_enemy.aeprefab", "name": "quick_enemy"}},
	}
	stamp := map[string]time.Time{}
	snap := map[string][]byte{}
	filepath.WalkDir(e.dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.Contains(p, ".aego") {
			st, _ := d.Info()
			stamp[p], snap[p] = st.ModTime(), e.read(strings.TrimPrefix(filepath.ToSlash(p), filepath.ToSlash(e.dir)+"/"))
		}
		return nil
	})
	for _, w := range writes {
		a := agent.Args{"dry-run": "true"}
		for k, v := range w.args {
			a[k] = v
		}
		out := e.must(w.cmd, a)
		if !out.DryRun {
			t.Fatalf("%s: not reported as a dry run", w.cmd)
		}
	}
	for p, ts := range stamp {
		st, err := os.Stat(p)
		if err != nil || !st.ModTime().Equal(ts) || !bytes.Equal(e.read(strings.TrimPrefix(filepath.ToSlash(p), filepath.ToSlash(e.dir)+"/")), snap[p]) {
			t.Fatalf("a dry run touched %s", p)
		}
	}
	e.must("scene.set", writes[0].args)
	if got := e.must("scene.get", agent.Args{"file": level, "node": "Player", "path": "Body2D.gravityScale"}).Text; got != "2" {
		t.Fatalf("gravityScale %q after set", got)
	}
	e.must("data.set", writes[4].args)
	v := e.must("data.get", agent.Args{"file": "assets/data/enemy_stats.aedata"}).Value.(map[string]any)
	if v["fields"].(map[string]string)["speed"] != "3" {
		t.Fatalf("data %v", v)
	}
}

func TestApplyAndInverse(t *testing.T) {
	e := open(t)
	before := e.read(level)
	tx := mutate.Tx{Label: "t", Ops: []mutate.Op{
		mutate.AddNode("Root", "Ledge").WithID("c00000000000000000000000000000aa"),
		mutate.Set("Player", "Body2D.gravityScale", "3"),
		mutate.Rename("Camera", "Eye"),
	}}
	src, _ := tx.Encode()
	s, err := agent.Open(e.p, e.opt)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := mutate.Load(before, level, mutate.Env{Types: s.Types, Library: s.Library})
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	res, err := doc.Apply(tx)
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := res.Inverse.Encode()
	e.must("scene.apply", agent.Args{"file": level, "tx": string(src)})
	if bytes.Equal(e.read(level), before) {
		t.Fatal("apply changed nothing")
	}
	e.write("inverse.aetext", inv)
	e.must("scene.apply", agent.Args{"file": level, "tx-file": filepath.Join(e.dir, "inverse.aetext")})
	if !bytes.Equal(e.read(level), before) {
		t.Fatalf("the inverse did not restore the scene:\n%s", e.read(level))
	}
}

func TestPrefabFlow(t *testing.T) {
	e := open(t)
	out := e.must("prefab.create", agent.Args{"file": level, "node": "GroundLeft", "out": "prefabs/ground"})
	if len(out.Created) != 1 {
		t.Fatalf("create %+v", out)
	}
	ins := e.must("prefab.instantiate", agent.Args{"file": level, "prefab": "prefabs/ground.aeprefab", "name": "Ground2"})
	second := ins.Created[0]
	e.must("scene.set", agent.Args{"file": level, "node": "GroundLeft", "path": "SpriteRenderer.tint", "value": "#123456ff"})
	e.must("prefab.apply", agent.Args{"file": level, "node": "GroundLeft", "level": "property", "owner": "engine.SpriteRenderer", "field": "tint"})
	if got := e.must("scene.get", agent.Args{"file": level, "node": second, "path": "SpriteRenderer.tint"}).Text; got != "#123456ff" {
		t.Fatalf("second instance tint %q", got)
	}
	again := e.must("prefab.apply", agent.Args{"file": level, "node": "GroundLeft", "level": "property", "owner": "engine.SpriteRenderer", "field": "tint"})
	if len(again.Notes) != 1 || again.Notes[0] != "nothing to apply" {
		t.Fatalf("second apply %+v", again)
	}
}

func TestDeleteMissingFix(t *testing.T) {
	e := open(t)
	const stats = "assets/data/enemy_stats.aedata"
	data := e.read(stats)
	if _, err := e.run("asset.delete", agent.Args{"asset": stats}); err == nil || !strings.Contains(err.Error(), "enemy.aeprefab") {
		t.Fatalf("delete of a used asset: %v", err)
	}
	e.must("asset.delete", agent.Args{"asset": stats, "force": "true"})
	miss := e.must("refs.missing", agent.Args{}).Value.(map[string]any)["assets"].([]agent.MissingView)
	if len(miss) != 1 || miss[0].Was != stats {
		t.Fatalf("missing %+v", miss)
	}
	e.write(stats, data)
	e.must("refs.fix", agent.Args{"from": miss[0].Asset, "to": stats})
	if got := e.must("refs.missing", agent.Args{}).Value.(map[string]any)["assets"].([]agent.MissingView); len(got) != 0 {
		t.Fatalf("still missing %+v", got)
	}
}

func runHash(t *testing.T, e *env, file string) agent.RunReport {
	t.Helper()
	out := e.must("scene.run", agent.Args{"file": file, "frames": "120"})
	return out.Value.(agent.RunReport)
}

func TestRenameScriptAndRun(t *testing.T) {
	e := open(t)
	e.must("asset.rename", agent.Args{"asset": "scripts/player/player.aes", "name": "hero"})
	a, b := runHash(t, e, level), runHash(t, e, level)
	if a.Error != "" || a.Frames != 120 || a.Hash != b.Hash {
		t.Fatalf("runs %+v %+v", a, b)
	}
}

func TestRunReportsScriptFault(t *testing.T) {
	e := open(t)
	e.write("scripts/boom/boom.aes", []byte("package boom\n\nimport \"aego/log\"\n\nscript Boom {\n\tfunc Update(dt float) {\n\t\tvar z int = 0\n\t\tlog.Info(string(1 / z))\n\t}\n}\n"))
	src := strings.Replace(string(e.read(level)), `script "mygame.player.PlayerController" enabled=true {`,
		`script "mygame.boom.Boom" enabled=true {}
      script "mygame.player.PlayerController" enabled=true {`, 1)
	e.write(level, []byte(src))
	out, err := e.run("scene.run", agent.Args{"file": level, "frames": "30"})
	if err != nil {
		t.Fatal(err)
	}
	rep := out.Value.(agent.RunReport)
	if rep.Error == "" && !strings.Contains(strings.Join(rep.Log, "\n"), "zero") {
		t.Fatalf("no fault reported: %+v", rep)
	}
	if !out.Errors() && rep.Error != "" {
		t.Fatal("the run error is not an error-level problem")
	}
}

func TestCreateRefusesSharedIndex(t *testing.T) {
	e := open(t)
	e.must("asset.list", agent.Args{})
	s, err := agent.Open(e.p, e.opt)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	other, err := pipeline.Open(e.p, mustImporters(t, e), log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := e.run("scene.new", agent.Args{"file": "scenes/fresh"}); err == nil || !strings.Contains(err.Error(), "holds the asset index") {
		t.Fatalf("scene new while the index is held: %v", err)
	}
}

func mustImporters(t *testing.T, e *env) *pipeline.ImporterRegistry {
	t.Helper()
	return pipeline.NewImporterRegistry()
}

// add-comp brings the components a component requires unless no-required asks for the old strict behaviour (D144).
func TestAddCompAddsRequired(t *testing.T) {
	e := open(t)
	out := e.must("scene.add-comp", agent.Args{"file": level, "node": "PatrolA1", "comp": "CameraFollow2D"})
	if len(out.Notes) != 1 || !strings.Contains(out.Notes[0], "added Virtual Camera 2D (required by Camera Follow 2D)") {
		t.Fatalf("notes %v", out.Notes)
	}
	text := string(e.read(level))
	if !strings.Contains(text, `component "camera.VirtualCamera2D"`) || !strings.Contains(text, `component "camera.CameraFollow2D"`) {
		t.Fatal("the scene lacks the follow camera or its virtual camera")
	}
	strict := e.must("scene.add-comp", agent.Args{"file": level, "node": "PatrolB1", "comp": "CameraFollow2D", "no-required": true})
	if len(strict.Notes) != 0 {
		t.Fatalf("no-required added %v", strict.Notes)
	}
}

func TestAddCompRefusesScript(t *testing.T) {
	e := open(t)
	for _, strict := range []bool{false, true} {
		if _, err := e.run("scene.add-comp", agent.Args{"file": level, "node": "Player", "comp": "mygame.player.PlayerController", "no-required": strict}); err == nil {
			t.Fatalf("no-required=%v: add-comp accepted a script type", strict)
		}
	}
}
