package agent

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/gpu/null"
	"aego/platform/input"
	"aego/ui/core"
)

func TestImageCardHistoryKeepsExactAttachment(t *testing.T) {
	root := t.TempDir()
	p := Panel{d: Deps{Root: root}}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 5, 3))); err != nil {
		t.Fatal(err)
	}
	keys := p.storeImages([][]byte{data.Bytes()})
	if len(keys) != 1 {
		t.Fatal("attachment not stored")
	}
	log, err := agentui.NewTranscript(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = log.Append(agentui.Entry{Kind: "result", Images: keys}); err != nil {
		t.Fatal(err)
	}
	log.Close()
	history, err := agentui.Load(root, log.ID)
	if err != nil || len(history) != 1 || len(history[0].Images) != 1 {
		t.Fatalf("history: %v / %v", history, err)
	}
	raw, err := os.ReadFile(filepath.Join(root, attachmentDir, history[0].Images[0]+".png"))
	if err != nil || !bytes.Equal(raw, data.Bytes()) {
		t.Fatal("history lost image pixels")
	}
}

func TestImageViewerUsesOriginalAndCloses(t *testing.T) {
	p := &Panel{d: Deps{Device: null.New()}}
	ui := core.New(nil)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 1024, 600))); err != nil {
		t.Fatal(err)
	}
	in := &input.State{}
	ui.Begin(in, mathx.Vec2{X: 900, Y: 700}, 1)
	p.openImage(ui, data.Bytes())
	p.DrawImageViewer(ui)
	ui.End()
	if !ui.ModalOpen(imageViewerID) || p.imageViewer.region.Size() != (mathx.Vec2{X: 1024, Y: 600}) {
		t.Fatalf("viewer did not load the original image: %+v", p.imageViewer.region.Size())
	}
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyEscape, 0, 0, false)
	ui.Begin(in, mathx.Vec2{X: 900, Y: 700}, 1)
	p.DrawImageViewer(ui)
	ui.End()
	if ui.ModalOpen(imageViewerID) || p.imageViewer.data != nil || !p.imageViewer.region.Texture.IsZero() {
		t.Fatal("closing the viewer left its modal, data or texture alive")
	}
}
