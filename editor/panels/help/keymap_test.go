package help

import (
	"strings"
	"testing"

	"aego/editor/core/command"
	"aego/platform/input"
)

func TestKeymapShowsBothSearchBindings(t *testing.T) {
	reg := command.New()
	err := reg.Register(command.Desc{
		ID: "search.everywhere", Name: "Search Everywhere", Category: "Edit",
		Shortcut: command.Key(input.KeyP, command.ModPrimary|command.ModShift), Hint: "Shift Shift",
		Run: func() error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	k := NewKeymap(reg)
	k.build()
	for _, r := range k.rows {
		if r.name != "Search Everywhere" {
			continue
		}
		if !strings.Contains(r.keys, "Shift Shift") || !strings.Contains(r.keys, "P") {
			t.Fatalf("keys %q", r.keys)
		}
		return
	}
	t.Fatal("Search Everywhere is not in the keymap")
}
