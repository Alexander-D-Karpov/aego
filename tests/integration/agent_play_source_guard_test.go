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

func TestIsolatedPlayPreservesExternalScriptConflict(t *testing.T) {
	const rel = "scripts/probe/main.aes"
	disk := []byte("package probe\n\nscript Probe {}\n")
	edit := append(bytes.Clone(disk), []byte("// unsaved editor change\n")...)
	external := []byte("package probe\n\nscript Probe {}\n// a different external change\n")
	var got string
	e := bridgeSetup(t, agentbridge.Config{Run: func(p *project.Project, _ agent.RunRequest) (agent.RunReport, error) {
		b, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel)))
		got = string(b)
		return agent.RunReport{Frames: 1}, err
	}})
	e.ctx.Code.(*scripteditor.Host).Autosave = false
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
	r := results(e.turn(t, []backend.Step{{Tool: "script_write", Args: map[string]any{"file": rel, "text": string(edit)}}}, e.approveAll))
	if len(r) != 1 || r[0].Failed || !d.Dirty() {
		t.Fatalf("script edit did not leave a dirty buffer: %+v", r)
	}
	if err := os.WriteFile(path, external, 0644); err != nil {
		t.Fatal(err)
	}
	e.ctx.Docs.CheckExternalScripts()
	if !d.Conflict {
		t.Fatal("external change did not become a conflict")
	}
	r = results(e.turn(t, []backend.Step{{Tool: "scene_run", Args: map[string]any{"file": bridgeScene, "frames": 1}}}, e.approveAll))
	if len(r) != 1 || r[0].Failed || got != string(edit) {
		t.Fatalf("play did not read the unsaved buffer: %+v, got %q", r, got)
	}
	if err := e.ctx.Docs.SaveScript(d); err == nil {
		t.Fatal("Save overwrote an unresolved external change")
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, external) {
		t.Fatalf("Play or Save changed the external source: %q, %v", data, err)
	}
	props := e.b.Proposals()
	if !d.Dirty() || !d.Conflict || !d.Code.CanUndo() || !bytes.Equal(d.Code.Bytes(), edit) || d.SavedEdits != 0 || len(props) != 1 || e.b.CanUndo(props[0].ID) {
		t.Fatal("Play or Save changed the conflicting buffer, history, or save state")
	}
}

func TestIsolatedPlayAutosaveRespectsPendingRecovery(t *testing.T) {
	const rel = "scripts/probe/main.aes"
	disk := []byte("package probe\n\nscript Probe {}\n")
	edit := append(bytes.Clone(disk), []byte("// unsaved editor change\n")...)
	var got string
	e := bridgeSetup(t, agentbridge.Config{Run: func(p *project.Project, _ agent.RunRequest) (agent.RunReport, error) {
		b, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel)))
		got = string(b)
		return agent.RunReport{Frames: 1}, err
	}})
	host := e.ctx.Code.(*scripteditor.Host)
	host.Autosave = false
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
	r := results(e.turn(t, []backend.Step{
		{Tool: "script_write", Args: map[string]any{"file": rel, "text": string(edit)}},
		{Tool: "scene_run", Args: map[string]any{"file": bridgeScene, "frames": 1}},
	}, e.approveAll))
	if len(r) != 2 || r[0].Failed || r[1].Failed || got != string(edit) || !d.Dirty() {
		t.Fatalf("play did not use the dirty buffer: %+v, got %q", r, got)
	}
	journalDir := filepath.Join(e.root, ".aego", "source-transactions", "active")
	if err := os.MkdirAll(journalDir, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(journalDir, "unresolved")
	if err := os.WriteFile(marker, []byte("leave recovery untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.ctx.Pipeline.CheckSourceRecovery(); err == nil {
		t.Fatal("fixture did not establish pending source recovery")
	}
	host.Autosave = true
	now := e.s.Engine.Platform().Now()
	host.Tick(now)
	host.Tick(now + scripteditor.AutosaveDelay + .01)
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, disk) {
		t.Fatalf("autosave modified the source while recovery was pending: %q, %v", data, err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "leave recovery untouched" {
		t.Fatalf("autosave modified pending recovery: %q, %v", data, err)
	}
	if !d.Dirty() || !d.Code.CanUndo() || d.Conflict || d.SavedEdits != 0 || !bytes.Equal(d.Code.Bytes(), edit) {
		t.Fatal("blocked autosave changed the dirty buffer, history, or save state")
	}
}
