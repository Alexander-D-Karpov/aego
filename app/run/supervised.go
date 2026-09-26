package run

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"aego/core/log"
	"aego/scene/sceneio"
)

const (
	RestartCommand = "restart"
	StateFile      = "run-state.aescene"
)

func StatePath(root string) string {
	return filepath.Join(root, ".aego", StateFile)
}

func (a *App) superviseStdin() {
	path := StatePath(a.Project.Root)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) != RestartCommand {
				continue
			}
			a.Engine.Jobs().OnMain(func() { a.SaveStateAndQuit(path) })
			return
		}
	}()
}

func (a *App) SaveStateAndQuit(path string) {
	if a.Scene != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			a.log.Error("cannot create the state directory", log.F("err", err.Error()))
		} else if err := os.WriteFile(path, sceneio.Save(a.Scene), 0o644); err != nil {
			a.log.Error("cannot save the run state", log.F("err", err.Error()))
		} else {
			a.log.Info("run state saved for restart", log.F("path", path))
		}
	}
	a.Engine.Quit()
}
