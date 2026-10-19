//go:build aego_editor

package docsview

import (
	playmods "aego/app/assembly/playground"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/engine"
	"aego/engine/module"
	"aego/scene"
	"aego/script/bind"
	"aego/script/bytecode"
	scriptruntime "aego/script/runtime"
)

type scriptHost struct {
	reg   *bind.Registry
	types *scene.Types
	man   *module.Manifest
	log   *log.Logger
	mods  []*bytecode.Module
}

func (h *scriptHost) Errors() int {
	return 0
}

func (h *scriptHost) Attach(w *engine.World, s *scene.Scene) (func(), error) {
	rt := scriptruntime.New(scriptruntime.Options{Bindings: h.reg, Schema: h.types.Schema(), Log: h.log, VM: playmods.SandboxVM()})
	if err := rt.Link(h.mods); err != nil {
		rt.Close()
		return nil, err
	}
	w.SetService(scriptruntime.ServiceID, rt)
	if err := rt.Instantiate(s); err != nil {
		w.RemoveService(scriptruntime.ServiceID)
		rt.Close()
		return nil, err
	}
	detach, err := h.man.Attach(&module.AttachContext{World: w, Scene: s, Log: h.log, Editor: true, Play: true})
	if err != nil {
		w.RemoveService(scriptruntime.ServiceID)
		rt.Close()
		return nil, err
	}
	return func() {
		detach()
		w.RemoveService(scriptruntime.ServiceID)
		rt.Close()
	}, nil
}

func (h *scriptHost) Problems() ([]context.ScriptProblem, uint64) {
	return nil, 0
}

func (h *scriptHost) Stats() (context.ScriptStats, bool) {
	return context.ScriptStats{}, false
}

func (h *scriptHost) Status() (uint64, bool) {
	return 0, false
}

func (h *scriptHost) Watch(*engine.World, scene.NodeHandle, string, string, int) (string, float64, bool, bool) {
	return "", 0, false, false
}

var _ context.ScriptHost = (*scriptHost)(nil)
