// Package host adapts the shared art core to a browser or headless client.
// It owns tabs and save receipts, not a second pixel model or animation system.
// Like art.Session, a Documents instance is confined to the UI goroutine.
package host

import (
	"context"
	"fmt"
	"image"

	"aego/art"
)

const MaxDocuments = 16

type Tab struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Dirty    bool   `json:"dirty"`
	Revision uint64 `json:"revision"`
}
type entry struct {
	key, name string
	session   *art.Session
}
type Documents struct {
	entries   []*entry
	active    string
	serial    uint64
	receipts  map[string]receipt
	clipboard *art.Clipboard
	animation *art.AnimationClipboard
}

func New() *Documents { return &Documents{receipts: make(map[string]receipt)} }
func (d *Documents) next(prefix string) string {
	d.serial++
	return fmt.Sprintf("%s%x", prefix, d.serial)
}
func (d *Documents) lookup(key string) (*entry, error) {
	if key == "" {
		key = d.active
	}
	for _, e := range d.entries {
		if e.key == key {
			return e, nil
		}
	}
	return nil, fmt.Errorf("artstudio: document is no longer open")
}
func (d *Documents) Active() string { return d.active }
func (d *Documents) Session(key string) (*art.Session, error) {
	e, err := d.lookup(key)
	if err != nil {
		return nil, err
	}
	return e.session, nil
}
func (d *Documents) Tabs() []Tab {
	out := make([]Tab, 0, len(d.entries))
	for _, e := range d.entries {
		i := e.session.Info()
		out = append(out, Tab{e.key, e.name, i.Dirty || i.Gesture, i.Document.Revision})
	}
	return out
}

// NewEffect opens a fresh editable instance of the shared native/web template.
// Open retains the usual gesture, tab limit, dirty state and History contract.
func (d *Documents) NewEffect(id, name string) (string, error) {
	doc, err := art.NewEffectDocument(id, art.Limits{})
	if err != nil {
		return "", err
	}
	if name == "" {
		name = id + ".aeart"
	}
	key, err := d.Open(doc, name, true)
	if err != nil {
		doc.Close()
	}
	return key, err
}

func (d *Documents) Open(doc *art.Document, name string, unsaved bool) (string, error) {
	if doc == nil {
		return "", fmt.Errorf("artstudio: nil document")
	}
	// New and deferred imports share this entry point. Like Select, opening a
	// tab must not orphan the active gesture or redirect its remaining events.
	if current, err := d.Session(""); err == nil && current.Info().Gesture {
		return "", fmt.Errorf("artstudio: finish or cancel the active gesture before opening another document")
	}
	if len(d.entries) >= MaxDocuments {
		return "", fmt.Errorf("artstudio: close a document before opening another (maximum %d)", MaxDocuments)
	}
	for _, e := range d.entries {
		if e.session.Document == doc {
			return "", fmt.Errorf("artstudio: document already has an owner")
		}
	}
	if name == "" {
		name = "Untitled.aeart"
	}
	if len(name) > 1024 {
		return "", art.ErrLimit
	}
	session, err := art.NewSession(doc, art.NewJournal(art.Limits{}))
	if err != nil {
		return "", err
	}
	if unsaved {
		session.MarkUnsaved()
	}
	key := d.next("document-")
	d.entries = append(d.entries, &entry{key, name, session})
	d.active = key
	return key, nil
}
func (d *Documents) Select(key string) error {
	_, err := d.lookup(key)
	if err != nil {
		return err
	}
	if current, err := d.Session(""); err == nil && current.Info().Gesture {
		return fmt.Errorf("artstudio: finish or cancel the active gesture")
	}
	d.active = key
	return nil
}
func (d *Documents) Rename(key, name string) error {
	if name == "" || len(name) > 1024 {
		return fmt.Errorf("artstudio: invalid document name")
	}
	e, err := d.lookup(key)
	if err != nil {
		return err
	}
	e.name = name
	return nil
}

// Close refuses to discard dirty state unless the caller has obtained explicit
// confirmation. Receipts for a closed tab are revoked before its pixels close.
func (d *Documents) Close(key string, discard bool) error {
	e, err := d.lookup(key)
	if err != nil {
		return err
	}
	for _, r := range d.receipts {
		if r.key == e.key && r.writing {
			return fmt.Errorf("artstudio: a file write is in progress")
		}
	}
	i := e.session.Info()
	if (i.Dirty || i.Gesture) && !discard {
		return fmt.Errorf("artstudio: unsaved document needs confirmation")
	}
	for token, r := range d.receipts {
		if r.key == e.key {
			delete(d.receipts, token)
		}
	}
	e.session.CancelGesture()
	e.session.Document.Close()
	for n, item := range d.entries {
		if item == e {
			d.entries = append(d.entries[:n], d.entries[n+1:]...)
			if d.active == e.key {
				d.active = ""
				if len(d.entries) > 0 {
					d.active = d.entries[min(n, len(d.entries)-1)].key
				}
			}
			break
		}
	}
	return nil
}

func (d *Documents) Copy(key string) error {
	s, err := d.Session(key)
	if err != nil {
		return err
	}
	if err = s.Copy(); err != nil {
		return err
	}
	d.clipboard = s.Clipboard
	return nil
}
func (d *Documents) Paste(key string, at image.Point) error {
	s, err := d.Session(key)
	if err != nil {
		return err
	}
	if d.clipboard == nil {
		return fmt.Errorf("artstudio: clipboard is empty")
	}
	s.Clipboard = d.clipboard
	return s.Paste(at)
}

func (d *Documents) CopyAnimation(key string, frames []art.ID) error {
	s, err := d.Session(key)
	if err != nil {
		return err
	}
	clip, err := s.Document.CopyAnimation(frames)
	if err != nil {
		return err
	}
	d.animation = clip
	return nil
}
func (d *Documents) PasteAnimation(key string) error {
	s, err := d.Session(key)
	if err != nil {
		return err
	}
	if s.Info().Gesture {
		return fmt.Errorf("artstudio: finish or cancel gesture")
	}
	e, err := s.Document.Begin(s.Document.Revision(), "paste-animation")
	if err != nil {
		return err
	}
	frames, err := e.PasteAnimation(context.Background(), d.animation, s.Frame)
	if err != nil {
		e.Cancel()
		return err
	}
	change, err := e.Prepare()
	if err != nil {
		e.Cancel()
		return err
	}
	if err = s.History.Exec(change); err != nil {
		return err
	}
	s.Frame = frames[0]
	return nil
}

// ImportPalette applies a deferred file read to its original tab. A stale read
// or closed tab cannot modify whichever document happens to be active later.
func (d *Documents) ImportPalette(key string, expected uint64, data []byte, format string, quantize bool) error {
	s, err := d.Session(key)
	if err != nil {
		return err
	}
	if s.Document.Revision() != expected {
		return art.ErrConflict
	}
	p, err := art.DecodePalette(data, format)
	if err != nil {
		return err
	}
	_, err = s.Execute(context.Background(), art.Operation{Kind: "palette-load", Palette: p, Quantize: quantize})
	return err
}
