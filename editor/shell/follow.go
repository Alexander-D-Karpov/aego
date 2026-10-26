package shell

import (
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/follow"
	"aego/editor/core/prefs"
	"aego/editor/core/presence"
	"aego/editor/core/selection"
	"aego/editor/core/workspace"
	agentpanel "aego/editor/panels/agent"
	"aego/engine/i18n"
	"aego/platform"
	"aego/schema"
)

// followNav is what follow mode does in this editor: it reaches the panels through the App (11b5b).
type followNav struct {
	a *App
}

func (a *App) setupFollow() {
	a.presence = presence.New()
	a.agentSel = selection.New()
	a.follow = follow.New(a.presence, a.agentSel, followNav{a: a})
	a.follow.SetOn(a.prefs.AgentFollow)
	a.follow.Resume = a.prefs.AgentResume
}

// followChanged keeps the Follow button in the preferences.
func (a *App) followChanged(on bool) {
	a.prefs.AgentFollow = on
	if err := prefs.Save(a.opt.PrefsDir, a.prefs); err != nil {
		a.ctx.Notify(context.LevelWarn, i18n.Note("editor.prefs.save_failed", "Cannot save the editor preferences: {err}", i18n.Str("err", err.Error())))
	}
}

// savePrefs writes the preferences, telling the user when it cannot.
func (a *App) savePrefs() {
	if err := prefs.Save(a.opt.PrefsDir, a.prefs); err != nil {
		a.ctx.Notify(context.LevelWarn, i18n.Note("editor.prefs.save_failed", "Cannot save the editor preferences: {err}", i18n.Str("err", err.Error())))
	}
}

// CommandProxyConfirm turns on confirming the changes that come through the editor's proxy (11b6).
const CommandProxyConfirm = "editor.proxy.confirm"

// ProxyConfirm reports whether changes from the CLI and outside agents wait for the user.
func (a *App) ProxyConfirm() bool {
	return a.prefs != nil && a.prefs.ProxyConfirm
}

// ExternalCall shows a call that came through the proxy the way follow mode shows the panel's agent.
func (a *App) ExternalCall(tool, args string) {
	if a.follow != nil {
		a.follow.Step(tool, args, a.eng.Platform().Now())
	}
}

// budgetsChanged keeps the agent's read limits in the preferences.
func (a *App) budgetsChanged(b agentui.Budgets) {
	a.prefs.AgentBudgets = b
	if err := prefs.Save(a.opt.PrefsDir, a.prefs); err != nil {
		a.ctx.Notify(context.LevelWarn, i18n.Note("editor.prefs.save_failed", "Cannot save the editor preferences: {err}", i18n.Str("err", err.Error())))
	}
}

// userActive pauses following on the user's own clicks, keys and scrolling, except those in the Agent panel.
func (a *App) userActive(ev platform.Event) {
	if a.follow == nil || !a.follow.On() {
		return
	}
	switch ev.Kind {
	case platform.EvKeyDown, platform.EvTextInput:
		if a.ws != nil && a.ws.Focused() == agentpanel.ID {
			return
		}
	case platform.EvMouseDown, platform.EvMouseWheel:
		if a.ws != nil {
			if r, ok := a.ws.Tree.PanelRect(agentpanel.ID); ok && r.Contains(a.ui.Mouse()) {
				return
			}
		}
	default:
		return
	}
	a.follow.UserActive(a.eng.Platform().Now())
}

// doc is the document follow mode works in: the running game's copy of the scene during Play, else the open one.
func (n followNav) doc(rel string) *document.SceneDocument {
	docs := n.a.ctx.Docs
	if p := docs.PlayDoc(); p != nil && p.Path == rel {
		return p
	}
	return docs.Find(rel)
}

func (n followNav) OpenScene(rel string) (selection.DocID, bool) {
	docs := n.a.ctx.Docs
	if p := docs.PlayDoc(); p != nil && p.Path == rel {
		return p.ID, true
	}
	d := docs.Find(rel)
	if d == nil {
		var err error
		if d, err = docs.Open(rel); err != nil {
			return 0, false
		}
	} else if docs.Edit() != d {
		docs.SetActive(d.ID)
	}
	if n.a.ws != nil {
		n.a.ws.Show(workspace.Scene)
	}
	return d.ID, true
}

func (n followNav) Node(rel, addr string) (ids.NodeID, bool) {
	d := n.doc(rel)
	if d == nil {
		return ids.NodeID{}, false
	}
	s := d.Scene()
	if hex, ok := strings.CutPrefix(addr, "#"); ok {
		id, err := ids.ParseNodeID(hex)
		if err != nil {
			return ids.NodeID{}, false
		}
		h := s.Handle(id)
		return id, !h.IsZero() && s.IsAlive(h)
	}
	p := strings.TrimPrefix(strings.TrimPrefix(addr, "/"), "Root/")
	if p == "" || p == "Root" {
		return ids.NodeID{}, false
	}
	h := s.Find(s.Root(), p)
	if h.IsZero() || !s.IsAlive(h) {
		return ids.NodeID{}, false
	}
	return s.ID(h), true
}

func (n followNav) Reveal(rel string, node ids.NodeID) {
	if d := n.doc(rel); d != nil && n.a.hierarchy != nil {
		n.a.hierarchy.Reveal(d, node)
	}
}

func (n followNav) Frame(rel string, node ids.NodeID) {
	if n.a.sceneView != nil {
		n.a.sceneView.FrameNodes([]ids.NodeID{node})
	}
}

func (n followNav) Inspect(rel string, node ids.NodeID, comp string) {
	if n.a.inspectorPanel == nil {
		return
	}
	if n.a.ws != nil {
		n.a.ws.Show(workspace.Inspector)
	}
	if comp == "" {
		return
	}
	if full := n.compName(rel, node, comp); full != "" {
		n.a.inspectorPanel.ScrollTo(full)
	}
}

// compName is the full name of a component the agent names by its full name or a unique suffix.
func (n followNav) compName(rel string, node ids.NodeID, comp string) string {
	reg := n.a.opt.Types.Schema()
	if td, ok := reg.ByName(comp); ok {
		return td.FullName
	}
	d := n.doc(rel)
	if d == nil {
		return ""
	}
	s := d.Scene()
	h := s.Handle(node)
	if h.IsZero() {
		return ""
	}
	found := ""
	s.ComponentsOf(h, func(t schema.TypeID) {
		if td, ok := reg.ByID(t); ok && found == "" && strings.HasSuffix(td.FullName, "."+comp) {
			found = td.FullName
		}
	})
	return found
}

func (n followNav) OpenScript(rel string, line int) {
	if c := n.a.opt.Code; c != nil {
		c.Follow(rel, line)
	}
}

func (n followNav) ScriptLine(rel, text string) int {
	src, ok := "", false
	if c := n.a.opt.Code; c != nil {
		src, ok = c.Text(rel)
	}
	if !ok {
		d, err := n.a.ctx.Docs.OpenScript(rel)
		if err != nil {
			return 0
		}
		src = string(d.Code.Bytes())
	}
	i := strings.Index(src, text)
	if i < 0 {
		return 0
	}
	return strings.Count(src[:i], "\n") + 1
}

func (n followNav) Bounds(rel string, node ids.NodeID) (mathx.Rect, bool) {
	if n.a.sceneView == nil {
		return mathx.Rect{}, false
	}
	return n.a.sceneView.NodeBounds(node)
}

func (n followNav) ShowProblems() {
	n.a.ws.Show(workspace.Problems)
	n.a.problemsPanel.Mark(n.a.ui.Time)
}

func (n followNav) ShowConsole() {
	n.a.ws.Show(workspace.Console)
	n.a.consolePanel.Mark(n.a.ui.Time)
}

func (n followNav) OpenData(rel, row string) {
	n.a.ctx.Open(context.AssetAt(rel))
}

func (n followNav) TypeName(id schema.TypeID) string {
	if td, ok := n.a.opt.Types.Schema().ByID(id); ok {
		return td.FullName
	}
	return ""
}

func (n followNav) FieldName(typ schema.TypeID, field schema.FieldID) string {
	if td, ok := n.a.opt.Types.Schema().ByID(typ); ok {
		if fd, ok := td.FieldByID(field); ok {
			return fd.Name
		}
	}
	return ""
}

// Follow is follow mode, for tests.
func (a *App) Follow() *follow.Service {
	return a.follow
}

// AgentSelection is the agent's own selection, for tests.
func (a *App) AgentSelection() *selection.Service {
	return a.agentSel
}

// Presence is where the agent works, for tests.
func (a *App) Presence() *presence.Model {
	return a.presence
}
