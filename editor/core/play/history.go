package play

import (
	"reflect"
	"time"
	"unsafe"

	"aego/platform/input"
)

const DefaultHistoryLimit = 256 << 20

const (
	stepNone uint8 = iota
	stepFixed
	stepFrame
)

type frameRec struct {
	dt     float64
	scale  float64
	tick   uint64
	input  int32
	brk    int32
	paused bool
	step   uint8
}

type recording struct {
	data   []byte
	name   string
	frames []frameRec
	inputs []input.State
	bytes  int
	limit  int
	end    int
	why    string
}

var (
	frameSize = int(unsafe.Sizeof(frameRec{}))
	inputSize = int(unsafe.Sizeof(input.State{}))
)

func newHistory(data []byte, name string, limit int) *recording {
	if limit <= 0 {
		limit = DefaultHistoryLimit
	}
	return &recording{data: data, name: name, limit: limit, end: -1, bytes: len(data)}
}

func (h *recording) usable() int {
	if h.end >= 0 {
		return h.end
	}
	return len(h.frames)
}

func (h *recording) cut(why string) {
	h.cutAt(len(h.frames), why)
}

func (h *recording) cutAt(n int, why string) {
	if h.end < 0 || n < h.end {
		h.end, h.why = n, why
	}
}

func (h *recording) truncate(n int) {
	h.frames = h.frames[:n]
	keep := 0
	if n > 0 {
		keep = int(h.frames[n-1].input) + 1
	}
	h.inputs = h.inputs[:keep]
	if h.end > n {
		h.end, h.why = -1, ""
	}
	h.bytes = len(h.data) + n*frameSize + keep*inputSize
}

func (h *recording) add(dt, scale float64, paused bool, step uint8, in *input.State) bool {
	if h.end >= 0 {
		return false
	}
	if h.bytes+frameSize+inputSize > h.limit {
		h.cut("the history reached its memory limit")
		return false
	}
	idx := len(h.inputs) - 1
	if idx < 0 || !reflect.DeepEqual(&h.inputs[idx], in) {
		h.inputs = append(h.inputs, *in)
		idx++
		h.bytes += inputSize
	}
	h.frames = append(h.frames, frameRec{dt: dt, scale: scale, paused: paused, step: step, input: int32(idx)})
	h.bytes += frameSize
	return true
}

type Timeline struct {
	Frames   int
	Cursor   int
	Usable   int
	Floor    int
	Keys     int
	KeyBytes int
	Bytes    int
	KeyEvery uint64
	// Cached and CacheBytes are the transient keys kept for scrubbing (D346).
	Cached     int
	CacheBytes int
	LastKey    int
	KeyTime    time.Duration
	Tick       uint64
	Why        string
	Blocked    string
	KeysOff    string
	Beyond     bool
}
