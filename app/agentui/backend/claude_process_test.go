package backend

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeProcessFixture(t *testing.T) {
	if os.Getenv("AEGO_CLAUDE_PROCESS_FIXTURE") != "1" {
		return
	}
	for i := 0; i < 200; i++ {
		fmt.Fprintln(os.Stdout, `{"type":"assistant","message":{"content":[{"type":"text","text":"stream"}]}}`)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestClaudeCloseReapsProcessWithUndrainedEvents(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestClaudeProcessFixture$")
	cmd.Env = append(os.Environ(), "AEGO_CLAUDE_PROCESS_FIXTURE=1")
	conf := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(conf, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &claudeConv{cmd: cmd, conf: conf, events: make(chan Event, 64), done: make(chan struct{})}
	var err error
	c.in, err = cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = &c.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go c.read(out)
	t.Cleanup(func() { _ = c.Close() })
	deadline := time.Now().Add(5 * time.Second)
	for len(c.events) < cap(c.events) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(c.events) != cap(c.events) {
		t.Fatal("fixture did not reach a blocked event delivery")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case _, ok := <-c.Events():
			if !ok {
				if cmd.ProcessState == nil {
					t.Fatal("child was not reaped")
				}
				if _, err := os.Stat(conf); !os.IsNotExist(err) {
					t.Fatalf("temporary MCP credentials still exist: %v", err)
				}
				return
			}
		case <-timeout.C:
			t.Fatal("closing a replaced Claude conversation leaked its process or reader")
		}
	}
}
