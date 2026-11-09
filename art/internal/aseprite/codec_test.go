package aseprite

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestReaderBoundsAndText(t *testing.T) {
	r := NewReader([]byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 2, 0, 'o', 'k'})
	if r.U8() != 255 || r.I16() != -514 || r.I16() != -1028 || r.Text() != "ok" || r.Offset() != 9 || r.Err() != nil {
		t.Fatalf("unexpected decoded fields: offset=%d error=%v", r.Offset(), r.Err())
	}
	if r.U32() != 0 || !errors.Is(r.Err(), io.ErrUnexpectedEOF) || r.Offset() != 9 {
		t.Fatalf("short read: offset=%d error=%v", r.Offset(), r.Err())
	}
	if r.Take(0) != nil || !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatal("read error must remain sticky")
	}
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"bad UTF-8", []byte{1, 0, 0xff}, "invalid UTF-8"},
		{"truncated text", []byte{2, 0, 'x'}, "unexpected EOF"},
		{"truncated length", []byte{1}, "unexpected EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewReader(tc.data)
			_ = r.Text()
			if r.Err() == nil || r.Err().Error() != tc.want {
				t.Fatalf("error = %v, want %q", r.Err(), tc.want)
			}
		})
	}
	r = NewReader([]byte{1})
	if r.Take(-1) != nil || !errors.Is(r.Err(), io.ErrUnexpectedEOF) || r.Offset() != 0 {
		t.Fatal("negative read was not rejected")
	}
}

func testFrame(duration, oldCount, newCount int, chunks ...[]byte) []byte {
	frame := make([]byte, 16)
	binary.LittleEndian.PutUint16(frame[4:], 0xf1fa)
	binary.LittleEndian.PutUint16(frame[6:], uint16(oldCount))
	binary.LittleEndian.PutUint16(frame[8:], uint16(duration))
	binary.LittleEndian.PutUint32(frame[12:], uint32(newCount))
	for _, chunk := range chunks {
		frame = append(frame, chunk...)
	}
	binary.LittleEndian.PutUint32(frame, uint32(len(frame)))
	return frame
}

func testChunk(kind uint16, payload ...byte) []byte {
	b := make([]byte, 6, 6+len(payload))
	binary.LittleEndian.PutUint32(b, uint32(6+len(payload)))
	binary.LittleEndian.PutUint16(b[4:], kind)
	return append(b, payload...)
}

func testFile(frames ...[]byte) []byte {
	b := make([]byte, 128)
	for _, frame := range frames {
		b = append(b, frame...)
	}
	return b
}

func TestReadFramesOrderAndDurations(t *testing.T) {
	// The nonzero new count overrides oldCount. The final frame falls back to
	// the default 100 ms when both duration and header speed are zero.
	data := testFile(testFrame(42, 1, 2, testChunk(0x2004, 1), testChunk(0x2005, 2, 3)), testFrame(0, 0, 0))
	var calls []string
	err := ReadFrames(data, 2, 0, func(d int) { calls = append(calls, fmt.Sprintf("frame:%d", d)) },
		func(frame int, kind uint16, b []byte) error {
			calls = append(calls, fmt.Sprintf("chunk:%d:%x:%x", frame, kind, b))
			return nil
		})
	if err != nil || strings.Join(calls, ",") != "frame:42,chunk:0:2004:01,chunk:0:2005:0203,frame:100" {
		t.Fatalf("calls=%v error=%v", calls, err)
	}
	var duration int
	if err := ReadFrames(testFile(testFrame(0, 0, 0)), 1, 77, func(d int) { duration = d }, func(int, uint16, []byte) error { return nil }); err != nil || duration != 77 {
		t.Fatalf("header speed fallback=%d error=%v", duration, err)
	}
}

func TestReadFramesErrors(t *testing.T) {
	invalidMagic := testFrame(1, 0, 0)
	invalidMagic[4] = 0
	invalidSize := testFrame(1, 0, 0)
	binary.LittleEndian.PutUint32(invalidSize, 15)
	invalidCount := testFrame(1, 1, 0)
	invalidChunk := testChunk(0x2004)
	binary.LittleEndian.PutUint32(invalidChunk, 5)
	trailingFrame := testFrame(1, 0, 0, []byte{1})
	tests := []struct {
		name, want string
		data       []byte
	}{
		{"short frame header", "unexpected EOF", make([]byte, 128)},
		{"bad frame magic", "invalid frame 0", testFile(invalidMagic)},
		{"bad frame length", "invalid frame 0", testFile(invalidSize)},
		{"bad chunk count", "invalid chunk count", testFile(invalidCount)},
		{"bad chunk length", "invalid chunk size at frame 0", testFile(testFrame(1, 1, 0, invalidChunk))},
		{"trailing frame", "trailing frame bytes", testFile(trailingFrame)},
		{"trailing file", "trailing file bytes", append(testFile(testFrame(1, 0, 0)), 0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ReadFrames(tc.data, 1, 0, func(int) {}, func(int, uint16, []byte) error { return nil })
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error=%v want=%q", err, tc.want)
			}
		})
	}
	marker := errors.New("chunk failed")
	err := ReadFrames(testFile(testFrame(1, 1, 0, testChunk(0x2004))), 1, 0, func(int) {}, func(int, uint16, []byte) error { return marker })
	if !errors.Is(err, marker) || err.Error() != "frame 0 chunk 0x2004: chunk failed" {
		t.Fatalf("chunk context lost: %v", err)
	}
	// Count is rejected before visiting chunks even when enough frame bytes exist.
	frame := testFrame(1, 0, 65537, make([]byte, 6*65537))
	err = ReadFrames(testFile(frame), 1, 0, func(int) { t.Fatal("frame visited") }, func(int, uint16, []byte) error { t.Fatal("chunk visited"); return nil })
	if err == nil || err.Error() != "invalid chunk count" {
		t.Fatalf("chunk limit: %v", err)
	}
}

func TestInflateExactAndDeflateRows(t *testing.T) {
	var encoded bytes.Buffer
	rows := [][]byte{[]byte("abc"), []byte("def")}
	if err := DeflateRows(&encoded, len(rows), func(y int) ([]byte, error) { return rows[y], nil }); err != nil {
		t.Fatal(err)
	}
	// Compare against the old direct zlib write sequence, not against our reader.
	var reference bytes.Buffer
	z := zlib.NewWriter(&reference)
	for _, row := range rows {
		if _, err := z.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded.Bytes(), reference.Bytes()) {
		t.Fatal("compressed stream differs from direct zlib writes")
	}
	if got, err := InflateExact(encoded.Bytes(), 6); err != nil || string(got) != "abcdef" {
		t.Fatalf("inflate=%q error=%v", got, err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
		size int64
	}{
		{"short decoded", encoded.Bytes(), 7},
		{"excess decoded", encoded.Bytes(), 5},
		{"trailing bytes", append(bytes.Clone(encoded.Bytes()), 0), 6},
		{"truncated stream", encoded.Bytes()[:encoded.Len()-1], 6},
		{"invalid stream", []byte("invalid"), 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := InflateExact(tc.raw, tc.size); err == nil {
				t.Fatal("accepted invalid compressed data")
			}
		})
	}
	marker := errors.New("row failed")
	if err := DeflateRows(io.Discard, 2, func(int) ([]byte, error) { return nil, marker }); !errors.Is(err, marker) {
		t.Fatalf("row error lost: %v", err)
	}
	limit := errors.New("limit")
	out := NewOutput(0, limit)
	if err := DeflateRows(out, 1, func(int) ([]byte, error) { return []byte{1}, nil }); !errors.Is(err, limit) || !errors.Is(out.Err(), limit) {
		t.Fatalf("compressed write limit lost: %v / %v", err, out.Err())
	}
}

func TestOutputChunkFramingAndLimit(t *testing.T) {
	limit := errors.New("limit")
	w := NewOutput(20, limit)
	w.Chunk(0x2004, func() { w.U8(7); w.Text("ok") })
	if w.Err() != nil || !bytes.Equal(w.Bytes(), []byte{11, 0, 0, 0, 4, 0x20, 7, 2, 0, 'o', 'k'}) {
		t.Fatalf("chunk=%x error=%v", w.Bytes(), w.Err())
	}
	before := bytes.Clone(w.Bytes())
	w.Zeros(10)
	w.U8(1)
	w.PatchSize(0)
	if !errors.Is(w.Err(), limit) || !bytes.Equal(before, w.Bytes()) {
		t.Fatalf("limit must be sticky and leave bytes intact: %x / %v", w.Bytes(), w.Err())
	}
}
