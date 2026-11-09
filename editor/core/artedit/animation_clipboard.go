package artedit

import (
	"context"
	"fmt"

	"aego/art"
)

// PasteAnimation uses the existing art transfer algorithm and native History.
// No source file is changed until the destination document is explicitly saved.
func (b *Document) PasteAnimation(ctx context.Context, clip *art.AnimationClipboard) error {
	if err := b.checkEdit(); err != nil {
		return err
	}
	s := b.Session
	if s.Info().Gesture {
		return fmt.Errorf("art: finish or cancel the current gesture")
	}
	e, err := s.Document.Begin(s.Document.Revision(), "Paste animation")
	if err != nil {
		return err
	}
	frames, err := e.PasteAnimation(ctx, clip, s.Frame)
	if err != nil {
		e.Cancel()
		return err
	}
	change, err := e.Prepare()
	if err != nil {
		e.Cancel()
		return err
	}
	if err := s.History.Exec(change); err != nil {
		return err
	}
	if len(frames) != 0 {
		return s.Select("", frames[0], "")
	}
	return nil
}
