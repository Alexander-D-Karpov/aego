package animedit

import (
	"aego/core/errs"
	"aego/editor/core/history"
	"aego/scene/mutate"
)

type Files interface {
	Read(rel string) ([]byte, error)
	Write(rel string, data []byte) error
}

// FileEdit is a History step that applies a mutate transaction to a clip or controller file; Undo writes the old bytes back (D337).
type FileEdit struct {
	Name   string
	Rel    string
	Ctrl   bool
	Tx     mutate.Tx
	Files  Files
	Loaded func(rel string, data []byte)
	before []byte
	after  []byte
	done   bool
}

// KeyEdit is a FileEdit on a clip.
type KeyEdit = FileEdit

type fileDoc interface {
	Apply(mutate.Tx) (*mutate.Result, error)
	Bytes() []byte
}

func (c *FileEdit) load(src []byte) (fileDoc, error) {
	if c.Ctrl {
		return mutate.LoadCtrl(src, c.Rel)
	}
	return mutate.LoadAnim(src, c.Rel)
}

func (c *FileEdit) Label() string {
	return c.Name
}

func (c *FileEdit) Do(history.Target) error {
	if c.done {
		return c.put(c.after)
	}
	src, err := c.Files.Read(c.Rel)
	if err != nil {
		return err
	}
	d, err := c.load(src)
	if err != nil {
		return err
	}
	res, err := d.Apply(c.Tx)
	if err != nil {
		return err
	}
	for _, p := range res.Problems {
		if p.Error {
			return errs.New(errs.EditorCommand, p.Message)
		}
	}
	c.before, c.after, c.done = src, d.Bytes(), true
	return c.put(c.after)
}

func (c *FileEdit) Undo(history.Target) error {
	return c.put(c.before)
}

func (c *FileEdit) put(data []byte) error {
	if err := c.Files.Write(c.Rel, data); err != nil {
		return err
	}
	if c.Loaded != nil {
		c.Loaded(c.Rel, data)
	}
	return nil
}

func (c *FileEdit) key() (string, bool) {
	if len(c.Tx.Ops) != 1 || c.Tx.Ops[0].Kind != mutate.KindAnimKeySet {
		return "", false
	}
	o := c.Tx.Ops[0]
	return c.Rel + "\x00" + o.Node + "\x00" + o.Path + "\x00" + o.Key, true
}

func (c *FileEdit) Merge(next history.Command) bool {
	n, ok := next.(*FileEdit)
	if !ok {
		return false
	}
	a, ok := c.key()
	if !ok {
		return false
	}
	if b, ok := n.key(); !ok || a != b {
		return false
	}
	c.after, c.Tx = n.after, n.Tx
	return true
}

var _ history.Command = (*FileEdit)(nil)
