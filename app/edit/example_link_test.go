package edit

import (
	"path/filepath"
	"testing"
	"time"

	"aego/core/log"
	"aego/editor/shell/launcher"
	"aego/platform"
	"aego/project"
	"aego/project/remoteexample"
)

type countingLauncherUpdates struct{ ticks int }

func (u *countingLauncherUpdates) Tick(time.Time) { u.ticks++ }

func TestOSProtocolArgumentsRejectFlagsPathsAndExtraArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"aego://example?manifest=https%3A%2F%2Fexample.com%2Fx", "--tool", "run"},
		{"--tool"},
		{"/tmp/project"},
		{"https://example.com/project.json"},
		{"aego://example?manifest=https%3A%2F%2Fexample.com%2Fx&shell=echo"},
	} {
		if err := StartExampleLink(args); err == nil {
			t.Fatalf("invalid OS-handler arguments accepted: %q", args)
		}
		if err := StartCLIExampleLink(args); err == nil {
			t.Fatalf("invalid CLI OS-handler arguments accepted: %q", args)
		}
	}
}

func TestProtocolLinkLauncherStartsWithoutSelectingProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	link, err := remoteexample.MakeLink("https://example.invalid/example.json", "scene")
	if err != nil {
		t.Fatal(err)
	}
	ls, err := OpenLauncher(Config{Headless: true, ExampleLink: link, Recent: filepath.Join(t.TempDir(), "recent"), Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Close()
	if ls.UI.Tab() != launcher.TabExamples {
		t.Fatal("protocol request did not open the examples confirmation")
	}
	if _, ok := ls.UI.Result(); ok {
		t.Fatal("protocol request selected a project without confirmation")
	}
	if !ls.Step() {
		t.Fatal("launcher stopped before confirmation")
	}
	if _, ok := ls.UI.Result(); ok {
		t.Fatal("rendering the launcher selected a project without confirmation")
	}
}

// This covers the launcher's SDL event route with synthetic headless events.
// macOS Launch Services and an actual browser are separate acceptance gates.
func TestProtocolDropOnOpenLauncherRequiresConfirmation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	link, err := remoteexample.MakeLink("https://example.invalid/demo.json", "scene")
	if err != nil {
		t.Fatal(err)
	}
	ls, err := OpenLauncher(Config{Headless: true, Recent: filepath.Join(t.TempDir(), "recent"), Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Close()
	if !ls.Step() {
		t.Fatal("launcher stopped before the synthetic URL event")
	}
	ls.driver.Event(platform.Event{Kind: platform.EvDropFile, Str: link})
	ls.driver.Event(platform.Event{Kind: platform.EvDropComplete})
	if ls.UI.Tab() != launcher.TabExamples {
		t.Fatal("URL event did not route to example confirmation")
	}
	if _, ok := ls.UI.Result(); ok {
		t.Fatal("URL event selected a project without confirmation")
	}
	if !ls.Step() {
		t.Fatal("launcher stopped while awaiting confirmation")
	}
	if _, ok := ls.UI.Result(); ok {
		t.Fatal("rendering the confirmation selected a project")
	}
}

func TestProtocolConfirmationDefersAutomaticUpdates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("SDL_AUDIO_DRIVER", "dummy")
	t.Setenv("SDL_AUDIODRIVER", "dummy")
	link, err := remoteexample.MakeLink("https://example.invalid/demo.json", "project")
	if err != nil {
		t.Fatal(err)
	}
	ls, err := OpenLauncher(Config{Headless: true, ExampleLink: link, Recent: filepath.Join(t.TempDir(), "recent"), Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Close()
	counter := &countingLauncherUpdates{}
	ls.driver.updates = counter
	if !ls.UI.AwaitingExampleConfirmation() || ls.driver.automaticUpdatesAllowed() {
		t.Fatal("automatic updater was allowed before the requested origin was confirmed")
	}
	if !ls.Step() || counter.ticks != 0 {
		t.Fatal("launcher ticked the automatic updater before confirmation")
	}
	// Choosing another local project explicitly cancels the pending remote
	// request; it does not mutate the saved update policy.
	local, err := project.Create(filepath.Join(t.TempDir(), "local"), "Local", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ls.UI.Open(local.Root); err != nil {
		t.Fatal(err)
	}
	if ls.UI.AwaitingExampleConfirmation() || !ls.driver.automaticUpdatesAllowed() {
		t.Fatal("canceled request continued suppressing ordinary automatic updates")
	}
	_ = ls.Step() // Choosing a project may ask the engine to quit in this frame.
	if counter.ticks != 1 {
		t.Fatalf("automatic updater ticks after explicit cancel: %d", counter.ticks)
	}
}
