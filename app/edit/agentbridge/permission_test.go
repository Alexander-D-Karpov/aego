package agentbridge

import (
	"strings"
	"testing"
	"time"

	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/project"
)

func TestPlaytestPermissionChoicesAndRevocation(t *testing.T) {
	root := t.TempDir()
	trust := agentui.NewPlaytestTrust(t.TempDir(), root, true)
	tasks := make(chan func(), 4)
	ctx := &context.Editor{Project: &project.Project{Root: root, Native: true}}
	b := &Bridge{ctx: ctx, cfg: Config{PlaytestTrust: trust, OnMain: func(f func()) { tasks <- f }, Timeout: time.Second}}
	b.panelGeneration.Store(1)
	ask := func(choice int) error {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- b.permitPlaytest("native playtest", b.panelGeneration.Load()) }()
		select {
		case f := <-tasks:
			f()
		case <-time.After(time.Second):
			t.Fatal("no permission question")
		}
		questions := ctx.TakeQuestions()
		if len(questions) != 1 || len(questions[0].Buttons) != 3 || !strings.Contains(questions[0].Buttons[1], "always") || questions[0].Buttons[2] != "Deny" {
			t.Fatalf("unexpected permission question: %+v", questions)
		}
		questions[0].Then(choice)
		select {
		case err := <-done:
			return err
		case <-time.After(time.Second):
			t.Fatal("permission did not complete")
			return nil
		}
	}
	if err := ask(2); err == nil || trust.Allowed() {
		t.Fatal("Deny must not save permission")
	}
	if err := ask(0); err != nil || trust.Allowed() {
		t.Fatal("session approval must not persist")
	}
	if err := ask(1); err != nil || !trust.Allowed() {
		t.Fatalf("always approval: %v", err)
	}
	b.panelGeneration.Add(1)
	if err := b.permitPlaytest("native playtest", b.panelGeneration.Load()); err != nil {
		t.Fatalf("persistent permission not used in new conversation: %v", err)
	}
	b.SetPlanOnly(true)
	if err := b.permitPlaytest("native playtest", b.panelGeneration.Load()); err != errPlanOnly {
		t.Fatalf("plan mode bypassed by trust: %v", err)
	}
	b.SetPlanOnly(false)
	if err := trust.Revoke(); err != nil {
		t.Fatal(err)
	}
	b.RevokePlaytest()
	if err := ask(2); err == nil {
		t.Fatal("revocation must restore the permission question")
	}
}

func TestScopedPermissionEscapeDenies(t *testing.T) {
	tasks := make(chan func(), 1)
	ctx := &context.Editor{Project: &project.Project{Root: t.TempDir()}}
	b := &Bridge{ctx: ctx, cfg: Config{OnMain: func(f func()) { tasks <- f }, Timeout: time.Second}}
	b.panelGeneration.Store(1)
	done := make(chan error, 1)
	go func() { done <- b.permitScoped("export", 1, "export:web") }()
	(<-tasks)()
	q := ctx.TakeQuestions()[0]
	if len(q.Buttons) != 3 || q.Buttons[2] != "Deny" {
		t.Fatalf("Escape must target Deny: %v", q.Buttons)
	}
	q.Then(len(q.Buttons) - 1)
	if err := <-done; err == nil {
		t.Fatal("Escape granted a session permission")
	}
}
