package edit

import (
	"aego/app/run"
	scriptImporter "aego/asset/importers/script"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/context"
	"aego/engine"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
	"aego/script/bytecode"
	"aego/script/runtime"
)

type scriptHost struct {
	bindings *bind.Registry
	schema   *schema.Registry
	loader   *loader.Loader
	pipeline *pipeline.Pipeline
	importer *scriptImporter.Importer
	log      *log.Logger
	mods     []*bytecode.Module
	last     *runtime.Runtime
	errRev   uint64
	errValid bool
	errs     int
	probRev  uint64
	probs    []context.ScriptProblem
	stats    context.ScriptStats
	gen      uint32
}

func (h *scriptHost) Reload() error {
	gen := h.loader.Generation()
	blobs, err := h.loader.Scripts()
	if err != nil {
		return err
	}
	mods, err := runtime.DecodeAll(blobs)
	if err != nil {
		return err
	}
	if err := runtime.Publish(h.schema, mods, h.bindings.Surface()); err != nil {
		return err
	}
	h.mods, h.gen = mods, gen
	return nil
}

func (h *scriptHost) sync() error {
	if h.loader.Generation() == h.gen {
		return nil
	}
	return h.Reload()
}

func (h *scriptHost) Errors() int {
	if rev := h.pipeline.Revision(); !h.errValid || rev != h.errRev {
		h.errRev, h.errValid = rev, true
		h.errs = len(run.ScriptErrors(h.pipeline))
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
			Code: p.Code, Error: p.Error, Message: p.Message,
		})
	}
	if err := h.sync(); err != nil {
		h.log.With(runtime.Channel).Error("cannot load the compiled scripts", log.F("err", err.Error()))
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
		Types: types,
	}
	return h.stats, true
}

func (h *scriptHost) Attach(w *engine.World, s *scene.Scene) (func(), error) {
	if err := h.sync(); err != nil {
		return nil, err
	}
	rt := runtime.New(runtime.Options{Bindings: h.bindings, Schema: h.schema, Log: h.log})
	if err := rt.Link(h.mods); err != nil {
		return nil, err
	}
	if err := rt.Instantiate(s); err != nil {
		rt.Close()
		return nil, err
	}
	w.SetService(runtime.ServiceID, rt)
	h.last = rt
	return func() {
		w.RemoveService(runtime.ServiceID)
		rt.Close()
		if h.last == rt {
			h.last = nil
		}
	}, nil
}
