package integration

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/editor/panels/scripteditor"
	"aego/project"
)

func TestIsolatedPlayReadsUnsavedScript(t *testing.T) {
	var got, root, sourcePath string
	var sourceDuring, sourceAfter []byte
	var sameFile bool
	e := bridgeSetup(t, agentbridge.Config{Run: func(p *project.Project, _ agent.RunRequest) (agent.RunReport, error) {
		root = p.Root
		copyPath := filepath.Join(p.Root, "scripts", "probe", "main.aes")
		b, err := os.ReadFile(copyPath)
		got = string(b)
		if err != nil {
			return agent.RunReport{}, err
		}
		srcInfo, err := os.Stat(sourcePath)
		if err != nil {
			return agent.RunReport{}, err
		}
		copyInfo, err := os.Stat(copyPath)
		if err != nil {
			return agent.RunReport{}, err
		}
		sameFile = os.SameFile(srcInfo, copyInfo)
		if sameFile {
			return agent.RunReport{}, errors.New("play source and original are the same file")
		}
		sourceDuring, err = os.ReadFile(sourcePath)
		if err != nil {
			return agent.RunReport{}, err
		}
		// A write by the isolated run must not reach the original.
		if err := os.WriteFile(copyPath, []byte("modified in isolated run\n"), 0600); err != nil {
			return agent.RunReport{}, err
		}
		sourceAfter, err = os.ReadFile(sourcePath)
		return agent.RunReport{Frames: 1}, err
	}})
	// Measure writes caused by Play separately from the user's normal
	// time-based autosave, which runs while the test steps the editor.
	e.ctx.Code.(*scripteditor.Host).Autosave = false
	const rel = "scripts/probe/main.aes"
	path := filepath.Join(e.root, rel)
	sourcePath = path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	disk := []byte("package probe\n\nscript Probe {\n}\n")
	if err := os.WriteFile(path, disk, 0644); err != nil {
		t.Fatal(err)
	}
	d, err := e.ctx.Docs.OpenScript(rel)
	if err != nil {
		t.Fatal(err)
	}
	ev := results(e.turn(t, []backend.Step{{Tool: "script_write", Args: map[string]any{"file": rel, "text": string(disk) + "// unsaved witness\n"}}, {Tool: "scene_run", Args: map[string]any{"file": bridgeScene, "frames": 1}}}, e.approveAll))
	if len(ev) != 2 || ev[0].Failed || ev[1].Failed {
		t.Fatalf("tool results: %+v", ev)
	}
	prior, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(prior, disk) || root == e.root || got != string(disk)+"// unsaved witness\n" || sameFile || !bytes.Equal(sourceDuring, disk) || !bytes.Equal(sourceAfter, disk) {
		t.Fatalf("source isolation: root=%q, got %q, disk %q, same file=%v, before/after run write=%q/%q, err=%v", root, got, prior, sameFile, sourceDuring, sourceAfter, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("temporary run project was retained")
	}
	if !d.Dirty() || !d.Code.CanUndo() || d.SavedEdits != 0 || string(d.Code.Bytes()) != got {
		t.Fatal("Play changed the original buffer, its dirty status, or its undo history")
	}
	props := e.b.Proposals()
	if len(props) != 1 || !e.b.CanUndo(props[0].ID) {
		t.Fatal("Play lost the agent edit's undo step")
	}
	if err := e.b.Undo(props[0].ID); err != nil || d.Dirty() || !bytes.Equal(d.Code.Bytes(), disk) {
		t.Fatalf("Undo did not restore the original clean buffer: %v", err)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, disk) {
		t.Fatalf("Undo changed the source: %q, %v", after, err)
	}
}
