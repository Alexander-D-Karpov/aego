package edit

import (
	"strconv"
	"strings"

	"aego/app/crash"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/script/runtime"
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

func (s *Session) moduleIDs() string {
	var out []string
	for _, m := range s.Modules.Modules {
		out = append(out, m.ID)
	}
	return strings.Join(out, ",")
}

func (s *Session) crash(kind, where string, rec any, stack []byte, script string) {
	defer func() { recover() }()
	b := crash.Bundle{
		Root: s.Project.Root, Kind: kind, Where: where, Panic: rec, Stack: stack, Script: script,
		Trace: s.Engine.Trace(), Log: s.ring,
		Info: [][2]string{{"project", s.Project.Name}, {"gpu", s.Engine.Caps().Renderer}, {"modules", s.moduleIDs()}},
	}
	var ctx *context.Editor
	if s.App != nil {
		ctx = s.App.Context()
	}
	if ctx != nil {
		ctx.Docs.WriteRecovery()
		if pc := s.App.Play(); pc != nil && pc.World() != nil {
			b.Events = pc.Recorder()
			if sc := pc.Scene(); sc != nil {
				b.Names = names(sc)
				if s.crashScene {
					b.Scene = sceneio.Save(sc)
				}
			}
		}
	}
	dir, err := crash.Write(b)
	if err != nil {
		s.log.Error("cannot write the crash bundle", log.F("err", err.Error()))
		return
	}
	s.log.Error("crash bundle written", log.F("dir", dir), log.F("where", where))
	if ctx != nil {
		ctx.Notify(context.LevelError, "Crash report saved to "+dir)
	}
}

func (s *Session) scriptPanic(boundary string, v any, stack []byte) {
	s.crash("panic", boundary, v, stack, "")
}

func (s *Session) fault(rep runtime.FaultReport) {
	if rep.Fault != nil && rep.Fault.Code == vm.CodeNativePanic {
		s.crash("fault", rep.Type, rep.Fault.Message, []byte(rep.Fault.Detail), scriptStack(rep.Fault))
	}
}

func (s *Session) spike() {
	if s.spikeMs <= 0 {
		return
	}
	ms := s.Engine.Stats().FrameTime * 1000
	now := s.Engine.Platform().Now()
	if ms < s.spikeMs || now-s.lastSpike < 10 {
		return
	}
	s.lastSpike = now
	s.crash("spike", strconv.FormatFloat(ms, 'f', 1, 64)+" ms frame", nil, nil, "")
}
