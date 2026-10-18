package engine

import (
	"sync/atomic"

	"aego/core/clock"
	"aego/core/rng"
	"aego/engine/recorder"
	"aego/platform/input"
)

type WorldID uint64

var worldSeq atomic.Uint64

type World struct {
	id       WorldID
	name     string
	services []serviceEntry
	Input    *input.State
	Time     *clock.Time
	Random   *rng.Source
	Recorder *recorder.Recorder
	Break    func() bool
}

func (c *Context) Input() *input.State {
	if c.World != nil && c.World.Input != nil {
		return c.World.Input
	}
	return c.Engine.Input()
}

func NewWorld(name string) *World {
	return &World{id: WorldID(worldSeq.Add(1)), name: name, Time: clock.NewTime(clock.Config{})}
}

func (w *World) ID() WorldID {
	return w.id
}

func (w *World) Name() string {
	return w.name
}

func (w *World) SetService(id string, v any) {
	for i := range w.services {
		if w.services[i].id == id {
			w.services[i].value = v
			return
		}
	}
	w.services = append(w.services, serviceEntry{id: id, value: v})
}

func (w *World) RemoveService(id string) {
	for i := range w.services {
		if w.services[i].id == id {
			w.services = append(w.services[:i], w.services[i+1:]...)
			return
		}
	}
}

func (w *World) ServiceAny(id string) (any, bool) {
	for i := range w.services {
		if w.services[i].id == id {
			return w.services[i].value, true
		}
	}
	return nil, false
}

func ServiceOf[T any](w *World, id string) (T, bool) {
	var zero T
	if w == nil {
		return zero, false
	}
	v, ok := w.ServiceAny(id)
	if !ok {
		return zero, false
	}
	typed, ok := v.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}
