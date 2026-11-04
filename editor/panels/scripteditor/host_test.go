package scripteditor

import (
	"os"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/code"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	edws "aego/editor/core/workspace"
	"aego/script/langsvc"
	"aego/script/workspace"
)

func TestStableAssetRenameReusesAndClosesExistingCodeView(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     asset.ArtifactKind
		ext      string
		metadata bool
	}{{"shader source", asset.ArtShader, ".aeshader", false}, {"image metadata", asset.ArtTexture, ".png", true}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
			if err != nil {
				t.Fatal(err)
			}
			docs := document.NewManager(document.Options{Root: root})
			handler := document.Handler{ID: "test.source", Kinds: []asset.ArtifactKind{tc.kind}, Capabilities: document.View | document.EditSource, TextSource: true}
			if tc.metadata {
				handler.Capabilities, handler.TextSource = document.View|document.EditMetadata, false
			}
			if err := docs.Handlers().Register(handler); err != nil {
				t.Fatal(err)
			}
			old, next := "assets/original"+tc.ext, "assets/renamed"+tc.ext
			if err := document.WriteAtomic(docs.Abs(old), []byte("saved source\n")); err != nil {
				t.Fatal(err)
			}
			if tc.metadata {
				if err := document.WriteAtomic(docs.Abs(old+".meta"), []byte("saved metadata\n")); err != nil {
					t.Fatal(err)
				}
			}
			d, err := docs.OpenAsset(document.AssetRequest{Asset: ids.AssetID{Lo: 27}, Kind: tc.kind, Path: old})
			if err != nil {
				t.Fatal(err)
			}
			source := d.Source
			oldCode, newCode := old, next
			if tc.metadata {
				source, oldCode, newCode = d.Metadata, old+".meta", next+".meta"
			}
			ed := &context.Editor{Docs: docs, Commands: command.New()}
			h := NewHost(langsvc.New(ws, nil))
			h.Attach(Deps{Docs: ed, Notices: ed, Commands: ed.Commands}, edws.New(root, nil))
			defer h.offDocs()
			p := h.show(source)
			code := source.Code
			code.ReplaceSnapshot([]byte("unsaved shared edit\n"), false)
			h.back = []spot{{path: oldCode, off: 2}}
			if err := os.Rename(docs.Abs(old), docs.Abs(next)); err != nil {
				t.Fatal(err)
			}
			if tc.metadata {
				if err := os.Rename(docs.Abs(old+".meta"), docs.Abs(next+".meta")); err != nil {
					t.Fatal(err)
				}
			}
			// The file watcher/index discovers the same AssetID at another path;
			// no Asset Browser rename command has notified the Code host.
			again, err := docs.AcquireAsset(document.AssetRequest{Asset: d.Asset, Kind: tc.kind, Path: next})
			if err != nil || again != d || source.Code != code || !source.Dirty() || !code.CanUndo() {
				t.Fatal("stable rename lost the shared journal", err)
			}
			if h.panels[oldCode] != nil || h.panels[newCode] != p || !h.owns(p) || h.show(source) != p || len(h.panels) != 1 {
				t.Fatal("stable rename left a stale path key or duplicated the Code view")
			}
			if h.back[0].path != newCode {
				t.Fatal("navigation retained the old source path")
			}
			h.close(p)
			if h.dock.Has(p.ID()) || len(h.panels) != 0 {
				t.Fatal("renamed Code view could not close")
			}
			if err := docs.CloseAsset(d.Asset, true); err != nil || len(docs.Scripts()) != 0 {
				t.Fatal("renamed source retained an orphaned view reference", err)
			}
		})
	}
}

func TestFindInScriptsShortcutIsUnique(t *testing.T) {
	ws, err := workspace.Open(workspace.Config{Root: t.TempDir(), Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHost(langsvc.New(ws, nil))
	ed := &context.Editor{Commands: command.New()}
	h.d = Deps{Commands: ed.Commands, Notices: ed}
	h.registerCommands()
	want, ok := h.d.Commands.Get(CommandFindInScripts)
	if !ok || want.Shortcut.String() == "" {
		t.Fatal("Find in Scripts has no shortcut")
	}
	n := 0
	for _, d := range h.d.Commands.All() {
		if d.Shortcut.String() == want.Shortcut.String() {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d commands share %s", n, want.Shortcut.String())
	}
}

func TestForeignSourceNeverEntersAegoLanguageWorkspace(t *testing.T) {
	ws, err := workspace.Open(workspace.Config{Root: t.TempDir(), Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHost(langsvc.New(ws, nil))
	for _, name := range []string{"assets/shader.aeshader", "assets/include.glsl", "assets/source.aemat"} {
		d := &document.ScriptDocument{Path: name, Code: code.New([]byte("void fragment() { aego_out = vec4(1.); }"))}
		d.Code.GLSL = name != "assets/source.aemat"
		p := newPanel(h, d)
		p.push()
		if _, ok := ws.FileID(name); ok {
			t.Fatal("GLSL or material source polluted the Aego script compiler", name)
		}
		if _, ok := p.file(); ok {
			t.Fatal("foreign source enabled Aego completion/diagnostics", name)
		}
	}
}

func TestTabTitleAddsFolderOnCollision(t *testing.T) {
	h := &Host{panels: map[string]*Panel{"scripts/player/move.aes": nil, "scripts/enemy/move.aes": nil, "scripts/ui/menu.aes": nil}}
	if got := h.tabTitle("scripts/player/move.aes"); got != "move — player" {
		t.Fatalf("collision title = %q", got)
	}
	if got := h.tabTitle("scripts/ui/menu.aes"); got != "menu" {
		t.Fatalf("plain title = %q", got)
	}
}

func TestDiscardCodeTabUpdatesTheSharedAssetJournal(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.Open(workspace.Config{Root: root, Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	docs := document.NewManager(document.Options{Root: root})
	if err := docs.Handlers().Register(document.Handler{ID: "shader", Kinds: []asset.ArtifactKind{asset.ArtShader}, TextSource: true, Capabilities: document.View | document.EditSource}); err != nil {
		t.Fatal(err)
	}
	const path = "assets/shared.aeshader"
	if err := document.WriteAtomic(docs.Abs(path), []byte("saved shader\n")); err != nil {
		t.Fatal(err)
	}
	d, err := docs.OpenAsset(document.AssetRequest{Asset: ids.AssetID{Lo: 11}, Kind: asset.ArtShader, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	ed := &context.Editor{Docs: docs}
	h := NewHost(langsvc.New(ws, nil))
	h.d, h.dock = Deps{Docs: ed, Notices: ed}, edws.New(root, nil)
	p := h.show(d.Source)
	d.Source.Code.ReplaceSnapshot([]byte("unsaved shader\n"), false)
	h.requestClose(p)
	drain := func() {
		for len(h.queue) > 0 {
			queue := h.queue
			h.queue = nil
			for _, fn := range queue {
				fn()
			}
		}
	}
	drain()
	questions := ed.TakeQuestions()
	if len(questions) != 1 {
		t.Fatal("closing dirty Code view did not ask what to do with shared edits")
	}
	questions[0].Then(1)
	drain()
	if docs.Script(path) != d.Source || d.Dirty() || string(d.Source.Code.Bytes()) != "saved shader\n" {
		t.Fatal("Discard hid a still-dirty shared shader buffer")
	}
	if h.dock.Has(p.ID()) {
		t.Fatal("discarded Code tab remained registered")
	}
	if reopened := h.show(d.Source); reopened.doc != d.Source {
		t.Fatal("Code reopen created a second journal")
	} else {
		h.close(reopened)
	}
}
