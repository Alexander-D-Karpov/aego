//go:build aego_gltest

package draw

import (
	"strings"

	"aego/core/mathx"
	"aego/render/text"
)

// TestDrawnItem identifies a rendered label or texture and its effective clip
// in the native GUI acceptance tests. Coordinates are logical window pixels.
type TestDrawnItem struct {
	Text         string
	Bounds, Clip mathx.Rect
}

func (l *List) TestDrawnItems(screen mathx.Rect, fallback *text.Font, prefix string, imageWidth, imageHeight int) []TestDrawnItem {
	var out []TestDrawnItem
	for _, stream := range l.streams {
		clips := []mathx.Rect{screen}
		for _, cmd := range stream {
			clip := clips[len(clips)-1]
			switch cmd.kind {
			case cmdPushClip:
				clips = append(clips, intersectRect(clip, cmd.rect))
			case cmdPushClipAbs:
				clips = append(clips, intersectRect(screen, cmd.rect))
			case cmdPopClip:
				clips = clips[:len(clips)-1]
			case cmdTexture:
				s := cmd.region.Size()
				if imageWidth > 0 && s == mathx.V2(float32(imageWidth), float32(imageHeight)) {
					out = append(out, TestDrawnItem{Bounds: cmd.rect, Clip: clip})
				}
			case cmdText:
				if prefix == "" {
					continue
				}
				label := string(l.chars[cmd.textAt : cmd.textAt+cmd.textN])
				if !strings.HasPrefix(label, prefix) {
					continue
				}
				f := cmd.font
				if f == nil {
					f = fallback
				}
				if f == nil {
					continue
				}
				size := f.Measure(label)
				out = append(out, TestDrawnItem{Text: label, Bounds: mathx.Rect{X: cmd.rect.X, Y: cmd.rect.Y, W: size.X, H: size.Y}, Clip: clip})
			}
		}
	}
	return out
}
