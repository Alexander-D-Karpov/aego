package agent_test

import (
	"bytes"
	"strings"
	"testing"

	"aego/anim"
	"aego/app/agent"
	"aego/format/aetext"
	"aego/schema"
)

const (
	hitClip    = "assets/anim/player_hit.aeanim"
	playerCtrl = "assets/anim/player.aectrl"
)

func TestAnimSetKeyWritesTheClip(t *testing.T) {
	e := open(t)
	before := e.read(hitClip)
	out := e.must("anim.set-key", agent.Args{"file": hitClip, "target": ".", "prop": "SpriteRenderer.tint", "time": "0.35", "value": "#ffffffff"})
	if len(out.Files) != 1 {
		t.Fatalf("files %v", out.Files)
	}
	after := e.read(hitClip)
	if bytes.Equal(after, before) || !strings.Contains(string(after), "t=0.35") {
		t.Fatalf("the clip did not get the key:\n%s", after)
	}
	if _, err := e.run("anim.set-key", agent.Args{"file": hitClip, "target": ".", "prop": "SpriteRenderer.tint", "time": "9", "value": "#ffffffff"}); err == nil {
		t.Fatal("a key past the end of the clip was written")
	}
	if !bytes.Equal(e.read(hitClip), after) {
		t.Fatal("a refused edit changed the file")
	}
}

func TestAnimSetKeyWritesHurtMaterialFlash(t *testing.T) {
	const file = "assets/anim/player_hurt.aeanim"
	e := open(t)
	e.must("anim.set-key", agent.Args{"file": file, "target": ".", "prop": "MaterialOverride.Flash", "time": "0.35", "value": "0.75"})
	root, err := aetext.Parse(e.read(file), file)
	if err != nil {
		t.Fatal(err)
	}
	clip, err := anim.Decode(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, track := range clip.Tracks {
		if track.Target != "" || track.Property != "MaterialOverride.Flash" {
			continue
		}
		if track.Kind != schema.KindFloat32 || track.OnEnd != anim.EndRestore {
			t.Fatalf("Flash track kind=%v onEnd=%v", track.Kind, track.OnEnd)
		}
		for _, key := range track.Keys {
			if key.T == 0.35 && key.V.L[0] == 0.75 {
				return
			}
		}
	}
	t.Fatal("missing material Flash key at 0.35 with value 0.75")
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
