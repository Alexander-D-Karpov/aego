package backend

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"testing"
)

func TestToolImagesDecodeProviderShapes(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 7, 3))); err != nil {
		t.Fatal(err)
	}
	data := base64.StdEncoding.EncodeToString(b.Bytes())
	for _, raw := range []any{map[string]any{"content": []any{map[string]any{"type": "image", "mimeType": "image/png", "data": data}}}, []any{map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": data}}}} {
		s, _ := json.Marshal(raw)
		g := toolImages(s)
		if len(g) != 1 || !bytes.Equal(g[0], b.Bytes()) {
			t.Fatal("lost provider image")
		}
	}
	if len(toolImages(json.RawMessage(`[{"type":"text","text":"data:image/png;base64,test"}]`))) != 0 {
		t.Fatal("text is not an image block")
	}
}
