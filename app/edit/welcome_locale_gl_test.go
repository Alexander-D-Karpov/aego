//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/play"
	"aego/editor/panels/welcome"
	"aego/editor/shell"
	"aego/engine/docs"
	"aego/engine/i18n"
	"aego/ui/draw"
)

// Synthetic SDL/GL coverage of actual startup and language commands. The
// project and preferences are test-owned, and the runner must use dummy audio.
func TestWelcomeLocaleNativeStartup(t *testing.T) {
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	for _, language := range []string{i18n.Russian, i18n.English} {
		t.Run(language, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			t.Setenv("AEGO_LANG", language)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			s, err := Open(platformerAuthoringProject(t), Config{Hidden: true, NoWatch: true, Width: 1280, Height: 900, Logger: log.Nop()}, platformerAuthoringModule{})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			step := func() {
				t.Helper()
				if !s.Step() {
					t.Fatal("native localized Welcome startup stopped")
				}
			}
			step()
			if i18n.Default().Language() != language {
				t.Fatal("startup did not apply its requested locale")
			}
			ed, ws := s.App.Context(), s.App.Workspace()
			if err := ed.Commands.Run(shell.CommandGettingStarted); err != nil {
				t.Fatal(err)
			}
			ws.Focus(welcome.ID)
			if !ws.ToggleMaximize(welcome.ID) {
				t.Fatal("could not maximize Welcome")
			}
			for range 3 {
				step()
			}
			ui := ed.UI
			screen := mathx.Rect{W: ui.Size().X, H: ui.Size().Y}
			visible := func(item draw.TestDrawnItem) bool {
				r, c := item.Bounds, item.Clip
				return r.X >= c.X-0.1 && r.Y >= c.Y-0.1 && r.X+r.W <= c.X+c.W+0.1 && r.Y+r.H <= c.Y+c.H+0.1
			}
			assertLabel := func(label string) {
				t.Helper()
				for _, item := range s.App.UIList().TestDrawnItems(screen, ui.Font(), label, 0, 0) {
					if item.Text == label && visible(item) {
						return
					}
				}
				t.Fatalf("localized label missing or clipped: %q", label)
			}
			assertBody := func(body string) {
				t.Helper()
				remaining := strings.Join(strings.Fields(body), " ")
				for remaining != "" {
					first := strings.Fields(remaining)[0]
					longest := ""
					for _, item := range s.App.UIList().TestDrawnItems(screen, ui.Font(), first, 0, 0) {
						line := strings.Join(strings.Fields(item.Text), " ")
						if visible(item) && strings.HasPrefix(remaining, line) && len(line) > len(longest) {
							longest = line
						}
					}
					if longest == "" {
						t.Fatalf("localized body missing or clipped: %q", remaining)
					}
					remaining = strings.TrimSpace(remaining[len(longest):])
				}
			}
			verify := func(current, stage string) {
				t.Helper()
				if i18n.Default().Language() != current {
					t.Fatalf("incorrect current language: %s", current)
				}
				heading := "Getting started with Aego"
				if current == i18n.Russian {
					heading = "Начало работы с Aego"
				}
				assertLabel(heading)
				for index, id := range []string{"node", "attach", "play", "tweak", "reload", "error"} {
					entry, ok := s.Docs.Get("welcome/" + id)
					if !ok {
						t.Fatal("canonical Welcome documentation missing", id)
					}
					title, body := entry.Title, entry.Body
					if current == i18n.Russian {
						tr, ok := entry.Translations[i18n.Russian]
						if !ok || tr.Title == "" || tr.Body == "" || tr.Title == title || tr.Body == body {
							t.Fatal("real Russian translation missing", id)
						}
						title, body = tr.Title, tr.Body
					} else if tr, ok := entry.Translations[i18n.English]; ok && (tr.Title != "" || tr.Body != "") {
						t.Fatal("English no longer tests source fallback", id)
					}
					assertLabel(fmt.Sprintf("%d. %s", index+1, title))
					assertBody(docs.Plain(body))
				}
				if dir := os.Getenv("AEGO_WELCOME_LOCALE_SHOTS"); dir != "" {
					var pixels *image.RGBA
					var captureErr error
					s.Engine.CaptureFrame(func(img *image.RGBA, err error) { pixels, captureErr = img, err })
					step()
					if captureErr != nil || pixels == nil {
						t.Fatal("Welcome GPU capture failed", captureErr)
					}
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
					f, err := os.OpenFile(filepath.Join(dir, language+"-"+stage+".png"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, pixels)
					if closeErr := f.Close(); err != nil || closeErr != nil {
						t.Fatal(err, closeErr)
					}
				}
			}
			verify(language, "startup")
			if _, err := ed.Docs.Open("scenes/main.aescene"); err != nil {
				t.Fatal(err)
			}
			if err := ed.Commands.Run(command.PlayStart); err != nil {
				t.Fatal(err)
			}
			progressPath := filepath.Join(ed.Project.Root, ".aego", "user", "getting-started")
			deadline := time.Now().Add(5 * time.Second)
			var progress []byte
			for {
				step()
				if s.App.Play().State() != play.Playing {
					t.Fatal("Play stopped before Welcome observed its action")
				}
				progress, err = os.ReadFile(progressPath)
				if err == nil && strings.Contains(string(progress), "play\n") {
					break
				}
				if err != nil && !os.IsNotExist(err) || time.Now().After(deadline) {
					t.Fatalf("Welcome did not record Play: %v", err)
				}
			}
			if err := ed.Commands.Run(command.PlayStop); err != nil {
				t.Fatal(err)
			}
			step()
			ws.Focus(welcome.ID)
			if maximized := ws.Tree.Maximized(); maximized != welcome.ID {
				if maximized != "" {
					ws.ToggleMaximize(maximized)
				}
				ws.ToggleMaximize(welcome.ID)
			}
			completed := len(strings.Fields(string(progress)))
			other := i18n.English
			if language == other {
				other = i18n.Russian
			}
			t.Setenv("AEGO_LANG", "")
			for index, next := range []string{other, language} {
				if err := ed.Commands.Run("window.language." + next); err != nil {
					t.Fatal(err)
				}
				for range 3 {
					step()
				}
				verify(next, fmt.Sprintf("switch-%d", index+1))
				assertLabel(i18n.Tr("editor.welcome.n_of_checks_done", "{n} of {checks} done", i18n.Str("n", strconv.Itoa(completed)), i18n.Str("checks", "6")))
				current, err := os.ReadFile(progressPath)
				if err != nil || !bytes.Equal(current, progress) {
					t.Fatal("language switch changed completed checklist", err)
				}
			}
		})
	}
}
