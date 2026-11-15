package artedit

import "aego/art"

// PreviewRevision is monotonically increasing for this binding, including a
// recovery that replaces its Document. A new Document's revision starts again
// at one, so publishing solely by that revision would accept pre-recovery jobs.
func (b *Document) PreviewRevision() uint64 {
	if b.closed || b.Session == nil {
		return 0
	}
	d := b.Session.Document
	r := d.Revision()
	if b.previewDocument != d || b.previewArtRevision != r {
		b.previewDocument, b.previewArtRevision = d, r
		b.previewRevision++
	}
	return b.previewRevision
}

func (b *Document) PreviewCurrent(revision uint64) bool {
	return !b.closed && b.Asset.Binding.Model == b && revision != 0 && b.PreviewRevision() == revision
}

func (b *Document) PreviewSnapshot() ([]byte, uint64, error) {
	if b.closed {
		return nil, 0, art.ErrClosed
	}
	revision := b.PreviewRevision()
	d := b.Session.Document
	snapshot, err := d.Snapshot()
	if err != nil {
		return nil, 0, err
	}
	data, err := snapshot.Encode()
	if err != nil {
		return nil, 0, err
	}
	if d != b.Session.Document || d.Revision() != snapshot.Revision() || !b.PreviewCurrent(revision) {
		return nil, 0, art.ErrConflict
	}
	return data, revision, nil
}
