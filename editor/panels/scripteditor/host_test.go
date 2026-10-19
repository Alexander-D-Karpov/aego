package scripteditor

import (
	"testing"

	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/script/langsvc"
	"aego/script/workspace"
)

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

func TestTabTitleAddsFolderOnCollision(t *testing.T) {
	h := &Host{panels: map[string]*Panel{"scripts/player/move.aes": nil, "scripts/enemy/move.aes": nil, "scripts/ui/menu.aes": nil}}
	if got := h.tabTitle("scripts/player/move.aes"); got != "move — player" {
		t.Fatalf("collision title = %q", got)
	}
	if got := h.tabTitle("scripts/ui/menu.aes"); got != "menu" {
		t.Fatalf("plain title = %q", got)
	}
}
