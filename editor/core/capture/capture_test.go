package capture

import (
	"bytes"
	"image/png"
	"testing"
	"time"
)

func TestSaveFlipsRows(t *testing.T) {
	rgba := []byte{
		255, 0, 0, 255, 255, 0, 0, 255,
		0, 0, 255, 255, 0, 0, 255, 255,
	}
	data, err := Encode(2, 2, rgba, true)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if r, _, b, _ := img.At(0, 0).RGBA(); b == 0 || r != 0 {
		t.Fatal("flip must put the last row on top")
	}
	rel, err := Save(t.TempDir(), "main scene", 2, 2, rgba, false, time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC))
	if err != nil || rel != ".aego/captures/main-scene-20261019-030000.png" {
		t.Fatalf("rel=%q err=%v", rel, err)
	}
}
