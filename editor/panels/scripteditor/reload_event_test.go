package scripteditor

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/code"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	edws "aego/editor/core/workspace"
	"aego/platform/input"
	"aego/script/langsvc"
	"aego/script/workspace"
	"aego/ui/core"
)

func TestSharedScriptReplacementInvalidatesOpenViews(t *testing.T) {
	for _, tc := range []struct {
		name              string
		missing, readOnly bool
	}{
		{"reload changed source", false, false},
		{"discard removed source", true, false},
		{"reload read-only source", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
			if err != nil {
				t.Fatal(err)
			}
			docs := document.NewManager(document.Options{Root: root})
			const path = "scripts/test.aes"
			if err := document.WriteAtomic(docs.Abs(path), []byte("package c\nvar old = 1\n")); err != nil {
				t.Fatal(err)
			}
			doc, err := docs.OpenScript(path)
			if err != nil {
				t.Fatal(err)
			}
			doc.ReadOnly = tc.readOnly
			jobs := &queuedAnalysisJobs{}
			host := NewHost(langsvc.New(ws, jobs))
			ed := &context.Editor{Docs: docs, Commands: command.New()}
			host.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands}, edws.New(root, nil))
			defer host.offDocs()
			panel := host.show(doc)
			defer panel.releaseDoc()
			clone := newPanel(host, doc)
			clone.clone = true
			host.clones["synthetic-clone"] = clone
			defer clone.releaseDoc()
			panel.ask(1)
			jobs.completeWork(t)
			jobs.deliver(t)
			clone.follow()
			if panel.snap == nil || clone.snap != panel.snap || host.liveSnap != panel.snap {
				t.Fatal("fixture did not create shared analysis views")
			}
			panel.hover = hoverState{off: 3, done: true, ok: true}
			clone.hover = hoverState{off: 2, done: true, ok: true}
			panel.ask(2)
			jobs.completeWork(t)
			generation, old := host.lang.Current(), doc.Code
			panel.mini.ready, panel.mini.rev = true, old.Rev()
			clone.mini.ready, clone.mini.rev = true, old.Rev()
			panel.own, panel.ownRev, panel.ownSet = code.Selection{Anchor: 4, Head: 7}, old.Rev(), true
			clone.own, clone.ownRev, clone.ownSet = code.Selection{Anchor: 5, Head: 8}, old.Rev(), true
			var events []document.Event
			off := docs.Subscribe(func(ev document.Event) {
				if ev.Kind == document.EvScriptReplaced {
					events = append(events, ev)
				}
			})
			defer off()
			if tc.missing {
				if err := os.Remove(docs.Abs(path)); err != nil {
					t.Fatal(err)
				}
				if err := docs.DiscardScript(doc); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := document.WriteAtomic(docs.Abs(path), []byte("package c\nvar replacement = 2\n")); err != nil {
					t.Fatal(err)
				}
				if err := docs.ReloadScript(doc); err != nil {
					t.Fatal(err)
				}
			}
			if doc.Code == old || doc.Code.Rev() != old.Rev() {
				t.Fatal("fixture did not replace the code with an equal revision")
			}
			if len(events) != 1 || events[0].Doc != doc.ID || events[0].Path != doc.Path {
				t.Fatal("replacement did not emit exactly one event for the shared document")
			}
			if panel.snap != nil || clone.snap != nil || panel.hover.done || clone.hover.done || host.liveSnap != nil {
				t.Fatal("shared script replacement did not invalidate open views")
			}
			if panel.mini.ready || clone.mini.ready || panel.ownSet || clone.ownSet {
				t.Fatal("shared script replacement retained source-relative minimap or selection")
			}
			if host.lang.Current() == generation {
				t.Fatal("shared script replacement did not invalidate the old analysis generation")
			}
			jobs.deliver(t)
			if panel.snap != nil || clone.snap != nil || host.liveSnap != nil {
				t.Fatal("late analysis republished a replaced source")
			}
			panel.ask(3)
			jobs.completeWork(t)
			jobs.deliver(t)
			file, ok := ws.FileID(path)
			if !ok || panel.snap == nil || !bytes.Equal(panel.snap.Sources.Source(file), doc.Code.Bytes()) {
				t.Fatal("fresh analysis did not read the replacement")
			}
		})
	}
}

func TestAssetDiscardResetsSharedMetadataViews(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	if err := docs.Handlers().Register(document.Handler{ID: "test.audio", Kinds: []asset.ArtifactKind{asset.ArtAudio}, Capabilities: document.View | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	const path = "audio/test.wav"
	source, saved := []byte("synthetic audio"), []byte("saved metadata\n")
	for name, data := range map[string][]byte{path: source, path + ".meta": saved} {
		if err := document.WriteAtomic(docs.Abs(name), data); err != nil {
			t.Fatal(err)
		}
	}
	d, err := docs.OpenAsset(document.AssetRequest{Asset: ids.AssetID{Lo: 51}, Kind: asset.ArtAudio, Path: path})
	if err != nil || d.Metadata == nil {
		t.Fatal("cannot open shared metadata", err)
	}
	journal := d.Metadata.Code
	if err := journal.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if changed, err := journal.ReplaceVolatile(journal.Rev(), []byte("unsaved in-memory edit")); err != nil || !changed {
		t.Fatal("cannot create volatile metadata", err)
	}
	recovery := filepath.Join(root, filepath.FromSlash(document.RecoveryDir), hex.EncodeToString([]byte(d.Metadata.Path))+document.ScriptExt)
	if err := os.MkdirAll(filepath.Dir(recovery), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recovery, []byte("unowned recovery"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	jobs := &queuedAnalysisJobs{}
	host := NewHost(langsvc.New(ws, jobs))
	ed := &context.Editor{Docs: docs, Commands: command.New()}
	host.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands}, edws.New(root, nil))
	defer host.offDocs()
	panel := host.show(d.Metadata)
	defer panel.releaseDoc()
	clone := newPanel(host, d.Metadata)
	clone.clone = true
	host.clones["clone"] = clone
	defer clone.releaseDoc()
	panel.ask(1)
	jobs.completeWork(t)
	jobs.deliver(t)
	clone.follow()
	if panel.snap == nil || clone.snap != panel.snap {
		t.Fatal("fixture did not analyze shared metadata")
	}
	panel.ask(2)
	jobs.completeWork(t)
	if err := docs.CloseAsset(d.Asset, true); err != nil {
		t.Fatal("explicit discard failed", err)
	}
	if docs.AssetDocument(d.Asset) != nil || docs.Script(d.Metadata.Path) != d.Metadata || d.Metadata.Code == journal || d.Metadata.Dirty() {
		t.Fatal("asset discard did not replace the shared metadata journal")
	}
	if panel.snap != nil || clone.snap != nil || host.liveSnap != nil {
		t.Fatal("asset discard retained a metadata view snapshot")
	}
	jobs.deliver(t)
	if panel.snap != nil || host.liveSnap != nil {
		t.Fatal("late analysis republished discarded metadata")
	}
	for name, want := range map[string][]byte{path: source, path + ".meta": saved} {
		if got, err := os.ReadFile(docs.Abs(name)); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("discard changed %s: %v", name, err)
		}
	}
	if got, err := os.ReadFile(recovery); err != nil || string(got) != "unowned recovery" {
		t.Fatal("asset discard changed an unowned recovery", err)
	}
}

func TestScriptReplacementRefreshesHeadlessOverlay(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	const target, other = "scripts/target.aes", "scripts/other.aes"
	for path, text := range map[string]string{
		target: "package c\nvar target_old = 1\n",
		other:  "package c\nvar other = 2\n",
	} {
		if err := document.WriteAtomic(docs.Abs(path), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	jobs := &queuedAnalysisJobs{}
	host := NewHost(langsvc.New(ws, jobs))
	ed := &context.Editor{Docs: docs, Commands: command.New()}
	host.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands}, edws.New(root, nil))
	defer host.offDocs()
	host.find.query, host.find.repl = "target_old", "target_draft"
	host.find.matches = []workspace.Match{{Path: target}}
	host.find.replaceAll()
	doc := docs.Script(target)
	if doc == nil || !doc.Dirty() || host.panels[target] != nil {
		t.Fatal("replace all did not create a headless dirty script")
	}
	otherDoc, err := docs.OpenScript(other)
	if err != nil {
		t.Fatal(err)
	}
	panel := host.show(otherDoc)
	defer panel.releaseDoc()
	panel.ask(1)
	jobs.completeWork(t)
	jobs.deliver(t)
	file, ok := ws.FileID(target)
	if !ok || panel.snap == nil || !bytes.Contains(panel.snap.Sources.Source(file), []byte("target_draft")) {
		t.Fatal("fixture did not analyze the headless working overlay")
	}
	panel.conflict = true // A different document's conflict must survive.
	panel.ask(2)
	jobs.completeWork(t)
	gen := host.lang.Current()
	if err := docs.DiscardScript(doc); err != nil {
		t.Fatal(err)
	}
	src, ok := ws.Source(target)
	if !ok || !bytes.Equal(src, doc.Code.Bytes()) {
		t.Fatal("discard left a stale headless workspace overlay")
	}
	if host.lang.Current() == gen || host.liveSnap != nil || panel.snap != nil || !panel.conflict {
		t.Fatal("headless replacement retained dependent analysis or cleared an unrelated conflict")
	}
	jobs.deliver(t)
	if panel.snap != nil || host.liveSnap != nil {
		t.Fatal("late dependent analysis survived headless replacement")
	}
	host.ensure()
	latest := host.lang.Latest()
	if latest == nil || latest.Gen != ws.Version() || !bytes.Equal(latest.Sources.Source(file), doc.Code.Bytes()) {
		t.Fatal("synchronous language queries retained the replaced source")
	}
	panel.ask(3)
	jobs.completeWork(t)
	jobs.deliver(t)
	if panel.snap == nil || !bytes.Equal(panel.snap.Sources.Source(file), doc.Code.Bytes()) {
		t.Fatal("new dependent analysis did not read the discarded overlay")
	}
	// With no view of target, a clean discarded document must not leave an
	// overlay that masks later disk updates from the workspace watcher.
	later := []byte("package c\nvar target_later = 3\n")
	if err := document.WriteAtomic(docs.Abs(target), later); err != nil {
		t.Fatal(err)
	}
	ws.Changed([]string{target})
	if src, ok := ws.Source(target); !ok || !bytes.Equal(src, later) {
		t.Fatal("discarded headless overlay masked a later disk update")
	}
}

func TestScriptReplacementRemovesHeadlessMissingSource(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	const path = "scripts/removed.aes"
	if err := document.WriteAtomic(docs.Abs(path), []byte("package c\nvar old = 1\n")); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	host := NewHost(langsvc.New(ws, nil))
	ed := &context.Editor{Docs: docs, Commands: command.New()}
	host.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands}, edws.New(root, nil))
	defer host.offDocs()
	host.find.query, host.find.repl = "old", "working"
	host.find.matches = []workspace.Match{{Path: path}}
	host.find.replaceAll()
	doc := docs.Script(path)
	if doc == nil || !doc.Dirty() || host.panels[path] != nil {
		t.Fatal("fixture did not create a headless working overlay")
	}
	if err := os.Remove(docs.Abs(path)); err != nil {
		t.Fatal(err)
	}
	if err := docs.DiscardScript(doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := ws.Source(path); ok || doc.OnDisk || doc.Dirty() {
		t.Fatal("discard retained a removed headless source")
	}
	if _, err := os.Stat(docs.Abs(path)); !os.IsNotExist(err) {
		t.Fatal("discard recreated the removed file", err)
	}
	later := []byte("package c\nvar recreated = 2\n")
	if err := document.WriteAtomic(docs.Abs(path), later); err != nil {
		t.Fatal(err)
	}
	ws.Changed([]string{path})
	if got, ok := ws.Source(path); !ok || !bytes.Equal(got, later) {
		t.Fatal("removed headless overlay masked a later recreation")
	}
}

func TestScriptReplacementClearsKnownNonScriptOverlay(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	const path = "audio/sample.wav.meta"
	saved := []byte("old settings\n")
	if err := document.WriteAtomic(docs.Abs(path), saved); err != nil {
		t.Fatal(err)
	}
	doc, err := docs.OpenScript(path)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	host := NewHost(langsvc.New(ws, nil))
	ed := &context.Editor{Docs: docs, Commands: command.New()}
	host.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands}, edws.New(root, nil))
	defer host.offDocs()
	// A non-script text view can enter the search workspace; Replace All can
	// subsequently keep its overlay even when that view is gone.
	ws.SetOverlay(path, doc.Code.Bytes())
	host.find.query, host.find.repl = "old", "working"
	host.find.matches = []workspace.Match{{Path: path}}
	host.find.replaceAll()
	if !doc.Dirty() || host.panels[path] != nil {
		t.Fatal("fixture did not create a headless non-script overlay")
	}
	if err := docs.DiscardScript(doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := ws.Source(path); ok {
		t.Fatal("discard retained a known headless non-script overlay")
	}
	if got, err := os.ReadFile(docs.Abs(path)); err != nil || !bytes.Equal(got, saved) {
		t.Fatal("discard changed saved metadata", err)
	}
}

func TestForeignReplacementLeavesOtherLanguageViews(t *testing.T) {
	root := t.TempDir()
	docs := document.NewManager(document.Options{Root: root})
	const script, shader = "scripts/other.aes", "assets/shader.glsl"
	for path, data := range map[string]string{script: "package c\nvar other = 1\n", shader: "old shader\n"} {
		if err := document.WriteAtomic(docs.Abs(path), []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	scriptDoc, err := docs.OpenScript(script)
	if err != nil {
		t.Fatal(err)
	}
	shaderDoc, err := docs.OpenScript(shader)
	if err != nil {
		t.Fatal(err)
	}
	jobs := &queuedAnalysisJobs{}
	host := NewHost(langsvc.New(ws, jobs))
	ed := &context.Editor{Docs: docs, Commands: command.New()}
	host.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands}, edws.New(root, nil))
	defer host.offDocs()
	other := host.show(scriptDoc)
	defer other.releaseDoc()
	other.ask(1)
	jobs.completeWork(t)
	jobs.deliver(t)
	panel := host.show(shaderDoc)
	defer panel.releaseDoc()
	panel.hover = hoverState{off: 1, done: true}
	panel.mini.ready, panel.ownSet = true, true
	gen, version, snap, findGen := host.lang.Current(), ws.Version(), other.snap, host.find.gen
	if snap == nil || host.liveSnap != snap {
		t.Fatal("fixture did not create a live language snapshot")
	}
	if err := document.WriteAtomic(docs.Abs(shader), []byte("new shader\n")); err != nil {
		t.Fatal(err)
	}
	if err := docs.ReloadScript(shaderDoc); err != nil {
		t.Fatal(err)
	}
	if panel.hover.done || panel.mini.ready || panel.ownSet {
		t.Fatal("foreign source replacement did not reset its own view")
	}
	if _, ok := ws.FileID(shader); ok || host.lang.Current() != gen || ws.Version() != version || other.snap != snap || host.liveSnap != snap || host.find.gen != findGen {
		t.Fatal("foreign replacement altered unrelated language state")
	}
}

func TestEnsureKeepsOrdinaryEditsAsynchronous(t *testing.T) {
	ws, err := workspace.Open(workspace.Config{Root: t.TempDir(), Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	host := NewHost(langsvc.New(ws, nil))
	ws.SetOverlay("scripts/test.aes", []byte("package c\nvar old = 1\n"))
	host.ensure()
	previous := host.lang.Latest()
	if previous == nil {
		t.Fatal("fixture did not build an initial language snapshot")
	}
	ws.SetOverlay("scripts/test.aes", []byte("package c\nvar edit = 2\n"))
	host.ensure()
	if host.lang.Latest() != previous {
		t.Fatal("ordinary edit forced a synchronous workspace rebuild")
	}
}

func TestFailedConflictReloadPreservesOpenViews(t *testing.T) {
	for _, failure := range []string{"volatile", "binary", "missing"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
			if err != nil {
				t.Fatal(err)
			}
			docs := document.NewManager(document.Options{Root: root})
			const path = "scripts/test.aes"
			if err := document.WriteAtomic(docs.Abs(path), []byte("package c\nvar saved = 1\n")); err != nil {
				t.Fatal(err)
			}
			doc, err := docs.OpenScript(path)
			if err != nil {
				t.Fatal(err)
			}
			jobs := &queuedAnalysisJobs{}
			host := NewHost(langsvc.New(ws, jobs))
			ed := &context.Editor{Docs: docs, Commands: command.New()}
			host.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands}, edws.New(root, nil))
			defer host.offDocs()
			panel := host.show(doc)
			defer panel.releaseDoc()
			clone := newPanel(host, doc)
			clone.clone = true
			host.clones["clone"] = clone
			defer clone.releaseDoc()
			panel.ask(1)
			jobs.completeWork(t)
			jobs.deliver(t)
			clone.follow()
			if panel.snap == nil || clone.snap != panel.snap {
				t.Fatal("fixture did not create shared views")
			}
			if failure == "volatile" {
				if err := doc.Code.AttachVolatile(); err != nil {
					t.Fatal(err)
				}
				if changed, err := doc.Code.ReplaceVolatile(doc.Code.Rev(), []byte("working state")); err != nil || !changed {
					t.Fatal("fixture could not create volatile state", err)
				}
			} else if failure == "binary" {
				if err := document.WriteAtomic(docs.Abs(path), []byte{0xff, 0}); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(docs.Abs(path)); err != nil {
				t.Fatal(err)
			}
			panel.conflict, clone.conflict, doc.Conflict = true, true, true
			old, snap, gen, version := doc.Code, panel.snap, host.lang.Current(), ws.Version()
			events := 0
			off := docs.Subscribe(func(ev document.Event) {
				if ev.Kind == document.EvScriptReplaced {
					events++
				}
			})
			defer off()
			ui, state := core.New(nil), &input.State{}
			rect, at := mathx.Rect{W: 400, H: 30}, mathx.Vec2{X: 250, Y: 10}
			draw := func() {
				ui.Begin(state, mathx.Vec2{X: 400, Y: 30}, 1)
				panel.drawConflict(ui, rect)
				ui.End()
			}
			state.BeginFrame()
			state.ApplyMouseMove(at, mathx.Vec2{})
			state.ApplyMouseDown(at, input.MouseLeft)
			draw()
			state.BeginFrame()
			state.ApplyMouseUp(at, input.MouseLeft)
			draw()
			if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
				t.Fatal("failed reload did not report an error")
			}
			if doc.Code != old || !doc.Conflict || !panel.conflict || !clone.conflict || panel.snap != snap || clone.snap != snap || host.liveSnap != snap {
				t.Fatal("failed reload changed the journal, warning, or open views")
			}
			if events != 0 || host.lang.Current() != gen || ws.Version() != version {
				t.Fatal("failed reload emitted replacement or invalidated the workspace")
			}
		})
	}
}

func TestScriptReplacementInvalidatesFindResults(t *testing.T) {
	for _, tc := range []struct {
		name            string
		pending, usages bool
	}{
		{"completed search", false, false},
		{"late search", true, false},
		{"usages", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			docs := document.NewManager(document.Options{Root: root})
			const path = "scripts/target.aes"
			if err := document.WriteAtomic(docs.Abs(path), []byte("package c\nvar needle_old = 1\n")); err != nil {
				t.Fatal(err)
			}
			ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
			if err != nil {
				t.Fatal(err)
			}
			doc, err := docs.OpenScript(path)
			if err != nil {
				t.Fatal(err)
			}
			jobs := &queuedAnalysisJobs{}
			host := NewHost(langsvc.New(ws, jobs))
			ed := &context.Editor{Docs: docs, Commands: command.New()}
			host.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands, Submit: jobs.Submit, OnMain: jobs.OnMain}, edws.New(root, nil))
			defer host.offDocs()
			host.find.start("needle_old")
			host.find.tick(1)
			jobs.completeWork(t)
			jobs.deliver(t)
			if len(host.find.matches) != 1 || len(host.find.rows) == 0 {
				t.Fatal("fixture did not find the old source")
			}
			if tc.pending {
				host.find.changed(-1)
				host.find.tick(2)
				jobs.completeWork(t)
			}
			if tc.usages {
				host.find.showUsages("old symbol", []workspace.Match{{Path: path, Line: 2, Text: "var needle_old = 1"}})
			}
			generation := host.find.gen
			if err := document.WriteAtomic(docs.Abs(path), []byte("package c\nvar needle_new = 2\n")); err != nil {
				t.Fatal(err)
			}
			if err := docs.ReloadScript(doc); err != nil {
				t.Fatal(err)
			}
			if host.find.gen == generation || host.find.running || len(host.find.matches) != 0 || len(host.find.rows) != 0 || host.find.usages || !host.find.dirty {
				t.Fatal("replacement retained stale find or usage results")
			}
			if tc.pending {
				jobs.deliver(t)
				if len(host.find.matches) != 0 || host.find.running {
					t.Fatal("late find results survived replacement")
				}
			}
			host.find.tick(3)
			jobs.completeWork(t)
			jobs.deliver(t)
			if len(host.find.matches) != 0 {
				t.Fatal("repeated search still used the old source")
			}
			host.find.start("needle_new")
			host.find.tick(4)
			jobs.completeWork(t)
			jobs.deliver(t)
			if len(host.find.matches) != 1 || host.find.matches[0].Path != path {
				t.Fatal("fresh search did not find the replacement source")
			}
		})
	}
}
