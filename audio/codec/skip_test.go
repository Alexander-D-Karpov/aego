package codec

import "testing"

type sized struct {
	Decoder
	frames int64
}

func (s sized) Info() Info { return Info{Frames: s.frames} }

func TestSkipTo(t *testing.T) {
	d := sized{frames: 1000}
	for _, c := range []struct {
		o    StreamOptions
		to   int64
		want int64
		ok   bool
	}{
		{StreamOptions{}, 400, 400, true},
		{StreamOptions{}, 1000, 0, false},
		{StreamOptions{Loop: true}, 2300, 300, true},
		{StreamOptions{Loop: true, LoopStart: 200, LoopEnd: 800}, 900, 300, true},
		{StreamOptions{Loop: true, LoopStart: 200, LoopEnd: 800}, 799, 799, true},
	} {
		got, ok := skipTo(d, c.o, c.to)
		if got != c.want || ok != c.ok {
			t.Errorf("skipTo(%+v, %d) = %d, %v; want %d, %v", c.o, c.to, got, ok, c.want, c.ok)
		}
	}
}
