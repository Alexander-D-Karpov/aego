package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/editor/panels/scripteditor"
	"aego/project"
)

// Normal autosave can happen while a playtest is still using its copy. It is
// a separate source write, not a write through the isolated project's files.
func TestIsolatedPlayAndAutosaveAreIndependent(t *testing.T) {
	const rel = "scripts/probe/main.aes"
	disk := []byte("package probe\n\nscript Probe {\n}\n")
	want := append(bytes.Clone(disk), []byte("// unsaved witness\n")...)
	ready := make(chan string, 1)
	release := make(chan struct{})
	var got string
	e := bridgeSetup(t, agentbridge.Config{Run: func(p *project.Project, _ agent.RunRequest) (agent.RunReport, error) {
		ready <- p.Root
		<-release
		b, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel)))
		got = string(b)
		return agent.RunReport{Frames: 1}, err
	}})
	host := e.ctx.Code.(*scripteditor.Host)
	host.Autosave = false
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	path := filepath.Join(e.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, disk, 0644); err != nil {
		t.Fatal(err)
	}
	d, err := e.ctx.Docs.OpenScript(rel)
	if err != nil {
		t.Fatal(err)
	}
	var runRoot string
	var before, after, copyBefore, copyAfter []byte
	var beforeErr, afterErr, copyBeforeErr, copyAfterErr, statErr error
	var same, dirtyBefore, dirtyAfter bool
	var savedBefore, savedAfter uint64
	processed := false
	ev := results(e.turn(t, []backend.Step{
		{Tool: "script_write", Args: map[string]any{"file": rel, "text": string(want)}},
		{Tool: "scene_run", Args: map[string]any{"file": bridgeScene, "frames": 1}},
	}, func() {
		e.approveAll()
		if processed {
			return
		}
		select {
		case runRoot = <-ready:
			processed = true
			before, beforeErr = os.ReadFile(path)
			copyPath := filepath.Join(runRoot, filepath.FromSlash(rel))
			copyBefore, copyBeforeErr = os.ReadFile(copyPath)
			a, err1 := os.Stat(path)
			b, err2 := os.Stat(copyPath)
			if err1 != nil {
				statErr = err1
			} else if err2 != nil {
				statErr = err2
			} else {
				same = os.SameFile(a, b)
			}
			dirtyBefore, savedBefore = d.Dirty(), d.SavedEdits
			host.Autosave = true
			now := e.s.Engine.Platform().Now()
			host.Tick(now)
			host.Tick(now + scripteditor.AutosaveDelay + .01)
			dirtyAfter, savedAfter = d.Dirty(), d.SavedEdits
			after, afterErr = os.ReadFile(path)
			copyAfter, copyAfterErr = os.ReadFile(copyPath)
			close(release)
			released = true
		default:
		}
	}))
	if len(ev) != 2 || ev[0].Failed || ev[1].Failed {
		t.Fatalf("tool results: %+v", ev)
	}
	if !processed || beforeErr != nil || afterErr != nil || copyBeforeErr != nil || copyAfterErr != nil || statErr != nil {
		t.Fatalf("processed=%v errors=%v %v %v %v %v", processed, beforeErr, afterErr, copyBeforeErr, copyAfterErr, statErr)
	}
	t.Logf("project=%q run=%q samefile=%v before=%q after=%q copy-before=%q copy-after=%q got=%q dirty=%v->%v saved=%d->%d", e.root, runRoot, same, before, after, copyBefore, copyAfter, got, dirtyBefore, dirtyAfter, savedBefore, savedAfter)
	if same || !bytes.Equal(before, disk) || !bytes.Equal(copyBefore, want) || !dirtyBefore || savedBefore != 0 {
		t.Fatal("pre-autosave observations did not match the isolated dirty play")
	}
	if !bytes.Equal(after, want) || !bytes.Equal(copyAfter, want) || got != string(want) || dirtyAfter || savedAfter != 1 {
		t.Fatal("the normal autosave did not save independently of the play copy")
	}
}
