package agent

import (
	"strconv"
	"strings"

	"aego/editor/core/agentui"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

func (p *Panel) diagnostics(ui *core.Context, busy bool) {
	h, ok := p.d.Host.(agentui.SceneHost)
	if !ok {
		return
	}
	widgets.Label(ui, i18n.Tr("editor.agent.connection_title", "Diagnostics"))
	d := h.Diagnostics()
	doc := d.Document
	if doc == "" {
		doc = i18n.Tr("editor.agent.no_open_scene", "No open scene")
	}
	state := d.State
	switch state {
	case "missing tools":
		state = i18n.Tr("editor.agent.conn_missing", "Required scene tools are missing")
	case "disconnected", "not connected":
		state = i18n.Tr("editor.agent.conn_offline", "Bridge disconnected")
	case "call rejected":
		state = i18n.Tr("editor.agent.conn_rejected", "Editor call rejected")
	case "verified":
		state = i18n.Tr("editor.agent.conn_probe_verified", "Editor probe succeeded")
	case "tools listed":
		state = i18n.Tr("editor.agent.conn_listed", "Tools listed; see last call")
	case "starting":
		state = i18n.Tr("editor.agent.conn_starting", "Connecting")
	}
	vision := i18n.Tr("editor.agent.cap_unknown", "Unverified")
	if d.Vision == "supported" {
		vision = i18n.Tr("editor.agent.cap_yes", "Supported")
	} else if d.Vision == "unsupported" {
		vision = i18n.Tr("editor.agent.cap_no", "Unsupported")
	}
	last := d.Last
	if last == "" {
		last = i18n.Tr("editor.agent.no_model_tool_call", "No model tool result in this thread")
	}
	s := i18n.Tr("editor.agent.connection_split", "{provider} · {model}\nProject: {project}\nDocument: {document}\nMCP: {state} ({count})\nThread: {thread}\nImage input: {vision}\nEditor probe: {probe}\nModel tool call: {last}", i18n.Str("provider", d.Provider), i18n.Str("model", d.Model), i18n.Str("project", d.Project), i18n.Str("document", doc), i18n.Str("state", state), i18n.Str("count", strconv.Itoa(d.Tools)), i18n.Str("thread", d.Thread), i18n.Str("vision", vision), i18n.Str("probe", d.Probe), i18n.Str("last", last))
	for _, line := range strings.Split(s, "\n") {
		widgets.LabelWrapped(ui, line, true)
	}
	if !busy && widgets.Button(ui, i18n.Tr("editor.agent.connection_check", "Check connection")) {
		p.settingError(h.CheckConnection())
	}
	if !busy && widgets.Button(ui, i18n.Tr("editor.agent.connection_reset", "Reconnect")) {
		p.NewSession()
		p.settingError(h.CheckConnection())
	}
	if ui.Clipboard != nil && widgets.Button(ui, i18n.Tr("editor.agent.connection_copy", "Copy diagnostics")) {
		ui.Clipboard.SetText(s)
	}
}
