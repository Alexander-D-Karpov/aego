package history

import (
	"aego/core/errs"
	"aego/engine/module"
	"aego/scene"
)

type Mode uint8

const (
	ModeDocument Mode = iota
	ModePlay
)

type Target interface {
	Scene() *scene.Scene
}

type Command interface {
	Label() string
	Do(t Target) error
	Undo(t Target) error
	Merge(next Command) bool
}

type Recorder interface {
	Record(cs *module.ChangeSet, undo bool)
}

type entry struct {
	label string
	cmds  []Command
	seq   uint64
}

type subscriber struct {
	id uint64
	fn func()
}

type History struct {
	target    Target
	undo      []entry
	redo      []entry
	seq       uint64
	base      uint64
	saved     uint64
	savedLost bool
	limit     int
	tx        *Transaction
	subs      []subscriber
	nextSub   uint64
	locked    string
	mode      Mode
	changed   func(*module.ChangeSet)
	cs        module.ChangeSet
	ver       uint64
	guard     Guard
	filter    Filter
}

type Guard interface {
	Before()
	After()
}

type Filter func(c Command) (Command, bool)

func (h *History) SetGuard(g Guard) {
	h.guard = g
}

func (h *History) SetFilter(f Filter) {
	h.filter = f
}

func (h *History) before() {
	if h.guard != nil {
		h.guard.Before()
	}
}

func (h *History) after() {
	if h.guard != nil {
		h.guard.After()
	}
}

func New(target Target) *History {
	return &History{target: target, limit: 1000}
}

func (h *History) SetLimit(n int) {
	if n > 0 {
		h.limit = n
	}
}

func (h *History) Target() Target {
	return h.target
}

func (h *History) state() uint64 {
	if n := len(h.undo); n > 0 {
		return h.undo[n-1].seq
	}
	return h.base
}

func (h *History) Cursor() int {
	return len(h.undo)
}

func (h *History) SetMode(m Mode) {
	h.mode = m
	h.notify()
}

func (h *History) Mode() Mode {
	return h.mode
}

func (h *History) MarkSaved() {
	if h.mode == ModePlay {
		return
	}
	h.saved = h.state()
	h.savedLost = false
	h.notify()
}

func (h *History) Dirty() bool {
	if h.mode == ModePlay {
		return false
	}
	return h.savedLost || h.state() != h.saved
}

func (h *History) SetLocked(reason string) {
	h.locked = reason
	h.notify()
}

func (h *History) Locked() (string, bool) {
	return h.locked, h.locked != ""
}

func (h *History) CanUndo() bool {
	return h.tx == nil && h.locked == "" && len(h.undo) > 0
}

func (h *History) CanRedo() bool {
	return h.tx == nil && h.locked == "" && len(h.redo) > 0
}

func (h *History) InTransaction() bool {
	return h.tx != nil
}

func (h *History) Begin(label string) *Transaction {
	if h.tx != nil {
		h.tx.depth++
		return h.tx
	}
	h.tx = &Transaction{h: h, label: label, depth: 1}
	return h.tx
}

func (h *History) Exec(c Command) error {
	tx := h.Begin(c.Label())
	if err := tx.Exec(c); err != nil {
		tx.Abort()
		return err
	}
	tx.Commit()
	return nil
}

func (h *History) push(e entry) {
	h.seq++
	e.seq = h.seq
	for _, r := range h.redo {
		if r.seq == h.saved {
			h.savedLost = true
		}
	}
	h.redo = nil
	h.undo = append(h.undo, e)
	if over := len(h.undo) - h.limit; over > 0 {
		h.base = h.undo[over-1].seq
		h.undo = append([]entry(nil), h.undo[over:]...)
	}
	h.notify()
	h.publish(e.cmds, false)
}

func (h *History) Undo() error {
	if h.locked != "" {
		return errs.New(errs.EditorHistory, h.locked)
	}
	if h.tx != nil {
		return errs.New(errs.EditorHistory, "cannot undo inside an open transaction")
	}
	n := len(h.undo)
	if n == 0 {
		return nil
	}
	e := h.undo[n-1]
	h.undo[n-1] = entry{}
	h.undo = h.undo[:n-1]
	var first error
	h.before()
	for i := len(e.cmds) - 1; i >= 0; i-- {
		if err := e.cmds[i].Undo(h.target); err != nil && first == nil {
			first = err
		}
	}
	h.after()
	h.redo = append(h.redo, e)
	h.publish(e.cmds, true)
	h.notify()
	return first
}

func (h *History) Redo() error {
	if h.locked != "" {
		return errs.New(errs.EditorHistory, h.locked)
	}
	if h.tx != nil {
		return errs.New(errs.EditorHistory, "cannot redo inside an open transaction")
	}
	n := len(h.redo)
	if n == 0 {
		return nil
	}
	e := h.redo[n-1]
	h.redo[n-1] = entry{}
	h.redo = h.redo[:n-1]
	var first error
	h.before()
	for _, c := range e.cmds {
		if err := c.Do(h.target); err != nil && first == nil {
			first = err
		}
	}
	h.after()
	h.undo = append(h.undo, e)
	h.publish(e.cmds, false)
	h.notify()
	return first
}

func (h *History) Labels(n int) (undo, redo []string) {
	prefix := ""
	if h.mode == ModePlay {
		prefix = "[Play] "
	}
	for i := len(h.undo) - 1; i >= 0 && len(undo) < n; i-- {
		undo = append(undo, prefix+h.undo[i].label)
	}
	for i := len(h.redo) - 1; i >= 0 && len(redo) < n; i-- {
		redo = append(redo, prefix+h.redo[i].label)
	}
	return undo, redo
}

func (h *History) Steps(dst []string) []string {
	for _, e := range h.undo {
		dst = append(dst, e.label)
	}
	for i := len(h.redo) - 1; i >= 0; i-- {
		dst = append(dst, h.redo[i].label)
	}
	return dst
}

func (h *History) Seek(n int) error {
	if h.locked != "" {
		return errs.New(errs.EditorHistory, h.locked)
	}
	if h.tx != nil {
		return errs.New(errs.EditorHistory, "cannot move through the history inside an open transaction")
	}
	n = min(max(n, 0), len(h.undo)+len(h.redo))
	for len(h.undo) > n {
		if err := h.Undo(); err != nil {
			return err
		}
	}
	for len(h.undo) < n {
		if err := h.Redo(); err != nil {
			return err
		}
	}
	return nil
}

func (h *History) Walk(fn func(Command)) {
	for i := range h.undo {
		for _, c := range h.undo[i].cmds {
			fn(c)
		}
	}
}

func (h *History) Clear() {
	h.undo = nil
	h.redo = nil
	h.base = h.seq
	h.saved = h.seq
	h.savedLost = false
	h.notify()
}

func (h *History) Subscribe(fn func()) func() {
	h.nextSub++
	id := h.nextSub
	h.subs = append(h.subs, subscriber{id: id, fn: fn})
	return func() {
		for i := range h.subs {
			if h.subs[i].id == id {
				h.subs = append(h.subs[:i], h.subs[i+1:]...)
				return
			}
		}
	}
}

func (h *History) OnChange(fn func(*module.ChangeSet)) {
	h.changed = fn
}

func (h *History) Version() uint64 {
	return h.ver
}

func (h *History) publish(cmds []Command, undo bool) {
	if h.changed == nil || h.target == nil {
		return
	}
	h.cs.Reset(h.target.Scene())
	for _, c := range cmds {
		if r, ok := c.(Recorder); ok {
			r.Record(&h.cs, undo)
		}
	}
	if !h.cs.Empty() {
		h.changed(&h.cs)
	}
}

func (h *History) notify() {
	h.ver++
	for i := range h.subs {
		h.subs[i].fn()
	}
}

type Transaction struct {
	h     *History
	label string
	cmds  []Command
	depth int
	done  bool
}

func (t *Transaction) Label() string {
	return t.label
}

func (t *Transaction) SetLabel(s string) {
	t.label = s
}

func (t *Transaction) Len() int {
	return len(t.cmds)
}

func (t *Transaction) Exec(c Command) error {
	if t.done {
		return errs.New(errs.EditorHistory, "transaction is already closed")
	}
	if t.h.locked != "" {
		return errs.New(errs.EditorHistory, t.h.locked)
	}
	if f := t.h.filter; f != nil {
		if r, ok := f(c); ok {
			if r == nil {
				return nil
			}
			c = r
		}
	}
	t.h.before()
	err := c.Do(t.h.target)
	t.h.after()
	if err != nil {
		return err
	}
	if n := len(t.cmds); n > 0 && t.cmds[n-1].Merge(c) {
		return nil
	}
	t.cmds = append(t.cmds, c)
	return nil
}

func (t *Transaction) Commit() {
	if t.done {
		return
	}
	t.depth--
	if t.depth > 0 {
		return
	}
	t.done = true
	t.h.tx = nil
	if len(t.cmds) > 0 {
		t.h.push(entry{label: t.label, cmds: t.cmds})
	}
}

func (t *Transaction) Abort() {
	if t.done {
		return
	}
	t.h.before()
	for i := len(t.cmds) - 1; i >= 0; i-- {
		t.cmds[i].Undo(t.h.target)
	}
	t.h.after()
	t.cmds = nil
	t.done = true
	t.h.tx = nil
}

// Top is the label of the step Undo would take back and how many steps can be undone, depth 0 when there are none.
func (h *History) Top() (label string, depth int) {
	if len(h.undo) == 0 {
		return "", 0
	}
	return h.undo[len(h.undo)-1].label, len(h.undo)
}
