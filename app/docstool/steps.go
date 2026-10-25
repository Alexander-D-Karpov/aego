package docstool

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"aego/app/playground"
	"aego/engine/docs"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/format/aetext"
	"aego/project/refs"
)

var scriptDecl = regexp.MustCompile(`(?m)^package (\w+)[\s\S]*?^script (\w+)`)

// CheckSteps plays every aescene check block of a page with the aes check scripts above it, the later version of a script replacing the earlier.
func CheckSteps(reg *docs.Registry, w io.Writer) int {
	var pg *playground.Playground
	bad := 0
	for _, e := range reg.All() {
		files := map[string]string{}
		for _, s := range e.Snippets {
			if !s.Check {
				continue
			}
			switch s.Lang {
			case "aes":
				name := "main"
				if m := scriptDecl.FindStringSubmatch(s.Code); m != nil {
					name = m[1] + "/" + m[2]
				}
				files[name] = s.Code
				continue
			case "aescene":
			default:
				continue
			}
			if pg == nil {
				var err error
				if pg, err = playground.New(); err != nil {
					fmt.Fprintln(w, "steps: cannot start the docs runtime:", err)
					return 1
				}
				pg.SetGame(playground.GameConfig{Headless: true, FrameTime: 1.0 / 60})
				defer pg.Stop()
			}
			if why := playStep(pg, s.Code, files); why != "" {
				fmt.Fprintf(w, "%s: error: scene at line %d %s\n", e.ID, s.Line, why)
				bad++
			}
		}
	}
	return bad
}

func playStep(pg *playground.Playground, scene string, files map[string]string) string {
	doc, err := aetext.Parse([]byte(scene), "step.aescene")
	if err != nil {
		return "does not parse: " + err.Error()
	}
	for _, m := range refs.MissingNodesIn(doc, "step") {
		return "has " + m.Owner + "." + m.Field + " pointing at node " + m.Target.String() + ", which the scene does not define"
	}
	code := ""
	if len(files) > 0 {
		c, err := pg.TryFiles(files)
		if err != nil {
			return "has scripts that do not compile: " + err.Error()
		}
		code = c
	}
	if res := pg.PlayScene("step", []byte(scene), code, nil); !res.Started {
		return "does not load: " + res.Error
	}
	if err := pg.Game().Step(tryFrames); err != nil {
		return "stops the engine: " + err.Error()
	}
	poll := pg.Poll()
	if len(poll.Faults) > 0 {
		return "faults: " + poll.Faults[0].Code + " " + poll.Faults[0].Message
	}
	for _, l := range poll.Log {
		if lv := strings.ToUpper(l.Level); lv == "ERROR" || strings.HasPrefix(lv, "WARN") {
			return "logs " + strings.ToLower(lv) + ": " + l.Text
		}
	}
	for _, d := range module.ValidateScene(pg.Game().Scene(), nil) {
		if d.Severity != problems.Info {
			return "has problem " + d.Code + ": " + d.Message
		}
	}
	return ""
}
