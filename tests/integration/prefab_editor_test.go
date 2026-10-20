package integration

import (
	"os"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/prefabs"
	"aego/editor/core/templates"
	"aego/scene/sceneio"
)

func TestPrefabEditorFlow(t *testing.T) {
	s, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/level.aescene"); err != nil {
		t.Fatal(err)
	}
	d := ctx.Doc()
	sc := d.Scene()
	enemy := sc.Create(sc.Root(), "Enemy")
	sc.Create(enemy, "Stomp")
	sc.Flush()
	enemyID := sc.ID(enemy)
	svc := prefabs.New(ctx)
	if err := svc.CreateFrom(d, []ids.NodeID{enemyID}, ""); err != nil {
		t.Fatal(err)
	}
	pid, ok := ctx.Pipeline.Lookup("prefabs/Enemy.aeprefab")
	if !ok {
		t.Fatal("prefab file was not created")
	}
	if pi, ok := sceneio.InstanceOf(sc, sc.Handle(enemyID)); !ok || !pi.Owner || pi.Prefab != pid {
		t.Fatal("node must become an instance of the new prefab")
	}
	tmpl, ok := s.App.Templates().ForAsset(asset.ArtPrefab, nil, pid, "Enemy")
	if !ok {
		t.Fatal("prefab template missing")
	}
	created, err := templates.Instantiate(ctx, tmpl, ids.NodeID{}, -1, nil)
	if err != nil || len(created) != 1 {
		t.Fatalf("instantiate: %v", err)
	}
	second := created[0]
	childName := func(root ids.NodeID) string {
		h := sc.Handle(root)
		return sc.Name(sc.FirstChild(h))
	}
	stomp := sc.ID(sc.FirstChild(sc.Handle(enemyID)))
	if err := d.Props.Rename(nil, stomp, "Feet"); err != nil {
		t.Fatal(err)
	}
	if !svc.Row(d, stomp).Orphans && svc.Row(d, stomp).Status != prefabs.StatusInside {
		t.Fatal("stomp must be marked as part of the instance")
	}
	if info, ok := svc.Info(d, stomp); !ok || !info.Overridden(stomp, "node", "name") {
		t.Fatal("rename must be an override")
	}
	if err := svc.Apply(d, prefabs.Change{Node: stomp, Level: prefabs.LevelNode}); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		data, err := os.ReadFile(ctx.Pipeline.Abs("prefabs/Enemy.aeprefab"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if !strings.Contains(read(), `name="Feet"`) || childName(second) != "Feet" {
		t.Fatal("apply must reach the file and the other instance")
	}
	if info, _ := svc.Info(d, stomp); info.Overridden(stomp, "node", "name") {
		t.Fatal("applied value is no longer an override")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(), `name="Feet"`) || childName(second) != "Stomp" || childName(enemyID) != "Feet" {
		t.Fatal("undo must restore the file, the other instance and keep the source override")
	}
	if err := d.History.Redo(); err != nil {
		t.Fatal(err)
	}
	if childName(second) != "Feet" {
		t.Fatal("redo must apply again")
	}
	secondStomp := sc.ID(sc.FirstChild(sc.Handle(second)))
	if err := d.Props.Rename(nil, secondStomp, "Claws"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Revert(d, prefabs.Change{Node: secondStomp, Level: prefabs.LevelNode}); err != nil {
		t.Fatal(err)
	}
	if childName(second) != "Feet" {
		t.Fatal("revert must return the prefab value")
	}
	if err := svc.Unpack(d, second, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := sceneio.InstanceOf(sc, sc.Handle(second)); ok {
		t.Fatal("unpack must drop the link")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if pi, ok := sceneio.InstanceOf(sc, sc.Handle(second)); !ok || !pi.Owner {
		t.Fatal("undo of unpack must restore the instance")
	}
	if err := ctx.Docs.Save(d.ID); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Docs.Close(d.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := ctx.NewScene("scenes/other.aescene"); err != nil {
		t.Fatal(err)
	}
	other := ctx.Doc()
	made, err := templates.Instantiate(ctx, tmpl, ids.NodeID{}, -1, nil)
	if err != nil {
		t.Fatal(err)
	}
	osc := other.Scene()
	oStomp := osc.ID(osc.FirstChild(osc.Handle(made[0])))
	if err := other.Props.Rename(nil, oStomp, "Hooves"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Apply(other, prefabs.Change{Node: oStomp, Level: prefabs.LevelNode}); err != nil {
		t.Fatal(err)
	}
	reopened, err := ctx.Docs.Open("scenes/level.aescene")
	if err != nil {
		t.Fatal(err)
	}
	rs := reopened.Scene()
	if rs.Name(rs.FirstChild(rs.Handle(second))) != "Hooves" {
		t.Fatal("a closed scene must pick up the prefab change when it opens")
	}
	if rs.Name(rs.FirstChild(rs.Handle(enemyID))) != "Hooves" {
		t.Fatal("instances without an override follow the prefab")
	}
}

func TestCreatePrefabUndo(t *testing.T) {
	_, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/undo.aescene"); err != nil {
		t.Fatal(err)
	}
	d := ctx.Doc()
	sc := d.Scene()
	crate := sc.Create(sc.Root(), "Crate")
	sc.Create(crate, "Lid")
	sc.Flush()
	id := sc.ID(crate)
	before := sceneio.SaveSubtree(sc, crate, true)
	if err := prefabs.New(ctx).CreateFrom(d, []ids.NodeID{id}, ""); err != nil {
		t.Fatal(err)
	}
	if pi, ok := sceneio.InstanceOf(sc, sc.Handle(id)); !ok || !pi.Owner {
		t.Fatal("node must become a prefab instance")
	}
	if got := d.History.Steps(nil); len(got) == 0 || got[len(got)-1] != "Create Prefab" {
		t.Fatalf("history %v", got)
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, ok := sceneio.InstanceOf(sc, sc.Handle(id)); ok {
		t.Fatal("undo must turn the instance back into plain nodes")
	}
	if got := sceneio.SaveSubtree(sc, sc.Handle(id), true); string(got) != string(before) {
		t.Fatalf("undo changed the subtree:\n%s", got)
	}
	if err := d.History.Redo(); err != nil {
		t.Fatal(err)
	}
	if pi, ok := sceneio.InstanceOf(sc, sc.Handle(id)); !ok || !pi.Owner {
		t.Fatal("redo must link the instance again")
	}
}
