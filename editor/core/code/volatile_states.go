package code

import (
	"bytes"
	"context"
	"errors"
	"fmt"
)

// ValidateVolatileStates visits current and Undo/Redo pairs using bounded, cancellable copies on the owner thread; callbacks must not mutate d, and adoption requires identity, revision, source, disk and consent checks.
func (d *Doc) ValidateVolatileStates(ctx context.Context, validate func(text, value []byte) error) error {
	if ctx == nil || d == nil || d.buf == nil || d.idx == nil || d.volatile == nil ||
		d.batch != nil || d.rev >= maxVolatileJournalRevision || d.Len() > maxVolatileJournalTextBytes ||
		len(d.volatile.current) > maxVolatileBytes || len(d.undo)+len(d.redo) > historyLimit || validate == nil {
		return errVolatileJournal
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rev, saved, selection, glsl, volatile := d.rev, d.saved, d.Sel, d.GLSL, d.volatile
	unchanged := func() bool {
		return d.rev == rev && d.saved == saved && d.Sel == selection && d.GLSL == glsl &&
			d.volatile == volatile && d.batch == nil
	}
	text, value := d.Bytes(), bytes.Clone(volatile.current)
	if err := validateVolatileJournal(ctx, text, value, selection, d.undo, d.redo); err != nil {
		return err
	}
	undo, err := cloneVolatileGroups(ctx, d.undo)
	if err != nil {
		return err
	}
	redo, err := cloneVolatileGroups(ctx, d.redo)
	if err != nil {
		return err
	}
	if !unchanged() {
		return errors.New("document changed during volatile state validation")
	}
	// Bound retained callback copies separately from replay work.
	remainingCopies := int64(maxVolatileJournalBytes)
	remainingReplay := int64(maxVolatileJournalReplayBytes)
	visit := func(text, value []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !unchanged() {
			return errors.New("document changed during volatile state validation")
		}
		cost := int64(len(text)) + int64(len(value))
		if cost > remainingCopies {
			return errVolatileJournal
		}
		remainingCopies -= cost
		err := validate(bytes.Clone(text), bytes.Clone(value))
		if !unchanged() {
			return errors.New("document changed during volatile state validation")
		}
		if canceled := ctx.Err(); canceled != nil {
			return canceled
		}
		if err != nil {
			return fmt.Errorf("volatile state validation: %w", err)
		}
		return nil
	}
	if err := visit(text, value); err != nil {
		return err
	}
	for _, branch := range []struct {
		groups []group
		undo   bool
	}{{undo, true}, {redo, false}} {
		currentText, currentValue := text, value
		for i := len(branch.groups) - 1; i >= 0; i-- {
			if err := ctx.Err(); err != nil {
				return err
			}
			g := branch.groups[i]
			if g.volatile {
				want, next := g.volatileBefore, g.volatileAfter
				if branch.undo {
					want, next = next, want
				}
				cost := int64(len(currentValue)) + int64(len(want))
				if cost > remainingReplay || !bytes.Equal(currentValue, want) {
					return errVolatileJournal
				}
				remainingReplay -= cost
				currentValue = next
			} else if branch.undo {
				for j := len(g.recs) - 1; j >= 0; j-- {
					currentText, err = replayVolatileRecord(ctx, currentText, g.recs[j], true, &remainingReplay)
					if err != nil {
						return err
					}
				}
			} else {
				for _, rec := range g.recs {
					currentText, err = replayVolatileRecord(ctx, currentText, rec, false, &remainingReplay)
					if err != nil {
						return err
					}
				}
			}
			if err := visit(currentText, currentValue); err != nil {
				return err
			}
		}
	}
	return nil
}
