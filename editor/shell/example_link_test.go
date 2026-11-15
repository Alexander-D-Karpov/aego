package shell

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine"
	"aego/platform"
	"aego/project/remoteexample"
)

func exampleDropApp(t *testing.T, ed *context.Editor, opt Options) *App {
	t.Helper()
	e, err := engine.NewBuilder(engine.Config{Headless: true, ResRoot: t.TempDir(), UserRoot: t.TempDir()}).WithLogger(log.Nop()).Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return &App{eng: e, ctx: ed, opt: opt}
}

// macOS SDL delivers URL requests as DropFile/DropComplete. These synthetic
// events exercise the shell route, not OS registration or physical input.
func TestDroppedExampleLinkKeepsDirtyProjectAndUsesSeparateLauncher(t *testing.T) {
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	root := t.TempDir()
	file := filepath.Join(root, "draft.aes")
	if err := os.WriteFile(file, []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docs := document.NewManager(document.Options{Root: root})
	draft, err := docs.OpenScript("draft.aes")
	if err != nil {
		t.Fatal(err)
	}
	draft.Code.ReplaceSnapshot([]byte("unsaved\n"), false)
	link, err := remoteexample.MakeLink("https://example.invalid/a.json", "scene")
	if err != nil {
		t.Fatal(err)
	}
	var received []string
	a := exampleDropApp(t, &context.Editor{Docs: docs}, Options{OpenExampleLink: func(raw string) error {
		received = append(received, raw)
		return nil
	}})
	a.Event(platform.Event{Kind: platform.EvDropFile, Str: link})
	if len(received) != 0 {
		t.Fatal("drop was dispatched before the complete event")
	}
	a.Event(platform.Event{Kind: platform.EvDropComplete})
	if len(received) != 1 || received[0] != link || len(a.drops) != 0 {
		t.Fatalf("wrong link dispatch: %q, pending %q", received, a.drops)
	}
	if a.quitting || !draft.Dirty() || docs.Script("draft.aes") != draft || string(draft.Code.Bytes()) != "unsaved\n" {
		t.Fatal("opening an example link altered or closed the dirty document")
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "saved\n" {
		t.Fatalf("opening an example link changed its source: %q %v", data, err)
	}
}

func TestDroppedExampleRejectsMalformedOrMixedRequests(t *testing.T) {
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	link, err := remoteexample.MakeLink("https://example.invalid/a.json", "project")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		drops []platform.Event
		fail  bool
	}{
		{"malformed", []platform.Event{{Kind: platform.EvDropFile, Str: "aego://example?manifest=http%3A%2F%2Fexample.invalid"}}, false},
		{"extra-parameter", []platform.Event{{Kind: platform.EvDropFile, Str: link + "&command=run"}}, false},
		{"mixed-file", []platform.Event{{Kind: platform.EvDropFile, Str: link}, {Kind: platform.EvDropFile, Str: "/tmp/other.png"}}, false},
		{"mixed-text", []platform.Event{{Kind: platform.EvDropText, Str: link}, {Kind: platform.EvDropFile, Str: "/tmp/other.png"}}, false},
		{"launcher-failed", []platform.Event{{Kind: platform.EvDropText, Str: link}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			a := exampleDropApp(t, &context.Editor{}, Options{OpenExampleLink: func(string) error {
				calls++
				if tc.fail {
					return errors.New("launcher failed")
				}
				return nil
			}})
			for _, ev := range tc.drops {
				a.Event(ev)
			}
			a.Event(platform.Event{Kind: platform.EvDropComplete})
			want := 0
			if tc.fail {
				want = 1
			}
			if calls != want || len(a.drops) != 0 || a.quitting {
				t.Fatalf("request was forwarded incorrectly: calls=%d pending=%q", calls, a.drops)
			}
			if notice, ok := a.ctx.LastNotice(); !ok || notice.Level != context.LevelError {
				t.Fatalf("rejection was not reported: %+v", notice)
			}
		})
	}
}

func TestOrdinaryDropsRemainAvailableForProjectOrAssetHandling(t *testing.T) {
	a := &App{ctx: &context.Editor{}, drops: []string{"/tmp/example.png"}}
	if a.dropExampleLink() || len(a.drops) != 1 {
		t.Fatal("ordinary file was consumed by example-link handling")
	}
}
