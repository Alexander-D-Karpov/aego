package frame

import (
	"aego/core/ids"
	"aego/engine/recorder"
	"aego/scene"
)

func Record(s *scene.Scene, rec *recorder.Recorder) func() {
	if s == nil || rec == nil {
		return func() {}
	}
	return s.Events().Subscribe(func(ev scene.Event) {
		switch ev.Kind {
		case scene.EvNodeCreated:
			rec.Add(recorder.Spawn, ev.ID, ids.NodeID{}, s.Name(ev.Node), "")
		case scene.EvNodeDestroyed:
			rec.Add(recorder.Despawn, ev.ID, ids.NodeID{}, s.Name(ev.Node), "")
		}
	})
}
