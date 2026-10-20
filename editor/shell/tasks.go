package shell

import (
	"aego/editor/core/tasks"
)

func (a *App) taskCounts() (active, failed int) {
	for _, t := range a.tasks.Tasks() {
		if t.Kind != "preview" {
			active++
		}
	}
	for _, t := range a.tasks.Recent() {
		if t.State() == tasks.Failed {
			failed++
		}
	}
	return active, failed
}
