//go:build js && wasm

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"syscall/js"

	"aego/art"
	"aego/tools/artstudio/host"
)

type request struct {
	Frames           []art.ID      `json:"frames"`
	Handle           string        `json:"handle"`
	Name             string        `json:"name"`
	Template         string        `json:"template"`
	Receipt          string        `json:"receipt"`
	Action           string        `json:"action"`
	Tool             string        `json:"tool"`
	Points           []image.Point `json:"points"`
	Brush            art.Brush     `json:"brush"`
	Filled           bool          `json:"filled"`
	Operation        art.Operation `json:"operation"`
	Layer            art.ID        `json:"layer"`
	Frame            art.ID        `json:"frame"`
	Channel          art.Channel   `json:"channel"`
	Width            int           `json:"width"`
	Height           int           `json:"height"`
	Mode             art.Mode      `json:"mode"`
	Tolerance        int           `json:"tolerance"`
	Format           string        `json:"format"`
	Columns          int           `json:"columns"`
	Tag              art.ID        `json:"tag"`
	TimeMS           int64         `json:"timeMS"`
	Confirm          bool          `json:"confirm"`
	ExpectedRevision uint64        `json:"expectedRevision"`
}
type response struct {
	Active            string               `json:"active"`
	Tabs              []host.Tab           `json:"tabs"`
	State             art.SessionInfo      `json:"state"`
	ID                art.ID               `json:"id,omitempty"`
	Color             *art.Color           `json:"color,omitempty"`
	Paint             *art.Paint           `json:"paint,omitempty"`
	Error             string               `json:"error,omitempty"`
	Hash              string               `json:"hash,omitempty"`
	Warnings          []string             `json:"warnings,omitempty"`
	NeedsConfirmation bool                 `json:"needsConfirmation,omitempty"`
	Templates         []art.EffectTemplate `json:"templates,omitempty"`
}

var documents = host.New()
var session *art.Session
var callbacks []js.Func

func openDocument(d *art.Document, name string, unsaved bool) error {
	if _, err := documents.Open(d, name, unsaved); err != nil {
		d.Close()
		return err
	}
	session, _ = documents.Session("")
	return nil
}
func currentResponse(out response) response {
	if current, err := documents.Session(""); err == nil {
		session = current
		out.State = current.Info()
	}
	out.Active, out.Tabs = documents.Active(), documents.Tabs()
	return out
}
func invoke(r request) (art.ID, *art.Paint, error) {
	if r.ExpectedRevision != 0 && session.Document.Revision() != r.ExpectedRevision {
		return "", nil, art.ErrConflict
	}
	switch r.Action {
	case "state", "templates":
		return "", nil, nil
	case "new":
		if r.Template != "" {
			_, err := documents.NewEffect(r.Template, r.Name)
			if err == nil {
				session, _ = documents.Session("")
			}
			return "", nil, err
		}
		d, err := art.New(r.Width, r.Height, r.Mode, art.Limits{})
		if err != nil {
			return "", nil, err
		}
		return "", nil, openDocument(d, r.Name, true)
	case "document-select":
		err := documents.Select(r.Handle)
		if err == nil {
			session, _ = documents.Session("")
		}
		return "", nil, err
	case "document-name":
		return "", nil, documents.Rename(r.Handle, r.Name)
	case "document-close":
		if err := documents.Close(r.Handle, r.Confirm); err != nil {
			return "", nil, err
		}
		if documents.Active() == "" {
			d, err := art.New(64, 64, art.RGBA, art.Limits{})
			if err != nil {
				return "", nil, err
			}
			if err = openDocument(d, "Untitled.aeart", false); err != nil {
				return "", nil, err
			}
		} else {
			session, _ = documents.Session("")
		}
		return "", nil, nil
	case "begin-save":
		return "", nil, documents.BeginWrite(r.Receipt)
	case "cancel-save":
		documents.CancelSave(r.Receipt)
		return "", nil, nil
	case "select":
		return "", nil, session.Select(r.Layer, r.Frame, r.Channel)
	case "isolate":
		return "", nil, session.SetIsolation(r.Layer)
	case "begin":
		if len(r.Points) != 1 {
			return "", nil, fmt.Errorf("art: begin needs one point")
		}
		return "", nil, session.BeginGesture(r.Tool, r.Points[0], r.Brush, r.Filled)
	case "move":
		return "", nil, session.MoveGesture(r.Points)
	case "shape-preview":
		return "", nil, session.PreviewGesturePoints(r.Points)
	case "end":
		return "", nil, session.EndGesture()
	case "cancel":
		session.CancelGesture()
		return "", nil, nil
	case "apply":
		id, err := session.Execute(context.Background(), r.Operation)
		return id, nil, err
	case "undo":
		return "", nil, session.Undo()
	case "redo":
		return "", nil, session.Redo()
	case "selection":
		return "", nil, session.SetSelection(r.Tool, r.Points, r.Tolerance)
	case "copy-animation":
		return "", nil, documents.CopyAnimation("", r.Frames)
	case "paste-animation":
		return "", nil, documents.PasteAnimation("")
	case "copy":
		return "", nil, documents.Copy("")
	case "capture-brush":
		return "", nil, session.CaptureBrush()
	case "clear-brush":
		session.ClearBrush()
		return "", nil, nil
	case "paste":
		if len(r.Points) != 1 {
			return "", nil, fmt.Errorf("art: paste needs one point")
		}
		return "", nil, documents.Paste("", r.Points[0])
	case "sample":
		if len(r.Points) != 1 {
			return "", nil, fmt.Errorf("art: sample needs one point")
		}
		c, err := session.SamplePaint(r.Points[0])
		return "", &c, err
	case "frame-at":
		id, err := session.Document.FrameAt(r.Tag, r.TimeMS)
		return id, nil, err
	case "recovered":
		session.MarkUnsaved()
		return "", nil, nil
	case "saved":
		return "", nil, documents.Saved(r.Receipt)
	default:
		return "", nil, fmt.Errorf("art: unknown action %q", r.Action)
	}
}
func jsonCall(_ js.Value, args []js.Value) any {
	out := response{}
	var req request
	if len(args) != 1 || args[0].Type() != js.TypeString {
		out.Error = "art: expected one JSON request"
	} else if len(args[0].String()) > 1<<20 {
		out.Error = "art: request exceeds limit"
	} else if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		out.Error = err.Error()
	} else {
		id, c, err := invoke(req)
		out.ID, out.Paint = id, c
		if c != nil {
			out.Color = &c.Color
		}
		if err != nil {
			out.Error = err.Error()
		}
		if err == nil && req.Action == "templates" {
			out.Templates = art.EffectTemplates()
		}
	}
	out = currentResponse(out)
	b, _ := json.Marshal(out)
	return string(b)
}
func pixelsCall(_ js.Value, args []js.Value) any {
	frame := art.ID("")
	onion, references := false, true
	if len(args) > 0 && args[0].Type() == js.TypeString {
		frame = art.ID(args[0].String())
	}
	if len(args) > 1 && args[1].Type() == js.TypeBoolean {
		onion = args[1].Bool()
	}
	if len(args) > 2 && args[2].Type() == js.TypeBoolean {
		references = args[2].Bool()
	}
	im, err := session.Preview(frame, onion, references)
	if err != nil {
		return js.Null()
	}
	v := js.Global().Get("Uint8ClampedArray").New(len(im.Pix))
	js.CopyBytesToJS(v, im.Pix)
	return v
}
func thumbnailCall(_ js.Value, args []js.Value) any {
	if len(args) != 1 || args[0].Type() != js.TypeString {
		return js.Null()
	}
	im, err := session.Document.Thumbnail(art.ID(args[0].String()), session.Frame,
		art.RenderOptions{Channel: session.Channel, References: true}, 32)
	if err != nil {
		return js.Null()
	}
	v := js.Global().Get("Uint8ClampedArray").New(len(im.Pix))
	js.CopyBytesToJS(v, im.Pix)
	return v
}

func exportCall(_ js.Value, args []js.Value) any {
	format, columns, key := "aeart", 8, ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		format = args[0].String()
	}
	if len(args) > 1 && args[1].Type() == js.TypeNumber {
		columns = args[1].Int()
	}
	if len(args) > 2 && args[2].Type() == js.TypeString {
		key = args[2].String()
	}
	result, err := documents.Export(key, format, columns)
	if err != nil {
		return js.ValueOf(map[string]any{"error": err.Error()})
	}
	v := js.Global().Get("Uint8Array").New(len(result.Bytes))
	js.CopyBytesToJS(v, result.Bytes)
	return js.ValueOf(map[string]any{"bytes": v, "revision": float64(result.Revision), "handle": result.Key, "receipt": result.Receipt})
}

// snapshotCall is read-only: preview must not create save receipts, mark a
// document clean or publish a stale version back into the authoring session.
func snapshotCall(_ js.Value, _ []js.Value) any {
	snapshot, err := session.Document.Snapshot()
	if err != nil {
		return js.ValueOf(map[string]any{"error": err.Error()})
	}
	b, err := snapshot.Encode()
	if err != nil {
		return js.ValueOf(map[string]any{"error": err.Error()})
	}
	v := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(v, b)
	return js.ValueOf(map[string]any{"bytes": v, "handle": documents.Active(), "revision": float64(snapshot.Revision())})
}
func paletteCall(_ js.Value, args []js.Value) any {
	out := response{}
	if len(args) < 2 || args[0].Type() != js.TypeObject || args[0].IsNull() || !args[0].InstanceOf(js.Global().Get("Uint8Array")) || args[1].Type() != js.TypeString {
		out.Error = "art: palette import needs bytes and a format"
	} else if n := args[0].Get("byteLength").Int(); n > art.MaxPaletteBytes || n < 1 {
		out.Error = art.ErrLimit.Error()
	} else {
		data := make([]byte, n)
		js.CopyBytesToGo(data, args[0])
		key, revision := documents.Active(), session.Document.Revision()
		if len(args) > 3 && args[3].Type() == js.TypeString {
			key = args[3].String()
		}
		if len(args) > 4 && args[4].Type() == js.TypeNumber {
			revision = uint64(args[4].Float())
		}
		quantize := len(args) > 2 && args[2].Type() == js.TypeBoolean && args[2].Bool()
		err := documents.ImportPalette(key, revision, data, args[1].String(), quantize)
		if err != nil {
			out.Error = err.Error()
		}
	}
	out = currentResponse(out)
	b, _ := json.Marshal(out)
	return string(b)
}

func importCall(_ js.Value, args []js.Value) any {
	out := response{}
	if len(args) < 2 || args[0].Type() != js.TypeObject || args[0].IsNull() || !args[0].InstanceOf(js.Global().Get("Uint8Array")) || args[1].Type() != js.TypeString {
		out.Error = "art: import needs a byte array and format"
	} else {
		n := args[0].Get("byteLength").Int()
		if n < 0 || n > 64<<20 {
			out.Error = "art: import exceeds limit"
		} else {
			data := make([]byte, n)
			js.CopyBytesToGo(data, args[0])
			var d *art.Document
			var err error
			// Lossy imports require a separate explicit confirmation. The same
			// bytes are parsed again on confirmation; no tab is opened here.
			confirmLoss := len(args) > 4 && args[4].Type() == js.TypeBoolean && args[4].Bool()
			switch args[1].String() {
			case "aeart":
				d, err = art.Decode(data, art.Limits{})
			case "png":
				d, err = art.ImportPNG(data, art.Limits{})
			case "ase", "aseprite":
				var diagnostics []art.ImportDiagnostic
				d, diagnostics, err = art.ImportAseprite(data, art.Limits{})
				for _, note := range diagnostics {
					out.Warnings = append(out.Warnings, fmt.Sprintf("%s (frame %d, chunk %#x): %s", note.Code, note.Frame+1, note.Chunk, note.Message))
				}
			default:
				err = fmt.Errorf("art: unsupported import")
			}
			if err == nil && d == nil {
				err = fmt.Errorf("art: importer returned no document")
			}
			if err == nil && len(out.Warnings) > 0 && !confirmLoss {
				d.Close()
				d = nil
				out.NeedsConfirmation = true
			}
			if err == nil && d != nil {
				name := "Imported.aeart"
				if len(args) > 3 && args[3].Type() == js.TypeString {
					name = args[3].String()
				}
				err = openDocument(d, name, args[1].String() != "aeart")
			}
			if err != nil {
				out.Error = err.Error()
			}
		}
	}
	out = currentResponse(out)
	b, _ := json.Marshal(out)
	return string(b)
}
func hashCall(_ js.Value, args []js.Value) any {
	frame := session.Frame
	if len(args) > 0 && args[0].Type() == js.TypeString && args[0].String() != "" {
		frame = art.ID(args[0].String())
	}
	channel := session.Channel
	if len(args) > 1 && args[1].Type() == js.TypeString && args[1].String() != "" {
		channel = art.Channel(args[1].String())
	}
	im, err := session.Document.Render(frame, art.RenderOptions{Channel: channel})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(im.Pix)
	return hex.EncodeToString(sum[:])
}
func main() {
	d, err := art.New(64, 64, art.RGBA, art.Limits{})
	if err != nil {
		panic(err)
	}
	if err = openDocument(d, "Untitled.aeart", false); err != nil {
		panic(err)
	}
	obj := js.Global().Get("Object").New()
	for _, v := range []struct {
		name string
		fn   func(js.Value, []js.Value) any
	}{{"request", jsonCall}, {"pixels", pixelsCall}, {"thumbnail", thumbnailCall}, {"exportBytes", exportCall}, {"snapshotBytes", snapshotCall}, {"importPalette", paletteCall}, {"importBytes", importCall}, {"hash", hashCall}} {
		f := js.FuncOf(v.fn)
		callbacks = append(callbacks, f)
		obj.Set(v.name, f)
	}
	js.Global().Set("aegoArt", obj)
	select {}
}
