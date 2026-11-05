package agent

import (
	"aego/editor/core/agentui"
	"aego/gpu/null"
	"aego/platform/clip"
	"aego/ui/core"
	"bytes"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
)

type screenshotClipboard struct{ data []byte }

func (screenshotClipboard) Text() string                   { return "unrelated clipboard text" }
func (screenshotClipboard) SetText(string)                 {}
func (s screenshotClipboard) Data(m string) ([]byte, bool) { return s.data, m == clip.MimePNG }

type textClipboard struct{ screenshotClipboard }

func (textClipboard) Has(m string) bool { return m == clip.MimeText }

func TestPlainTextPasteUsesAdvertisedFormats(t *testing.T) {
	ui := core.New(nil)
	ui.Clipboard = textClipboard{screenshotClipboard{[]byte("текст после снимка")}}
	p := &Panel{}
	if p.pasteImage(ui) || len(p.draftImages) != 0 {
		t.Fatal("text-only clipboard was consumed as an image")
	}
}

func TestDraftImageMarkersFollowRemoval(t *testing.T) {
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 7, 3))); err != nil {
		t.Fatal(err)
	}
	ui := core.New(nil)
	ui.Clipboard = screenshotClipboard{img.Bytes()}
	p := &Panel{d: Deps{Device: null.New()}, input: "before "}
	if !p.pasteImage(ui) || p.input != "before [Image 1]" {
		t.Fatalf("first marker: %q", p.input)
	}
	p.input += " between "
	if !p.pasteImage(ui) || p.input != "before [Image 1] between [Image 2]" {
		t.Fatalf("second marker: %q", p.input)
	}
	p.removeDraftImage(0)
	if p.input != "before  between [Image 1]" || len(p.draftImages) != 1 {
		t.Fatalf("remove/renumber: %q (%d attachments)", p.input, len(p.draftImages))
	}
	p.input = strings.Replace(p.input, "[Image 1]", "", 1)
	p.dropMissingDraftImages()
	if len(p.draftImages) != 0 {
		t.Fatal("deleting a marker must remove its attachment")
	}
}

type screenshotHost struct {
	*providerTestHost
	fail bool
	text string
	data [][]byte
}

func (h *screenshotHost) SendImages(s string, imgs [][]byte) error {
	if h.fail {
		return errors.New("vision unavailable")
	}
	h.text, h.data = s, imgs
	return nil
}
func TestScreenshotPastePreviewSendAndHistory(t *testing.T) {
	h := &screenshotHost{providerTestHost: newProviderTestHost()}
	p := providerPanel(t, h.providerTestHost)
	p.d.Host = h
	p.d.Device = null.New()
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 7, 3))); err != nil {
		t.Fatal(err)
	}
	ui := core.New(nil)
	ui.Clipboard = screenshotClipboard{img.Bytes()}
	if !p.pasteImage(ui) || len(p.draftImages) != 1 || p.draftImages[0].region.Size().X != 7 {
		t.Fatal("missing preview")
	}
	p.removeDraftImage(0)
	if len(p.draftImages) != 0 || len(h.data) != 0 {
		t.Fatal("removing the draft sent it")
	}
	p.pasteImage(ui)
	p.input = "Посмотри\nна этот экран"
	h.fail = true
	p.send()
	if len(p.draftImages) != 1 || p.input == "" {
		t.Fatal("rejected submission lost its draft")
	}
	h.fail = false
	p.send()
	if len(h.data) != 1 || !bytes.Equal(h.data[0], img.Bytes()) || h.text == "" || p.input != "" || len(p.draftImages) != 0 {
		t.Fatal("successful submission lost screenshot or kept draft")
	}
	id := p.log.ID
	p.Close()
	entries, err := agentui.Load(p.d.Root, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Kind == "user" && len(e.Images) == 1 {
			return
		}
	}
	t.Fatal("sent image missing from saved user message")
}
