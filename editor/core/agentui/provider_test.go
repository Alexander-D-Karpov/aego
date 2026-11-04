package agentui

import (
	"strings"
	"testing"
	"time"
)

func TestProviderTranscriptsDoNotMergeWithinOneSecond(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	var ids []string
	for n := 0; n < 12; n++ {
		log, err := NewTranscript(root, now)
		if err != nil {
			t.Fatal(err)
		}
		provider := "claude"
		if n%2 == 1 {
			provider = "codex"
		}
		if err := log.Append(Entry{Kind: "session", Provider: provider}); err != nil {
			t.Fatal(err)
		}
		if err := log.Append(Entry{Kind: "text", Text: "Привет\n", Delta: true, MessageID: "message"}); err != nil {
			t.Fatal(err)
		}
		if err := log.Close(); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, log.ID)
	}
	list, err := List(root)
	if err != nil || len(list) != len(ids) {
		t.Fatalf("sessions=%v err=%v", list, err)
	}
	for n, id := range ids {
		if list[len(list)-1-n] != id {
			t.Fatalf("same-second sessions not ordered by creation: %v", list)
		}
		entries, err := Load(root, id)
		if err != nil || len(entries) != 2 {
			t.Fatalf("session merged: %s entries=%v err=%v", id, entries, err)
		}
		want := "claude"
		if n%2 == 1 {
			want = "codex"
		}
		if entries[0].Provider != want || !entries[1].Delta || entries[1].MessageID != "message" || entries[1].Text != "Привет\n" {
			t.Fatalf("provider/delta metadata lost: %+v", entries)
		}
	}
}

func TestContextMeterKeepsStreamedTextVerbatim(t *testing.T) {
	m := NewMeter("", "")
	m.User("Напиши код")
	for _, part := range []string{"При", "вет\n", "```go\n", "x := 1\n", "```"} {
		m.TextDelta("first", part)
	}
	want := "User: Напиши код\nAgent: Привет\n```go\nx := 1\n```"
	if m.items[m.open].Text != want {
		t.Fatalf("streamed text changed: %q", m.items[m.open].Text)
	}
	m.TextDelta("second", "Другое сообщение")
	if !strings.HasSuffix(m.items[m.open].Text, "\nAgent: Другое сообщение") {
		t.Fatal("distinct assistant messages were merged without a boundary")
	}
	m.Reset()
	m.User("Ещё")
	m.TextDelta("second", "Новый ответ")
	if m.items[m.open].Text != "User: Ещё\nAgent: Новый ответ" {
		t.Fatal("reset retained the previous streaming message")
	}
}

func TestContextMeterEmptyIDStreamRespectsMessageBoundaries(t *testing.T) {
	m := NewMeter("", "")
	check := func(want string) {
		t.Helper()
		if got := m.items[m.open].Text; got != want {
			t.Fatalf("message boundary changed: got %q, want %q", got, want)
		}
	}
	m.User("question")
	m.TextDelta("", "")
	m.TextDelta("", "an")
	m.TextDelta("", "swer")
	check("User: question\nAgent: answer")

	m.Call("scene_get", "call", "{}", "Read scene")
	m.TextDelta("", "after ")
	m.TextDelta("", "tool")
	check("User: question\nAgent: answer\nTool scene_get {}\nAgent: after tool")

	m.Text("complete message")
	m.TextDelta("", "next ")
	m.TextDelta("", "message")
	check("User: question\nAgent: answer\nTool scene_get {}\nAgent: after tool\nAgent: complete message\nAgent: next message")

	m.User("next question")
	m.TextDelta("", "next answer")
	check("User: next question\nAgent: next answer")

	m.Reset()
	m.TextDelta("", "fresh ")
	m.TextDelta("", "answer")
	check("Agent: fresh answer")
}
