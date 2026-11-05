package edit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/editor/core/agentui"
	"aego/scene/mutate"
)

// agentHost runs the model and the bridge for the Agent panel; the bridge and the process start on the first message (D343).
type agentHost struct {
	lastConn             agentui.Connection
	lastTools            map[string]string
	sceneTurn, sceneRead bool
	// An explicit backend is fixed (test/replay); nil follows the saved provider.
	backend       backend.Backend
	dir           string
	bridgeOf      func() (*agentbridge.Bridge, error)
	bridgeOpen    func() bool
	settings      func(string) agentui.ProviderSettings
	save          func(agentui.ProviderSettings)
	bridge        *agentbridge.Bridge
	playtestTrust *agentui.PlaytestTrust
	conv          backend.Conversation
	busy          bool
	auto          bool
	plan          bool
	budgets       agentui.Budgets
	static        map[int]agentui.Card
}

func (h *agentHost) PlaytestTrusted() bool { return h.playtestTrust.Allowed() }

func (h *agentHost) RevokePlaytestTrust() error {
	if err := h.playtestTrust.Revoke(); err != nil {
		return err
	}
	if h.bridge != nil {
		h.bridge.RevokePlaytest()
	}
	return nil
}

func newAgentHost(b backend.Backend, bridge func() (*agentbridge.Bridge, error), settings func(string) agentui.ProviderSettings, save func(agentui.ProviderSettings)) *agentHost {
	return &agentHost{backend: b, bridgeOf: bridge, settings: settings, save: save, static: map[int]agentui.Card{}, budgets: agentui.DefaultBudgets}
}

func (h *agentHost) options() agentui.ProviderSettings {
	provider := ""
	if h.backend != nil && (h.backend.Name() == "claude" || h.backend.Name() == "codex") {
		provider = h.backend.Name()
	}
	return h.settings(provider)
}

func (h *agentHost) currentBackend() (backend.Backend, error) {
	if h.backend != nil {
		return h.backend, nil
	}
	return backend.ForProvider(h.options().Provider)
}

func (h *agentHost) Available(cmd string) (string, error) {
	b, err := h.currentBackend()
	if err != nil {
		return "", err
	}
	return b.Available(backend.Config{Command: cmd})
}

func (h *agentHost) Provider() string {
	if h.backend != nil {
		return h.backend.Name()
	}
	provider := h.options().Provider
	if provider == "" {
		provider = "claude"
	}
	return provider
}

func (h *agentHost) SetProvider(provider string) error {
	if _, err := backend.ForProvider(provider); err != nil {
		return err
	}
	if provider == "" {
		provider = "claude"
	}
	if provider == h.Provider() {
		return nil
	}
	if h.backend != nil {
		return errors.New("the injected agent backend cannot be changed")
	}
	if h.busy {
		return errors.New("stop the current agent turn before changing provider")
	}
	h.Reset()
	h.save(h.settings(provider))
	return nil
}

func (h *agentHost) Command() string {
	if c := h.options().Command; c != "" {
		return c
	}
	if h.Provider() == "codex" {
		return "codex"
	}
	return "claude"
}

func (h *agentHost) SetCommand(path string) {
	if h.busy {
		return
	}
	s := h.options()
	if path == h.Provider() {
		path = ""
	}
	if path == s.Command {
		return
	}
	h.Reset()
	s.Command = path
	h.save(s)
}

func (h *agentHost) Model() string { return h.options().Model }

func (h *agentHost) Effort() string { return h.options().Effort }

func (h *agentHost) ListModels(ctx context.Context, provider, command string) ([]agentui.ModelOption, error) {
	if provider != "codex" {
		return nil, nil
	}
	models, err := backend.CodexModels(ctx, command, h.dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentui.ModelOption, 0, len(models))
	for _, m := range models {
		out = append(out, agentui.ModelOption{ID: m.ID, Label: m.Label, Description: m.Description, Efforts: m.Efforts})
	}
	return out, nil
}

func agentSetting(value, name string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 256 || strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("invalid agent %s", name)
	}
	return value, nil
}

func (h *agentHost) SetModel(model string) error {
	model, err := agentSetting(model, "model")
	if err != nil {
		return err
	}
	s := h.options()
	if model == s.Model {
		return nil
	}
	if h.busy {
		return errors.New("stop the current agent turn before changing model")
	}
	h.Reset()
	s.Model = model
	h.save(s)
	return nil
}

func (h *agentHost) SetEffort(effort string) error {
	effort, err := agentSetting(effort, "reasoning effort")
	if err != nil {
		return err
	}
	if h.Provider() != "codex" && effort != "" {
		return errors.New("reasoning effort is configured here only for Codex")
	}
	s := h.options()
	if effort == s.Effort {
		return nil
	}
	if h.busy {
		return errors.New("stop the current agent turn before changing reasoning effort")
	}
	h.Reset()
	s.Effort = effort
	h.save(s)
	return nil
}

func (h *agentHost) Running() bool {
	return h.conv != nil
}

func (h *agentHost) Busy() bool {
	return h.busy
}

// attach takes the editor's bridge with the panel's settings; the proxy may have started it before the panel's first message.
func (h *agentHost) attach() error {
	if h.bridge == nil {
		b, err := h.bridgeOf()
		if err != nil {
			return err
		}
		b.SetAutoApply(h.auto)
		b.SetPlanOnly(h.plan)
		b.SetBudgets(h.budgets)
		h.bridge = b
	}
	return nil
}

func (h *agentHost) start() error {
	if err := h.attach(); err != nil {
		return err
	}
	if h.conv == nil {
		b, err := h.currentBackend()
		if err != nil {
			return err
		}
		cfg := h.bridge.Backend()
		options := h.options()
		// The bridge may predate a provider/path change (for example when the CLI proxy started it).
		cfg.Command, cfg.Model, cfg.Effort = options.Command, options.Model, options.Effort
		c, err := b.Start(cfg)
		if err != nil {
			return err
		}
		h.lastConn = agentui.Connection{}
		h.conv = c
	}
	return nil
}

func (h *agentHost) SendImages(text string, pngs [][]byte) error {
	if err := h.start(); err != nil {
		return err
	}
	sender, ok := h.conv.(interface{ SendImages(string, [][]byte) error })
	if !ok {
		return errors.New("this agent does not support image input")
	}
	if d, ok := h.conv.(backend.Diagnostics); ok && d.Connection().Vision == "unsupported" {
		return errors.New("the selected model does not support image input")
	}
	if err := sender.SendImages(text, pngs); err != nil {
		return err
	}
	h.sceneTurn = false
	h.busy = true
	return nil
}
func (h *agentHost) Send(text string) error {
	if err := h.start(); err != nil {
		return err
	}
	h.sceneTurn = false
	if err := h.conv.Send(text); err != nil {
		return err
	}
	h.busy = true
	return nil
}

func (h *agentHost) Cancel() {
	if h.conv != nil {
		h.conv.Cancel()
	}
	if h.bridge != nil {
		h.bridge.RejectAll()
	}
}

func (h *agentHost) Close() {
	if h.conv != nil {
		h.conv.Close()
		h.conv = nil
	}
	if h.bridge != nil {
		h.bridge.Rebind()
		h.bridge = nil
	}
	h.busy = false
}

func eventOf(e backend.Event) agentui.Event {
	out := agentui.Event{Images: e.Images, Text: e.Text, Tool: e.Tool, CallID: e.CallID, Args: string(e.Args), Result: e.Result, Failed: e.Failed, Cost: e.Cost, Millis: e.Millis,
		Tokens: e.Tokens, Window: e.Window, Model: e.Model, Delta: e.Delta, MessageID: e.MessageID}
	switch e.Kind {
	case backend.EvText:
		out.Kind = agentui.EvText
	case backend.EvToolStart:
		out.Kind = agentui.EvToolStart
	case backend.EvToolResult:
		out.Kind = agentui.EvToolResult
	case backend.EvTurnEnd:
		out.Kind = agentui.EvTurnEnd
	case backend.EvUsage:
		out.Kind = agentui.EvUsage
	case backend.EvCompact:
		out.Kind = agentui.EvCompact
	default:
		out.Kind = agentui.EvError
		if e.Err != nil {
			out.Text = e.Err.Error()
		}
	}
	return out
}

// Poll takes the events waiting in the conversation without blocking.
func (h *agentHost) Poll(dst []agentui.Event) []agentui.Event {
	for h.conv != nil {
		select {
		case e, ok := <-h.conv.Events():
			if !ok {
				h.lastConn = h.Diagnostics()
				if h.lastConn.State == "verified" || h.lastConn.State == "tools listed" {
					h.lastConn.State = "disconnected"
				}
				h.conv.Close()
				h.conv, h.busy = nil, false
				return append(dst, agentui.Event{Kind: agentui.EvError, Text: "the agent process ended; the next message starts a new one"})
			}
			if e.Kind == backend.EvTurnEnd || e.Kind == backend.EvError {
				h.busy = false
			}
			switch e.Kind {
			case backend.EvError:
				if e.Err != nil {
					msg := strings.ToLower(e.Err.Error())
					if strings.Contains(msg, "does not support image") || strings.Contains(msg, "image input is not supported") || strings.Contains(msg, "image inputs are not supported") {
						h.lastConn.Vision = "unsupported"
					}
				}
			case backend.EvToolStart:
				if h.lastTools == nil {
					h.lastTools = map[string]string{}
				}
				h.lastTools[e.CallID] = e.Tool
			case backend.EvToolResult:
				tool := e.Tool
				if tool == "" {
					tool = h.lastTools[e.CallID]
				}
				delete(h.lastTools, e.CallID)
				if e.Failed {
					h.lastConn.Last = tool + ": failed"
				} else {
					h.lastConn.Last = tool + ": OK"
					if tool == "scene_tree" || tool == "script_read" || tool == "editor_scene_context" {
						h.sceneRead = true
					}
				}
			case backend.EvTurnEnd:
				if h.sceneTurn && !h.sceneRead {
					h.lastConn.Last = "Explain: snapshot attached; model did not read a scene tool"
				}
				h.sceneTurn = false
			}
			dst = append(dst, eventOf(e))
		default:
			return dst
		}
	}
	return dst
}

func stateOf(s agentbridge.State) agentui.State {
	switch s {
	case agentbridge.Approved:
		return agentui.Approved
	case agentbridge.Applied:
		return agentui.Applied
	case agentbridge.Rejected:
		return agentui.Rejected
	case agentbridge.Expired:
		return agentui.Expired
	case agentbridge.Failed:
		return agentui.Failed
	case agentbridge.Undone:
		return agentui.Undone
	}
	return agentui.Pending
}

func cardOf(p *agentbridge.Proposal, v agentView) agentui.Card {
	c := agentui.Card{ID: p.ID, Command: p.Command, Destructive: p.Destructive}
	if b, err := json.Marshal(p.Args); err == nil {
		c.Args = string(b)
	}
	c.File = p.Args.Str("file")
	if out := p.Preview; out != nil {
		for _, img := range out.Images {
			if img.Mime == "image/png" && len(c.PreviewImages) < 3 && len(img.Data) <= 5<<20 {
				c.PreviewImages = append(c.PreviewImages, img.Data)
			}
		}
		c.Changes = changesOf(out.Changes)
		c.Rows = v.rows(c.File, out.Changes)
		c.Notes = append(c.Notes, out.Notes...)
		c.Problems = problemTexts(out)
	}
	return c
}

func changesOf(list []mutate.Change) []agentui.Change {
	var out []agentui.Change
	for _, ch := range list {
		c := agentui.Change{Op: ch.Op, Node: ch.Node, Path: ch.Path, Before: ch.Before, After: ch.After, NodeID: ch.NodeID, Type: ch.Type, Field: ch.Field}
		if ch.Op == "script-edit" || ch.Op == "script-new" {
			c.Line, _ = strconv.Atoi(strings.TrimPrefix(ch.Path, "line "))
			if ch.Op == "script-new" {
				c.Line = 1
			}
		}
		out = append(out, c)
	}
	return out
}

func problemTexts(out *agent.Output) []string {
	var list []string
	for _, p := range out.Problems {
		level := "warning: "
		if p.Error {
			level = "error: "
		}
		list = append(list, level+p.Message)
	}
	return list
}

// Poll the shared editor bridge for outside proposals, including before the
// panel's first conversation. An idle frame must never start a listener.
func (h *agentHost) canPollBridge() bool {
	if h.bridge != nil {
		return true
	}
	return h.bridgeOpen != nil && h.bridgeOpen() && h.attach() == nil
}

// Cards are the bridge's proposals; their fixed parts are built once.
func (h *agentHost) Cards(dst []agentui.Card) []agentui.Card {
	if !h.canPollBridge() {
		return dst
	}
	for _, p := range h.bridge.Proposals() {
		c, ok := h.static[p.ID]
		if !ok {
			c = cardOf(p, agentView{ctx: h.bridge.Editor()})
			h.static[p.ID] = c
		}
		if p.State == agentbridge.Applied && c.Applied == nil && p.Result != nil {
			c.Applied = changesOf(p.Result.Changes)
			if c.Applied == nil {
				c.Applied = []agentui.Change{}
			}
			h.static[p.ID] = c
		}
		c.State, c.Err, c.CanUndo = stateOf(p.State), p.Err, h.bridge.CanUndo(p.ID)
		dst = append(dst, c)
	}
	return dst
}

func (h *agentHost) Decide(id int, apply bool) {
	if h.bridge != nil {
		h.bridge.Decide(id, apply)
	}
}

func (h *agentHost) Undo(id int) error {
	if h.bridge == nil {
		return nil
	}
	return h.bridge.Undo(id)
}

func (h *agentHost) AutoApply() bool {
	return h.auto
}

func (h *agentHost) SetAutoApply(on bool) {
	h.auto = on
	if h.bridge != nil {
		h.bridge.SetAutoApply(on)
	}
}

var _ agentui.Host = (*agentHost)(nil)

func (h *agentHost) PlanOnly() bool {
	return h.plan
}

func (h *agentHost) SetPlanOnly(on bool) {
	h.plan = on
	if h.bridge != nil {
		h.bridge.SetPlanOnly(on)
	}
}

// Reset ends the conversation and dismisses what waits for the user; the bridge stays, so the next message starts a new conversation quickly.
func (h *agentHost) Reset() {
	h.lastConn = agentui.Connection{}
	clear(h.lastTools)
	h.sceneRead = false
	h.sceneTurn = false
	if h.bridge != nil {
		h.bridge.Rebind()
	}
	if h.conv != nil {
		h.conv.Close()
		h.conv = nil
	}
	h.busy = false
}

func (h *agentHost) Questions(dst []agentui.Question) []agentui.Question {
	if !h.canPollBridge() {
		return dst
	}
	for _, q := range h.bridge.Questions() {
		out := agentui.Question{ID: q.ID, Text: q.Text, Multiple: q.Multiple, Other: q.Other, State: stateOf(q.State), Answer: q.Answer}
		for _, o := range q.Options {
			out.Options = append(out.Options, agentui.Option{Label: o.Label, Detail: o.Detail})
		}
		dst = append(dst, out)
	}
	return dst
}

func (h *agentHost) Answer(id int, picks []int, text string) {
	if h.bridge != nil {
		_ = h.bridge.Answer(id, picks, text)
	}
}

func (h *agentHost) ContextBase() (string, string) {
	if h.bridge == nil {
		return "", ""
	}
	return h.bridge.ContextBase()
}

func (h *agentHost) Budgets() agentui.Budgets {
	return h.budgets
}

func (h *agentHost) SetBudgets(b agentui.Budgets) {
	h.budgets = b
	if h.bridge != nil {
		h.bridge.SetBudgets(b)
	}
}
