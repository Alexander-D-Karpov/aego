package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/agent"
	"aego/app/edit"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/project"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/sdk"
	"aego/tests/fixture"
)

const (
	levelPath  = "scenes/main.aescene"
	enemyLeft  = "c0000000000000000000000000000004"
	playerNode = "c0000000000000000000000000000003"
)

type external struct {
	t   *testing.T
	s   *edit.Session
	ctx *context.Editor
	p   *project.Project
	opt agent.Options
}

func openExternal(t *testing.T) *external {
	t.Helper()
	p, err := project.Load(fixture.Copy(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	mods := []sdk.Module{fixture.Platformer{}}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, mods...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	return &external{t: t, s: s, ctx: s.App.Context(), p: p, opt: agent.Options{Mods: mods}}
}

func (e *external) agent(name string, a agent.Args) {
	e.t.Helper()
	sess, err := agent.Open(e.p, e.opt)
	if err != nil {
		e.t.Fatal(err)
	}
	defer sess.Close()
	if _, err := agent.Run(sess, name, a); err != nil {
		e.t.Fatalf("%s: %v", name, err)
	}
}

func (e *external) scene() *document.SceneDocument {
	e.t.Helper()
	d, err := e.ctx.Docs.Open(levelPath)
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

func nodeOf(t *testing.T, s string) ids.NodeID {
	t.Helper()
	id, err := ids.ParseNodeID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *external) field(d *document.SceneDocument, node, comp, field string) schema.Value {
	e.t.Helper()
	sc := d.Scene()
	td, ok := sc.Registry().ByName(comp)
	if !ok {
		e.t.Fatalf("no %s", comp)
	}
	fd, _ := td.Field(field)
	v, err := sc.GetProperty(sc.Handle(nodeOf(e.t, node)), td.ID, fd.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func (e *external) text(d *document.SceneDocument, node, comp, field string) string {
	e.t.Helper()
	td, _ := d.Scene().Registry().ByName(comp)
	fd, _ := td.Field(field)
	return mutate.FormatValue(sceneio.WriteValue(d.Scene().Registry(), fd, e.field(d, node, comp, field)))
}

func TestExternalSceneEdit(t *testing.T) {
	e := openExternal(t)
	e.scene()
	e.agent("scene.set", agent.Args{"file": levelPath, "node": "Player", "path": "Body2D.gravityScale", "value": "3"})
	reloaded, conflicts := e.ctx.Docs.CheckExternal()
	if len(reloaded) != 1 || len(conflicts) != 0 {
		t.Fatalf("reloaded %v, conflicts %v", reloaded, conflicts)
	}
	d := e.scene()
	if got := e.field(d, playerNode, "physics.Body2D", "gravityScale").AsFloat(); got != 3 {
		t.Fatalf("gravityScale %v after the reload", got)
	}
	if err := d.Props.Rename(nil, nodeOf(t, playerNode), "Hero"); err != nil {
		t.Fatal(err)
	}
	e.agent("scene.set", agent.Args{"file": levelPath, "node": "Player", "path": "Body2D.gravityScale", "value": "4"})
	if _, conflicts := e.ctx.Docs.CheckExternal(); len(conflicts) != 1 {
		t.Fatalf("a dirty document did not raise a conflict: %v", conflicts)
	}
}

func TestExternalPrefabEdit(t *testing.T) {
	e := openExternal(t)
	d := e.scene()
	order := e.field(d, enemyLeft, "engine.SpriteRenderer", "order").I
	if err := d.Props.Rename(nil, nodeOf(t, playerNode), "Hero"); err != nil {
		t.Fatal(err)
	}
	dirty := d.Dirty()
	e.agent("scene.set", agent.Args{"file": "prefabs/enemy.aeprefab", "node": "Enemy", "path": "SpriteRenderer.tint", "value": "#123456ff"})
	id, ok := e.ctx.Pipeline.Lookup("prefabs/enemy.aeprefab")
	if !ok {
		t.Fatal("no enemy prefab")
	}
	batch, err := e.ctx.Pipeline.Reimport(id)
	if err != nil {
		t.Fatal(err)
	}
	e.ctx.ApplyBatch(batch)
	if got := e.text(d, enemyLeft, "engine.SpriteRenderer", "tint"); got != "#123456ff" {
		t.Fatalf("the instance has tint %s", got)
	}
	sc := d.Scene()
	if sc.Name(sc.Handle(nodeOf(t, enemyLeft))) != "EnemyLeft" || e.field(d, enemyLeft, "engine.SpriteRenderer", "order").I != order {
		t.Fatal("the instance lost its overrides")
	}
	if d.Dirty() != dirty {
		t.Fatal("refreshing the instances changed the dirty state")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if sc.Name(sc.Handle(nodeOf(t, playerNode))) != "Player" {
		t.Fatal("undo of the earlier edit failed after the refresh")
	}
}

func TestExternalDataEdit(t *testing.T) {
	e := openExternal(t)
	data := e.s.App.Data()
	id, ok := e.ctx.Pipeline.Lookup("assets/data/enemy_stats.aedata")
	if !ok {
		t.Fatal("no enemy stats")
	}
	doc, err := data.Doc(id)
	if err != nil {
		t.Fatal(err)
	}
	fd, _ := doc.Type.Field("speed")
	if err := doc.Set(fd, schema.Float32(7), true); err != nil {
		t.Fatal(err)
	}
	mine := doc.Bytes()
	e.agent("data.set", agent.Args{"file": "assets/data/enemy_stats.aedata", "path": "damage", "value": "9"})
	batch, err := e.ctx.Pipeline.Reimport(id)
	if err != nil {
		t.Fatal(err)
	}
	e.ctx.ApplyBatch(batch)
	warnings := func() int {
		n := 0
		for _, x := range e.ctx.Notices() {
			if x.Level == context.LevelWarn && strings.Contains(x.Text, "changed on disk") {
				n++
			}
		}
		return n
	}
	if _, err := data.Doc(id); err != nil {
		t.Fatal(err)
	}
	if warnings() != 1 {
		t.Fatalf("%d warnings after the external edit", warnings())
	}
	e.ctx.AssetRev++
	if _, err := data.Doc(id); err != nil || warnings() != 1 {
		t.Fatalf("the warning repeated: %d, %v", warnings(), err)
	}
	if err := data.SaveDoc(doc); err != nil {
		t.Fatal(err)
	}
	disk, _ := os.ReadFile(filepath.Join(e.p.Root, "assets", "data", "enemy_stats.aedata"))
	if !bytes.Equal(disk, mine) {
		t.Fatalf("Save did not write the editor's version:\n%s", disk)
	}
}
