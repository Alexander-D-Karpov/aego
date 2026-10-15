package edit

import (
	"strconv"

	"aego/app"
	scriptImporter "aego/asset/importers/script"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/engine"
	"aego/engine/module"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
	"aego/script/diag"
	"aego/script/reload"
	"aego/script/runtime"
	"aego/script/workspace"
)

type scriptHost struct {
	bindings *bind.Registry
	schema   *schema.Registry
	pipeline *pipeline.Pipeline
	importer *scriptImporter.Importer
	ws       *workspace.Workspace
	log      *log.Logger
	modules  *module.Manifest
	prog     reload.Program
	target   uint64
	last     *runtime.Runtime
	errRev   uint64
	errValid bool
	errs     int
	probRev  uint64
	probs    []context.ScriptProblem
	stats    context.ScriptStats
}

func (h *scriptHost) channel() *log.ChannelLogger {
	return h.log.With(runtime.Channel)
}

func (h *scriptHost) next() reload.Program {
	g := h.ws.Committed()
	return reload.NewProgram(g.ID, g.Modules, g.Hashes)
}

func (h *scriptHost) publish(p reload.Program) error {
	if err := runtime.Publish(h.schema, p.Ordered(), h.bindings.Surface()); err != nil {
		return err
	}
	h.prog, h.target = p, p.ID
	return nil
}

func (h *scriptHost) sync() error {
	if h.prog.Modules != nil && h.ws.Committed().ID == h.prog.ID {
		return nil
	}
	return h.publish(h.next())
}

func (h *scriptHost) Reload() error {
	g := h.ws.Committed()
	if h.prog.Modules != nil && (g.ID == h.prog.ID || g.ID == h.target) {
		return nil
	}
	next := h.next()
	rt := h.last
	if rt == nil || h.prog.Modules == nil {
		return h.publish(next)
	}
	plan, err := reload.Build(h.prog, next)
	if err != nil {
		return err
	}
	h.target = next.ID
	for _, d := range plan.Diags {
		h.channel().Warn(d.Message)
	}
	rt.Defer(func() {
		if h.last != rt {
			return
		}
		rep, err := reload.Apply(rt, h.schema, plan)
		if err != nil {
			h.target = h.prog.ID
			h.channel().Error("hot reload failed, generation "+strconv.FormatUint(h.prog.ID, 10)+" keeps running",
				log.F("err", err.Error()))
			return
		}
		h.prog = next
		h.channel().Info(rep.String())
	})
	return nil
}

func (h *scriptHost) Status() (uint64, bool) {
	return h.prog.ID, len(h.ws.Committed().Failed) > 0
}

func (h *scriptHost) Errors() int {
	if rev := h.pipeline.Revision(); !h.errValid || rev != h.errRev {
		h.errRev, h.errValid = rev, true
		h.errs = len(app.ScriptErrors(h.pipeline))
	}
	return h.errs
}

func (h *scriptHost) Problems() ([]context.ScriptProblem, uint64) {
	if h.importer == nil {
		return nil, 0
	}
	ps, rev := h.importer.Problems()
	if rev == h.probRev {
		return h.probs, rev
	}
	h.probRev = rev
	h.probs = h.probs[:0]
	for _, p := range ps {
		h.probs = append(h.probs, context.ScriptProblem{
			Package: p.Package, Path: p.Path, Line: p.Line, Col: p.Col,
			Code: p.Code, Error: p.Error, Message: p.Message, Help: diag.HelpText(p.Code),
		})
	}
	return h.probs, rev
}

func (h *scriptHost) Stats() (context.ScriptStats, bool) {
	if h.last == nil {
		return context.ScriptStats{}, false
	}
	s := h.last.Stats()
	types := h.stats.Types[:0]
	for _, t := range s.Types {
		types = append(types, context.ScriptTypeStats{Name: t.Name, Instances: t.Instances, Calls: t.Calls, Time: t.Time, Faults: t.Faults})
	}
	h.stats = context.ScriptStats{
		Instances: s.Instances, Update: s.Update, Fixed: s.Fixed,
		Instructions: s.Instructions, HostCalls: s.HostCalls,
		HeapObjects: s.HeapObjects, HeapBytes: s.HeapBytes,
		GCRuns: s.GCRuns, GCTime: s.GCTime,
		Faults: s.Faults, BudgetAborts: s.BudgetAborts,
		Coroutines: context.ScriptCoroutineStats{
			Active: s.Coroutines, Sleeping: s.Sleeping, ResumesPerFrame: s.Resumes, Canceled: s.CoCanceled,
		},
		Signals: context.ScriptSignalStats{
			Connections: s.Connections, EmitsPerFrame: s.Emits, HandlersPerFrame: s.Handlers,
		},
		Types: types,
	}
	return h.stats, true
}

func (h *scriptHost) Attach(w *engine.World, s *scene.Scene) (func(), error) {
	if err := h.sync(); err != nil {
		return nil, err
	}
	rt := runtime.New(runtime.Options{Bindings: h.bindings, Schema: h.schema, Log: h.log})
	if err := rt.Link(h.prog.Ordered()); err != nil {
		return nil, err
	}
	if err := rt.Instantiate(s); err != nil {
		rt.Close()
		return nil, err
	}
	w.SetService(runtime.ServiceID, rt)
	h.last = rt
	detachMods := func() {}
	if h.modules != nil {
		d, err := h.modules.Attach(&module.AttachContext{World: w, Scene: s, Log: h.log, Editor: true, Play: true})
		if err != nil {
			w.RemoveService(runtime.ServiceID)
			rt.Close()
			h.last = nil
			return nil, err
		}
		detachMods = d
	}
	return func() {
		detachMods()
		w.RemoveService(runtime.ServiceID)
		rt.Close()
		if h.last != rt {
			return
		}
		h.last = nil
		h.target = h.prog.ID
		if err := h.sync(); err != nil {
			h.channel().Error("cannot publish script types", log.F("err", err.Error()))
		}
	}, nil
}
