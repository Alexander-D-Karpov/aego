package app

import (
	"slices"
	"sort"

	"aego/core/ids"
	"aego/core/log"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/session"
	"aego/editor/state"
	"aego/platform"
	"aego/platform/clip"
	"aego/scene/sceneio"
)

const clipTTL = 0.5

type clipboard struct {
	win   platform.Window
	now   func() float64
	mimes []string
	at    float64
	fresh bool
}

func (c *clipboard) invalidate() {
	c.fresh = false
}

func (c *clipboard) Text() string {
	if cb, ok := c.win.(platform.Clipboard); ok {
		return cb.ClipboardText()
	}
	return ""
}

func (c *clipboard) SetText(s string) {
	c.invalidate()
	if cb, ok := c.win.(platform.Clipboard); ok {
		cb.SetClipboardText(s)
	}
}

func (c *clipboard) SetData(mime string, data []byte, text string) bool {
	c.invalidate()
	if cb, ok := c.win.(platform.Clipboard); ok {
		return cb.SetClipboardData(mime, data, text)
	}
	return false
}

func (c *clipboard) Data(mime string) ([]byte, bool) {
	if cb, ok := c.win.(platform.Clipboard); ok {
		return cb.ClipboardData(mime)
	}
	return nil, false
}

func (c *clipboard) SetItems(items []clip.Item) bool {
	c.invalidate()
	if cb, ok := c.win.(platform.ClipboardItems); ok {
		return cb.SetClipboardItems(items)
	}
	if len(items) == 0 {
		return false
	}
	text := ""
	for _, it := range items {
		if it.Mime == clip.MimeText {
			text = string(it.Data)
		}
	}
	return c.SetData(items[0].Mime, items[0].Data, text)
}

func (c *clipboard) Has(mime string) bool {
	cb, ok := c.win.(platform.ClipboardItems)
	if !ok {
		data, has := c.Data(mime)
		return has && len(data) > 0
	}
	if now := c.now(); !c.fresh || now-c.at > clipTTL {
		c.mimes, c.at, c.fresh = cb.ClipboardMimes(), now, true
	}
	return slices.Contains(c.mimes, mime)
}

func (a *App) applyPlatform() {
	win := a.eng.Window()
	ui := a.ui
	if cs, ok := win.(platform.Cursorer); ok {
		cs.SetCursor(platform.CursorShape(ui.CursorShape()))
	}
	if mc, ok := win.(platform.MouseCapturer); ok {
		if want := ui.Captured() != 0; want != a.captured {
			mc.CaptureMouse(want)
			a.captured = want
		}
	}
	ti, ok := win.(platform.TextInputer)
	if !ok {
		return
	}
	wants, r, cursor := ui.WantsTextInput()
	switch {
	case wants && !a.textOn:
		ti.StartTextInput()
		a.textOn = true
	case !wants && a.textOn:
		ti.StopTextInput()
		a.textOn = false
		ui.SetComposition("", 0)
	}
	if wants {
		ti.SetTextInputArea(r, cursor)
	}
}

func (a *App) restoreState() {
	root := a.opt.Project.Root
	if a.opt.Supervised {
		if h, ok := session.Take(root); ok {
			a.applyState(h.State, h.Scenes)
			a.ctx.Notify(context.LevelInfo, "Session restored after the native rebuild")
			return
		}
	}
	st, err := state.Load(root)
	if err != nil {
		a.log.Warn("editor state is unreadable", log.F("err", err.Error()))
	}
	a.applyState(st, nil)
}

func (a *App) applyState(st *state.Editor, scenes map[string][]byte) {
	set := &a.ctx.Settings
	set.Tool = uint8(st.Tool)
	set.Grid, set.Snap = st.Grid, st.Snap
	set.SnapMove, set.SnapRotate, set.SnapScale = st.SnapMove, st.SnapRotate, st.SnapScale
	if c := a.opt.Code; c != nil {
		c.Autosave, c.Hints, c.Minimap, c.Rainbow, c.Block = st.Autosave, st.InlayHints, st.Minimap, st.RainbowBrackets, st.HighlightBlock
	}
	for _, k := range st.Collapsed {
		a.ctx.Collapsed[k] = true
	}
	var active *document.SceneDocument
	var sel []ids.NodeID
	for _, sd := range st.Docs {
		var d *document.SceneDocument
		var err error
		if data, ok := scenes[sd.Path]; ok {
			d, err = a.ctx.Docs.OpenData(sd.Path, data, true)
		} else {
			d, err = a.ctx.Docs.Open(sd.Path)
		}
		if err != nil {
			a.log.Warn("cannot reopen scene", log.F("path", sd.Path), log.F("err", err.Error()))
			continue
		}
		d.View.Camera = sd.Camera
		if d.View.Camera.Zoom <= 0 {
			d.View.Camera.Zoom = 64
		}
		d.View.Mode = document.ViewMode(sd.Mode)
		d.View.Orbit = sd.Orbit
		for _, id := range sd.Expanded {
			d.View.Expanded[id] = true
		}
		if sd.Path == st.Active {
			active, sel = d, sd.Selection
		}
	}
	if active == nil {
		return
	}
	a.ctx.Docs.SetActive(active.ID)
	s := active.Scene()
	alive := sel[:0]
	for _, id := range sel {
		if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
			alive = append(alive, id)
		}
	}
	a.ctx.Select(alive...)
}

func (a *App) collect(all bool) *state.Editor {
	set := a.ctx.Settings
	st := state.Default()
	st.Tool = int(set.Tool)
	st.Grid, st.Snap = set.Grid, set.Snap
	st.SnapMove, st.SnapRotate, st.SnapScale = set.SnapMove, set.SnapRotate, set.SnapScale
	if c := a.opt.Code; c != nil {
		st.Autosave, st.InlayHints, st.Minimap, st.RainbowBrackets, st.HighlightBlock = c.Autosave, c.Hints, c.Minimap, c.Rainbow, c.Block
	}
	for k, closed := range a.ctx.Collapsed {
		if closed {
			st.Collapsed = append(st.Collapsed, k)
		}
	}
	sort.Strings(st.Collapsed)
	m := a.eng.Metrics()
	st.WindowW, st.WindowH = m.LogicalW, m.LogicalH
	active := a.ctx.Doc()
	for _, d := range a.ctx.Docs.Docs() {
		if !d.OnDisk && !all {
			continue
		}
		sd := state.Doc{Path: d.Path, Camera: d.View.Camera, Mode: int(d.View.Mode), Orbit: d.View.Orbit}
		for id, open := range d.View.Expanded {
			if open {
				sd.Expanded = append(sd.Expanded, id)
			}
		}
		state.SortNodes(sd.Expanded)
		if active != nil && d.ID == active.ID {
			st.Active = d.Path
			sd.Selection = a.ctx.Selection.Nodes(d.ID, nil)
		}
		st.Docs = append(st.Docs, sd)
	}
	return st
}

func (a *App) saveState() {
	if err := state.Save(a.opt.Project.Root, a.collect(false)); err != nil {
		a.ctx.Notify(context.LevelWarn, "cannot save the editor state: "+err.Error())
	}
}

func (a *App) handoff() *session.Handoff {
	h := &session.Handoff{State: a.collect(true), Scenes: map[string][]byte{}}
	for _, d := range a.ctx.Docs.Docs() {
		if d.Dirty() {
			h.Scenes[d.Path] = sceneio.Save(d.Scene())
		}
	}
	return h
}
