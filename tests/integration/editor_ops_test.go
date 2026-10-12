package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/edit"
	"aego/app/run"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	edapp "aego/editor/app"
	"aego/editor/assets"
	"aego/editor/context"
	"aego/project"
)

func openEditor(t *testing.T) (*edit.Session, *context.Editor) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "EditorOps", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	return s, s.App.Context()
}

func fileExists(abs string) bool {
	_, err := os.Stat(abs)
	return err == nil
}

func assetAt(t *testing.T, ctx *context.Editor, rel string) ids.AssetID {
	t.Helper()
	id, ok := ctx.Pipeline.Lookup(rel)
	if !ok {
		t.Fatalf("%s is not tracked by the pipeline", rel)
	}
	if !fileExists(ctx.Pipeline.Abs(rel) + pipeline.MetaSuffix) {
		t.Fatalf("%s has no meta", rel)
	}
	return id
}

func readRel(t *testing.T, ctx *context.Editor, rel string) string {
	t.Helper()
	data, err := os.ReadFile(ctx.Pipeline.Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEditorCreatesFromMenu(t *testing.T) {
	_, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/level.aescene"); err != nil {
		t.Fatal(err)
	}
	assetAt(t, ctx, "scenes/level.aescene")
	if ctx.Docs.Find("scenes/level.aescene") == nil || ctx.Doc() == nil || ctx.Doc().Path != "scenes/level.aescene" {
		t.Fatal("new scene must be open and active")
	}
	prefab := "prefabs/crate" + assets.PrefabExt
	if err := ctx.NewPrefab(prefab); err != nil {
		t.Fatal(err)
	}
	assetAt(t, ctx, prefab)
	rel, err := assets.CreateScript(ctx, "scripts/enemy", "Enemy")
	if err != nil {
		t.Fatal(err)
	}
	assetAt(t, ctx, rel)
	src := readRel(t, ctx, rel)
	if !strings.HasPrefix(src, "package enemy\n") || !strings.Contains(src, "script Enemy {") {
		t.Fatalf("script source:\n%s", src)
	}
}

func TestEditorFileOpsKeepIDs(t *testing.T) {
	_, ctx := openEditor(t)
	src := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(src, testPNG(t, 16, 16), 0o644); err != nil {
		t.Fatal(err)
	}
	created, batch, err := ctx.Pipeline.ImportExternal([]string{src}, "assets/chars")
	ctx.ApplyBatch(batch)
	if err != nil || len(created) != 1 {
		t.Fatalf("import: %v %v", created, err)
	}
	tex := created[0]
	var ops assets.Ops
	if err := ops.Apply(ctx, []assets.FileOp{{Dir: true, From: "assets/chars", To: "assets/heroes"}}); err != nil {
		t.Fatal(err)
	}
	if id := assetAt(t, ctx, "assets/heroes/hero.png"); id != tex {
		t.Fatal("move changed the asset id")
	}
	if e, ok := ctx.Pipeline.Entry(tex); !ok || e.Missing || e.Source != "assets/heroes/hero.png" {
		t.Fatalf("entry after move = %+v", e)
	}
	if err := ops.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	assetAt(t, ctx, "assets/chars/hero.png")
	if err := ops.Apply(ctx, []assets.FileOp{{ID: tex, From: "assets/chars/hero.png", To: "assets/chars/knight.png"}}); err != nil {
		t.Fatal(err)
	}
	if id := assetAt(t, ctx, "assets/chars/knight.png"); id != tex {
		t.Fatal("rename changed the asset id")
	}
	dup, err := assets.Duplicate(ctx, tex)
	if err != nil {
		t.Fatal(err)
	}
	if id := assetAt(t, ctx, "assets/chars/knight copy.png"); id != dup || dup == tex {
		t.Fatal("duplicate must land next to the source with a fresh id")
	}
	if err := assets.Delete(ctx, []ids.AssetID{dup}); err != nil {
		t.Fatal(err)
	}
	abs := ctx.Pipeline.Abs("assets/chars/knight copy.png")
	if fileExists(abs) || fileExists(abs+pipeline.MetaSuffix) {
		t.Fatal("delete must remove the file and its meta")
	}
}

func TestEditorScriptMoveRetargetsPackage(t *testing.T) {
	_, ctx := openEditor(t)
	rel, err := assets.CreateScript(ctx, "scripts/enemy", "Enemy")
	if err != nil {
		t.Fatal(err)
	}
	id := assetAt(t, ctx, rel)
	if err := ctx.Pipeline.Mkdir("scripts/shared"); err != nil {
		t.Fatal(err)
	}
	var ops assets.Ops
	dst := "scripts/shared/enemy.aes"
	if err := ops.Apply(ctx, []assets.FileOp{{ID: id, From: rel, To: dst}}); err != nil {
		t.Fatal(err)
	}
	if src := readRel(t, ctx, dst); !strings.HasPrefix(src, "package shared\n") {
		t.Fatalf("package not updated:\n%s", src)
	}
	if n, ok := ctx.LastNotice(); !ok || n.Level != context.LevelWarn || !strings.Contains(n.Text, "package shared") {
		t.Fatalf("move must warn about imports: %+v", n)
	}
	if err := ops.Apply(ctx, []assets.FileOp{{Dir: true, From: "scripts/shared", To: "scripts/common"}}); err != nil {
		t.Fatal(err)
	}
	if src := readRel(t, ctx, "scripts/common/enemy.aes"); !strings.HasPrefix(src, "package common\n") {
		t.Fatalf("package folder rename must rewrite the package:\n%s", src)
	}
}

func TestEditorSceneSaveAsStartupClose(t *testing.T) {
	s, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/a.aescene"); err != nil {
		t.Fatal(err)
	}
	d := ctx.Docs.Find("scenes/a.aescene")
	assets.PromptSaveAs(ctx, d)
	qs := ctx.TakeQuestions()
	if len(qs) != 1 || qs[0].Submit == nil {
		t.Fatalf("save as must ask for a path: %+v", qs)
	}
	qs[0].Submit([]string{"scenes/b"})
	if d.Path != "scenes/b.aescene" || !fileExists(ctx.Pipeline.Abs(d.Path)) {
		t.Fatalf("save as wrote %s", d.Path)
	}
	if _, _, err := ctx.Pipeline.Scan(); err != nil {
		t.Fatal(err)
	}
	id := assetAt(t, ctx, d.Path)
	assets.SetStartup(ctx, id, d.Path)
	back, err := project.Load(ctx.Project.Root)
	if err != nil || back.MainScene != id {
		t.Fatalf("startup scene not saved: %v", err)
	}
	d.MarkDirty()
	ctx.Commands.Run(edapp.CommandCloseScene)
	s.Step()
	if ctx.Docs.Find(d.Path) == nil {
		t.Fatal("a dirty scene must not close without the dialog")
	}
}

func TestEditorCreatesDefaultDirs(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Dirs", false)
	if err != nil {
		t.Fatal(err)
	}
	scenePath := filepath.Join(p.ScenesDir(), "main.aescene")
	if err := os.WriteFile(scenePath, []byte(rotatorScene), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := []string{"scripts", "prefabs"}
	for _, d := range gone {
		if err := os.RemoveAll(filepath.Join(p.Root, d)); err != nil {
			t.Fatal(err)
		}
	}
	app, err := run.Open(p, run.Config{Headless: true, Logger: log.Nop(), ScenePath: scenePath})
	if err != nil {
		t.Fatal(err)
	}
	app.Close()
	for _, d := range gone {
		if fileExists(filepath.Join(p.Root, d)) {
			t.Fatalf("aego run created %s", d)
		}
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, d := range project.DefaultDirs {
		if !fileExists(filepath.Join(p.Root, d)) {
			t.Fatalf("editor did not create %s", d)
		}
	}
}
