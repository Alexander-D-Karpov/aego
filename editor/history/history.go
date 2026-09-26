package history

import (
	"aego/core/errs"
	"aego/scene"
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

func (h *History) MarkSaved() {
	h.saved = h.state()
	h.savedLost = false
	h.notify()
}

func (h *History) Dirty() bool {
	return h.savedLost || h.state() != h.saved
}

func (h *History) CanUndo() bool {
	return h.tx == nil && len(h.undo) > 0
}

func (h *History) CanRedo() bool {
	return h.tx == nil && len(h.redo) > 0
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
	h.redo = h.redo[:0]
	h.undo = append(h.undo, e)
	if over := len(h.undo) - h.limit; over > 0 {
		h.base = h.undo[over-1].seq
		h.undo = append(h.undo[:0], h.undo[over:]...)
	}
	h.notify()
}

func (h *History) Undo() error {
	if h.tx != nil {
		return errs.New(errs.EditorHistory, "cannot undo inside an open transaction")
	}
	n := len(h.undo)
	if n == 0 {
		return nil
	}
	e := h.undo[n-1]
	h.undo = h.undo[:n-1]
	var first error
	for i := len(e.cmds) - 1; i >= 0; i-- {
		if err := e.cmds[i].Undo(h.target); err != nil && first == nil {
			first = err
		}
	}
	h.redo = append(h.redo, e)
	h.notify()
	return first
}

func (h *History) Redo() error {
	if h.tx != nil {
		return errs.New(errs.EditorHistory, "cannot redo inside an open transaction")
	}
	n := len(h.redo)
	if n == 0 {
		return nil
	}
	e := h.redo[n-1]
	h.redo = h.redo[:n-1]
	var first error
	for _, c := range e.cmds {
		if err := c.Do(h.target); err != nil && first == nil {
			first = err
		}
	}
	h.undo = append(h.undo, e)
	h.notify()
	return first
}

func (h *History) Labels(n int) (undo, redo []string) {
	for i := len(h.undo) - 1; i >= 0 && len(undo) < n; i-- {
		undo = append(undo, h.undo[i].label)
	}
	for i := len(h.redo) - 1; i >= 0 && len(redo) < n; i-- {
		redo = append(redo, h.redo[i].label)
	}
	return undo, redo
}

func (h *History) Clear() {
	h.undo = h.undo[:0]
	h.redo = h.redo[:0]
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

func (h *History) notify() {
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
	if err := c.Do(t.h.target); err != nil {
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
	for i := len(t.cmds) - 1; i >= 0; i-- {
		t.cmds[i].Undo(t.h.target)
	}
	t.cmds = nil
	t.done = true
	t.h.tx = nil
}
