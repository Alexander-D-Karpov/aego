package agent

import (
	stdcontext "context"
	"errors"
	"strings"
	"testing"
	"time"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/platform/input"
	"aego/ui/core"
)

type providerTestHost struct {
	agentui.Host
	provider  string
	commands  map[string]string
	models    map[string]string
	efforts   map[string]string
	busy      bool
	resets    int
	sent      []string
	cards     []agentui.Card
	questions []agentui.Question
	probe     func(string) (string, error)
	catalog   func(stdcontext.Context, string, string) ([]agentui.ModelOption, error)
}

func newProviderTestHost() *providerTestHost {
	return &providerTestHost{provider: "claude", commands: map[string]string{"claude": "claude", "codex": "codex"}, models: map[string]string{}, efforts: map[string]string{}}
}

func (h *providerTestHost) Provider() string { return h.provider }
func (h *providerTestHost) SetProvider(v string) error {
	if v != "claude" && v != "codex" {
		return errors.New("unsupported provider")
	}
	h.provider = v
	return nil
}
func (h *providerTestHost) Command() string     { return h.commands[h.provider] }
func (h *providerTestHost) SetCommand(v string) { h.commands[h.provider] = v }
func (h *providerTestHost) Model() string       { return h.models[h.provider] }
func (h *providerTestHost) SetModel(v string) error {
	h.models[h.provider] = v
	return nil
}
func (h *providerTestHost) Effort() string { return h.efforts[h.provider] }
func (h *providerTestHost) SetEffort(v string) error {
	h.efforts[h.provider] = v
	return nil
}
func (h *providerTestHost) Available(command string) (string, error) {
	if h.probe != nil {
		return h.probe(command)
	}
	return command + " version", nil
}
func (h *providerTestHost) ListModels(ctx stdcontext.Context, provider, command string) ([]agentui.ModelOption, error) {
	if h.catalog != nil {
		return h.catalog(ctx, provider, command)
	}
	return nil, nil
}
func (h *providerTestHost) Running() bool { return false }
func (h *providerTestHost) Busy() bool    { return h.busy }
func (h *providerTestHost) Reset() {
	h.resets++
	for i := range h.cards {
		if h.cards[i].State == agentui.Pending || h.cards[i].State == agentui.Approved {
			h.cards[i].State = agentui.Rejected
		}
	}
	for i := range h.questions {
		if h.questions[i].State == agentui.Pending {
			h.questions[i].State = agentui.Rejected
		}
	}
}
func (h *providerTestHost) AutoApply() bool { return false }
func (h *providerTestHost) PlanOnly() bool  { return false }
func (h *providerTestHost) Send(text string) error {
	h.sent = append(h.sent, text)
	return nil
}
func (h *providerTestHost) Close() {}
func (h *providerTestHost) Poll(dst []agentui.Event) []agentui.Event {
	return dst
}
func (h *providerTestHost) Cards(dst []agentui.Card) []agentui.Card {
	return append(dst, h.cards...)
}
func (h *providerTestHost) Questions(dst []agentui.Question) []agentui.Question {
	return append(dst, h.questions...)
}
func (h *providerTestHost) Describe(tool, args, result string, failed bool) agentui.Step {
	return agentui.Step{Title: tool}
}
func (h *providerTestHost) ContextBase() (string, string) {
	return h.provider + " instructions", "tool schema"
}

func providerPanel(t *testing.T, host *providerTestHost) *Panel {
	t.Helper()
	p := New(Deps{Host: host, Root: t.TempDir(), Notices: &context.Editor{}})
	t.Cleanup(p.Close)
	return p
}

func TestProviderSwitchSeparatesContextAndSavedTranscripts(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.Submit("old provider question")
	p.event(agentui.Event{Kind: agentui.EvText, Text: "old provider answer"})
	old := p.log
	if old == nil {
		t.Fatal("missing first transcript")
	}
	p.seed = "old provider summary"
	p.seen[7] = true // Existing bridge cards must not reappear in the new session.
	if err := p.SelectProvider("codex"); err != nil {
		t.Fatal(err)
	}
	if p.log != nil || p.meter != nil || p.seed != "" || len(p.blocks) != 0 || !p.seen[7] {
		t.Fatal("provider switch retained context or forgot historical bridge cards")
	}
	if err := old.Append(agentui.Entry{Kind: "text", Text: "must be closed"}); err == nil {
		t.Fatal("previous transcript is still writable")
	}
	p.Submit("new provider question")
	if p.log == nil || p.log.ID == old.ID {
		t.Fatal("two provider sessions share a transcript file")
	}
	if len(h.sent) != 2 || strings.Contains(h.sent[1], "old provider") {
		t.Fatalf("old conversation leaked into new provider: %q", h.sent)
	}
	for id, want := range map[string]string{old.ID: "Claude Code · old provider question", p.log.ID: "Codex · new provider question"} {
		if got := sessionName(p.d.Root, id); got != want {
			t.Fatalf("session %s: got %q, want %q", id, got, want)
		}
	}
	entries, err := agentui.Load(p.d.Root, old.ID)
	if err != nil || len(entries) != 3 || entries[0].Provider != "claude" {
		t.Fatalf("old session not preserved: %#v, %v", entries, err)
	}
}

func TestProviderSwitchRejectsBusyAndInvalidChoices(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.Submit("keep this conversation")
	old := p.log
	h.busy = true
	if err := p.SelectProvider("codex"); err == nil {
		t.Fatal("busy provider switch succeeded")
	}
	h.busy = false
	if err := p.SelectProvider("unknown"); err == nil {
		t.Fatal("unknown provider switch succeeded")
	}
	if p.Provider() != "claude" || p.log != old || len(p.blocks) != 1 || h.resets != 0 {
		t.Fatal("rejected switch changed the live session")
	}
}

func TestProviderSwitchKeepsDismissedBridgeWorkInPreviousTranscript(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.ver = "ready"
	p.Submit("old provider request")
	oldID := p.log.ID
	h.cards = append(h.cards, agentui.Card{ID: 7, Command: "scene.add-node", State: agentui.Pending})
	h.questions = append(h.questions, agentui.Question{ID: 8, Text: "Old question", State: agentui.Pending})
	p.Tick()
	// These arrived after the last Tick and must not attach to the next session.
	h.cards = append(h.cards, agentui.Card{ID: 9, Command: "scene.rename-node", State: agentui.Pending})
	h.questions = append(h.questions, agentui.Question{ID: 10, Text: "Unseen old question", State: agentui.Pending})
	if err := p.SelectProvider("codex"); err != nil {
		t.Fatal(err)
	}
	// Submit before the next Tick reproduces the race with recording Reset's
	// Pending -> Rejected changes into a new provider's already-open log.
	p.Submit("new provider request")
	p.ver = "ready"
	p.Tick()
	entries, err := agentui.Load(p.d.Root, p.log.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Kind != "session" || entries[1].Kind != "user" || len(p.blocks) != 1 {
		t.Fatalf("previous bridge work leaked into new session: entries=%#v, blocks=%#v", entries, p.blocks)
	}
	old, err := agentui.Load(p.d.Root, oldID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{7, 8, 9, 10} {
		found := 0
		for _, e := range old {
			if e.Card == id && e.State == "rejected" {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("old transcript has %d final rejection records for %d; want one", found, id)
		}
	}
}

func TestProviderDropdownSwitchesFromMissingClaudeByMouse(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.verErr = errors.New("Claude is missing")
	ui, in := core.New(nil), &input.State{}
	r := mathx.Rect{W: 420, H: 520}
	frame := func(at mathx.Vec2, down, up bool) {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		if down {
			in.ApplyMouseDown(at, input.MouseLeft)
		}
		if up {
			in.ApplyMouseUp(at, input.MouseLeft)
		}
		ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		ui.Region(r)
		p.Draw(&services.Frame{UI: ui}, r)
		ui.PopCursor()
		ui.End()
	}
	row := ui.Theme.RowHeight
	pad := ui.Theme.Padding
	choice := mathx.Vec2{X: pad + 20, Y: pad/2 + row/2}
	frame(choice, false, false)
	frame(choice, true, false)
	frame(choice, false, true)
	frame(choice, false, false)
	if !ui.PopupOpen(ui.ID("provider")) {
		t.Fatal("provider dropdown cannot be opened from the missing-CLI screen")
	}
	codex := mathx.Vec2{X: pad + 20, Y: pad/2 + row + pad + row + ui.Theme.Spacing + row/2}
	frame(codex, true, false)
	frame(codex, false, true)
	frame(codex, false, false)
	if p.Provider() != "codex" || p.Missing() != nil || ui.PopupOpen(ui.ID("provider")) {
		t.Fatalf("mouse selection did not switch provider: %q, %v", p.Provider(), p.Missing())
	}
	h.busy = true
	frame(choice, true, false)
	frame(choice, false, true)
	if ui.PopupOpen(ui.ID("provider")) || p.Provider() != "codex" {
		t.Fatal("provider selection remained interactive during a turn")
	}
}

func TestProviderSwitchDiscardsPreviousAvailabilityResult(t *testing.T) {
	h := newProviderTestHost()
	started, release := make(chan struct{}), make(chan struct{})
	h.probe = func(command string) (string, error) {
		if command == "claude" {
			close(started)
			<-release
			return "", errors.New("old Claude error")
		}
		return "Codex ready", nil
	}
	p := providerPanel(t, h)
	p.check()
	<-started
	oldChannel := p.checked
	if err := p.SelectProvider("codex"); err != nil {
		t.Fatal(err)
	}
	p.check()
	select {
	case result := <-p.checked:
		p.checked <- result
	case <-time.After(2 * time.Second):
		t.Fatal("Codex availability probe did not finish")
	}
	p.Tick()
	close(release)
	select {
	case <-oldChannel:
	case <-time.After(2 * time.Second):
		t.Fatal("old availability probe did not finish")
	}
	p.Tick()
	if p.ver != "Codex ready" || p.Missing() != nil || !p.Checked() {
		t.Fatalf("old probe replaced the selected provider: %q, %v", p.ver, p.Missing())
	}
}

func TestProviderPathChangeStartsNewConversationAndKeepsOtherPath(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.Submit("old executable")
	p.setCommand(" /opt/claude ")
	if h.Command() != "/opt/claude" || p.log != nil || p.meter != nil || len(p.blocks) != 0 {
		t.Fatal("executable change retained old conversation")
	}
	if err := p.SelectProvider("codex"); err != nil {
		t.Fatal(err)
	}
	p.setCommand("/opt/codex")
	if err := p.SelectProvider("claude"); err != nil {
		t.Fatal(err)
	}
	if p.path != "/opt/claude" || h.commands["codex"] != "/opt/codex" {
		t.Fatalf("CLI paths mixed across providers: %#v", h.commands)
	}
}

func TestModelCatalogSearchAndStaleResults(t *testing.T) {
	h := newProviderTestHost()
	h.provider = "codex"
	started, release := make(chan struct{}), make(chan struct{})
	h.catalog = func(_ stdcontext.Context, _, command string) ([]agentui.ModelOption, error) {
		if command == "codex" {
			close(started)
			<-release
			return []agentui.ModelOption{{ID: "old"}}, nil
		}
		return []agentui.ModelOption{{ID: "gpt-new", Label: "New model", Description: "Fast coding", Efforts: []string{"low", "high"}}}, nil
	}
	p := providerPanel(t, h)
	p.loadModels()
	<-started
	old := p.modelResults
	h.commands["codex"] = "/new/codex"
	p.loadModels()
	select {
	case result := <-p.modelResults:
		p.modelResults <- result
	case <-time.After(2 * time.Second):
		t.Fatal("new catalog did not load")
	}
	p.pollModels()
	close(release)
	<-old
	p.pollModels()
	if len(p.models) != 1 || p.models[0].ID != "gpt-new" {
		t.Fatalf("old catalog replaced the current provider: %#v", p.models)
	}
	p.modelQuery = "CODING"
	if got := p.filteredModels(); len(got) != 1 || got[0].ID != "gpt-new" {
		t.Fatalf("model search: %#v", got)
	}
	p.selectModel(h, "gpt-new")
	if h.Model() != "gpt-new" || len(p.modelEfforts()) != 2 || p.modelEfforts()[1] != "high" {
		t.Fatalf("selected model or supported effort was lost: %#v", p.models)
	}
	p.selectModel(h, "")
	if h.Model() != "" {
		t.Fatal("CLI default did not clear the override")
	}
}

func TestClosingPanelCancelsModelCatalog(t *testing.T) {
	h := newProviderTestHost()
	h.provider = "codex"
	started, canceled := make(chan struct{}), make(chan struct{})
	h.catalog = func(ctx stdcontext.Context, _, _ string) ([]agentui.ModelOption, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}
	p := providerPanel(t, h)
	p.loadModels()
	<-started
	p.Close()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("closing the panel did not cancel the model catalog")
	}
}

func TestModelPickerSearchAndKeyboard(t *testing.T) {
	h := newProviderTestHost()
	h.provider = "codex"
	p := providerPanel(t, h)
	p.models = []agentui.ModelOption{{ID: "gpt-fast", Label: "Fast"}, {ID: "gpt-deep", Label: "Deep"}}
	ui, in := core.New(nil), &input.State{}
	field := mathx.Rect{X: 30, Y: 20, W: 240, H: 24}
	frame := func(at mathx.Vec2, down, up bool, text string, enter bool) {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		if down {
			in.ApplyMouseDown(at, input.MouseLeft)
		}
		if up {
			in.ApplyMouseUp(at, input.MouseLeft)
		}
		if text != "" {
			in.ApplyText([]byte(text))
		}
		if enter {
			in.ApplyKeyDown(input.KeyEnter, 0, 0, false)
		}
		ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		p.modelPicker(ui, field, h)
		ui.End()
	}
	at := mathx.Vec2{X: 60, Y: 30}
	frame(at, true, false, "", false)
	frame(at, false, true, "", false)
	frame(at, false, false, "", false)
	frame(at, false, false, "deep", false)
	if p.modelQuery != "deep" {
		t.Fatalf("search did not receive keyboard input: %q", p.modelQuery)
	}
	frame(at, false, false, "", true)
	if h.Model() != "gpt-deep" {
		t.Fatalf("Enter did not select the filtered model: %q", h.Model())
	}
}

func TestNestedModelPickerAndCustomID(t *testing.T) {
	h := newProviderTestHost()
	h.provider = "codex"
	p := providerPanel(t, h)
	p.models = []agentui.ModelOption{{ID: "gpt-fast", Label: "Fast"}}
	ui, in := core.New(nil), &input.State{}
	frame := func(open bool, text string, enter bool) {
		in.BeginFrame()
		if text != "" {
			in.ApplyText([]byte(text))
		}
		if enter {
			in.ApplyKeyDown(input.KeyEnter, 0, 0, false)
		}
		ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		if open {
			ui.OpenPopup(p.providerSettings, mathx.Vec2{X: 20, Y: 20})
			ui.PushID("provider-settings")
			ui.OpenPopup(ui.ID("model-picker"), mathx.Vec2{X: 230, Y: 120})
			ui.PushID("model-picker")
			ui.SetFocus(ui.ID("search"))
			ui.PopID()
			ui.PopID()
		}
		p.providerPopup(ui)
		ui.End()
	}
	frame(true, "", false)
	frame(false, "my-custom-model", false)
	if p.modelQuery != "my-custom-model" {
		t.Fatalf("nested picker lost focus: %q", p.modelQuery)
	}
	frame(false, "", true)
	if h.Model() != "my-custom-model" {
		t.Fatalf("custom model was not saved: %q", h.Model())
	}
}

func TestModelPickerReopenAfterMouseSelection(t *testing.T) {
	h := newProviderTestHost()
	h.provider = "codex"
	p := providerPanel(t, h)
	p.models = []agentui.ModelOption{{ID: "gpt-fast", Label: "Fast"}, {ID: "gpt-deep", Label: "Deep"}}
	ui, in := core.New(nil), &input.State{}
	field := mathx.Rect{X: 30, Y: 20, W: 240, H: 24}
	frame := func(at mathx.Vec2, down, up bool, text string) {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		if down {
			in.ApplyMouseDown(at, input.MouseLeft)
		}
		if up {
			in.ApplyMouseUp(at, input.MouseLeft)
		}
		if text != "" {
			in.ApplyText([]byte(text))
		}
		ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		p.modelPicker(ui, field, h)
		ui.End()
	}
	button := mathx.Vec2{X: 60, Y: 30}
	frame(button, true, false, "")
	frame(button, false, true, "")
	frame(button, false, false, "")
	frame(button, false, false, "deep")
	row := ui.Theme.RowHeight
	choice := mathx.Vec2{X: field.X + 20, Y: field.Y + field.H + ui.Theme.Padding + row + ui.Theme.Spacing + row/2}
	frame(choice, true, false, "")
	frame(choice, false, true, "")
	if h.Model() != "gpt-deep" {
		t.Fatalf("mouse selection failed: %q", h.Model())
	}
	frame(button, true, false, "")
	frame(button, false, true, "")
	frame(button, false, false, "")
	frame(button, false, false, "fast")
	if p.modelQuery != "fast" {
		t.Fatalf("reopened search retained old text: %q", p.modelQuery)
	}
}

func TestTextDeltasPreserveUTF8CodeAndSeparateMessageIDs(t *testing.T) {
	p := &Panel{}
	p.textEvent(agentui.Event{Delta: true, MessageID: "first", Text: "Привет\n```go\n"})
	p.textEvent(agentui.Event{Delta: true, MessageID: "second", Text: "Second message"})
	p.textEvent(agentui.Event{Delta: true, MessageID: "first", Text: "x := 1\n```"})
	if len(p.blocks) != 2 || p.blocks[0].text != "Привет\n```go\nx := 1\n```" || p.blocks[1].text != "Second message" {
		t.Fatalf("streamed text changed or messages mixed: %#v", p.blocks)
	}
	p.textEvent(agentui.Event{MessageID: "first", Text: "complete first message"})
	if len(p.blocks) != 2 || p.blocks[0].text != "complete first message" {
		t.Fatal("completed message duplicated its streamed content")
	}
	p.text("existing Claude paragraph one")
	p.text("existing Claude paragraph two")
	if got := p.blocks[len(p.blocks)-1].text; !strings.HasSuffix(got, "existing Claude paragraph one\n\nexisting Claude paragraph two") {
		t.Fatalf("complete-text backend behavior changed: %q", got)
	}
}

func TestStreamedTranscriptRoundTrip(t *testing.T) {
	h := newProviderTestHost()
	p := providerPanel(t, h)
	p.Submit("stream this answer")
	for _, text := range []string{"Текст ", "без ", "лишних\n", "абзацев"} {
		p.event(agentui.Event{Kind: agentui.EvText, Delta: true, MessageID: "a1", Text: text})
	}
	entries, err := agentui.Load(p.d.Root, p.log.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored := p.blocksFrom(entries)
	if len(restored) != 2 || restored[1].text != "Текст без лишних\nабзацев" {
		t.Fatalf("history changed streamed text: %#v", restored)
	}
}
