// Package audioedit owns non-destructive audio authoring data shared by editor
// and asset processing. Decoders, UI, mixer and file access remain host-owned.
package audioedit

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

const SchemaVersion = 1
const DefaultHistoryLimit = 128

// These are standalone authoring limits, not limits on source PCM length.
// They bound snapshots/history as well as waveform and render working memory.
const (
	maxHistoryLimit       = 128
	maxDocumentOperations = 4096
	maxDocumentMarkers    = 4096
	maxDocumentTextBytes  = 256 << 10
	maxDocumentJSONBytes  = 4 << 20
)

// Range represents a half-open range of PCM frames (not interleaved samples).
type Range struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

func (r Range) Valid(frames int64) bool { return r.Start >= 0 && r.Start < r.End && r.End <= frames }
func (r Range) Length() int64           { return r.End - r.Start }

type Source struct {
	AssetID    string `json:"asset_id"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	Frames     int64  `json:"frames"`
}

// Validate checks the identity's format and bounds; it cannot compare a digest
// to a real source or guard an external file on behalf of the host.
func (s Source) Validate() error {
	if s.AssetID == "" || s.SHA256 == "" {
		return errors.New("audio source needs stable AssetID and source digest")
	}
	if len(s.SHA256) != 64 {
		return errors.New("audio source digest must be a SHA-256 hex string")
	}
	if _, err := hex.DecodeString(s.SHA256); err != nil {
		return errors.New("audio source digest must be a SHA-256 hex string")
	}
	remaining := maxDocumentTextBytes
	if err := consumeTextBudget(&remaining, s.AssetID, s.Path, s.SHA256, s.Codec); err != nil {
		return err
	}
	if s.Frames <= 0 || s.Frames > math.MaxInt64/8 {
		return errors.New("audio source has invalid frame count")
	}
	if s.SampleRate < 8000 || s.SampleRate > 384000 || s.Channels < 1 || s.Channels > 32 {
		return errors.New("unsupported sample rate or channel count")
	}
	return nil
}

type OperationKind string

const (
	Gain      OperationKind = "gain"
	FadeIn    OperationKind = "fade_in"
	FadeOut   OperationKind = "fade_out"
	Silence   OperationKind = "silence"
	Reverse   OperationKind = "reverse"
	Normalize OperationKind = "normalize"
)

// Operation applies to source-relative PCM frames. Normalize.Value is the
// linear multiplier derived from the effective signal before this operation.
type Operation struct {
	Kind   OperationKind `json:"kind"`
	Region Range         `json:"region"`
	Value  float64       `json:"value,omitempty"`
}

func (o Operation) Validate(frames int64) error {
	if !o.Region.Valid(frames) {
		return fmt.Errorf("invalid %s range", o.Kind)
	}
	if math.IsNaN(o.Value) || math.IsInf(o.Value, 0) {
		return fmt.Errorf("invalid %s value", o.Kind)
	}
	switch o.Kind {
	case Gain, Normalize:
		if o.Value < 0 || o.Value > 100 {
			return fmt.Errorf("invalid %s multiplier", o.Kind)
		}
	case FadeIn, FadeOut, Silence, Reverse:
	default:
		return fmt.Errorf("unsupported audio operation %q", o.Kind)
	}
	return nil
}

type Marker struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Frame int64  `json:"frame"`
	Beat  bool   `json:"beat,omitempty"`
}

// Document is sidecar/authoring metadata. It never contains audio samples and
// never implies permission to overwrite the encoded source file.
type Document struct {
	Schema     int         `json:"schema"`
	Source     Source      `json:"source"`
	Trim       Range       `json:"trim"`
	Selection  *Range      `json:"selection,omitempty"`
	Loop       *Range      `json:"loop,omitempty"`
	Intro      *Range      `json:"intro,omitempty"`
	BPM        float64     `json:"bpm,omitempty"`
	Operations []Operation `json:"operations,omitempty"`
	Markers    []Marker    `json:"markers,omitempty"`
	Revision   uint64      `json:"revision"`
}

func New(source Source) (Document, error) {
	if err := source.Validate(); err != nil {
		return Document{}, err
	}
	return Document{Schema: SchemaVersion, Source: source, Trim: Range{Start: 0, End: source.Frames}}, nil
}

func (d Document) Validate() error {
	if d.Schema != SchemaVersion {
		return fmt.Errorf("unsupported audio document schema %d", d.Schema)
	}
	if err := d.Source.Validate(); err != nil {
		return err
	}
	if !d.Trim.Valid(d.Source.Frames) {
		return errors.New("invalid audio trim")
	}
	for _, r := range []*Range{d.Selection, d.Loop, d.Intro} {
		if r != nil && (!r.Valid(d.Source.Frames) || r.Start < d.Trim.Start || r.End > d.Trim.End) {
			return errors.New("range outside trimmed audio")
		}
	}
	if math.IsNaN(d.BPM) || math.IsInf(d.BPM, 0) || d.BPM < 0 || d.BPM > 999 {
		return errors.New("invalid BPM")
	}
	if len(d.Operations) > maxDocumentOperations || len(d.Markers) > maxDocumentMarkers {
		return errors.New("audio document exceeds operation or marker limit")
	}
	remaining := maxDocumentTextBytes
	if err := consumeTextBudget(&remaining, d.Source.AssetID, d.Source.Path, d.Source.SHA256, d.Source.Codec); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, m := range d.Markers {
		if err := consumeTextBudget(&remaining, m.ID, m.Label); err != nil {
			return err
		}
		if m.ID == "" || seen[m.ID] || m.Frame < d.Trim.Start || m.Frame > d.Trim.End {
			return errors.New("invalid or duplicate marker")
		}
		seen[m.ID] = true
	}
	for _, op := range d.Operations {
		if err := op.Validate(d.Source.Frames); err != nil {
			return err
		}
	}
	return nil
}

func consumeTextBudget(remaining *int, values ...string) error {
	for _, value := range values {
		if !utf8.ValidString(value) || len(value) > *remaining {
			return errors.New("invalid or oversized audio document text")
		}
		*remaining -= len(value)
	}
	return nil
}

func (d Document) Clone() Document {
	d.Operations = slices.Clone(d.Operations)
	d.Markers = slices.Clone(d.Markers)
	if d.Selection != nil {
		r := *d.Selection
		d.Selection = &r
	}
	if d.Loop != nil {
		r := *d.Loop
		d.Loop = &r
	}
	if d.Intro != nil {
		r := *d.Intro
		d.Intro = &r
	}
	return d
}

func (d Document) stableBytes() ([]byte, error) {
	d.Revision = 0 // Undo to the original content is clean despite monotonically increasing revision.
	return d.Encode()
}

func (d Document) Encode() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data) > maxDocumentJSONBytes {
		return nil, errors.New("encoded audio document exceeds size limit")
	}
	return data, nil
}
func Decode(raw []byte) (Document, error) {
	if len(raw) > maxDocumentJSONBytes {
		return Document{}, errors.New("encoded audio document exceeds size limit")
	}
	if !utf8.Valid(raw) {
		return Document{}, errors.New("encoded audio document is not UTF-8")
	}
	if err := validateJSONStructure(raw); err != nil {
		return Document{}, err
	}
	var d Document
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return Document{}, err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Document{}, errors.New("unexpected trailing audio document data")
	}
	if err := d.Validate(); err != nil {
		return Document{}, err
	}
	return d.Clone(), nil
}

// encoding/json accepts duplicate keys, including case variants matching the
// same struct field. Reject them rather than silently discarding author data.
// The depth and item limits also bound validation of untrusted JSON.
func validateJSONStructure(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := scanJSONValue(dec, 0); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing audio document data")
	}
	return nil
}

func scanJSONValue(dec *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("audio document nesting limit exceeded")
	}
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		var keys []string
		for dec.More() {
			if len(keys) >= 32 {
				return errors.New("audio document object has too many fields")
			}
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("audio document object key is invalid")
			}
			for _, old := range keys {
				if strings.EqualFold(old, key) {
					return fmt.Errorf("duplicate audio document key %q", key)
				}
			}
			keys = append(keys, key)
			if err := scanJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for count := 0; dec.More(); count++ {
			if count >= max(maxDocumentOperations, maxDocumentMarkers) {
				return errors.New("audio document array limit exceeded")
			}
			if err := scanJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected audio document delimiter")
	}
	closeToken, err := dec.Token()
	if err != nil {
		return err
	}
	if delim == '{' && closeToken != json.Delim('}') || delim == '[' && closeToken != json.Delim(']') {
		return errors.New("invalid audio document container")
	}
	return nil
}

// Session owns a standalone history for isolated editing. It is not the editor's
// shared AssetDocument history and must not be registered as a second Undo/Save
// binding alongside metadata. Integration must use a single guarded journal.
type Session struct {
	mu           sync.RWMutex
	doc          Document
	undo         []Document
	redo         []Document
	baseline     []byte
	historyLimit int
	revision     uint64
}

func Open(d Document, historyLimit int) (*Session, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if historyLimit <= 0 {
		historyLimit = DefaultHistoryLimit
	}
	if historyLimit > maxHistoryLimit {
		return nil, errors.New("audio history limit exceeds memory budget")
	}
	b, err := d.stableBytes()
	if err != nil {
		return nil, err
	}
	return &Session{doc: d.Clone(), baseline: b, historyLimit: historyLimit, revision: d.Revision}, nil
}
func (s *Session) Snapshot() Document { s.mu.RLock(); defer s.mu.RUnlock(); return s.doc.Clone() }
func (s *Session) CanUndo() bool      { s.mu.RLock(); defer s.mu.RUnlock(); return len(s.undo) > 0 }
func (s *Session) CanRedo() bool      { s.mu.RLock(); defer s.mu.RUnlock(); return len(s.redo) > 0 }
func (s *Session) Dirty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := s.doc.stableBytes()
	return err != nil || string(b) != string(s.baseline)
}

// Mutate is one standalone gesture and one standalone Undo entry. The
// callback must not call Session methods or mutate its argument concurrently.
// It cannot change the source identity; hosts must guard external source data.
func (s *Session) Mutate(expectedRevision uint64, fn func(*Document) error) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next, changed, err := PrepareMutation(s.doc, expectedRevision, fn)
	if err != nil {
		return s.revision, err
	}
	if !changed {
		return s.revision, nil // no-op is not a new undo step
	}
	if len(s.undo) == s.historyLimit {
		copy(s.undo, s.undo[1:])
		s.undo = s.undo[:len(s.undo)-1]
	}
	s.undo = append(s.undo, s.doc.Clone())
	s.redo = nil
	s.revision = next.Revision
	s.doc = next
	return s.revision, nil
}
func (s *Session) Undo() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.undo) == 0 {
		return errors.New("nothing to undo")
	}
	if s.revision == ^uint64(0) {
		return errors.New("audio revision exhausted")
	}
	n := len(s.undo) - 1
	s.redo = append(s.redo, s.doc.Clone())
	s.doc = s.undo[n]
	s.undo = s.undo[:n]
	s.revision++
	s.doc.Revision = s.revision
	return nil
}
func (s *Session) Redo() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.redo) == 0 {
		return errors.New("nothing to redo")
	}
	if s.revision == ^uint64(0) {
		return errors.New("audio revision exhausted")
	}
	n := len(s.redo) - 1
	s.undo = append(s.undo, s.doc.Clone())
	s.doc = s.redo[n]
	s.redo = s.redo[:n]
	s.revision++
	s.doc.Revision = s.revision
	return nil
}

// MarkSaved is called only after the shared Aego save transaction commits.
// It must not clear dirty state after a concurrent authoring mutation.
func (s *Session) MarkSaved(expectedRevision uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedRevision != s.revision {
		return errors.New("save completed for a stale audio revision")
	}
	b, err := s.doc.stableBytes()
	if err != nil {
		return err
	}
	s.baseline = b
	return nil
}

func SetSelection(d *Document, r *Range) {
	if r == nil {
		d.Selection = nil
	} else {
		v := *r
		d.Selection = &v
	}
}
func SetTrim(d *Document, r Range) {
	d.Trim = r
	if d.Loop != nil && (d.Loop.Start < r.Start || d.Loop.End > r.End) {
		d.Loop = nil
	}
	if d.Selection != nil && (d.Selection.Start < r.Start || d.Selection.End > r.End) {
		d.Selection = nil
	}
	if d.Intro != nil && (d.Intro.Start < r.Start || d.Intro.End > r.End) {
		d.Intro = nil
	}
	d.Markers = slices.DeleteFunc(d.Markers, func(m Marker) bool { return m.Frame < r.Start || m.Frame > r.End })
}
func SetLoop(d *Document, r *Range) {
	if r == nil {
		d.Loop = nil
	} else {
		v := *r
		d.Loop = &v
	}
}
func AddOperation(d *Document, op Operation) { d.Operations = append(d.Operations, op) }
func AddMarker(d *Document, marker Marker)   { d.Markers = append(d.Markers, marker) }
func (d Document) DurationSeconds() float64 {
	if d.Source.SampleRate <= 0 {
		return 0
	}
	return float64(d.Trim.Length()) / float64(d.Source.SampleRate)
}
