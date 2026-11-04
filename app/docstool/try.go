package docstool

import (
	"fmt"
	"io"
	"strings"

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
	for _, e := range reg.All() {
		if e.Try == "" {
			continue
		}
		var msgs []string
		add := func(msg string) {
			for _, m := range msgs {
				if m == msg || strings.Contains(m, msg) || strings.Contains(msg, m) {
					return
				}
			}
			msgs = append(msgs, msg)
		}
		var code string
		var err error
		if e.TryScript != "" {
			code, err = pg.TryCode(e.TryScript)
		}
		switch res := pg.PlayScene(e.ID, []byte(e.Try), code, e.TryPrefabs, e.TryArtifacts); {
		case !sandbox[e.ID]:
			add("belongs to a module the docs runtime does not include")
		case err != nil:
			add("script does not compile: " + err.Error())
		case !res.Started:
			add("does not load: " + res.Error)
		default:
			if err := pg.Game().Step(tryFrames); err != nil {
				add("stops the engine: " + err.Error())
			}
		}
		poll := pg.Poll()
		for _, l := range poll.Log {
			if l.Level != "INFO" && l.Level != "DEBUG" {
				add(l.Text)
			}
		}
		for _, f := range poll.Faults {
			add(f.Code + " " + f.Message)
		}
		for _, m := range msgs {
			fmt.Fprintf(w, "%s: error: try scene %s\n", e.ID, m)
		}
		bad += len(msgs)
	}
	pg.Stop()
	return bad
}
