package audioedit

import (
	"bytes"
	"errors"
	"fmt"
)

// PrepareMutation computes one independent candidate for an owner's existing
// journal. It creates no history, performs no I/O, and does not authorize a
// save or publication. The caller must serialize access and check that its
// own document, source and metadata are still current before adopting it.
//
// expectedRevision is checked against current, not against any external host.
// The caller and callback must not mutate current during the call. The callback
// may not mutate its argument concurrently or change source identity. It may
// retain its argument, but the returned candidate never shares its mutable
// slices or range pointers with that argument or current.
// No-op changes retain the original revision and return changed=false; errors
// return a zero document and leave current unchanged.
func PrepareMutation(current Document, expectedRevision uint64, fn func(*Document) error) (candidate Document, changed bool, err error) {
	if fn == nil {
		return Document{}, false, errors.New("audio mutation is nil")
	}
	if current.Revision != expectedRevision {
		return Document{}, false, fmt.Errorf("audio document changed: have revision %d, expected %d", current.Revision, expectedRevision)
	}
	// Validate caller-owned lengths before copying potentially untrusted slices.
	if err := current.Validate(); err != nil {
		return Document{}, false, err
	}
	current = current.Clone()
	before, err := current.stableBytes()
	if err != nil {
		return Document{}, false, err
	}
	next := current.Clone()
	if err := fn(&next); err != nil {
		return Document{}, false, err
	}
	if next.Source != current.Source {
		return Document{}, false, errors.New("audio mutation cannot replace the source identity")
	}
	after, err := next.stableBytes()
	if err != nil {
		return Document{}, false, err
	}
	if bytes.Equal(before, after) {
		return current, false, nil
	}
	if current.Revision == ^uint64(0) {
		return Document{}, false, errors.New("audio revision exhausted")
	}
	// A callback cannot choose the generation used by its host's guards.
	next.Revision = current.Revision + 1
	return next.Clone(), true, nil
}
