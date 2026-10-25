package agent_test

import (
	"bytes"
	"strings"
	"testing"

	"aego/app/agent"
)

const (
	hurtClip   = "assets/anim/player_hurt.aeanim"
	playerCtrl = "assets/anim/player.aectrl"
)

func TestAnimSetKeyWritesTheClip(t *testing.T) {
	e := open(t)
	before := e.read(hurtClip)
	out := e.must("anim.set-key", agent.Args{"file": hurtClip, "target": ".", "prop": "SpriteRenderer.tint", "time": "0.35", "value": "#ffffffff"})
	if len(out.Files) != 1 {
		t.Fatalf("files %v", out.Files)
	}
	after := e.read(hurtClip)
	if bytes.Equal(after, before) || !strings.Contains(string(after), "t=0.35") {
		t.Fatalf("the clip did not get the key:\n%s", after)
	}
	if _, err := e.run("anim.set-key", agent.Args{"file": hurtClip, "target": ".", "prop": "SpriteRenderer.tint", "time": "9", "value": "#ffffffff"}); err == nil {
		t.Fatal("a key past the end of the clip was written")
	}
	if !bytes.Equal(e.read(hurtClip), after) {
		t.Fatal("a refused edit changed the file")
	}
}

func TestCtrlRemoveStateTakesItsTransitions(t *testing.T) {
	e := open(t)
	e.must("ctrl.remove-state", agent.Args{"file": playerCtrl, "name": "Fall"})
	v := e.must("ctrl.get", agent.Args{"file": playerCtrl}).Value.(map[string]any)
	for _, s := range v["states"].([]map[string]any) {
		if s["name"] == "Fall" {
			t.Fatal("Fall is still a state")
		}
	}
	for _, tr := range v["transitions"].([]map[string]any) {
		if tr["from"] == "Fall" || tr["to"] == "Fall" {
			t.Fatalf("a transition of Fall is left: %v", tr)
		}
	}
	e.must("ctrl.set-state", agent.Args{"file": playerCtrl, "name": "Idle", "attr": "pos", "value": "10,20"})
	if !strings.Contains(string(e.read(playerCtrl)), "pos=[10, 20]") {
		t.Fatalf("pos was not written:\n%s", e.read(playerCtrl))
	}
}
