// Package aseprite contains the bounded Aseprite wire primitives shared by the
// Art importer and interchange encoder. Document semantics remain in art.
package aseprite

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"
)

// Reader reads little-endian fields from a bounded input, retaining the first
// read error. Take returns a view into the input, not a copy.
type Reader struct {
	b   []byte
	at  int
	err error
}

func NewReader(b []byte) *Reader { return &Reader{b: b} }
func (r *Reader) Offset() int    { return r.at }
func (r *Reader) Err() error     { return r.err }

func (r *Reader) Take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.b)-r.at {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	v := r.b[r.at : r.at+n]
	r.at += n
	return v
}
func (r *Reader) U8() uint8 {
	b := r.Take(1)
	if b == nil {
		return 0
	}
	return b[0]
}
func (r *Reader) U16() uint16 {
	b := r.Take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}
func (r *Reader) U32() uint32 {
	b := r.Take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}
func (r *Reader) I16() int { return int(int16(r.U16())) }
func (r *Reader) I32() int { return int(int32(r.U32())) }
func (r *Reader) Text() string {
	n := int(r.U16())
	b := r.Take(n)
	if !utf8.Valid(b) {
		r.err = fmt.Errorf("invalid UTF-8")
	}
	return string(b)
}

// ReadFrames walks a file after the caller has validated its 128-byte header.
// Frame and chunk callbacks run in file order, with chunk payloads referencing
// data. Framing limits and errors are independent of Art document types.
func ReadFrames(data []byte, frames, speed int, frameFn func(int), chunkFn func(int, uint16, []byte) error) error {
	off := 128
	totalChunks := uint64(0)
	for frame := 0; frame < frames; frame++ {
		if off > len(data)-16 {
			return io.ErrUnexpectedEOF
		}
		r := NewReader(data[off:])
		frameSize := int64(r.U32())
		fm := r.U16()
		oldCount := r.U16()
		duration := int(r.U16())
		r.Take(2)
		newCount := r.U32()
		if frameSize < 16 || frameSize > int64(len(data)-off) || fm != 0xf1fa {
			return fmt.Errorf("invalid frame %d", frame)
		}
		count := uint32(oldCount)
		if newCount != 0 {
			count = newCount
		}
		totalChunks += uint64(count)
		if totalChunks > 65536 || uint64(count) > uint64(frameSize-16)/6 {
			return fmt.Errorf("invalid chunk count")
		}
		if duration == 0 {
			duration = speed
		}
		if duration < 1 {
			duration = 100
		}
		frameFn(duration)
		end := off + int(frameSize)
		pos := off + 16
		for ci := uint32(0); ci < count; ci++ {
			if pos > end-6 {
				return io.ErrUnexpectedEOF
			}
			n := int64(binary.LittleEndian.Uint32(data[pos:]))
			kind := binary.LittleEndian.Uint16(data[pos+4:])
			if n < 6 || n > int64(end-pos) {
				return fmt.Errorf("invalid chunk size at frame %d", frame)
			}
			if err := chunkFn(frame, kind, data[pos+6:pos+int(n)]); err != nil {
				return fmt.Errorf("frame %d chunk %#x: %w", frame, kind, err)
			}
			pos += int(n)
		}
		if pos != end {
			return fmt.Errorf("trailing frame bytes")
		}
		off = end
	}
	if off != len(data) {
		return fmt.Errorf("trailing file bytes")
	}
	return nil
}

// InflateExact allows one extra decoded byte to detect overflow, and rejects
// both truncated output and bytes after the zlib stream.
func InflateExact(raw []byte, n int64) ([]byte, error) {
	source := bytes.NewReader(raw)
	z, err := zlib.NewReader(source)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(z, n+1))
	closeErr := z.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) != n || source.Len() != 0 {
		return nil, fmt.Errorf("compressed cel size mismatch or trailing data")
	}
	return data, nil
}
