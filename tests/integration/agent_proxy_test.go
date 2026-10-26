package integration

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/app/agentui/backend"
	"aego/app/edit"
	"aego/editor/core/document"
	"aego/editor/core/follow"
	"aego/editor/shell"
	"aego/project"
)

// proxySession is an editor open on the platformer with its proxy running, as the CLI finds it.
func proxySession(t *testing.T) (*edit.Session, *project.Project) {
	t.Helper()
	s, root := agentSession(t, &backend.Fake{})
	info, ok := document.ReadLock(root)
	if !ok || info.Proxy == "" || info.Token == "" {
		t.Fatalf("the editor lock names no proxy: %+v", info)
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return s, p
}

// background runs fn while the editor keeps stepping, as the editor's main loop would.
func background(t *testing.T, s *edit.Session, fn func(), each func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	stepUntil(t, s, func() bool {
		if each != nil {
			each()
		}
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
}

func lastStep(t *testing.T, s *edit.Session) string {
	t.Helper()
	undo, _ := s.App.Context().Docs.Find(bridgeScene).History.Labels(1)
	if len(undo) == 0 {
		return ""
	}
	return undo[0]
}

// With the editor open, a CLI command goes to it: the change is an Undo step named via CLI, the file waits for Save, follow mode sees it.
func TestCLIGoesThroughTheEditor(t *testing.T) {
	s, p := proxySession(t)
	var out, errb bytes.Buffer
	code := -1
	background(t, s, func() {
		code = agent.Main(p, []string{"scene", "rename", bridgeScene, "Player", "Hero"}, &out, &errb, agent.Options{})
	}, nil)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out.String(), errb.String())
	}
	sc := s.App.Context().Docs.Find(bridgeScene).Scene()
	if h := sc.Find(sc.Root(), "Hero"); h.IsZero() {
		t.Fatal("the editor's scene was not renamed")
	}
	if got := lastStep(t, s); !strings.HasPrefix(got, "via CLI: ") {
		t.Fatalf("the Undo step is %q", got)
	}
	if sp, _, ok := s.App.Presence().Where(follow.Agent, s.Engine.Platform().Now()); !ok || sp.Doc != bridgeScene {
		t.Fatalf("follow mode did not see the call: %+v", sp)
	}
	if !strings.Contains(out.String(), "Hero") {
		t.Fatalf("the CLI printed %q", out.String())
	}
	out.Reset()
	background(t, s, func() {
		code = agent.Main(p, []string{"scene", "new", "scenes/extra.aescene"}, &out, &errb, agent.Options{})
	}, nil)
	if _, err := os.Stat(filepath.Join(p.Root, "scenes", "extra.aescene")); code != 0 || err != nil {
		t.Fatalf("the editor did not create the scene (exit %d): %s%s", code, out.String(), errb.String())
	}
}

// With editor.proxy.confirm on, the CLI's change waits in the Agent panel until the user applies it.
func TestCLIChangeWaitsWhenConfirmIsOn(t *testing.T) {
	s, p := proxySession(t)
	if err := s.App.Context().Commands.Run(shell.CommandProxyConfirm); err != nil {
		t.Fatal(err)
	}
	panel := s.App.Agent()
	code := -1
	var out bytes.Buffer
	background(t, s, func() {
		code = agent.Main(p, []string{"scene", "rename", bridgeScene, "Player", "Hero"}, &out, io.Discard, agent.Options{})
	}, func() {
		for _, id := range panel.Pending() {
			panel.Decide(id, true)
		}
	})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if got := lastStep(t, s); !strings.HasPrefix(got, "via CLI: ") {
		t.Fatalf("the Undo step is %q", got)
	}
}

// aego mcp relays an outside agent to the open editor; its changes are named after the client.
func TestMCPRelayNamesTheAgent(t *testing.T) {
	s, p := proxySession(t)
	addr, token, ok := agent.EditorProxy(p.Root)
	if !ok {
		t.Fatal("no proxy")
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"codex","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"scene_rename","arguments":{"file":"` + bridgeScene + `","node":"Player","name":"Hero"}}}` + "\n")
	var out bytes.Buffer
	var err error
	background(t, s, func() { err = mcp.Relay(in, &out, addr, token) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(out.String(), "\n"); lines != 2 {
		t.Fatalf("the relay answered %d messages:\n%s", lines, out.String())
	}
	if got := lastStep(t, s); !strings.HasPrefix(got, "via agent: codex: ") {
		t.Fatalf("the Undo step is %q", got)
	}
}
