package editor

import (
	"testing"

	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/ext"
)

func TestArtCreateCommandUsesExtensionRegistryAvailability(t *testing.T) {
	t.Run("without commands", func(t *testing.T) {
		r := ext.New(ext.Deps{Editor: &context.Editor{Commands: command.New()}})
		if err := r.Run("render2d", registerArtCreate); err != nil {
			t.Fatalf("optional command service: %v", err)
		}
	})
	t.Run("with commands", func(t *testing.T) {
		commands := command.New()
		// The extension's dependencies, not Editor.Commands, own registrations.
		r := ext.New(ext.Deps{Editor: &context.Editor{}, Commands: commands})
		if err := r.Run("render2d", registerArtCreate); err != nil {
			t.Fatal(err)
		}
		if _, ok := commands.Get("asset.new_art"); !ok {
			t.Fatal("new art command not registered")
		}
		if commands.Enabled("asset.new_art") {
			t.Fatal("command must be disabled without a pipeline and documents")
		}
		r.Disable("render2d", "test")
		r.Flush()
		if _, ok := commands.Get("asset.new_art"); ok {
			t.Fatal("disabled extension left its command registered")
		}
	})
}
