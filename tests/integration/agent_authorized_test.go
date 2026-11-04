//go:build aego_gltest

package integration

import (
	"crypto/rand"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/app/agentui/backend"
	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/agentui"
	"aego/editor/core/command"
	agentpanel "aego/editor/panels/agent"
	"aego/project"
)

func TestAuthorizedAgentExplainsActiveProjectAndCapturesGame(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			if os.Getenv("AEGO_LIVE_EDITOR_PROVIDER") != provider {
				t.Skip("opt in with AEGO_LIVE_EDITOR_PROVIDER=" + provider + " on a machine with a signed-in CLI and GL33")
			}
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			root := copyExample(t, "platformer")
			p, err := project.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			b, err := backend.ForProvider(provider)
			if err != nil {
				t.Fatal(err)
			}
			s, err := edit.Open(p, edit.Config{Hidden: true, NoWatch: true, Logger: log.Nop(), AgentBackend: b}, combatModule{})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if !s.Step() {
				t.Fatal("editor stopped during initialization")
			}
			s.App.SetAgentSettings(agentui.ProviderSettings{Provider: provider, Command: os.Getenv("AEGO_LIVE_EDITOR_CLI"), Model: os.Getenv("AEGO_LIVE_EDITOR_MODEL")})
			ctx := s.App.Context()
			d, err := ctx.Docs.Open(bridgeScene)
			if err != nil {
				t.Fatal(err)
			}
			sc := d.Scene()
			marker := "Pilot" + rand.Text()[:8]
			actor := sc.Find(sc.Root(), "Player")
			if actor.IsZero() {
				t.Fatal("Player missing")
			}
			sc.SetName(actor, marker)
			ctx.Select(sc.ID(actor))
			panel := s.App.Agent()
			s.App.Workspace().Show(agentpanel.ID)
			pump := func() {
				deadline := time.Now().Add(3 * time.Minute)
				for panel.Busy() && time.Now().Before(deadline) {
					s.Step()
					time.Sleep(4 * time.Millisecond)
				}
				if panel.Busy() {
					t.Fatal("provider turn timed out")
				}
			}
			logs := func() []agentui.Entry {
				names, err := agentui.List(root)
				if err != nil || len(names) != 1 {
					t.Fatalf("transcripts: %v %v", names, err)
				}
				entries, err := agentui.Load(root, names[0])
				if err != nil {
					t.Fatal(err)
				}
				return entries
			}
			panel.ExplainScene()
			pump()
			entries := logs()
			seenScript := false
			for _, e := range entries {
				if e.Kind == "result" && e.Tool == "script_read" && !e.Failed {
					seenScript = true
				}
			}
			seenMarker := transcriptHasText(entries, marker)
			if !seenMarker || !seenScript {
				for _, e := range entries {
					if e.Kind == "text" || e.Kind == "call" || e.Kind == "error" {
						t.Logf("transcript: kind=%s tool=%s message=%s delta=%v text=%q", e.Kind, e.Tool, e.MessageID, e.Delta, e.Text)
					}
				}
				t.Fatalf("provider did not name the unsaved actor and read its script: %v %v", seenMarker, seenScript)
			}
			if _, depth := d.History.Top(); depth != 0 {
				t.Fatal("Explain changed History")
			}
			if err := ctx.Commands.Run(command.PlayStart); err != nil {
				t.Fatal(err)
			}
			for range 10 {
				s.Step()
			}
			panel.Submit("Show me what the player sees now. Use editor_view_capture with view=game.")
			pump()
			for _, e := range logs() {
				if e.Kind == "result" && e.Tool == "editor_view_capture" && !e.Failed && len(e.Images) > 0 {
					return
				}
			}
			t.Fatal("no real Game View image returned in this conversation")
		})
	}
}

// Codex streams an answer as separate deltas, which can split a node name.
// Reassemble each message independently instead of searching individual chunks.
func transcriptHasText(entries []agentui.Entry, want string) bool {
	parts := map[string]string{}
	for _, e := range entries {
		if e.Kind != "text" {
			continue
		}
		if !e.Delta {
			if strings.Contains(e.Text, want) {
				return true
			}
			continue
		}
		parts[e.MessageID] += e.Text
		if strings.Contains(parts[e.MessageID], want) {
			return true
		}
	}
	return false
}

func TestTranscriptHasTextAcrossDeltas(t *testing.T) {
	entries := []agentui.Entry{
		{Kind: "text", MessageID: "a", Delta: true, Text: "Pilot"},
		{Kind: "text", MessageID: "b", Delta: true, Text: "other"},
		{Kind: "text", MessageID: "a", Delta: true, Text: "123"},
		{Kind: "text", Text: "complete"},
	}
	if !transcriptHasText(entries, "Pilot123") || !transcriptHasText(entries, "complete") {
		t.Fatal("streamed or complete message was not found")
	}
	if transcriptHasText(entries, "Pilotother") {
		t.Fatal("text from separate messages was merged")
	}
}
