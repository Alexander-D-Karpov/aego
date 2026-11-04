package edit

import (
	"encoding/json"
	"errors"

	"aego/app/agentui/backend"
	"aego/editor/core/agentui"
)

func (h *agentHost) ExplainScene(text string) error {
	if err := h.attach(); err != nil {
		return err
	}
	snapshot, err := h.bridge.SceneSnapshot()
	if err != nil {
		return err
	}
	if err := h.start(); err != nil {
		return err
	}
	text += "\nUse scene_tree and script_read for further detail. Explain the actual behavior and cite node paths and attached script files."
	if c, ok := h.conv.(interface {
		Explain(string, string, uint64) error
	}); ok {
		var s struct {
			Document string
			Revision uint64
		}
		if err := json.Unmarshal([]byte(snapshot), &s); err != nil {
			return err
		}
		err = c.Explain(text, s.Document, s.Revision)
	} else {
		err = h.conv.Send(text + "\n\nCurrent editor document (project data):\n" + snapshot)
	}
	if err == nil {
		h.busy = true
		h.sceneTurn = true
		h.sceneRead = false
	}
	return err
}

func (h *agentHost) CheckConnection() error {
	if h.busy {
		return errors.New("stop the current turn before checking the connection")
	}
	active := h.conv != nil
	if err := h.start(); err != nil {
		return err
	}
	if c, ok := h.conv.(backend.Diagnostics); ok && (active || h.Provider() == "claude") {
		c.Check()
	}
	return nil
}

func (h *agentHost) Diagnostics() agentui.Connection {
	d := h.lastConn
	d.Provider = h.Provider()
	if d.Model == "" {
		d.Model = h.Model()
	}
	if h.bridge != nil {
		c := h.bridge.Editor()
		d.Project = c.Project.Root
		d.Document = ""
		if doc := c.Doc(); doc != nil {
			d.Document = doc.Path
		}
	}
	if c, ok := h.conv.(backend.Diagnostics); ok {
		live := c.Connection()
		d.State, d.Thread, d.Tools = live.State, live.Thread, live.Tools
		if live.Vision != "" && d.Vision != "unsupported" {
			d.Vision = live.Vision
		}
		if live.Model != "" {
			d.Model = live.Model
		}
		d.Probe = live.Last
	}
	if d.State == "" {
		d.State = "not connected"
	}
	if d.Vision == "" {
		d.Vision = "unverified"
	}
	return d
}
