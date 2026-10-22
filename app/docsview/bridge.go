//go:build aego_editor

package docsview

import (
	"encoding/json"
	"strconv"

	playmods "aego/app/assembly/playground"
	"aego/editor/core/docsession"
	"aego/scene/sceneio"
)

type request struct {
	Scene     string            `json:"scene"`
	Select    string            `json:"select"`
	Code      string            `json:"code"`
	Prefabs   map[string]string `json:"prefabs"`
	Component string            `json:"component"`
	Field     string            `json:"field"`
	Value     float64           `json:"value"`
	Node      string            `json:"node"`
}

type Reply struct {
	Text   string            `json:"text,omitempty"`
	Values map[string]string `json:"values,omitempty"`
	Error  string            `json:"error,omitempty"`
	State  *docsession.State `json:"state,omitempty"`
	Log    []Line            `json:"log,omitempty"`
	Pose   []float64         `json:"pose,omitempty"`
}

func (a *App) Call(method string, payload []byte) Reply {
	var req request
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &req); err != nil {
			return Reply{Error: err.Error()}
		}
	}
	s := a.session
	var err error
	switch method {
	case "load":
		mods, derr := playmods.DecodeTry(req.Code)
		if err = derr; err == nil {
			err = a.scripts.use(mods)
		}
		if err == nil {
			err = playmods.MountTry(a.loader, req.Prefabs)
		}
		if err == nil {
			err = s.Load([]byte(req.Scene), req.Select)
		}
	case "reset":
		err = s.Reset()
	case "text":
	case "play":
		s.TogglePause()
	case "undo":
		err = s.Undo()
	case "redo":
		err = s.Redo()
	case "set":
		err = s.Set(req.Component, req.Field, req.Value)
	case "state":
		st := s.State()
		return Reply{State: &st}
	case "pose":
		sc := s.Controller().Scene()
		if sc == nil {
			return Reply{Error: "the scene is not running"}
		}
		h := sc.Find(sc.Root(), req.Node)
		if h.IsZero() || !sc.IsAlive(h) {
			return Reply{Error: "no node " + strconv.Quote(req.Node)}
		}
		p := sc.WorldPosition(h)
		return Reply{Pose: []float64{float64(p.X), float64(p.Y)}}
	case "log":
		out := Reply{Log: a.lines}
		a.lines = nil
		return out
	default:
		return Reply{Error: "unknown method " + strconv.Quote(method)}
	}
	if err != nil {
		return Reply{Error: err.Error()}
	}
	st := s.State()
	return Reply{Text: string(s.Text()), Values: Values(s.Text()), State: &st}
}

// Values flattens a scene file into the field keys the docs scene tree uses.
func Values(text []byte) map[string]string {
	nodes, err := sceneio.Outline(text)
	if err != nil {
		return nil
	}
	return sceneio.OutlineValues(nodes)
}

func (a *App) Reply(method, payload string) string {
	out, err := json.Marshal(a.Call(method, []byte(payload)))
	if err != nil {
		return `{"error":"cannot encode the reply"}`
	}
	return string(out)
}
