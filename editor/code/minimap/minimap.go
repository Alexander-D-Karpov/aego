package minimap

import (
	"unicode/utf8"

	"aego/editor/code/highlight"
)

const (
	Cols      = 100
	LineH     = 2
	TileLines = 1024
	TileH     = TileLines * LineH
	RowBytes  = Cols * LineH * 4
	tabWidth  = 4
)

type Palette [highlight.KindCount][4]byte

func Paint(dst, text []byte, spans []highlight.Span, pal *Palette) {
	if len(dst) < RowBytes {
		return
	}
	clear(dst[:RowBytes])
	stride := Cols * 4
	col, si := 0, 0
	for i := 0; i < len(text) && col < Cols; {
		c := text[i]
		size := 1
		if c >= utf8.RuneSelf {
			_, size = utf8.DecodeRune(text[i:])
		}
		switch c {
		case '\t':
			col += tabWidth - col%tabWidth
		case ' ', '\r':
			col++
		default:
			for si < len(spans) && spans[si].End <= i {
				si++
			}
			k := highlight.Plain
			if si < len(spans) && spans[si].Start <= i {
				k = spans[si].Kind
			}
			px := pal[k]
			o := col * 4
			copy(dst[o:o+4], px[:])
			for j := 0; j < 4; j++ {
				dst[stride+o+j] = px[j] / 2
			}
			col++
		}
		i += size
	}
}
