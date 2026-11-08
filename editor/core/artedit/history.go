// Package artedit binds CPU art documents to Aego's existing document lifecycle
// and history. The native client never creates an art.Journal.
package artedit

import (
	"aego/art"
	"aego/editor/core/history"
	"fmt"
)

type historyHost struct {
	history  *history.History
	maxBytes int64
}
type artCommand struct{ change *art.Change }

func (c artCommand) Label() string              { return c.change.Label() }
func (c artCommand) Do(history.Target) error    { return c.change.Apply() }
func (c artCommand) Undo(history.Target) error  { return c.change.Undo() }
func (c artCommand) Merge(history.Command) bool { return false }
func (c artCommand) HistoryBytes() int64        { return c.change.Bytes() }
func (h *historyHost) Exec(c *art.Change) error {
	if c == nil {
		return nil
	}
	if c.Bytes() > h.maxBytes {
		return fmt.Errorf("art: operation exceeds undo budget")
	}
	if err := h.history.Exec(artCommand{c}); err != nil {
		return err
	}
	h.history.TrimBytes(h.maxBytes)
	return nil
}
func (h *historyHost) Undo() error { return h.history.Undo() }
func (h *historyHost) Redo() error { return h.history.Redo() }
func (h *historyHost) Counts() (int, int) {
	u := h.history.Cursor()
	return u, len(h.history.Steps(nil)) - u
}

var _ art.HistoryHost = (*historyHost)(nil)
