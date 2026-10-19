package player

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

func (p *Player) info() [][2]string {
	out := [][2]string{}
	if p.Data != nil {
		g := p.Data.Game
		out = append(out, [2]string{"game", g.Name}, [2]string{"gameVersion", g.Version}, [2]string{"profile", p.Data.Profile}, [2]string{"target", p.Data.Target})
	}
	if p.Engine != nil {
		out = append(out, [2]string{"gpu", p.Engine.Caps().Renderer})
	}
	if p.Modules != nil {
		ids := make([]string, len(p.Modules.Modules))
		for i, m := range p.Modules.Modules {
			ids[i] = m.ID
		}
		out = append(out, [2]string{"modules", strings.Join(ids, ",")})
	}
	return out
}

func (p *Player) crash(kind, where string, rec any, stack []byte, script string) {
	defer func() { recover() }()
	if p.crashDir == "" {
		return
	}
	b := crash.Bundle{Dir: p.crashDir, Kind: kind, Where: where, Panic: rec, Stack: stack, Script: script, Log: p.ring, Info: p.info()}
	if p.Engine != nil {
		b.Trace = p.Engine.Trace()
		if w := p.Engine.MainWorld(); w != nil {
			b.Events = w.Recorder
		}
	}
	if p.Scene != nil {
		b.Names = names(p.Scene)
		if p.crashScene {
			b.Scene = sceneio.Save(p.Scene)
		}
	}
	dir, err := crash.Write(b)
	if err != nil {
		p.log.Error("cannot write the crash bundle", log.F("err", err.Error()))
		return
	}
	p.log.Error("crash bundle written", log.F("dir", dir), log.F("where", where))
}

func (p *Player) scriptPanic(boundary string, v any, stack []byte) {
	p.crash("panic", boundary, v, stack, "")
}

func (p *Player) fault(rep scriptruntime.FaultReport) {
	if rep.Fault != nil && rep.Fault.Code == vm.CodeNativePanic {
		p.crash("fault", rep.Type, rep.Fault.Message, []byte(rep.Fault.Detail), scriptStack(rep.Fault))
	}
}
