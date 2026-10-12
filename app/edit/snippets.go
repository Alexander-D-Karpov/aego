package edit

import "aego/app/playground"

func snippets() func(code string) []string {
	var pg *playground.Playground
	var err error
	return func(code string) []string {
		if pg == nil && err == nil {
			pg, err = playground.New()
		}
		if err != nil {
			return []string{"cannot start the snippet runtime: " + err.Error()}
		}
		return playground.Format(pg.Run(map[string]string{"main.aes": code}, playground.DefaultFrames))
	}
}
