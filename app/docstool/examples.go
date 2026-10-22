package docstool

import (
	"fmt"
	"io"
	"strings"

	"aego/app/playground"
	"aego/engine/docs"
	"aego/engine/module"
)

// CheckExamples loads the example scene of every module code, validates it and runs it headless; the code must be among the problems it reports (D146).
func CheckExamples(reg *docs.Registry, w io.Writer) int {
	pg, err := playground.New()
	if err != nil {
		fmt.Fprintln(w, "examples: cannot start the docs runtime:", err)
		return 1
	}
	pg.SetGame(playground.GameConfig{Headless: true, FrameTime: 1.0 / 60})
	defer func() {
		if g := pg.Game(); g != nil {
			g.Close()
		}
	}()
	bad := 0
	for _, e := range reg.All() {
		if e.Kind != docs.Code || e.Diag == nil || e.Diag.Example == "" {
			continue
		}
		found, why := false, ""
		if res := pg.PlayScene("example-"+e.ID, []byte(e.Diag.Example), "", nil); !res.Started {
			why = "does not load: " + res.Error
		} else {
			for _, d := range module.ValidateScene(pg.Game().Scene(), nil) {
				found = found || d.Code == e.ID
			}
			if err := pg.Game().Step(tryFrames); err != nil {
				why = "stops the engine: " + err.Error()
			}
		}
		poll := pg.Poll()
		for _, f := range poll.Faults {
			found = found || f.Code == e.ID
		}
		for _, l := range poll.Log {
			found = found || strings.Contains(l.Text, e.ID)
		}
		switch {
		case why != "":
			fmt.Fprintf(w, "%s: error: example %s %s\n", e.ID, e.Diag.ExampleFile, why)
			bad++
		case !found:
			fmt.Fprintf(w, "%s: error: example of %s does not produce %s\n", e.ID, e.ID, e.ID)
			bad++
		}
	}
	pg.Stop()
	return bad
}
