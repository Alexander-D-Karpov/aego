package docstool

import (
	"fmt"
	"io"

	"aego/app/playground"
	"aego/engine/docs"
)

const tryFrames = 60

func CheckTry(reg *docs.Registry, w io.Writer) int {
	pg, err := playground.New()
	if err != nil {
		fmt.Fprintln(w, "try: cannot start the docs runtime:", err)
		return 1
	}
	pg.SetGame(playground.GameConfig{Headless: true, FrameTime: 1.0 / 60})
	defer func() {
		if g := pg.Game(); g != nil {
			g.Close()
		}
	}()
	sandbox := map[string]bool{}
	for _, e := range pg.TryScenes() {
		sandbox[e.ID] = true
	}
	bad := 0
	report := func(id, msg string) {
		bad++
		fmt.Fprintf(w, "%s: error: try scene %s\n", id, msg)
	}
	for _, e := range reg.All() {
		if e.Try == "" {
			continue
		}
		if !sandbox[e.ID] {
			report(e.ID, "belongs to a module the docs runtime does not include")
			continue
		}
		res := pg.PlayScene(e.ID, []byte(e.Try))
		if !res.Started {
			report(e.ID, "does not load: "+res.Error)
			continue
		}
		if err := pg.Game().Step(tryFrames); err != nil {
			report(e.ID, "stops the engine: "+err.Error())
			continue
		}
		poll := pg.Poll()
		for _, l := range poll.Log {
			if l.Level != "INFO" && l.Level != "DEBUG" {
				report(e.ID, l.Text)
			}
		}
		for _, f := range poll.Faults {
			report(e.ID, f.Code+" "+f.Message)
		}
	}
	pg.Stop()
	return bad
}
