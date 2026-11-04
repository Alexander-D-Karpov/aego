package agent_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/agent"
	"aego/app/projects"
	"aego/app/run"
	"aego/core/log"
	"aego/modules/input"
	"aego/project"
)

func TestAuthorBellRunFromEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bellrun")
	pr, err := projects.Create(projects.Options{Dir: dir, Template: projects.TemplateEmpty, Module: "bellrun"})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, dir: dir, p: pr, opt: agent.Options{Run: run.AgentRunner()}}
	for _, a := range []agent.Args{{"name": "Stride", "type": "axis1", "bind": []string{"Key.A/Key.D", "Key.Left/Key.Right"}}, {"name": "Hop", "type": "button", "bind": []string{"Key.Space"}}, {"name": "NewRun", "type": "button", "bind": []string{"Key.R"}}} {
		a["context"] = "Default"
		e.must("project.input-action", a)
	}
	const palette = "n=#15273c v=#314c61 c=#67dfcc m=#fff3c9 y=#ffbf59 s=#b66550 p=#f08378"
	makeGrid := func(name, grid string) *agent.DrawnView {
		t.Helper()
		o := e.must("asset.pixels", agent.Args{"path": "assets/art/" + name + ".png", "grid": grid, "palette": palette, "scale": 1})
		return o.Value.(*agent.DrawnView)
	}
	body := "...nnnn...\n..nccccn..\n..ncmncn..\n..nccccyy.\n...nccnn..\n..nvvvnn..\n.ncvvvvc..\n...nvvn...\n"
	legs := []string{"..nvv.vn..\n..mm..mm..", "...vv.vv..\n..mm...mm.", "...nvvvn..\n...mm.mm..", "..nv.vv...\n.mm..mm..."}
	frames := make([]string, 0, 4)
	for i, leg := range legs {
		frames = append(frames, fmt.Sprintf("run%d:\n%s%s", i, body, leg))
	}
	bird := makeGrid("courier", strings.Join(frames, "\n"))
	if len(bird.Frames) != 4 || bird.Flipbook == "" {
		t.Fatal(bird)
	}
	coin := makeGrid("chime", "..nnn..\n.nyyyn.\nnymysyn\nnymysyn\nnymysyn\n.nsssn.\n..nnn..")
	beacon := makeGrid("bell", "...nn...\n..nyyn..\n.nyyyyn.\n.nymyyn.\nnyyyyyyn\nnssssssn\n...yy...\n...nn...")
	ground := e.must("asset.paint", agent.Args{"path": "assets/art/causeway.png", "width": 96, "height": 12, "palette": palette, "ops": []map[string]any{
		{"op": "rect", "w": 96, "h": 12, "color": "v"}, {"op": "rect", "w": 96, "h": 2, "color": "c"}, {"op": "line", "x": 2, "y": 5, "x2": 7, "y2": 8, "color": "n"}, {"op": "rect", "x": 11, "y": 7, "w": 3, "h": 2, "color": "s"},
		{"op": "copy", "w": 16, "h": 12, "toX": 16, "flipX": true}, {"op": "copy", "w": 32, "h": 12, "toX": 32}, {"op": "copy", "w": 32, "h": 12, "toX": 64, "flipX": true},
	}}).Value.(*agent.DrawnView)
	sky := makeGrid("cloud", "...cccc.....\n..cmmmmc.cc.\n.ccmmmmmcmmc\ncccccccccccc")
	script, err := os.ReadFile("testdata/bellrun/runner.aes")
	if err != nil {
		t.Fatal(err)
	}
	res := e.must("script.create", agent.Args{"file": "scripts/game/runner.aes", "text": string(script)})
	if len(res.Problems) > 0 {
		t.Fatal(res.Problems)
	}
	var tx strings.Builder
	tx.WriteString(`tx version=1 { label="Build Chime Causeway" `)
	newNode := func(name string) { fmt.Fprintf(&tx, `op "add-node" { name=%q } `, name) }
	comp := func(node, typ string) { fmt.Fprintf(&tx, `op "add-comp" { node=%q comp=%q } `, node, typ) }
	set := func(node, field, val string) {
		fmt.Fprintf(&tx, `op "set" { node=%q path=%q value=%s } `, node, field, val)
	}
	sprite := func(name, pos, size, id string) {
		newNode(name)
		set(name, "transform.position", pos)
		comp(name, "SpriteRenderer")
		set(name, "SpriteRenderer.sprite", "asset(\""+id+"\")")
		set(name, "SpriteRenderer.size", size)
	}
	set("Camera", "Camera2D.clearColor", "#294a67ff")
	sprite("FarCloud", "[-3.5,-2.1]", "[2,0.58]", sky.ID)
	sprite("HighCloud", "[2.7,-2.5]", "[1.25,0.4]", sky.ID)
	sprite("Causeway", "[0,2.25]", "[13.5,0.5]", ground.ID)
	sprite("Shelf", "[-0.6,1.2]", "[3.2,0.35]", ground.ID)
	sprite("ChimeA", "[-3.5,1.35]", "[0.4,0.4]", coin.ID)
	sprite("ChimeB", "[-0.8,0.25]", "[0.4,0.4]", coin.ID)
	sprite("ChimeC", "[2.5,1.35]", "[0.4,0.4]", coin.ID)
	sprite("GoalBell", "[5.2,1.25]", "[0.75,0.75]", beacon.ID)
	sprite("Courier", "[-5.2,1.55]", "[0.88,0.88]", bird.Frames[0].ID)
	comp("Courier", "AnimationPlayer")
	set("Courier", "AnimationPlayer.clip", fmt.Sprintf("%q", bird.Flipbook))
	comp("Courier", "Runner")
	for _, v := range [][2]string{{"First", "ChimeA"}, {"Second", "ChimeB"}, {"Third", "ChimeC"}, {"Beacon", "GoalBell"}, {"Shelf", "Shelf"}} {
		set("Courier", "Runner."+v[0], fmt.Sprintf("%q", "Root/"+v[1]))
	}
	tx.WriteString("}")
	e.must("scene.apply", agent.Args{"file": "scenes/main.aescene", "tx": tx.String()})
	if o := e.must("scene.validate", agent.Args{"file": "scenes/main.aescene"}); o.Errors() {
		t.Fatal(o.Problems)
	}
	const inputs = "1-176 axis Stride 1; 48 press Hop; 180 press NewRun"
	const checks = "@31 ChimeA.transform.position.y < -20; @74 Courier.transform.position.y < 0.8; @85 ChimeB.transform.position.y < -20; @130 ChimeC.transform.position.y < -20; @176 GoalBell.transform.position.y < -1; @184 Courier.transform.position.x ~= -5.2; @184 ChimeA.transform.position.y ~= 1.35; @184 ChimeB.transform.position.y ~= 0.25; @184 ChimeC.transform.position.y ~= 1.35; @184 GoalBell.transform.position.y ~= 1.25"
	args := agent.Args{"file": "scenes/main.aescene", "frames": 185, "seed": 41, "input": inputs, "assert": checks, "watch": "Courier,GoalBell,ChimeA,ChimeB,ChimeC"}
	o := e.must("scene.run", args)
	report := o.Value.(agent.RunReport)
	if report.Error != "" || len(report.Checks) != 10 {
		t.Fatal(report.Error, report.Log, report.Checks)
	}
	for _, c := range report.Checks {
		if !c.OK {
			t.Fatalf("failed %s = %s: %s; %v", c.Assert, c.Value, c.Error, report.Log)
		}
	}
	recording, err := os.ReadFile("testdata/bellrun/route.aeinput")
	if err != nil {
		t.Fatal(err)
	}
	checkBellRecording(t, pr, recording)
	if dst := os.Getenv("AEGO_BELLRUN_OUT"); dst != "" {
		if err := os.Mkdir(dst, 0755); err != nil {
			t.Fatal(err)
		}
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, p)
			if rel == "." {
				return nil
			}
			if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "exports") {
				return filepath.SkipDir
			}
			if strings.HasSuffix(rel, ".md") {
				return nil
			}
			out := filepath.Join(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(out, 0755)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(out, b, 0644)
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "route.aeinput"), recording, 0644); err != nil {
			t.Fatal(err)
		}
		for n, v := range map[string]any{"playtest-args.json": args, "playtest-result.json": report} {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetIndent("", "  ")
			if err := enc.Encode(v); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dst, n), buf.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func checkBellRecording(t *testing.T, p *project.Project, recording []byte) {
	t.Helper()
	a, err := run.Open(p, run.Config{Headless: true, FrameTime: 1.0 / 60, Seed: 41, NoProfile: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	_, err = input.Of(a.Engine.MainWorld()).Replay(recording)
	if err != nil {
		t.Fatal(err)
	}
	bell := a.Scene.Find(a.Scene.Root(), "GoalBell")
	hero := a.Scene.Find(a.Scene.Root(), "Courier")
	for tick := 1; tick <= 185; tick++ {
		if !a.Engine.Step(a.Driver()) {
			t.Fatalf("playback stopped on tick %d", tick)
		}
		switch tick {
		case 175:
			if a.Scene.WorldPosition(bell).Y > 0 {
				t.Fatal("recorded input failed to collect all three chimes and reach goal")
			}
		case 184:
			if a.Scene.WorldPosition(bell).Y < 1 || a.Scene.WorldPosition(hero).X > -5 {
				t.Fatal("recorded restart failed")
			}
		}
	}
}
