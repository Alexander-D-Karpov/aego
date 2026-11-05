package agentbridge

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aego/app/agent"
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/selection"
	"aego/project"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type documentHarness struct {
	b     *Bridge
	ctx   *context.Editor
	tasks chan func()
}

func newDocumentHarness(root string, docs *document.Manager) *documentHarness {
	tasks := make(chan func(), 4)
	ctx := &context.Editor{Project: &project.Project{Root: root}, Docs: docs, Selection: selection.New()}
	b := &Bridge{ctx: ctx, cfg: Config{OnMain: func(f func()) { tasks <- f }, Timeout: time.Second}}
	b.panelGeneration.Store(1)
	return &documentHarness{b: b, ctx: ctx, tasks: tasks}
}

func (h *documentHarness) call(t *testing.T, op, file string, choice int, beforeAnswer func()) (*agent.Output, error) {
	t.Helper()
	type outcome struct {
		out *agent.Output
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := h.b.documentOperation(map[string]any{"file": file}, op)
		done <- outcome{out, err}
	}()
	for {
		select {
		case f := <-h.tasks:
			f()
			for _, q := range h.ctx.TakeQuestions() {
				if !strings.Contains(q.Message, file) {
					t.Fatalf("approval did not name the target: %q", q.Message)
				}
				if beforeAnswer != nil {
					beforeAnswer()
				}
				q.Then(choice)
			}
		case r := <-done:
			return r.out, r.err
		case <-time.After(3 * time.Second):
			t.Fatal("document operation timed out")
		}
	}
}

func TestSceneDocumentToolsTargetHistoryAndProtectDisk(t *testing.T) {
	root := t.TempDir()
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	docs := document.NewManager(document.Options{Root: root, Types: types})
	makeScene := func(path string) (*document.SceneDocument, scene.NodeHandle) {
		s := scene.New(types, nil, scene.Options{})
		n := s.Create(s.Root(), "actor")
		s.Flush()
		d := docs.New(path, s)
		if err := docs.Save(d.ID); err != nil {
			t.Fatal(err)
		}
		return d, n
	}
	one, node := makeScene("one.aescene")
	two, other := makeScene("two.aescene")
	change := func(d *document.SceneDocument, n scene.NodeHandle, x float32) {
		s := d.Scene()
		before := s.Local(n)
		after := before
		after.Position = mathx.Vec2{X: x}
		if err := d.History.Exec(&commands.SetTransform{Node: s.ID(n), Before: before, After: after}); err != nil {
			t.Fatal(err)
		}
	}
	change(one, node, 2)
	change(two, other, 7)
	h := newDocumentHarness(root, docs)
	if _, err := h.call(t, "undo", one.Path, 0, nil); err != nil || one.Scene().Local(node).Position.X != 0 || two.Scene().Local(other).Position.X != 7 {
		t.Fatalf("undo changed the wrong scene: %v", err)
	}
	if _, err := h.call(t, "redo", one.Path, 0, nil); err != nil || one.Scene().Local(node).Position.X != 2 {
		t.Fatalf("redo: %v", err)
	}
	if _, err := h.call(t, "undo", one.Path, 0, func() { change(one, node, 4) }); err == nil || one.Scene().Local(node).Position.X != 4 {
		t.Fatalf("stale scene approval changed newer work: %v", err)
	}
	external := []byte("external scene revision\n")
	if _, err := h.call(t, "save", one.Path, 0, func() {
		if err := os.WriteFile(filepath.Join(root, one.Path), external, 0o644); err != nil {
			t.Fatal(err)
		}
	}); err == nil {
		t.Fatal("scene save overwrote a changed disk file")
	}
	if raw, _ := os.ReadFile(filepath.Join(root, one.Path)); !bytes.Equal(raw, external) || !one.Dirty() || !two.Dirty() {
		t.Fatal("stale save lost disk or document changes")
	}
	if _, err := h.call(t, "save", one.Path, 1, nil); err == nil {
		t.Fatal("denied scene save succeeded")
	}
	if _, err := h.call(t, "save", one.Path, 0, nil); err != nil || one.Dirty() || !two.Dirty() {
		t.Fatalf("approved scene save did not target one document: %v", err)
	}
}

func TestAssetDocumentToolsTargetMetadata(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	if err := docs.Handlers().Register(document.Handler{ID: "test.image", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Capabilities: document.View | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"image.png": {137, 80, 78, 71}, "image.png.meta": []byte("meta old\n")} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := docs.OpenAsset(document.AssetRequest{Asset: ids.AssetID{Lo: 1}, Path: "image.png", Kind: asset.ArtTexture})
	if err != nil || d.Metadata == nil {
		t.Fatalf("open asset: %v", err)
	}
	d.Metadata.Code.ReplaceSnapshot([]byte("meta changed\n"), false)
	h := newDocumentHarness(root, docs)
	if _, err := h.call(t, "undo", d.Path, 0, nil); err != nil || string(d.Metadata.Code.Bytes()) != "meta old\n" {
		t.Fatalf("asset undo: %v", err)
	}
	if _, err := h.call(t, "redo", d.Path, 0, nil); err != nil || string(d.Metadata.Code.Bytes()) != "meta changed\n" {
		t.Fatalf("asset redo: %v", err)
	}
	if _, err := h.call(t, "undo", d.Path, 0, func() { d.Metadata.Code.ReplaceSnapshot([]byte("newer metadata\n"), false) }); err == nil || string(d.Metadata.Code.Bytes()) != "newer metadata\n" {
		t.Fatalf("stale asset approval changed newer metadata: %v", err)
	}
	if _, err := h.call(t, "save", d.Path, 0, nil); err != nil || d.Dirty() {
		t.Fatalf("asset save: %v", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "image.png.meta")); string(raw) != "newer metadata\n" {
		t.Fatalf("asset save wrote wrong metadata: %q", raw)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "image.png")); !bytes.Equal(raw, []byte{137, 80, 78, 71}) {
		t.Fatal("metadata operation changed image pixels")
	}
}
