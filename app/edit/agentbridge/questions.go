package agentbridge

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"aego/app/agent/mcp"
)

// Question is one ask_user call waiting in the Agent panel (11b5a).
type Question struct {
	ID       int
	Text     string
	Options  []Option
	Multiple bool
	Other    bool
	State    State
	Answer   string
	answer   chan string
}

type Option struct {
	Label  string
	Detail string
}

const askDoc = "Ask the user to choose before you go on: give 2 to 5 short options, each with a label and a one-line detail, and put the one you recommend first. " +
	"Use it instead of listing options in your answer whenever the user has to decide. The call waits until the user answers and returns their choice or their own text."

func (b *Bridge) tools() []mcp.Tool {
	opt := map[string]any{"type": "object", "properties": map[string]any{
		"label":  map[string]any{"type": "string", "description": "a few words"},
		"detail": map[string]any{"type": "string", "description": "one line on what this option means"},
	}, "required": []string{"label"}}
	return []mcp.Tool{b.contextTool(), b.materialSourceTool(), {
		Name: "ask_user", Description: askDoc, ReadOnly: true,
		Schema: map[string]any{"type": "object", "required": []string{"question", "options"}, "properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "the question, one sentence"},
			"options":  map[string]any{"type": "array", "items": opt, "minItems": 1, "maxItems": 6},
			"multiple": map[string]any{"type": "boolean", "description": "the user may pick several options"},
			"other":    map[string]any{"type": "boolean", "description": "the user may type an answer of their own; true by default"},
		}},
		Run: b.ask,
	}}
}

func (b *Bridge) ask(args map[string]any) (string, bool) {
	generation := b.panelGeneration.Load()
	q := &Question{Other: true, answer: make(chan string, 1)}
	q.Text, _ = args["question"].(string)
	q.Multiple, _ = args["multiple"].(bool)
	if v, ok := args["other"].(bool); ok {
		q.Other = v
	}
	list, _ := args["options"].([]any)
	for _, o := range list {
		m, _ := o.(map[string]any)
		label, _ := m["label"].(string)
		detail, _ := m["detail"].(string)
		if label != "" {
			q.Options = append(q.Options, Option{Label: label, Detail: detail})
		}
	}
	if strings.TrimSpace(q.Text) == "" || len(q.Options) == 0 {
		return "ask_user needs a question and at least one option", true
	}
	cancelled := false
	if err := b.main(func() {
		if b.cancelled(generation) {
			cancelled = true
			return
		}
		b.next++
		q.ID = b.next
		b.questions = append(b.questions, q)
	}); err != nil {
		return err.Error(), true
	}
	if cancelled {
		return errCancelled.Error(), true
	}
	t := time.NewTimer(b.timeout())
	defer t.Stop()
	select {
	case a := <-q.answer:
		if a == "" {
			return "the user dismissed the question; ask in your answer instead", true
		}
		return a, false
	case <-t.C:
	}
	_ = b.main(func() {
		if q.State == Pending {
			q.State = Expired
		}
	})
	return errExpired.Error(), true
}

// Questions are the agent's questions, oldest first; read them on the main thread.
func (b *Bridge) Questions() []*Question {
	return b.questions
}

// Answer answers a waiting question with the picked options, by index, or with text; no picks and no text dismisses it.
func (b *Bridge) Answer(id int, picks []int, text string) error {
	for _, q := range b.questions {
		if q.ID != id {
			continue
		}
		if q.State != Pending {
			return errors.New("the question is no longer waiting")
		}
		var parts []string
		for _, i := range picks {
			if i >= 0 && i < len(q.Options) {
				parts = append(parts, q.Options[i].Label)
			}
		}
		switch {
		case len(parts) > 0:
			q.Answer = "The user chose: " + strings.Join(parts, "; ")
			if text != "" {
				q.Answer += ". They added: " + text
			}
		case text != "":
			q.Answer = "The user answered: " + text
		}
		q.State = Applied
		if q.Answer == "" {
			q.State = Rejected
		}
		q.answer <- q.Answer
		return nil
	}
	return errors.New("no question " + strconv.Itoa(id))
}

func (b *Bridge) dismissQuestions() {
	for _, q := range b.questions {
		if q.State == Pending {
			q.State = Rejected
			q.answer <- ""
		}
	}
}
