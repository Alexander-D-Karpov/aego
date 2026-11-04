package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/project"
)

func TestIsolatedPlayReadsUnsavedScript(t *testing.T) {
	var got, root string
	e := bridgeSetup(t, agentbridge.Config{Run: func(p *project.Project, _ agent.RunRequest) (agent.RunReport, error) {
		root = p.Root
		b, err := os.ReadFile(filepath.Join(p.Root, "scripts", "probe", "main.aes"))
		got = string(b)
		return agent.RunReport{Frames: 1}, err
	}})
	const rel = "scripts/probe/main.aes"
	path := filepath.Join(e.root, rel)
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
	_ = d
	ev := results(e.turn(t, []backend.Step{{Tool: "script_write", Args: map[string]any{"file": rel, "text": string(disk) + "// unsaved witness\n"}}, {Tool: "scene_run", Args: map[string]any{"file": bridgeScene, "frames": 1}}}, e.approveAll))
	if len(ev) != 2 || ev[0].Failed || ev[1].Failed {
		t.Fatalf("tool results: %+v", ev)
	}
	prior, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(prior, disk) || root == e.root || got != string(disk)+"// unsaved witness\n" {
		t.Fatalf("source isolation: root=%q, got %q, disk %q", root, got, prior)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("temporary run project was retained")
	}
}
