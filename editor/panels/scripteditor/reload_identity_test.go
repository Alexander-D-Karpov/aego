package scripteditor

import (
	"bytes"
	"strings"
	"testing"

	"aego/editor/core/code"
	"aego/editor/core/document"
	"aego/script/langsvc"
	"aego/script/workspace"
)

// queuedAnalysisJobs lets a real language analysis finish while delaying its
// owner-thread callback until after the document has been reloaded or reset.
type queuedAnalysisJobs struct {
	work []func()
	main []func()
}

func TestPanelRejectsLateCompletionAfterDocumentReset(t *testing.T) {
	const source = "package c\nscript Mover {\n\tfunc Update(dt float) {\n\t\tyield \n\t}\n}\n"
	cursor := strings.Index(source, "yield ") + len("yield ")
	for _, tc := range []struct {
		name                   string
		replace, rename, reset bool
	}{
		{"same document reset", false, false, true},
		{"replacement then reset", true, false, true},
		{"replacement before reset", true, false, false},
		{"rename before reset", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, err := workspace.Open(workspace.Config{Root: t.TempDir(), Module: "game"})
			if err != nil {
				t.Fatal(err)
			}
			jobs := &queuedAnalysisJobs{}
			host := NewHost(langsvc.New(ws, jobs))
			doc := &document.ScriptDocument{Path: "scripts/test.aes", Code: code.New([]byte(source))}
			doc.Code.SetCursor(cursor, false)
			panel := newPanel(host, doc)
			defer panel.releaseDoc()
			panel.requestCompletion(nil)
			jobs.completeWork(t)
			jobs.deliver(t)
			if !panel.comp.open {
				t.Fatal("fixture did not produce a real completion")
			}
			panel.requestCompletion(nil)
			jobs.completeWork(t)
			// The earlier successful result must not be mistaken for a late one.
			panel.comp.open = false
			if tc.replace {
				old := doc.Code
				doc.Code = code.New([]byte(source + "\n// replacement\n"))
				doc.Code.SetCursor(cursor, false)
				if doc.Code.Rev() != old.Rev() {
					t.Fatal("fixture did not produce overlapping document revisions")
				}
			}
			if tc.rename {
				doc.Path = "scripts/renamed.aes"
			}
			if tc.reset {
				panel.resync()
			}
			jobs.deliver(t)
			if panel.comp.open {
				t.Fatal("late completion from before reset was published")
			}
			panel.requestCompletion(nil)
			jobs.completeWork(t)
			jobs.deliver(t)
			if !panel.comp.open {
				t.Fatal("fresh completion after reset was not published")
			}
			file, ok := ws.FileID(doc.Path)
			snap := host.lang.Latest()
			if !ok || snap == nil || !bytes.Equal(snap.Sources.Source(file), doc.Code.Bytes()) {
				t.Fatal("fresh completion used old document bytes")
			}
		})
	}
}

func (j *queuedAnalysisJobs) Submit(fn func()) { j.work = append(j.work, fn) }
func (j *queuedAnalysisJobs) OnMain(fn func()) { j.main = append(j.main, fn) }

func (j *queuedAnalysisJobs) completeWork(t *testing.T) {
	t.Helper()
	if len(j.work) != 1 {
		t.Fatalf("expected one analysis job, got %d", len(j.work))
	}
	fn := j.work[0]
	j.work = nil
	fn()
}

func (j *queuedAnalysisJobs) deliver(t *testing.T) {
	t.Helper()
	if len(j.main) != 1 {
		t.Fatalf("expected one owner-thread result, got %d", len(j.main))
	}
	fn := j.main[0]
	j.main = nil
	fn()
}

func TestPanelRejectsLateAnalysisAfterDocumentReset(t *testing.T) {
	for _, tc := range []struct {
		name           string
		replace, reset bool
	}{
		{"same document reset", false, true},
		{"replacement then reset", true, true},
		{"replacement before reset", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, err := workspace.Open(workspace.Config{Root: t.TempDir(), Module: "game"})
			if err != nil {
				t.Fatal(err)
			}
			jobs := &queuedAnalysisJobs{}
			host := NewHost(langsvc.New(ws, jobs))
			doc := &document.ScriptDocument{Path: "scripts/test.aes", Code: code.New([]byte("var old = 1\n"))}
			panel := newPanel(host, doc)
			defer panel.releaseDoc()
			panel.ask(1)
			jobs.completeWork(t)
			if tc.replace {
				old := doc.Code
				doc.Code = code.New([]byte("var different = 2\n"))
				if doc.Code.Rev() != old.Rev() {
					t.Fatal("fixture did not produce overlapping document revisions")
				}
			}
			if tc.reset {
				panel.resync()
			}
			jobs.deliver(t)
			if panel.snap != nil || panel.synced() || host.liveSnap != nil {
				t.Fatal("late analysis from before reset was published")
			}
			panel.ask(2)
			jobs.completeWork(t)
			jobs.deliver(t)
			if panel.snap == nil || !panel.synced() || host.liveSnap != panel.snap {
				t.Fatal("fresh analysis after reset was not published")
			}
			file, ok := ws.FileID(doc.Path)
			if !ok || !bytes.Equal(panel.snap.Sources.Source(file), doc.Code.Bytes()) {
				t.Fatal("fresh analysis used old document bytes")
			}
		})
	}
}

func TestPanelRetainsAnalysisAcrossEditsToSameDocument(t *testing.T) {
	ws, err := workspace.Open(workspace.Config{Root: t.TempDir(), Module: "game"})
	if err != nil {
		t.Fatal(err)
	}
	jobs := &queuedAnalysisJobs{}
	host := NewHost(langsvc.New(ws, jobs))
	doc := &document.ScriptDocument{Path: "scripts/test.aes", Code: code.New([]byte("var old = 1\n"))}
	panel := newPanel(host, doc)
	defer panel.releaseDoc()
	panel.ask(1)
	jobs.completeWork(t)
	revision := doc.Code.Rev()
	doc.Code.Replace(doc.Code.Len(), 0, "\n")
	jobs.deliver(t)
	if panel.snap == nil || panel.snapRev != revision || panel.synced() || host.liveSnap != panel.snap {
		t.Fatal("analysis for an earlier revision of the same document was discarded or marked current")
	}
}

func TestPanelResetClearsStaleHover(t *testing.T) {
	doc := &document.ScriptDocument{Path: "scripts/test.aes", Code: code.New([]byte("saved\n"))}
	panel := newPanel(&Host{}, doc)
	defer panel.releaseDoc()
	panel.hover = hoverState{off: 3, since: 1, done: true, ok: true, hasDiag: true, builtin: true}
	panel.resync()
	if panel.hover.off != -1 || panel.hover.done || panel.hover.ok || panel.hover.hasDiag || panel.hover.builtin {
		t.Fatal("reset retained stale hover state")
	}
}
