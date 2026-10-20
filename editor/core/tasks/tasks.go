package tasks

import (
	"math"
	"slices"
	"sync/atomic"
	"time"
)

type Lane uint8

const (
	LaneWorker Lane = iota
	LaneMain
	LaneRender
)

type State uint8

const (
	Queued State = iota
	Running
	Done
	Failed
	Canceled
)

const recentMax = 50

type Spec struct {
	Title    string
	Kind     string
	Detail   string
	Priority int64
}

type Step func(t *Task) (bool, error)

type Task struct {
	Spec
	ID       uint64
	lane     Lane
	state    State
	err      error
	progress atomic.Uint32
	canceled atomic.Bool
	work     func(t *Task) error
	step     Step
	then     []func(t *Task)
}

func (t *Task) Lane() Lane {
	return t.lane
}

func (t *Task) State() State {
	return t.state
}

func (t *Task) Err() error {
	return t.err
}

func (t *Task) Cancel() {
	t.canceled.Store(true)
}

func (t *Task) Canceled() bool {
	return t.canceled.Load()
}

func (t *Task) SetProgress(f float32) {
	t.progress.Store(math.Float32bits(min(max(f, 0), 1)))
}

func (t *Task) Progress() float32 {
	return math.Float32frombits(t.progress.Load())
}

func (t *Task) SetPriority(p int64) {
	t.Priority = p
}

func (t *Task) Then(fn func(t *Task)) *Task {
	if t.state >= Done {
		fn(t)
		return t
	}
	t.then = append(t.then, fn)
	return t
}

type Deps struct {
	Submit  func(func())
	OnMain  func(func())
	Workers int
	Budget  time.Duration
	Now     func() time.Time
}

type Queue struct {
	d       Deps
	next    uint64
	pending []*Task
	steps   []*Task
	running int
	live    []*Task
	recent  []*Task
}

func New(d Deps) *Queue {
	if d.Workers <= 0 {
		d.Workers = 2
	}
	if d.Budget <= 0 {
		d.Budget = 4 * time.Millisecond
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Queue{d: d}
}

func (q *Queue) add(s Spec, lane Lane) *Task {
	q.next++
	t := &Task{Spec: s, ID: q.next, lane: lane}
	q.live = append(q.live, t)
	return t
}

func (q *Queue) Work(s Spec, fn func(t *Task) error) *Task {
	t := q.add(s, LaneWorker)
	t.work = fn
	q.pending = append(q.pending, t)
	return t
}

func (q *Queue) Step(s Spec, lane Lane, fn Step) *Task {
	if lane == LaneWorker {
		lane = LaneMain
	}
	t := q.add(s, lane)
	t.step = fn
	q.steps = append(q.steps, t)
	return t
}

func byPriority(a, b *Task) int {
	switch {
	case a.Priority > b.Priority:
		return -1
	case a.Priority < b.Priority:
		return 1
	}
	return int(a.ID) - int(b.ID)
}

func (q *Queue) finish(t *Task, err error) {
	switch {
	case t.state >= Done:
		return
	case t.Canceled():
		t.state = Canceled
	case err != nil:
		t.state, t.err = Failed, err
	default:
		t.state = Done
	}
	q.live = slices.DeleteFunc(q.live, func(x *Task) bool { return x == t })
	if t.Kind != "preview" || t.state == Failed {
		q.recent = append(q.recent, t)
		if over := len(q.recent) - recentMax; over > 0 {
			q.recent = slices.Delete(q.recent, 0, over)
		}
	}
	then := t.then
	t.then = nil
	for _, fn := range then {
		fn(t)
	}
}

func (q *Queue) Tick() {
	slices.SortStableFunc(q.pending, byPriority)
	for len(q.pending) > 0 && q.running < q.d.Workers {
		t := q.pending[0]
		q.pending = q.pending[1:]
		if t.Canceled() {
			q.finish(t, nil)
			continue
		}
		t.state = Running
		q.running++
		q.d.Submit(func() {
			var err error
			if !t.Canceled() {
				err = t.work(t)
			}
			q.d.OnMain(func() {
				q.running--
				q.finish(t, err)
			})
		})
	}
	q.run(LaneMain)
}

func (q *Queue) Render() {
	q.run(LaneRender)
}

func (q *Queue) WantsRender() bool {
	for _, t := range q.steps {
		if t.lane == LaneRender {
			return true
		}
	}
	return false
}

func (q *Queue) run(lane Lane) {
	slices.SortStableFunc(q.steps, byPriority)
	deadline := q.d.Now().Add(q.d.Budget)
	list := slices.Clone(q.steps)
	for _, t := range list {
		if t.lane != lane {
			continue
		}
		done, err := true, error(nil)
		if !t.Canceled() {
			t.state = Running
			done, err = t.step(t)
		}
		if done || err != nil {
			q.steps = slices.DeleteFunc(q.steps, func(x *Task) bool { return x == t })
			q.finish(t, err)
		}
		if q.d.Now().After(deadline) {
			return
		}
	}
}

func (q *Queue) Busy() bool {
	return len(q.live) > 0
}

func (q *Queue) Tasks() []*Task {
	return q.live
}

func (q *Queue) CancelAll() {
	for _, t := range q.live {
		t.Cancel()
	}
}

func (q *Queue) Recent() []*Task {
	return q.recent
}

func (q *Queue) ClearRecent() {
	q.recent = q.recent[:0]
}
