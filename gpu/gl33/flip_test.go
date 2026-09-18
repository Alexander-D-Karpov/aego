package gl33

import "testing"

func TestFlipRows(t *testing.T) {
	// 2px wide, 3 rows tall, 1 byte per channel: rows tagged by their first byte.
	const stride, rows = 2 * 4, 3
	buf := make([]byte, stride*rows)
	for r := 0; r < rows; r++ {
		for i := 0; i < stride; i++ {
			buf[r*stride+i] = byte(r + 1)
		}
	}
	flipRows(buf, stride, rows)
	for r, want := range []byte{3, 2, 1} {
		if got := buf[r*stride]; got != want {
			t.Fatalf("row %d = %d, want %d", r, got, want)
		}
	}
}

func TestFlipRowsEvenAndSingle(t *testing.T) {
	single := []byte{9, 9, 9, 9}
	flipRows(single, 4, 1)
	if single[0] != 9 {
		t.Fatal("single row must be untouched")
	}
	even := []byte{1, 1, 1, 1, 2, 2, 2, 2}
	flipRows(even, 4, 2)
	if even[0] != 2 || even[4] != 1 {
		t.Fatalf("even swap wrong: %v", even)
	}
}
