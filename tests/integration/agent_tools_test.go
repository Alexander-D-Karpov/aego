package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
)

// ask_user waits in the panel until the user picks an option; the agent gets the choice back (11b5a).
func TestAgentAskUserWaitsForTheAnswer(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	events := e.turn(t, []backend.Step{{Tool: "ask_user", Args: map[string]any{"question": "Which way?", "options": []any{
		map[string]any{"label": "Teleport", "detail": "works now"}, map[string]any{"label": "Separate scenes"}}}}}, func() {
		for _, q := range e.b.Questions() {
			if q.State == agentbridge.Pending {
				if err := e.b.Answer(q.ID, []int{1}, ""); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
	res := results(events)
	if len(res) != 1 || res[0].Failed || res[0].Result != "The user chose: Separate scenes" {
		t.Fatalf("results = %+v", res)
	}
}

const enemyScript = "scripts/enemy/enemy.aes"

// An open script is edited in its buffer: the card applies one Undo step there and nothing is written until the user saves.
func TestAgentScriptEditGoesThroughTheOpenBuffer(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	d, err := e.ctx.Docs.OpenScript(enemyScript)
	if err != nil {
		t.Fatal(err)
	}
	before := string(d.Code.Bytes())
	disk, _ := os.ReadFile(filepath.Join(e.root, enemyScript))
	line := firstCodeLine(before)
	events := e.turn(t, []backend.Step{
		{Tool: "script_read", Args: map[string]any{"file": enemyScript}},
		{Tool: "script_edit", Args: map[string]any{"file": enemyScript, "old": line, "new": line + " // agent"}},
	}, e.approveAll)
	res := results(events)
	if len(res) != 2 || res[0].Failed || res[1].Failed || !strings.Contains(res[0].Result, line) {
		t.Fatalf("results = %+v", res)
	}
	if got := string(d.Code.Bytes()); !strings.Contains(got, line+" // agent") {
		t.Fatal("the edit did not reach the open buffer")
	}
	if now, _ := os.ReadFile(filepath.Join(e.root, enemyScript)); string(now) != string(disk) {
		t.Fatal("the edit of an open script was written to disk before the user saved")
	}
	p := e.b.Proposals()[len(e.b.Proposals())-1]
	if err := e.b.Undo(p.ID); err != nil {
		t.Fatal(err)
	}
	if string(d.Code.Bytes()) != before {
		t.Fatal("Undo did not restore the buffer")
	}
}

// A closed script is edited on disk; Undo writes the old text back, and script_create creates a file Undo removes.
func TestAgentScriptEditAndNewOnDisk(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	abs := filepath.Join(e.root, enemyScript)
	disk, _ := os.ReadFile(abs)
	line := firstCodeLine(string(disk))
	e.turn(t, []backend.Step{
		{Tool: "script_edit", Args: map[string]any{"file": enemyScript, "old": line, "new": line + " // agent"}},
		{Tool: "script_create", Args: map[string]any{"file": "scripts/level/exit.aes", "text": "package level\n"}},
	}, e.approveAll)
	if now, _ := os.ReadFile(abs); !strings.Contains(string(now), line+" // agent") {
		t.Fatal("the edit of a closed script was not written")
	}
	created := filepath.Join(e.root, "scripts", "level", "exit.aes")
	if _, err := os.Stat(created); err != nil {
		t.Fatal("script_create did not create the file")
	}
	props := e.b.Proposals()
	for i := len(props) - 1; i >= 0; i-- {
		if err := e.b.Undo(props[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatal("Undo of script_create left the file")
	}
	if now, _ := os.ReadFile(abs); string(now) != string(disk) {
		t.Fatal("Undo of script_edit did not restore the file")
	}
}

// Plan Only refuses write tools and says why, so the agent describes the change instead.
func TestAgentPlanOnlyRefusesWrites(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	e.b.SetPlanOnly(true)
	res := results(e.turn(t, []backend.Step{{Tool: "scene_add_node", Args: map[string]any{"file": bridgeScene, "name": "Planned"}}}, nil))
	if len(res) != 1 || !res[0].Failed || !strings.Contains(res[0].Result, "Plan only") || e.alive("Planned") {
		t.Fatalf("results = %+v", res)
	}
}

func firstCodeLine(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "//") && strings.Count(text, l) == 1 {
			return l
		}
	}
	return ""
}

// A script edit answers with what the compiler says about the new text, so the agent sees an AES error at once.
func TestAgentScriptEditReportsCompileErrors(t *testing.T) {
	e := bridgeSetup(t, agentbridge.Config{})
	res := results(e.turn(t, []backend.Step{{Tool: "script_create", Args: map[string]any{"file": "scripts/level/broken.aes", "text": "package level\n\nfunc f() int {\n\treturn \"text\"\n}\n", "dry-run": true}}}, nil))
	if len(res) != 1 || res[0].Failed || !strings.Contains(res[0].Result, "AES") {
		t.Fatalf("results = %+v", res)
	}
}
