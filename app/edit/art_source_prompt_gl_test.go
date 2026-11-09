//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/artedit"
	edcontext "aego/editor/core/context"
	"aego/editor/core/document"
	"aego/modules/render2d"
	"aego/platform/sdl"
	"aego/scene"
	"aego/ui/draw"
	"aego/ui/widgets"
)

// This uses the real Player PNG authoring operation to open the existing
// Question modal. SDL pointer/text events cancel and confirm it; rendered
// text bounds prove that every warning line and both actions are accessible.
func TestLinkedSourcePromptNativeLayout(t *testing.T) {
	for _, spec := range []struct {
		language      string
		width, height int
	}{
		{"en", 1280, 900}, {"ru", 1280, 900},
		{"en", 360, 540}, {"ru", 360, 540}, {"ru", 360, 220},
	} {
		name := fmt.Sprintf("%s-%dx%d", spec.language, spec.width, spec.height)
		t.Run(name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			t.Setenv("AEGO_LANG", spec.language)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			p := platformerAuthoringProject(t)
			s, err := Open(p, Config{Hidden: true, NoWatch: true, Width: spec.width, Height: spec.height, Logger: log.Nop()}, platformerAuthoringModule{})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			step := func() {
				t.Helper()
				if !s.Step() {
					t.Fatal("native source prompt stopped")
				}
			}
			step()
			ed := s.App.Context()
			doc, err := ed.Docs.Open("scenes/main.aescene")
			if err != nil {
				t.Fatal(err)
			}
			sc := doc.Scene()
			player := sc.Find(sc.Root(), "Player")
			sprite, ok := scene.Get[render2d.SpriteRenderer](sc, player)
			if !ok || sprite.Sprite.ID.IsZero() {
				t.Fatal("canonical Player source is missing")
			}
			originalRef := sprite.Sprite.ID
			ed.Select(sc.ID(player))
			for range 3 {
				step()
			}
			ui := ed.UI
			if ui.Size().X != float32(spec.width) || ui.Size().Y != float32(spec.height) {
				t.Fatalf("requested native viewport was not used: %+v", ui.Size())
			}
			screen := mathx.Rect{W: ui.Size().X, H: ui.Size().Y}
			originalPNG, err := ed.Pipeline.SourceBytes("assets/surfaces/runner.png")
			if err != nil {
				t.Fatal(err)
			}
			open := func() edcontext.Question {
				t.Helper()
				if err := ed.EditSourceArt(originalRef); err != nil {
					t.Fatal(err)
				}
				root, ok := ed.Pipeline.Lookup("assets/surfaces/runner.png")
				if !ok || ed.Docs.AssetDocument(root) == nil {
					t.Fatal("opening the PNG did not create a draft")
				}
				if err := ed.SaveAsset(ed.Docs.AssetDocument(root)); !errors.Is(err, document.ErrSavePending) {
					t.Fatalf("first Save did not wait for confirmation: %v", err)
				}
				questions := ed.TakeQuestions()
				if len(questions) != 1 || len(questions[0].Fields) != 1 || len(questions[0].Buttons) != 2 || questions[0].Submit == nil {
					t.Fatalf("PNG source did not request its normal authoring confirmation: %+v", questions)
				}
				q := questions[0]
				if spec.language == "ru" && q.Title != "Сохранить редактируемый исходник" {
					t.Fatalf("Russian native prompt fell back to another locale: %q", q.Title)
				}
				ed.Ask(q)
				step()
				step()
				if !ui.ModalActive() || ui.Focus() == 0 {
					t.Fatal("authoring prompt did not retain native text focus")
				}
				return q
			}
			capture := func(suffix string) {
				t.Helper()
				dir := os.Getenv("AEGO_ART_PROMPT_SHOTS")
				if dir == "" {
					return
				}
				var pixels *image.RGBA
				var captureErr error
				s.Engine.CaptureFrame(func(img *image.RGBA, err error) { pixels, captureErr = img, err })
				step()
				if captureErr != nil || pixels == nil {
					t.Fatal("native prompt GPU capture failed", captureErr)
				}
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, name+"-"+suffix+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, pixels)
				if closeErr := f.Close(); err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
			}
			inside := func(item draw.TestDrawnItem) bool {
				r, c := item.Bounds, item.Clip
				return r.X >= c.X-0.1 && r.Y >= c.Y-0.1 && r.X+r.W <= c.X+c.W+0.1 && r.Y+r.H <= c.Y+c.H+0.1
			}
			button := func(label string) draw.TestDrawnItem {
				t.Helper()
				var found draw.TestDrawnItem
				for _, item := range s.App.UIList().TestDrawnItems(screen, ui.Font(), label, 0, 0) {
					if item.Text == label && inside(item) && item.Bounds.Y > found.Bounds.Y {
						found = item
					}
				}
				if found.Text == "" {
					t.Fatalf("full native action label is clipped or missing: %q", label)
				}
				return found
			}
			click := func(at mathx.Vec2) {
				t.Helper()
				for phase := range 3 {
					if !sdl.TestPointerEvent(at.X, at.Y, phase) {
						t.Fatal("native prompt pointer event failed")
					}
					step()
				}
			}
			q := open()
			capture("warning")
			var body mathx.Rect
			firstWord := strings.Fields(q.Message)[0]
			prefixLength := 0
			for _, item := range s.App.UIList().TestDrawnItems(screen, ui.Font(), firstWord, 0, 0) {
				// The English main menu also contains "Create". Pick the full
				// first warning line, never that unrelated one-word label.
				if strings.HasPrefix(q.Message, item.Text) && inside(item) && len(item.Text) > prefixLength {
					body = item.Clip
					prefixLength = len(item.Text)
				}
			}
			if body.W <= 0 || body.H <= 0 || body == screen {
				t.Fatal("the shared-asset warning is clipped at its first line")
			}
			lines := widgets.WrapLines(ui, q.Message, body.W)
			seen := map[string]bool{}
			for attempt := 0; attempt < 30; attempt++ {
				for _, line := range lines {
					for _, item := range s.App.UIList().TestDrawnItems(screen, ui.Font(), line, 0, 0) {
						if item.Text == line && item.Clip == body && inside(item) {
							seen[line] = true
						}
					}
				}
				// Actions stay usable even while the warning and path scroll.
				primary, cancel := button(q.Buttons[0]), button(q.Buttons[1])
				a, b := primary.Bounds, cancel.Bounds
				if a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y {
					t.Fatal("full confirmation and cancellation labels overlap")
				}
				if len(seen) == len(lines) {
					break
				}
				at := body.Center()
				before := *ui.ModalScroll()
				if !sdl.TestPointerEvent(at.X, at.Y, 0) || !sdl.TestWheelEvent(at.X, at.Y, -0.5) {
					t.Fatal("native prompt warning scroll failed")
				}
				step()
				if *ui.ModalScroll() == before {
					t.Fatal("unread warning lines cannot be reached by native scrolling")
				}
			}
			if len(lines) < 2 || len(seen) != len(lines) {
				t.Fatalf("shared-asset warning lost rendered lines: %d of %d", len(seen), len(lines))
			}
			capture("readable-actions")
			click(button(q.Buttons[1]).Bounds.Center())
			if ui.ModalActive() {
				t.Fatal("native Cancel did not close the prompt")
			}
			if _, ok := ed.Pipeline.Lookup(q.Values[0]); ok {
				t.Fatal("Cancel created an authoring source")
			}
			q = open()
			target := "assets/surfaces/dialog-" + name + ".aeart"
			// Match normal typed input rather than overflowing the existing
			// bounded per-frame SDL input buffer with a single long event.
			for start := 0; start < len(target); start += 16 {
				if !sdl.TestTextInput(target[start:min(start+16, len(target))]) {
					t.Fatal("native path input failed")
				}
				step()
			}
			capture("edited-path")
			click(button(q.Buttons[0]).Bounds.Center())
			id, ok := ed.Pipeline.Lookup(target)
			if !ok || ui.ModalActive() {
				notice, _ := ed.LastNotice()
				t.Fatalf("native confirmation did not create the edited path: %+v", notice)
			}
			artDoc := ed.Docs.AssetDocument(id)
			if artDoc == nil {
				t.Fatal("confirmed art document did not open")
			}
			model, ok := artDoc.Binding.Model.(*artedit.Document)
			if !ok || len(model.Session.Info().Document.Frames) != 8 {
				t.Fatal("confirmed Player source is not its real eight-frame art document")
			}
			current, err := ed.Pipeline.SourceBytes("assets/surfaces/runner.png")
			if err != nil || !bytes.Equal(current, originalPNG) || sprite.Sprite.ID != originalRef {
				t.Fatal("authoring confirmation changed the original PNG or Player AssetRef", err)
			}
			if dir := os.Getenv("AEGO_ART_PROMPT_SHOTS"); dir != "" {
				receipt := struct {
					Locale, Message, CreatedSource string
					Width, Height, VisibleLines    int
					WarningBody                    mathx.Rect
					WarningLines                   []string
					CancelThenCreate               bool
				}{spec.language, q.Message, target, spec.width, spec.height, len(seen), body, lines, true}
				data, err := json.MarshalIndent(receipt, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name+".json"), append(data, '\n'), 0644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
