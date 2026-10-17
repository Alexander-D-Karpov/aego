package run

import (
	"strconv"
	"strings"

	"aego/app/crash"
	"aego/core/ids"
	"aego/core/log"
	"aego/scene"
	"aego/scene/sceneio"
	scriptruntime "aego/script/runtime"
	"aego/script/vm"
)

func scriptStack(f *vm.Fault) string {
	var b strings.Builder
	for _, fr := range f.Stack {
		b.WriteString(fr.File + ":" + strconv.Itoa(fr.Line) + " " + fr.Function + "\n")
	}
	return b.String()
}

func names(s *scene.Scene) func(ids.NodeID) string {
	return func(id ids.NodeID) string {
		if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
			return s.Path(h)
		}
		return id.String()[:8]
	}
}

func (a *App) moduleIDs() string {
	var out []string
	for _, m := range a.Modules.Modules {
		out = append(out, m.ID)
	}
	return strings.Join(out, ",")
}

func (a *App) crash(kind, where string, rec any, stack []byte, script string) {
	defer func() { recover() }()
	b := crash.Bundle{
		Root: a.Project.Root, Kind: kind, Where: where, Panic: rec, Stack: stack, Script: script,
		Trace: a.Engine.Trace(), Log: a.ring,
		Info: [][2]string{{"project", a.Project.Name}, {"gpu", a.Engine.Caps().Renderer}, {"modules", a.moduleIDs()}},
	}
	if w := a.Engine.MainWorld(); w != nil {
		b.Events = w.Recorder
	}
	if a.Scene != nil {
		b.Names = names(a.Scene)
		if a.crashScene {
			b.Scene = sceneio.Save(a.Scene)
		}
	}
	dir, err := crash.Write(b)
	if err != nil {
		a.log.Error("cannot write the crash bundle", log.F("err", err.Error()))
		return
	}
	a.log.Error("crash bundle written", log.F("dir", dir), log.F("where", where))
}

func (a *App) fault(rep scriptruntime.FaultReport) {
	if rep.Fault != nil && rep.Fault.Code == vm.CodeNativePanic {
		a.crash("fault", rep.Type, rep.Fault.Message, []byte(rep.Fault.Detail), scriptStack(rep.Fault))
	}
}
