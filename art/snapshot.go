package art

import (
	"fmt"
	"image"
)

// Snapshot is one immutable document revision. Exporters, recovery and previews
// must use a snapshot instead of independently reading Info and pixel buffers:
// a user edit between those reads would otherwise mix two different revisions.
// It shares immutable COW tiles; taking a snapshot does not copy pixel storage.
type Snapshot struct {
	owner *Document
	view  *Document
}

func (d *Document) Snapshot() (*Snapshot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	return &Snapshot{owner: d, view: &Document{
		id: d.id, current: d.current, revision: d.revision,
		next: d.next, limits: d.limits,
	}}, nil
}

func (s *Snapshot) Info() Info              { return s.view.Info() }
func (s *Snapshot) Revision() uint64        { return s.view.revision }
func (s *Snapshot) Encode() ([]byte, error) { return s.view.Encode() }
func (s *Snapshot) Render(frame ID, opts RenderOptions) (*image.NRGBA, error) {
	return s.view.Render(frame, opts)
}

// Publish holds the document revision stable for the host's final atomic
// publication. Prepare expensive output before this call. The callback must not
// call back into this document; it normally only swaps a file or a GPU resource.
// A close or intervening edit rejects the callback before it can publish.
func (d *Document) Publish(s *Snapshot, commit func() error) error {
	if s == nil || s.owner != d || commit == nil {
		return fmt.Errorf("art: invalid snapshot publication")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.revision != s.view.revision || d.current != s.view.current {
		return ErrConflict
	}
	return commit()
}

// MarkSnapshotSaved records exactly the persisted state, not whichever newer
// state is active when an asynchronous writer finishes. Call on the UI thread,
// like all Session methods. Later edits remain dirty; Undo to this state is clean.
func (s *Session) MarkSnapshotSaved(snapshot *Snapshot) error {
	if snapshot == nil || snapshot.owner != s.Document {
		return fmt.Errorf("art: saved snapshot belongs to another document")
	}
	s.saved = Checkpoint{owner: s.Document.id, state: snapshot.view.current}
	return nil
}

// PlaybackFrames returns stable frame identities in a tag's playback order.
// Ping-pong does not repeat either endpoint. Frame durations stay attached to
// their original identities, so runtime exporters need not resample to an FPS.
func (s *Snapshot) PlaybackFrames(tag ID) ([]Frame, error) {
	return frameOrder(s.view.current, tag)
}

// ExportSheet keeps pixels, rectangles, tags and durations on one revision.
func (s *Snapshot) ExportSheet(columns int, channel Channel) (Sheet, error) {
	return s.view.ExportSheet(columns, channel)
}
