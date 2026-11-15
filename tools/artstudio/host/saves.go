package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"

	"aego/art"
)

const MaxPendingSaves = 8

type receipt struct {
	key      string
	snapshot *art.Snapshot
	writing  bool
}

type Export struct {
	Bytes    []byte
	Revision uint64
	Key      string
	Receipt  string
}

// Export captures bytes, frame metadata and the save checkpoint together.
// Recovery exports must use a receipt-free format ("recovery"). A receipt is
// acknowledged only after a durable writer succeeds or a user confirms a
// completed download; merely clicking an <a download> is not proof of saving.
func (d *Documents) Export(key, format string, columns int) (Export, error) {
	e, err := d.lookup(key)
	if err != nil {
		return Export{}, err
	}
	if e.session.Info().Gesture {
		return Export{}, fmt.Errorf("artstudio: finish the gesture before export")
	}
	if format == "aeart" && len(d.receipts) >= MaxPendingSaves {
		return Export{}, fmt.Errorf("artstudio: too many unconfirmed saves")
	}
	snapshot, err := e.session.Document.Snapshot()
	if err != nil {
		return Export{}, err
	}
	var data []byte
	switch format {
	case "aeart", "recovery":
		data, err = snapshot.Encode()
	case "palette":
		data, err = art.EncodePalette(snapshot.Info().Palette)
	case "png":
		var im *image.NRGBA
		im, err = snapshot.Render(e.session.Frame, art.RenderOptions{Channel: e.session.Channel})
		if err == nil {
			var out bytes.Buffer
			err = png.Encode(&out, im)
			data = out.Bytes()
		}
	case "sheet", "sheet-json":
		var sheet art.Sheet
		sheet, err = snapshot.ExportSheet(columns, e.session.Channel)
		if err == nil {
			if format == "sheet" {
				data = sheet.PNG
			} else {
				data, err = json.MarshalIndent(sheet, "", "  ")
			}
		}
	default:
		return Export{}, fmt.Errorf("artstudio: unsupported export format %q", format)
	}
	if err != nil {
		return Export{}, err
	}
	result := Export{Bytes: data, Revision: snapshot.Revision(), Key: e.key}
	if format == "aeart" {
		result.Receipt = d.next("save-")
		d.receipts[result.Receipt] = receipt{key: e.key, snapshot: snapshot}
	}
	return result, nil
}

// BeginWrite validates a deferred file-picker result before opening its output.
// The receipt pins this document until Saved or CancelSave. Editing may continue;
// completion records exactly this snapshot rather than clearing newer changes.
func (d *Documents) BeginWrite(token string) error {
	r, ok := d.receipts[token]
	if !ok {
		return fmt.Errorf("artstudio: expired save receipt")
	}
	e, err := d.lookup(r.key)
	if err != nil {
		return err
	}
	if e.session.Info().Gesture {
		return fmt.Errorf("artstudio: finish the active gesture before saving")
	}
	for _, other := range d.receipts {
		if other.key == r.key && other.writing {
			return fmt.Errorf("artstudio: another write is in progress")
		}
	}
	if e.session.Document.Info().Revision != r.snapshot.Revision() {
		return art.ErrConflict
	}
	r.writing = true
	d.receipts[token] = r
	return nil
}

func (d *Documents) Saved(token string) error {
	r, ok := d.receipts[token]
	if !ok {
		return fmt.Errorf("artstudio: save receipt has expired")
	}
	if !r.writing {
		return fmt.Errorf("artstudio: a completed write is required before acknowledging a save")
	}
	e, err := d.lookup(r.key)
	if err != nil {
		delete(d.receipts, token)
		return err
	}
	if err = e.session.MarkSnapshotSaved(r.snapshot); err != nil {
		return err
	}
	delete(d.receipts, token)
	return nil
}
func (d *Documents) CancelSave(token string) { delete(d.receipts, token) }
