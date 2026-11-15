package aseprite

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"math"
)

// WriteHeader writes known fields without changing reserved or trailing bytes.
func WriteHeader(data []byte, h Header) error {
	if len(data) < HeaderSize {
		return io.ErrShortBuffer
	}
	binary.LittleEndian.PutUint32(data, h.FileSize)
	binary.LittleEndian.PutUint16(data[4:], h.Magic)
	binary.LittleEndian.PutUint16(data[6:], h.Frames)
	binary.LittleEndian.PutUint16(data[8:], h.Width)
	binary.LittleEndian.PutUint16(data[10:], h.Height)
	binary.LittleEndian.PutUint16(data[12:], h.Depth)
	binary.LittleEndian.PutUint32(data[14:], h.Flags)
	binary.LittleEndian.PutUint16(data[18:], h.Speed)
	data[28] = h.Transparent
	binary.LittleEndian.PutUint16(data[32:], h.Colors)
	data[34], data[35] = h.PixelWidth, h.PixelHeight
	return nil
}

// Output bounds the whole encoded file, including compression output. The
// caller supplies its limit error to keep the wire package independent of art.
type Output struct {
	bytes.Buffer
	limit    int64
	limitErr error
	err      error
}

func NewOutput(limit int64, limitErr error) *Output {
	return &Output{limit: limit, limitErr: limitErr}
}
func (w *Output) Err() error         { return w.err }
func (w *Output) SetError(err error) { w.err = err }
func (w *Output) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if int64(len(p)) > w.limit-int64(w.Len()) {
		w.err = w.limitErr
		return 0, w.err
	}
	return w.Buffer.Write(p)
}
func (w *Output) U8(v uint8) { _, _ = w.Write([]byte{v}) }
func (w *Output) U16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	_, _ = w.Write(b[:])
}
func (w *Output) U32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	_, _ = w.Write(b[:])
}
func (w *Output) Zeros(n int) { _, _ = w.Write(make([]byte, n)) }
func (w *Output) Text(s string) {
	w.U16(uint16(len(s)))
	_, _ = w.Write([]byte(s))
}
func (w *Output) PatchSize(start int) {
	if w.err != nil {
		return
	}
	n := w.Len() - start
	if uint64(n) > math.MaxUint32 {
		w.err = w.limitErr
		return
	}
	binary.LittleEndian.PutUint32(w.Bytes()[start:start+4], uint32(n))
}
func (w *Output) Chunk(kind uint16, payload func()) {
	start := w.Len()
	w.U32(0)
	w.U16(kind)
	payload()
	w.PatchSize(start)
}

// DeflateRows writes each supplied row as a separate zlib write, preserving
// the encoder's stream and early-error behavior without knowing pixel types.
func DeflateRows(w io.Writer, height int, row func(int) ([]byte, error)) error {
	z := zlib.NewWriter(w)
	for y := 0; y < height; y++ {
		b, err := row(y)
		if err != nil {
			_ = z.Close()
			return err
		}
		if _, err := z.Write(b); err != nil {
			_ = z.Close()
			return err
		}
	}
	return z.Close()
}
