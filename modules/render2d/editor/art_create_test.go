package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
	"aego/core/ids"
	"aego/editor/core/assetfiles"
	"aego/editor/core/assetops"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
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
		ed := &context.Editor{}
		r := ext.New(ext.Deps{Editor: ed, Commands: commands})
		if err := r.Run("render2d", registerArtCreate); err != nil {
			t.Fatal(err)
		}
		if _, ok := commands.Get("asset.new_art"); !ok {
			t.Fatal("new art command not registered")
		}
		if commands.Enabled("asset.new_art") {
			t.Fatal("command must be disabled without a pipeline and documents")
		}
		if len(ed.CreateActions) != 2 || ed.CreateActions[0].RunInFolder == nil || ed.CreateActions[1].RunInFolder == nil {
			t.Fatal("pixel art or PNG is missing from Create or the Assets folder menu")
		}
		r.Disable("render2d", "test")
		r.Flush()
		if _, ok := commands.Get("asset.new_art"); ok {
			t.Fatal("disabled extension left its command registered")
		}
		if len(ed.CreateActions) != 0 {
			t.Fatal("disabled extension left its Create action registered")
		}
	})
}

func TestCreatePixelArtFromPaletteAndSelectedFolder(t *testing.T) {
	ed, _, p := importTestEditor(t)
	var action *context.CreateAction
	for i := range ed.CreateActions {
		if ed.CreateActions[i].Key == "render2d.pixel_art" {
			action = &ed.CreateActions[i]
		}
	}
	if action == nil || action.RunInFolder == nil {
		t.Fatal("pixel art Create actions are missing")
	}
	if err := action.Run(ids.NodeID{}); err != nil {
		t.Fatal(err)
	}
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].Values[0] != "assets/sprite.aeart" {
		t.Fatalf("Create palette did not open the art dialog: %+v", q)
	}
	if err := os.MkdirAll(filepath.Join(p.Root, "assets", "selected"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := action.RunInFolder("assets/selected"); err != nil {
		t.Fatal(err)
	}
	q = ed.TakeQuestions()
	if len(q) != 1 || q[0].Values[0] != "assets/selected/sprite.aeart" {
		t.Fatalf("folder action did not use the selected folder: %+v", q)
	}
	usedHistory := false
	ed.CreateAsset = func(rel string, data []byte) (ids.AssetID, error) {
		usedHistory = true
		return assetops.CreateFile(ed, rel, data)
	}
	q[0].Submit([]string{"assets/selected/hero.aeart", "24", "16", "rgba"})
	data, err := os.ReadFile(filepath.Join(p.Root, "assets", "selected", "hero.aeart"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := art.Decode(data, art.DefaultLimits())
	if err != nil {
		t.Fatalf("created file is not a valid art source: %v", err)
	}
	defer d.Close()
	if c := d.Info().Canvas; c.Width != 24 || c.Height != 16 || !usedHistory {
		t.Fatalf("canvas=%+v, used shared creation history=%v", c, usedHistory)
	}
	id, ok := ed.Pipeline.Lookup("assets/selected/hero.aeart")
	if !ok || ed.Docs.AssetDocument(id) == nil {
		t.Fatal("new drawing was not imported and opened")
	}
}

func TestEmptyArtImportReportsFailureAndKeepsFile(t *testing.T) {
	ed, _, p := importTestEditor(t)
	input := filepath.Join(t.TempDir(), "manual.aeart")
	if err := os.WriteFile(input, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := assetfiles.New(ed).ImportExternal([]string{input}, "assets")
	if err == nil || !strings.Contains(err.Error(), "empty .aeart") || len(created) != 1 {
		t.Fatalf("empty import must report its real failure: created=%v err=%v", created, err)
	}
	for _, file := range []string{input, filepath.Join(p.Root, "assets", "manual.aeart")} {
		if data, err := os.ReadFile(file); err != nil || len(data) != 0 {
			t.Fatalf("import changed %s: %q, %v", file, data, err)
		}
	}
}

func TestDroppingAsepriteCreatesEditableCopyWithoutOverwriting(t *testing.T) {
	ed, _, p := importTestEditor(t)
	source, original := importTestSource(t, "tags3", ".aseprite")
	existing := filepath.Join(p.Root, "assets", "tags3.aeart")
	if err := os.WriteFile(existing, []byte("existing user file"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := assetfiles.New(ed).ImportExternal([]string{source}, "assets")
	if err != nil || len(created) != 1 {
		t.Fatalf("dropping Aseprite: ids=%v err=%v", created, err)
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original Aseprite was changed: %v", err)
	}
	if got, err := os.ReadFile(existing); err != nil || string(got) != "existing user file" {
		t.Fatalf("existing project source was overwritten: %q, %v", got, err)
	}
	rel, ok := ed.Pipeline.PathOf(created[0])
	if !ok || rel != "assets/tags3 2.aeart" {
		t.Fatalf("editable copy path = %q", rel)
	}
	doc, err := ed.OpenAsset(created[0])
	if err != nil || doc.Handler != artHandlerID {
		t.Fatalf("converted source did not open in Art Studio: %+v, %v", doc, err)
	}
	data, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	d, err := art.Decode(data, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if len(d.Info().Frames) != 12 || len(d.Info().Tags) != 3 {
		t.Fatalf("converted source lost the expected frames or tags: %+v", d.Info())
	}
}

func TestDroppingAsepriteWithWarningsDoesNotConvertSilently(t *testing.T) {
	ed, _, p := importTestEditor(t)
	source, original := importTestSource(t, "tags3x123reps", ".aseprite")
	target := filepath.Join(p.Root, "assets", "tags3x123reps.aeart")
	start := func() context.Question {
		t.Helper()
		created, err := assetfiles.New(ed).ImportExternal([]string{source}, "assets")
		if err != nil || len(created) != 0 {
			t.Fatalf("warning-bearing import must wait for consent: ids=%v err=%v", created, err)
		}
		q := ed.TakeQuestions()
		if len(q) != 1 || q[0].Then == nil || !strings.Contains(q[0].Message, "tag-repeat") {
			t.Fatalf("import did not expose the actual diagnostics: %+v", q)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("warning-bearing drop created a source without confirmation: %v", err)
		}
		return q[0]
	}
	start().Then(1)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("cancel created an asset")
	}
	q := start()
	if err := os.WriteFile(source, []byte("changed after review"), 0o600); err != nil {
		t.Fatal(err)
	}
	q.Then(0)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("changed source was imported from an old snapshot")
	}
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	q = start()
	if err := os.WriteFile(target, []byte("someone else's work"), 0o600); err != nil {
		t.Fatal(err)
	}
	q.Then(0)
	if got, err := os.ReadFile(target); err != nil || string(got) != "someone else's work" {
		t.Fatalf("confirmation overwrote a concurrently created target: %q, %v", got, err)
	}
	// The next unique path is confirmed explicitly; keep the existing target.
	created, err := assetfiles.New(ed).ImportExternal([]string{source}, "assets")
	if err != nil || len(created) != 0 {
		t.Fatalf("second target must also await consent: %v, %v", created, err)
	}
	qs := ed.TakeQuestions()
	if len(qs) != 1 || qs[0].Then == nil {
		t.Fatal("second target did not ask for consent")
	}
	qs[0].Then(0)
	if _, ok := ed.Pipeline.Lookup("assets/tags3x123reps 2.aeart"); !ok {
		t.Fatal("confirmed import did not create the editable copy")
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original Aseprite was changed: %v", err)
	}
}

func TestInitializeEmptyArtNeedsConfirmationAndGuardsChanges(t *testing.T) {
	for _, tc := range []struct {
		name      string
		before    []byte
		changed   bool
		collision bool
	}{
		{name: "empty", before: []byte{}},
		{name: "newline and existing backup", before: []byte("\n"), collision: true},
		{name: "changed while dialog is open", before: []byte("\n"), changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ed, _, p := importTestEditor(t)
			file := filepath.Join(p.Root, "assets", "empty.aeart")
			if err := os.WriteFile(file, tc.before, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ed.Pipeline.Scan(); err != nil {
				t.Fatal(err)
			}
			id, ok := ed.Pipeline.Lookup("assets/empty.aeart")
			if !ok {
				t.Fatal("empty source was not indexed")
			}
			batch, err := ed.Pipeline.Import(id)
			if err != nil {
				t.Fatal(err)
			}
			if len(tc.before) == 0 && (len(batch.Retry) != 1 || batch.Retry[0] != id) {
				t.Fatalf("empty source should be retried by the pipeline: %+v", batch)
			}
			if len(tc.before) > 0 && batch.Failed[id] == nil {
				t.Fatalf("blank text should report an invalid source: %+v", batch)
			}
			doc, err := ed.Docs.AcquireAsset(document.AssetRequest{Asset: id, Path: "assets/empty.aeart"})
			if err != nil {
				t.Fatal(err)
			}
			panel := &artPanel{ctx: ed, doc: doc, view: &artView{}, empty: true, blankBefore: append([]byte{}, tc.before...)}
			backup := filepath.Join(p.Root, ".aego", "art-blank-"+id.String()+".original")
			if tc.collision {
				if err := os.WriteFile(backup, []byte("existing backup"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			panel.askInitializeEmpty()
			q := ed.TakeQuestions()
			if len(q) != 1 || q[0].Submit == nil {
				t.Fatal("initialization did not ask for confirmation")
			}
			if data, _ := os.ReadFile(file); !bytes.Equal(data, tc.before) {
				t.Fatal("file was changed before confirmation")
			}
			if tc.changed {
				if err := os.WriteFile(file, []byte("other work"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			q[0].Submit([]string{"20", "14", "rgba"})
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if tc.changed {
				if !bytes.Equal(data, []byte("other work")) || panel.model != nil {
					t.Fatal("initialization overwrote a concurrently changed file")
				}
				return
			}
			if tc.collision {
				if original, err := os.ReadFile(backup); err != nil || string(original) != "existing backup" {
					t.Fatal("initialization overwrote an existing backup")
				}
				backup = filepath.Join(p.Root, ".aego", "art-blank-"+id.String()+"-2.original")
			}
			if len(tc.before) > 0 {
				if saved, err := os.ReadFile(backup); err != nil || !bytes.Equal(saved, tc.before) {
					t.Fatalf("original blank source was not preserved: %q, %v", saved, err)
				}
			}
			if panel.model == nil || panel.err != nil {
				t.Fatalf("initialized file did not open: %v", panel.err)
			}
			if again, ok := ed.Pipeline.Lookup("assets/empty.aeart"); !ok || again != id {
				t.Fatal("initialization changed the existing AssetID")
			}
			d, err := art.Decode(data, art.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if c := d.Info().Canvas; c.Width != 20 || c.Height != 14 {
				t.Fatalf("unexpected initialized canvas: %+v", c)
			}
		})
	}
}

func TestBlankArtSourceDetection(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		want bool
	}{
		{nil, false}, {[]byte{}, true}, {[]byte("\n \t"), true},
		{[]byte("not empty"), false}, {[]byte{0}, false}, {bytes.Repeat([]byte{' '}, 4097), false},
	} {
		if got := isBlankArtSource(tc.data); got != tc.want {
			t.Fatalf("isBlankArtSource(%q) = %v, want %v", tc.data, got, tc.want)
		}
	}
}
