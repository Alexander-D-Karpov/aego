package clip

import (
	"encoding/binary"
	"errors"
	"image"
	"math/bits"
)

var errBMP = errors.New("unsupported or truncated bmp")

func channel(v, m uint32) uint8 {
	if m == 0 {
		return 0
	}
	s := bits.TrailingZeros32(m)
	n := bits.OnesCount32(m)
	x := (v & m) >> s
	if n >= 8 {
		return uint8(x >> (n - 8))
	}
	return uint8(x * 255 / (1<<n - 1))
}

func DecodeBMP(data []byte) (*image.NRGBA, error) {
	le := binary.LittleEndian
	b := data
	off := -1
	if len(b) >= 14 && b[0] == 'B' && b[1] == 'M' {
		off = int(le.Uint32(b[10:])) - 14
		b = b[14:]
	}
	if len(b) < 40 {
		return nil, errBMP
	}
	hs := int(le.Uint32(b))
	w := int(int32(le.Uint32(b[4:])))
	h := int(int32(le.Uint32(b[8:])))
	bpp := int(le.Uint16(b[14:]))
	comp := le.Uint32(b[16:])
	if hs < 40 || hs > len(b) || w <= 0 || h == 0 || (bpp != 24 && bpp != 32) || (comp != 0 && comp != 3) {
		return nil, errBMP
	}
	masks := [4]uint32{0x00FF0000, 0x0000FF00, 0x000000FF, 0}
	if bpp == 32 {
		masks[3] = 0xFF000000
	}
	end := hs
	if comp == 3 {
		switch {
		case hs >= 56:
			masks = [4]uint32{le.Uint32(b[40:]), le.Uint32(b[44:]), le.Uint32(b[48:]), le.Uint32(b[52:])}
		case hs >= 52:
			masks = [4]uint32{le.Uint32(b[40:]), le.Uint32(b[44:]), le.Uint32(b[48:]), 0}
		default:
			if len(b) < hs+12 {
				return nil, errBMP
			}
			masks = [4]uint32{le.Uint32(b[hs:]), le.Uint32(b[hs+4:]), le.Uint32(b[hs+8:]), 0}
			end = hs + 12
		}
	}
	if off < 0 {
		off = end + int(le.Uint32(b[32:]))*4
	}
	top := h < 0
	if top {
		h = -h
	}
	stride := (w*bpp/8 + 3) &^ 3
	if off < 0 || w > 1<<15 || h > 1<<15 || off+stride*h > len(b) {
		return nil, errBMP
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	alpha := false
	for y := 0; y < h; y++ {
		row := h - 1 - y
		if top {
			row = y
		}
		src := b[off+row*stride:]
		dst := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			o := x * 4
			if bpp == 24 {
				dst[o], dst[o+1], dst[o+2], dst[o+3] = src[x*3+2], src[x*3+1], src[x*3], 255
				continue
			}
			v := le.Uint32(src[x*4:])
			dst[o], dst[o+1], dst[o+2], dst[o+3] = channel(v, masks[0]), channel(v, masks[1]), channel(v, masks[2]), 255
			if masks[3] != 0 {
				dst[o+3] = channel(v, masks[3])
				alpha = alpha || dst[o+3] != 0
			}
		}
	}
	if bpp == 32 && masks[3] != 0 && !alpha {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
	}
	return img, nil
}
