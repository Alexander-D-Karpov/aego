//go:build aego_editor

package docsview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	playmods "aego/app/assembly/playground"
	"aego/asset"
	"aego/editor/core/docsession"
	"aego/engine/i18n"
	"aego/scene/sceneio"
	"aego/ui/widgets"
)

type request struct {
	Scene        string            `json:"scene"`
	Select       string            `json:"select"`
	Code         string            `json:"code"`
	Prefabs      map[string]string `json:"prefabs"`
	Artifacts    []byte            `json:"artifacts"`
	Component    string            `json:"component"`
	Field        string            `json:"field"`
	Script       string            `json:"script"`
	Enabled      *bool             `json:"enabled"`
	Value        json.RawMessage   `json:"value"`
	Fields       []string          `json:"fields"`
	Dependencies []string          `json:"dependencies"`
	Node         string            `json:"node"`
	Theme        string            `json:"theme"`
	Lang         string            `json:"lang"`
	Frames       int               `json:"frames"`
	Keys         []string          `json:"keys"`
}

type Reply struct {
	Text       string            `json:"text,omitempty"`
	Values     map[string]string `json:"values,omitempty"`
	Error      string            `json:"error,omitempty"`
	State      *docsession.State `json:"state,omitempty"`
	Log        []Line            `json:"log,omitempty"`
	Pose       []float64         `json:"pose,omitempty"`
	Inspection *Inspection       `json:"inspection,omitempty"`
	Control    *ControlState     `json:"control,omitempty"`
	Viewport   *Viewport         `json:"viewport,omitempty"`
}

func (a *App) Call(method string, payload []byte) Reply {
	// Bound JSON/base64 expansion before the decoder allocates resource arrays.
	if len(payload) > asset.MaxTryArtifactsBytes*2 {
		return Reply{Error: "documentation request exceeds its byte budget"}
	}
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
			err = playmods.MountTry(a.loader, req.Prefabs, req.Artifacts)
		}
		if err == nil {
			err = s.Load([]byte(req.Scene), req.Select)
		}
	case "reset":
		err = s.Reset()
	case "theme":
		a.SetTheme(req.Theme)
	case "lang":
		widgets.Tr = i18n.Lookup
		i18n.Default().SetLanguage(req.Lang)
	case "text":
	case "play":
		s.TogglePause()
	case "undo":
		err = s.Undo()
	case "redo":
		err = s.Redo()
	case "scriptEnabled":
		if req.Enabled == nil {
			err = fmt.Errorf("scriptEnabled needs a boolean enabled value")
		} else {
			err = s.SetScriptEnabled(req.Script, *req.Enabled)
		}
	case "set":
		value := bytes.TrimSpace(req.Value)
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			err = fmt.Errorf("set needs a number, string or string-list value")
		} else if value[0] == '"' {
			var text string
			if err = json.Unmarshal(value, &text); err == nil {
				err = s.SetString(req.Component, req.Field, text)
			}
		} else if value[0] == '[' {
			// A bounded raw JSON budget also limits allocations before decoding.
			if len(value) > docsession.MaxStringListBytes*8 {
				err = fmt.Errorf("string list exceeds its encoded byte budget")
				break
			}
			var entries []json.RawMessage
			if err = json.Unmarshal(value, &entries); err != nil {
				break
			}
			if len(entries) > docsession.MaxStringListItems {
				err = fmt.Errorf("string list exceeds its item budget")
				break
			}
			values := make([]string, len(entries))
			for i, entry := range entries {
				entry = bytes.TrimSpace(entry)
				if len(entry) == 0 || entry[0] != '"' {
					err = fmt.Errorf("string list contains a non-string value")
					break
				}
				if err = json.Unmarshal(entry, &values[i]); err != nil {
					break
				}
			}
			if err == nil {
				err = s.SetStringList(req.Component, req.Field, values)
			}
		} else {
			var number float64
			if err = json.Unmarshal(value, &number); err == nil {
				err = s.Set(req.Component, req.Field, number)
			}
		}
	case "state":
		st := s.State()
		control := a.control
		return Reply{State: &st, Control: &control}
	case "control":
		if req.Enabled == nil {
			err = fmt.Errorf("control needs a boolean enabled value")
		} else {
			err = a.setControl(*req.Enabled)
		}
	case "viewport":
		viewport := a.viewport
		if _, err := viewport.pixelRect(viewport.CanvasWidth, viewport.CanvasHeight); err != nil {
			return Reply{Error: err.Error()}
		}
		return Reply{Viewport: &viewport}
	case "advance":
		err = a.advanceControl(req.Frames, req.Keys)
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
	case "inspect":
		inspection, err := a.inspectTry(req.Component, req.Fields, req.Dependencies)
		if err != nil {
			return Reply{Error: err.Error()}
		}
		return Reply{Inspection: inspection}
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
	// A document/world change must not leave a previously queued synthetic
	// gesture pressed or continue applying it to the replacement world.
	switch method {
	case "load", "reset":
		a.cancelControlAdvance()
		a.discardDocumentText()
	case "play", "undo", "redo", "scriptEnabled", "set":
		a.cancelControlAdvance()
		a.cancelTextInputFence()
	case "theme", "lang":
		a.cancelTextInputFence()
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
