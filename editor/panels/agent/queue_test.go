package agent

import (
	"errors"
	"strings"
	"testing"

	"aego/editor/core/agentui"
)

type queueTestHost struct {
	*providerTestHost
	events []agentui.Event
	fail   bool
}

func (h *queueTestHost) Send(text string) error {
	if h.fail {
		return errors.New("cannot send")
	}
	h.sent = append(h.sent, text)
	h.busy = true
	return nil
}

func (h *queueTestHost) Poll(dst []agentui.Event) []agentui.Event {
	for _, e := range h.events {
		if e.Kind == agentui.EvTurnEnd || e.Kind == agentui.EvError {
			h.busy = false
		}
		dst = append(dst, e)
	}
	h.events = nil
	return dst
}

func TestQueuedMessagesWaitAndSendInOrder(t *testing.T) {
	h := &queueTestHost{providerTestHost: newProviderTestHost()}
	p := providerPanel(t, h.providerTestHost)
	p.d.Host = h
	p.Submit("first")
	p.Submit("second")
	p.Submit("third")
	if len(h.sent) != 1 || len(p.queue) != 2 || p.input != "" {
		t.Fatalf("queue=%+v sent=%v draft=%q", p.queue, h.sent, p.input)
	}
	p.input = "unfinished draft"
	h.events = []agentui.Event{{Kind: agentui.EvTurnEnd}}
	p.Tick()
	if len(h.sent) != 2 || !strings.Contains(h.sent[1], "second") || len(p.queue) != 1 || p.input != "unfinished draft" {
		t.Fatalf("second send: sent=%v queue=%+v draft=%q", h.sent, p.queue, p.input)
	}
	h.events = []agentui.Event{{Kind: agentui.EvTurnEnd}}
	p.Tick()
	if len(h.sent) != 3 || !strings.Contains(h.sent[2], "third") || len(p.queue) != 0 || p.input != "unfinished draft" {
		t.Fatalf("third send: sent=%v queue=%+v draft=%q", h.sent, p.queue, p.input)
	}
}

func TestQueuedMessagesPauseOnErrorWithoutLoss(t *testing.T) {
	h := &queueTestHost{providerTestHost: newProviderTestHost()}
	p := providerPanel(t, h.providerTestHost)
	p.d.Host = h
	p.Submit("first")
	p.Submit("queued")
	h.events = []agentui.Event{{Kind: agentui.EvError, Text: "disconnected"}}
	p.Tick()
	if !p.queuePaused || len(p.queue) != 1 || len(h.sent) != 1 {
		t.Fatalf("error lost or sent the queue: %+v", p.queue)
	}
	p.queuePaused = false
	h.fail = true
	p.Tick()
	if !p.queuePaused || len(p.queue) != 1 {
		t.Fatal("failed send did not retain the queued message")
	}
	p.queuePaused = false
	h.fail = false
	p.Tick()
	if len(p.queue) != 0 || len(h.sent) != 2 || !strings.Contains(h.sent[1], "queued") {
		t.Fatalf("retry did not send the message: %v", h.sent)
	}
}

func TestQueuedMessagesBlockSessionChange(t *testing.T) {
	h := &queueTestHost{providerTestHost: newProviderTestHost()}
	p := providerPanel(t, h.providerTestHost)
	p.d.Host = h
	p.Submit("first")
	p.Submit("queued")
	h.busy = false
	p.NewSession()
	if h.resets != 0 || len(p.queue) != 1 {
		t.Fatal("new session discarded queued work")
	}
	if err := p.SelectProvider("codex"); err == nil || h.provider != "claude" {
		t.Fatal("provider change moved queued work to another session")
	}
}
