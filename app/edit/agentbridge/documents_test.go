package agentbridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aego/app/agent"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/project"
)

func TestDocumentToolsScopeApprovalAndStaleState(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one.aes", "two.aes"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	docs := document.NewManager(document.Options{Root: root})
	one, err := docs.OpenScript("one.aes")
	if err != nil {
		t.Fatal(err)
	}
	two, err := docs.OpenScript("two.aes")
	if err != nil {
		t.Fatal(err)
	}
	replace := func(d *document.ScriptDocument, text string) {
		d.Code.ReplaceRanges([][2]int{{0, len(d.Code.Bytes())}}, [][]byte{[]byte(text)})
	}
	replace(one, "first edit")
	replace(two, "unrelated edit")
	tasks := make(chan func(), 4)
	ctx := &context.Editor{Project: &project.Project{Root: root}, Docs: docs}
	b := &Bridge{ctx: ctx, cfg: Config{OnMain: func(f func()) { tasks <- f }, Timeout: time.Second}}
	b.panelGeneration.Store(1)
	type outcome struct {
		out *agent.Output
		err error
	}
	call := func(op, file string, choice int, beforeAnswer func()) outcome {
		t.Helper()
		done := make(chan outcome, 1)
		go func() {
			out, err := b.documentOperation(map[string]any{"file": file}, op)
			done <- outcome{out, err}
		}()
		for {
			select {
			case f := <-tasks:
				f()
				for _, q := range ctx.TakeQuestions() {
					if !strings.Contains(q.Message, file) {
						t.Fatalf("approval did not name the target: %q", q.Message)
					}
					if beforeAnswer != nil {
						beforeAnswer()
					}
					q.Then(choice)
				}
			case got := <-done:
				return got
			case <-time.After(3 * time.Second):
				t.Fatal("document operation timed out")
			}
		}
	}
	if r := call("save", "one.aes", 1, nil); r.err == nil {
		t.Fatal("denied save succeeded")
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "one.aes")); string(raw) != "old" {
		t.Fatal("denied save wrote to disk")
	}
	if r := call("save", "one.aes", 0, nil); r.err != nil || !r.out.OK {
		t.Fatalf("save: %+v", r)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "one.aes")); string(raw) != "first edit" || one.Dirty() {
		t.Fatal("approved save did not use the document's normal handler")
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "two.aes")); string(raw) != "old" || !two.Dirty() {
		t.Fatal("saving one document touched another")
	}
	if r := call("undo", "one.aes", 0, nil); r.err != nil || string(one.Code.Bytes()) != "old" {
		t.Fatalf("undo: %+v, text=%q", r, one.Code.Bytes())
	}
	if r := call("redo", "one.aes", 0, nil); r.err != nil || string(one.Code.Bytes()) != "first edit" {
		t.Fatalf("redo: %+v, text=%q", r, one.Code.Bytes())
	}
	if r := call("undo", "one.aes", 0, func() { replace(one, "newer user edit") }); r.err == nil || string(one.Code.Bytes()) != "newer user edit" {
		t.Fatalf("stale approval changed newer work: %+v", r)
	}
	if r := call("save", "../outside", 0, nil); r.err == nil {
		t.Fatal("out-of-project document was accepted")
	}
	if r := call("save", "closed.aes", 0, nil); r.err == nil {
		t.Fatal("closed document was accepted")
	}
	b.SetPlanOnly(true)
	if r := call("undo", "one.aes", 0, nil); r.err != errPlanOnly {
		t.Fatalf("Plan Only did not block undo: %v", r.err)
	}
}
