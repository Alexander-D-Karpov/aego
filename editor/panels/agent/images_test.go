package agent

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/editor/core/agentui"
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
