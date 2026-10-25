package agentbridge

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"aego/app/agent"
	"aego/project"
)

// scriptUndo is an applied script edit: the open buffer's revision after it, or the text on disk.
type scriptUndo struct {
	rel           string
	rev           uint64
	open          bool
	created       bool
	before, after string
}

func (b *Bridge) ScriptText(rel string) (string, bool) {
	d := b.ctx.Docs.Script(rel)
	if d == nil {
		return "", false
	}
	return string(d.Code.Bytes()), true
}

// ApplyScript edits an open script in its buffer, one Undo step of the code editor that the user saves; a closed one on disk.
func (b *Bridge) ApplyScript(e agent.ScriptEdit, label string) error {
	if e.New {
		abs := filepath.Join(b.ctx.Project.Root, filepath.FromSlash(e.Rel))
		if _, err := os.Stat(abs); err == nil {
			return errors.New(e.Rel + " exists; change it with script.edit")
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := project.WriteAtomic(abs, []byte(e.After)); err != nil {
			return err
		}
		b.last = &undoRef{label: label, script: &scriptUndo{rel: e.Rel, created: true, after: e.After}}
		b.Wrote([]string{e.Rel})
		return nil
	}
	if d := b.ctx.Docs.Script(e.Rel); d != nil {
		if string(d.Code.Bytes()) != e.Before {
			return errors.New(e.Rel + " changed since the preview; read it again with script.read")
		}
		lo, oldHi, newHi := changed(e.Before, e.After)
		d.Code.ReplaceRanges([][2]int{{lo, oldHi}}, [][]byte{[]byte(e.After[lo:newHi])})
		b.last = &undoRef{label: label, script: &scriptUndo{rel: e.Rel, rev: d.Code.Rev(), open: true, before: e.Before, after: e.After}}
		return nil
	}
	abs := filepath.Join(b.ctx.Project.Root, filepath.FromSlash(e.Rel))
	disk, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	if string(disk) != e.Before {
		return errors.New(e.Rel + " changed since the preview; read it again with script.read")
	}
	if err := project.WriteAtomic(abs, []byte(e.After)); err != nil {
		return err
	}
	b.last = &undoRef{label: label, script: &scriptUndo{rel: e.Rel, before: e.Before, after: e.After}}
	b.Wrote([]string{e.Rel})
	return nil
}

// changed is the range of a that differs from b: a[lo:aHi] became b[lo:bHi].
func changed(a, b string) (lo, aHi, bHi int) {
	for lo < len(a) && lo < len(b) && a[lo] == b[lo] {
		lo++
	}
	aHi, bHi = len(a), len(b)
	for aHi > lo && bHi > lo && a[aHi-1] == b[bHi-1] {
		aHi--
		bHi--
	}
	return lo, aHi, bHi
}

func (b *Bridge) canUndoScript(u *scriptUndo) bool {
	if d := b.ctx.Docs.Script(u.rel); d != nil {
		if u.open {
			return d.Code.Rev() == u.rev && d.Code.CanUndo()
		}
		return string(d.Code.Bytes()) == u.after
	}
	disk, err := os.ReadFile(filepath.Join(b.ctx.Project.Root, filepath.FromSlash(u.rel)))
	return err == nil && bytes.Equal(disk, []byte(u.after))
}

func (b *Bridge) undoScript(u *scriptUndo) error {
	if u.created {
		if d := b.ctx.Docs.Script(u.rel); d != nil && d.Dirty() {
			return errors.New(u.rel + " has unsaved changes; close it first")
		}
		if err := os.Remove(filepath.Join(b.ctx.Project.Root, filepath.FromSlash(u.rel))); err != nil {
			return err
		}
		b.Wrote([]string{u.rel})
		return nil
	}
	if d := b.ctx.Docs.Script(u.rel); d != nil {
		if u.open {
			d.Code.Undo()
			return nil
		}
		lo, oldHi, newHi := changed(u.after, u.before)
		d.Code.ReplaceRanges([][2]int{{lo, oldHi}}, [][]byte{[]byte(u.before[lo:newHi])})
		return nil
	}
	if err := project.WriteAtomic(filepath.Join(b.ctx.Project.Root, filepath.FromSlash(u.rel)), []byte(u.before)); err != nil {
		return err
	}
	b.Wrote([]string{u.rel})
	return nil
}
