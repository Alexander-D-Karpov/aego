package codec

import (
	"errors"
	"io"
)

const peakSpan = 256

func Peaks(d Decoder, cols int) ([]float32, error) {
	ch := d.Info().Channels
	if cols <= 0 || ch < 1 || ch > 2 {
		return nil, ErrLayout
	}
	buf := make([]float32, readFrames*ch)
	var blocks []float32
	var cur float32
	n := 0
	for {
		k, err := d.Read(buf)
		for f := 0; f < k; f++ {
			for c := 0; c < ch; c++ {
				v := buf[f*ch+c]
				if v < 0 {
					v = -v
				}
				cur = max(cur, v)
			}
			n++
			if n == peakSpan {
				blocks = append(blocks, cur)
				cur, n = 0, 0
			}
		}
		if errors.Is(err, io.EOF) || (err == nil && k == 0) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if n > 0 {
		blocks = append(blocks, cur)
	}
	out := make([]float32, cols)
	if len(blocks) == 0 {
		return out, nil
	}
	for c := range out {
		a := c * len(blocks) / cols
		b := max(a+1, (c+1)*len(blocks)/cols)
		for i := a; i < b && i < len(blocks); i++ {
			out[c] = max(out[c], blocks[i])
		}
	}
	return out, nil
}
