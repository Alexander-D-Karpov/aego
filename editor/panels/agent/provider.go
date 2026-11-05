package agent

import (
	"context"
	"errors"
	"slices"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

var providerIDs = [...]string{"claude", "codex"}
var providerLabels = [...]string{"Claude Code", "Codex"}

type modelCatalogResult struct {
	models []agentui.ModelOption
	err    error
}

func (p *Panel) cancelModelLoad() {
	if p.modelCancel != nil {
		p.modelCancel()
		p.modelCancel = nil
	}
	p.modelResults = nil
}

func (p *Panel) loadModels() {
	p.cancelModelLoad()
	p.modelError, p.models = nil, nil
	if p.Provider() == "claude" {
		p.models = []agentui.ModelOption{{ID: "sonnet", Label: "Sonnet"}, {ID: "opus", Label: "Opus"}, {ID: "haiku", Label: "Haiku"}}
		return
	}
	h, ok := p.d.Host.(agentui.ModelCatalogHost)
	if !ok {
		return
	}
	ch := make(chan modelCatalogResult, 1)
	p.modelResults = ch
	ctx, cancel := context.WithCancel(context.Background())
	p.modelCancel = cancel
	provider, command := p.Provider(), p.d.Host.Command()
	go func() {
		models, err := h.ListModels(ctx, provider, command)
		ch <- modelCatalogResult{models: models, err: err}
	}()
}

func (p *Panel) pollModels() {
	if p.modelResults == nil {
		return
	}
	select {
	case result := <-p.modelResults:
		p.cancelModelLoad()
		p.models, p.modelError = result.models, result.err
	default:
	}
}

func providerLabel(provider string) string {
	for i, id := range providerIDs {
		if id == provider {
			return providerLabels[i]
		}
	}
	return ""
}

// Provider is the selected backend, or empty for a host without provider settings.
func (p *Panel) Provider() string {
	if h, ok := p.d.Host.(agentui.ProviderHost); ok {
		return h.Provider()
	}
	return ""
}

// SelectProvider starts an independent conversation while preserving saved logs.
// A running turn must finish or be stopped before switching.
func (p *Panel) SelectProvider(provider string) error {
	h, ok := p.d.Host.(agentui.ProviderHost)
	if !ok || providerLabel(provider) == "" {
		return errors.New(i18n.Tr("editor.agent.provider_unavailable", "This agent provider is not available"))
	}
	if provider == h.Provider() {
		return nil
	}
	if p.d.Host.Busy() {
		return errors.New(i18n.Tr("editor.agent.provider_busy", "Stop the current turn before changing the agent"))
	}
	if len(p.queue) > 0 {
		return errors.New(i18n.Tr("editor.agent.queue_clear_first", "Send or clear queued messages before starting a new session."))
	}
	if err := h.SetProvider(provider); err != nil {
		return err
	}
	p.resetProviderSession()
	p.readProviderSettings()
	p.Recheck()
	p.cancelModelLoad()
	p.models, p.modelError = nil, nil
	return nil
}

func (p *Panel) resetProviderSession() {
	p.NewSession()
	// A provider has its own prompt, measured context size and model defaults.
	p.meter = nil
	p.layout = agentui.Layout{}
	p.layoutRev, p.ctxLabelRev = 0, 0
	p.ctxLabel, p.ctxWant = "", false
	p.summary.Reset()
}

func (p *Panel) readProviderSettings() {
	p.path = p.d.Host.Command()
	p.model, p.effort = "", ""
	p.customEffort = false
	if h, ok := p.d.Host.(agentui.ModelHost); ok {
		p.model, p.effort = h.Model(), h.Effort()
	}
}

func (p *Panel) setCommand(value string) {
	if len(p.queue) > 0 {
		p.notice(i18n.Tr("editor.agent.queue_clear_first", "Send or clear queued messages before starting a new session."))
		return
	}
	if p.d.Host.Busy() {
		return
	}
	value = strings.TrimSpace(value)
	effective := value
	if effective == "" {
		effective = p.Provider()
		if effective == "" {
			effective = "claude"
		}
	}
	if effective != p.d.Host.Command() {
		p.d.Host.SetCommand(value)
		p.resetProviderSession()
	}
	p.path = p.d.Host.Command()
	p.Recheck()
	p.loadModels()
}

func (p *Panel) settingError(err error) {
	if err != nil {
		p.push(block{kind: blockError, text: err.Error()})
		p.record(agentui.Entry{Kind: "error", Text: err.Error()})
	}
}

// providerRow is present only for selectable CLI hosts, not replay/test hosts.
func (p *Panel) providerRow(ui *core.Context, r mathx.Rect) float32 {
	provider := p.Provider()
	if providerLabel(provider) == "" {
		return 0
	}
	th := &ui.Theme
	height := th.RowHeight + th.Padding
	row := mathx.Rect{X: r.X + th.Padding, Y: r.Y + th.Padding/2, W: max(0, r.W-2*th.Padding), H: th.RowHeight}
	ui.Rect(mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: height}, th.Header)
	label := i18n.Tr("editor.agent.provider_settings", "Agent settings")
	settingsW := min(row.W, row.H)
	choice := mathx.Rect{X: row.X, Y: row.Y, W: min(140, max(0, row.W-settingsW-6)), H: row.H}
	index := 0
	if provider == "codex" {
		index = 1
	}
	if p.d.Host.Busy() {
		ui.Box(choice, th.Radius.Field, th.Widget, th.Border)
		ui.TextIn(choice, providerLabels[index], th.TextDisabled, core.AlignLeft)
		ui.MouseOver(ui.ID("provider"), choice)
		widgets.Tooltip(ui, ui.ID("provider"), i18n.Tr("editor.agent.provider_busy", "Stop the current turn before changing the agent"))
	} else if widgets.Combo(ui, ui.ID("provider"), choice, &index, providerLabels[:], false) {
		p.settingError(p.SelectProvider(providerIDs[index]))
		ui.ClosePopup(p.providerSettings)
	}
	settings := mathx.Rect{X: row.X + row.W - settingsW, Y: row.Y, W: settingsW, H: row.H}
	p.settingsRect = settings
	if agentButton(ui, ui.ID("provider-settings"), settings, "panel.settings", "", label, ui.PopupOpen(p.providerSettings)) {
		p.readProviderSettings()
		p.loadModels()
		ui.OpenPopup(p.providerSettings, mathx.Vec2{X: max(0, settings.X+settings.W-360), Y: row.Y + row.H + 4})
	}
	if h, ok := p.d.Host.(agentui.ModelHost); ok {
		model := h.Model()
		if p.meter != nil && p.meter.Model != "" {
			model = p.meter.Model
		}
		if model == "" {
			model = i18n.Tr("editor.agent.cli_default", "CLI default")
		}
		mr := mathx.Rect{X: choice.X + choice.W + 8, Y: row.Y, W: max(0, settings.X-choice.X-choice.W-16), H: row.H}
		ui.TextIn(mr, p.fits[4].get(ui, model, mr.W), th.TextDim, core.AlignLeft)
		ui.MouseOver(ui.ID("model-status"), mr)
		widgets.Tooltip(ui, ui.ID("model-status"), model)
	}
	p.providerPopup(ui)
	return height
}

func (p *Panel) providerPopup(ui *core.Context) {
	if !ui.BeginPopup(p.providerSettings, 360) {
		p.cancelModelLoad()
		ui.PushID("provider-settings")
		ui.PushID("model-picker")
		if ui.Focus() == ui.ID("search") {
			ui.CancelFocus()
		}
		ui.PopID()
		ui.PopID()
		return
	}
	ui.PushID("provider-settings")
	p.pollModels()
	widgets.Label(ui, providerLabel(p.Provider()))
	widgets.LabelWrapped(ui, i18n.Tr("editor.agent.settings_session", "Changing a setting starts a new session. Previous sessions stay in History."), true)
	busy := p.d.Host.Busy()
	if busy {
		widgets.LabelWrapped(ui, i18n.Tr("editor.agent.provider_busy", "Stop the current turn before changing the agent"), true)
	}
	field := widgets.Field(ui, i18n.Tr("editor.agent.path", "Path"))
	if busy {
		ui.TextIn(field, widgets.Elide(ui, p.path, field.W), ui.Theme.TextDisabled, core.AlignLeft)
	} else if _, committed := widgets.TextFieldRect(ui, ui.ID("command"), field, &p.path, false); committed {
		p.setCommand(p.path)
	}
	widgets.Tooltip(ui, ui.ID("command"), i18n.Tr("editor.agent.path_help", "Executable name or full path. Press Enter to apply; leave empty to use the CLI on PATH."))
	if h, ok := p.d.Host.(agentui.ModelHost); ok {
		field = widgets.Field(ui, i18n.Tr("editor.agent.model_override", "Model"))
		if busy {
			ui.TextIn(field, widgets.Elide(ui, p.modelLabel(), field.W), ui.Theme.TextDisabled, core.AlignLeft)
		} else {
			p.modelPicker(ui, field, h)
		}
		for _, model := range p.models {
			if model.ID == p.model && model.Description != "" {
				widgets.LabelWrapped(ui, model.Description, true)
				break
			}
		}
		if p.Provider() == "codex" {
			field = widgets.Field(ui, i18n.Tr("editor.agent.reasoning_effort", "Reasoning effort"))
			if busy {
				ui.TextIn(field, widgets.Elide(ui, effortLabel(p.effort), field.W), ui.Theme.TextDisabled, core.AlignLeft)
			} else {
				p.effortPicker(ui, field, h)
			}
			if p.customEffort && !busy {
				custom := widgets.Field(ui, i18n.Tr("editor.agent.effort_custom", "Custom level…"))
				if _, committed := widgets.TextFieldRect(ui, ui.ID("custom-effort"), custom, &p.effort, false); committed {
					p.selectEffort(h, p.effort)
					p.customEffort = false
				}
			}
			widgets.LabelWrapped(ui, effortHelp(p.effort), true)
			if efforts := p.modelEfforts(); p.effort != "" && len(efforts) > 0 && !slices.Contains(efforts, p.effort) {
				widgets.LabelWrapped(ui, i18n.Tr("editor.agent.effort_unsupported", "This model does not list the selected effort. Choose a supported level or CLI default."), true)
			}
		}
		widgets.LabelWrapped(ui, i18n.Tr("editor.agent.model_picker_help", "Choose a model or search by name. CLI default uses your existing CLI settings."), true)
		if p.modelResults != nil {
			widgets.LabelWrapped(ui, i18n.Tr("editor.agent.models_loading", "Loading models from Codex…"), true)
		} else if p.modelError != nil {
			widgets.LabelWrapped(ui, i18n.Tr("editor.agent.models_unavailable", "Could not load models. Check the CLI path and login, or enter a model ID in search."), true)
		}
	}
	if !busy && widgets.Button(ui, i18n.Tr("editor.agent.check_again", "Check Again")) {
		p.Recheck()
		p.loadModels()
	}
	if h, ok := p.d.Host.(agentui.PlaytestTrustHost); ok && h.PlaytestTrusted() {
		if widgets.Button(ui, i18n.Tr("editor.agent.revoke_playtest", "Revoke always-allowed playtests")) {
			p.settingError(h.RevokePlaytestTrust())
		}
	}
	p.diagnostics(ui, busy)
	ui.PopID()
	ui.EndPopup()
}

func (p *Panel) modelLabel() string {
	if p.model == "" {
		return i18n.Tr("editor.agent.cli_default", "CLI default")
	}
	for _, m := range p.models {
		if m.ID == p.model && m.Label != "" {
			return m.Label
		}
	}
	return p.model
}

func (p *Panel) selectModel(h agentui.ModelHost, value string) {
	if len(p.queue) > 0 {
		p.notice(i18n.Tr("editor.agent.queue_clear_first", "Send or clear queued messages before starting a new session."))
		return
	}
	value = strings.TrimSpace(value)
	if value != h.Model() {
		if err := h.SetModel(value); err != nil {
			p.settingError(err)
		} else {
			p.resetProviderSession()
		}
	}
	p.model = h.Model()
}

func (p *Panel) filteredModels() []agentui.ModelOption {
	query := strings.ToLower(strings.TrimSpace(p.modelQuery))
	out := make([]agentui.ModelOption, 0, len(p.models)+1)
	if query == "" {
		out = append(out, agentui.ModelOption{Label: i18n.Tr("editor.agent.cli_default", "CLI default")})
	}
	for _, m := range p.models {
		if query == "" || strings.Contains(strings.ToLower(m.ID+" "+m.Label+" "+m.Description), query) {
			out = append(out, m)
		}
	}
	return out
}

func (p *Panel) modelPicker(ui *core.Context, field mathx.Rect, h agentui.ModelHost) {
	id := ui.ID("model-picker")
	if widgets.ButtonRect(ui, id, field, p.modelLabel()) {
		p.modelQuery, p.modelPick = "", 0
		ui.OpenPopup(id, mathx.Vec2{X: field.X, Y: field.Y + field.H})
		ui.PushID("model-picker")
		ui.SetFocus(ui.ID("search"))
		ui.EndEdit()
		ui.PopID()
	}
	widgets.Chevron(ui, mathx.Rect{X: field.X + field.W - field.H, Y: field.Y, W: field.H, H: field.H}, true, ui.Theme.TextDim)
	if !ui.BeginPopup(id, max(field.W, 260)) {
		ui.PushID("model-picker")
		if ui.Focus() == ui.ID("search") {
			ui.CancelFocus()
		}
		ui.PopID()
		return
	}
	ui.PushID("model-picker")
	if widgets.SearchField(ui, ui.ID("search"), ui.Cursor().Next(0), &p.modelQuery) {
		p.modelPick = 0
	}
	options := p.filteredModels()
	query := strings.TrimSpace(p.modelQuery)
	custom := query != ""
	for _, m := range options {
		if strings.EqualFold(m.ID, query) {
			custom = false
		}
	}
	// Keep the popup within the screen; searching narrows a long catalog.
	const limit = 8
	visible := min(len(options), limit)
	count := visible
	if custom {
		count++
	}
	if count > 0 {
		if ui.KeyPressed(input.KeyDown) {
			p.modelPick = (p.modelPick + 1) % count
		}
		if ui.KeyPressed(input.KeyUp) {
			p.modelPick = (p.modelPick + count - 1) % count
		}
	}
	chosen := ""
	picked := false
	for i := 0; i < visible; i++ {
		m := options[i]
		label := m.Label
		if label == "" {
			label = m.ID
		} else if m.ID != "" && !strings.EqualFold(label, m.ID) {
			label += " · " + m.ID
		}
		rowID := ui.IDInt(i)
		if widgets.Selectable(ui, rowID, label, i == p.modelPick) {
			chosen, picked = m.ID, true
		}
		widgets.Tooltip(ui, rowID, m.Description)
	}
	if len(options) > visible {
		widgets.Label(ui, i18n.Tr("editor.agent.models_narrow", "Type to narrow the list…"))
	}
	if custom && widgets.Selectable(ui, ui.ID("custom"), i18n.Tr("editor.agent.model_use_custom", "Use custom ID")+": "+query, p.modelPick == visible) {
		chosen, picked = query, true
	}
	if count > 0 && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter)) {
		if p.modelPick < visible {
			chosen = options[p.modelPick].ID
		} else if custom {
			chosen = query
		}
		picked = true
	}
	if picked {
		p.selectModel(h, chosen)
		if ui.Focus() == ui.ID("search") {
			ui.CancelFocus()
		}
		ui.ClosePopup(id)
	}
	ui.PopID()
	ui.EndPopup()
}

func (p *Panel) modelEfforts() []string {
	for _, m := range p.models {
		if m.ID == p.model && len(m.Efforts) > 0 {
			return m.Efforts
		}
	}
	return nil
}

func (p *Panel) effortPicker(ui *core.Context, field mathx.Rect, h agentui.ModelHost) {
	values := append([]string{""}, p.modelEfforts()...)
	if len(values) == 1 {
		values = append(values, "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra")
	}
	index := -1
	for i, v := range values {
		if v == p.effort {
			index = i
		}
	}
	if index < 0 {
		values = append(values, p.effort)
		index = len(values) - 1
	}
	labels := make([]string, len(values), len(values)+1)
	for i, v := range values {
		labels[i] = effortLabel(v)
	}
	labels = append(labels, i18n.Tr("editor.agent.effort_custom", "Custom level…"))
	if widgets.Combo(ui, ui.ID("effort"), field, &index, labels, false) {
		if index == len(values) {
			p.customEffort = true
		} else {
			p.customEffort = false
			p.selectEffort(h, values[index])
		}
	}
}

func (p *Panel) selectEffort(h agentui.ModelHost, value string) {
	if len(p.queue) > 0 {
		p.notice(i18n.Tr("editor.agent.queue_clear_first", "Send or clear queued messages before starting a new session."))
		return
	}
	value = strings.TrimSpace(value)
	if value != h.Effort() {
		if err := h.SetEffort(value); err != nil {
			p.settingError(err)
		} else {
			p.resetProviderSession()
		}
	}
	p.effort = h.Effort()
}

func effortLabel(value string) string {
	switch value {
	case "":
		return i18n.Tr("editor.agent.cli_default", "CLI default")
	case "none":
		return i18n.Tr("editor.agent.effort_none", "None")
	case "minimal":
		return i18n.Tr("editor.agent.effort_minimal", "Minimal")
	case "low":
		return i18n.Tr("editor.agent.effort_low", "Low")
	case "medium":
		return i18n.Tr("editor.agent.effort_medium", "Medium")
	case "high":
		return i18n.Tr("editor.agent.effort_high", "High")
	case "xhigh":
		return i18n.Tr("editor.agent.effort_xhigh", "Extra high")
	case "max":
		return i18n.Tr("editor.agent.effort_max", "Maximum")
	case "ultra":
		return i18n.Tr("editor.agent.effort_ultra", "Ultra")
	default:
		return value
	}
}

func effortHelp(value string) string {
	switch value {
	case "", "none", "minimal", "low":
		return i18n.Tr("editor.agent.effort_help_fast", "Lower effort is faster; CLI default follows your Codex configuration.")
	case "medium":
		return i18n.Tr("editor.agent.effort_help_balanced", "Medium balances speed and depth of reasoning.")
	default:
		return i18n.Tr("editor.agent.effort_help_deep", "Higher effort can help with complex tasks, but may take longer and use more quota.")
	}
}
