// Package bufferdiff bounds comparisons of small dynamic GPU writes.
// A change in offset or length makes the next upload independent.
package bufferdiff

import "bytes"

const maximum = 16 << 10

type LastWrite struct {
	at   int
	data []byte
}

// ReserveSmall reserves only the backing for a small buffer of known size.
// The first write is still sent in full. Larger, partially used GPU buffers
// continue to acquire bounded comparison storage from their actual writes.
func (w *LastWrite) ReserveSmall(size int) {
	const limit = 1024
	if size > 0 && size <= limit && cap(w.data) < size {
		w.data = make([]byte, 0, size)
	}
}

func (w *LastWrite) Reset() { w.data = w.data[:0] }

func (w *LastWrite) Delta(at int, data []byte) (int, []byte) {
	if len(data) > maximum {
		w.Reset()
		return at, data
	}
	if len(w.data) != len(data) || w.at != at {
		w.at = at
		w.data = append(w.data[:0], data...)
		return at, data
	}
	if bytes.Equal(w.data, data) {
		return at, nil
	}
	// Keep one contiguous GL command. Comparing coarse blocks avoids running
	// a byte-wise WASM loop over a long unchanged sprite or vertex prefix.
	const block = 32
	start, end := 0, len(data)
	for end-start >= block && bytes.Equal(w.data[start:start+block], data[start:start+block]) {
		start += block
	}
	for end-start >= block && bytes.Equal(w.data[end-block:end], data[end-block:end]) {
		end -= block
	}
	for start < end && w.data[start] == data[start] {
		start++
	}
	for start < end && w.data[end-1] == data[end-1] {
		end--
	}
	copy(w.data[start:end], data[start:end])
	return at + start, data[start:end]
}
