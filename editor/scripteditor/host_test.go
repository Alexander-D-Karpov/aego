package scripteditor

import (
	"testing"

	"aego/editor/command"
	"aego/editor/context"
	"aego/script/langsvc"
	"aego/script/workspace"
)

func TestFindInScriptsShortcutIsUnique(t *testing.T) {
	ws, err := workspace.Open(workspace.Config{Root: t.TempDir(), Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHost(langsvc.New(ws, nil))
	h.ctx = &context.Editor{Commands: command.New()}
	h.registerCommands()
	want, ok := h.ctx.Commands.Get(CommandFindInScripts)
	if !ok || want.Shortcut.String() == "" {
		t.Fatal("Find in Scripts has no shortcut")
	}
	n := 0
	for _, d := range h.ctx.Commands.All() {
		if d.Shortcut.String() == want.Shortcut.String() {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d commands share %s", n, want.Shortcut.String())
	}
}
