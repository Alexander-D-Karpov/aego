package run

import (
	"aego/core/log"
	"aego/script/reload"
)

func (a *App) program() reload.Program {
	g := a.Workspace.Committed()
	return reload.NewProgram(g.ID, g.Modules, g.Hashes)
}

func (a *App) linkScripts() error {
	a.prog = a.program()
	a.target = a.prog.ID
	return a.Scripts.Link(a.prog.Ordered())
}

func (a *App) hotReload() {
	next := a.program()
	if next.ID == a.prog.ID || next.ID == a.target {
		return
	}
	plan, err := reload.Build(a.prog, next)
	if err != nil {
		a.log.Error("hot reload plan failed", log.F("err", err.Error()))
		return
	}
	a.target = next.ID
	for _, d := range plan.Diags {
		a.log.Warn(d.Message)
	}
	rt := a.Scripts
	rt.Defer(func() {
		rep, err := reload.Apply(rt, a.Types.Schema(), plan)
		if err != nil {
			a.target = a.prog.ID
			a.log.Error("hot reload failed, keeping the running generation",
				log.F("generation", a.prog.ID), log.F("err", err.Error()))
			return
		}
		a.prog = next
		a.log.Info(rep.String())
	})
}
