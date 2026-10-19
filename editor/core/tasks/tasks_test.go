package tasks

import (
	"errors"
	"testing"
	"time"
)

type harness struct {
	main []func()
}

func (h *harness) drain() {
	list := h.main
	h.main = nil
	for _, fn := range list {
		fn()
	}
}

func newQueue(workers int) (*Queue, *harness) {
	h := &harness{}
	q := New(Deps{
		Submit:  func(fn func()) { fn() },
		OnMain:  func(fn func()) { h.main = append(h.main, fn) },
		Workers: workers,
		Budget:  time.Hour,
	})
	return q, h
}

func TestWorkRunsByPriority(t *testing.T) {
	q, h := newQueue(1)
	var order []string
	for _, s := range []Spec{{Title: "low", Priority: 1}, {Title: "high", Priority: 9}, {Title: "mid", Priority: 5}} {
		title := s.Title
		q.Work(s, func(*Task) error {
			order = append(order, title)
			return nil
		})
	}
	for i := 0; i < 3; i++ {
		q.Tick()
		h.drain()
	}
	if len(order) != 3 || order[0] != "high" || order[1] != "mid" || order[2] != "low" {
		t.Fatalf("order = %v", order)
	}
	if q.Busy() {
		t.Fatal("queue must be idle")
	}
}

func TestCancelAndErrors(t *testing.T) {
	q, h := newQueue(4)
	ran := false
	c := q.Work(Spec{Title: "c"}, func(*Task) error {
		ran = true
		return nil
	})
	c.Cancel()
	f := q.Work(Spec{Title: "f"}, func(*Task) error { return errors.New("boom") })
	var states []State
	c.Then(func(t *Task) { states = append(states, t.State()) })
	f.Then(func(t *Task) { states = append(states, t.State()) })
	q.Tick()
	h.drain()
	if ran || c.State() != Canceled || f.State() != Failed || f.Err() == nil || len(states) != 2 {
		t.Fatalf("ran=%v c=%v f=%v states=%v", ran, c.State(), f.State(), states)
	}
}

func TestStepsRespectLanesAndBudget(t *testing.T) {
	now := time.Unix(0, 0)
	q := New(Deps{Submit: func(fn func()) { fn() }, OnMain: func(fn func()) { fn() }, Budget: time.Millisecond,
		Now: func() time.Time { return now }})
	calls := 0
	step := func(*Task) (bool, error) {
		calls++
		now = now.Add(2 * time.Millisecond)
		return calls >= 3, nil
	}
	q.Step(Spec{Title: "r1"}, LaneRender, step)
	q.Step(Spec{Title: "r2"}, LaneRender, step)
	q.Tick()
	if calls != 0 || !q.WantsRender() {
		t.Fatal("render steps must wait for the render lane")
	}
	q.Render()
	if calls != 1 {
		t.Fatalf("budget allows one step per call, got %d", calls)
	}
	q.Render()
	q.Render()
	q.Render()
	if q.WantsRender() || q.Busy() {
		t.Fatal("finished steps must leave the queue")
	}
}
