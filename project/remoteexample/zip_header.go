package remoteexample

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Count and bound central-directory records before archive/zip allocates its
// entry slice. The example profile has no need for ZIP64, split disks or SFX.
func preflightZip(name string, maxEntries int, maxBytes int64) error {
	f, e := os.Open(name)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	size := st.Size()
	if !st.Mode().IsRegular() || size < 22 || size > maxBytes {
		return fmt.Errorf("invalid archive size")
	}
	tail := make([]byte, min(int64(65557), size))
	if _, e = f.ReadAt(tail, size-int64(len(tail))); e != nil {
		return e
	}
	marker := []byte{'P', 'K', 5, 6}
	index := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if bytes.Equal(tail[i:i+4], marker) && int(binary.LittleEndian.Uint16(tail[i+20:i+22])) == len(tail)-i-22 {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("ZIP end record missing")
	}
	end := tail[index : index+22]
	u16 := binary.LittleEndian.Uint16
	u32 := binary.LittleEndian.Uint32
	count := int(u16(end[10:12]))
	cdSize := int64(u32(end[12:16]))
	offset := int64(u32(end[16:20]))
	endAt := size - int64(len(tail)) + int64(index)
	if u16(end[4:6]) != 0 || u16(end[6:8]) != 0 || u16(end[8:10]) != uint16(count) || count == 65535 || count > maxEntries || cdSize == 0xffffffff || offset == 0xffffffff {
		return fmt.Errorf("multi-volume, ZIP64 or excessive ZIP entry count")
	}
	if count == 0 || offset < 0 || cdSize < 0 || offset+cdSize != endAt {
		return fmt.Errorf("invalid ZIP central directory")
	}
	var signature [4]byte
	if _, e = f.ReadAt(signature[:], 0); e != nil {
		return e
	}
	if signature != [4]byte{'P', 'K', 3, 4} {
		return fmt.Errorf("self-extracting/prefixed archive rejected")
	}
	if _, e = f.Seek(offset, io.SeekStart); e != nil {
		return e
	}
	var head [46]byte
	var consumed int64
	actual := 0
	for consumed < cdSize {
		if cdSize-consumed < 46 {
			return fmt.Errorf("truncated central directory")
		}
		if _, e = io.ReadFull(f, head[:]); e != nil {
			return e
		}
		if !bytes.Equal(head[:4], []byte{'P', 'K', 1, 2}) {
			return fmt.Errorf("invalid central-directory record")
		}
		actual++
		if actual > maxEntries {
			return fmt.Errorf("ZIP entry count exceeds limit")
		}
		extra := int64(u16(head[28:30])) + int64(u16(head[30:32])) + int64(u16(head[32:34]))
		consumed += 46 + extra
		if consumed > cdSize {
			return fmt.Errorf("central-directory fields exceed boundary")
		}
		if u16(head[34:36]) != 0 || u32(head[20:24]) == 0xffffffff || u32(head[24:28]) == 0xffffffff || u32(head[42:46]) == 0xffffffff {
			return fmt.Errorf("ZIP64 or split entry rejected")
		}
		if int64(u32(head[42:46])) >= offset {
			return fmt.Errorf("local file header is outside data area")
		}
		if _, e = f.Seek(extra, io.SeekCurrent); e != nil {
			return e
		}
	}
	if actual != count {
		return fmt.Errorf("ZIP directory count mismatch")
	}
	return nil
}
