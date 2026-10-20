package capture

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aego/core/errs"
	"aego/core/log"
	"aego/editor/core/document"
)

const Dir = ".aego/captures"

func fileSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>| `, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" {
		return "scene"
	}
	return s
}

func Encode(w, h int, rgba []byte, flip bool) ([]byte, error) {
	if w <= 0 || h <= 0 || len(rgba) < w*h*4 {
		return nil, errs.New(errs.EditorCommand, "the capture is empty")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	stride := w * 4
	for y := 0; y < h; y++ {
		src := y
		if flip {
			src = h - 1 - y
		}
		copy(img.Pix[y*img.Stride:y*img.Stride+stride], rgba[src*stride:src*stride+stride])
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Save(root, name string, w, h int, rgba []byte, flip bool, now time.Time) (string, error) {
	data, err := Encode(w, h, rgba, flip)
	if err != nil {
		return "", err
	}
	rel := filepath.ToSlash(filepath.Join(Dir, fileSafe(name)+"-"+now.Format("20060102-150405")+".png"))
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", errs.Wrap(err, errs.EditorDocument, "cannot create the captures folder").With(log.F("path", rel))
	}
	if err := document.WriteAtomic(abs, data); err != nil {
		return "", err
	}
	return rel, nil
}
