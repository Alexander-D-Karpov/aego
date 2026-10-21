package wav

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	"aego/audio/codec"
)

const (
	fmtPCM        = 1
	fmtFloat      = 3
	fmtExtensible = 0xFFFE
	maxChunks     = 4096
	maxHeader     = 1 << 16
	maxRate       = 768000
)

var errFormat = errors.New("wav: unsupported or broken file")

func init() {
	codec.Register(codec.Format{Name: "wav", Match: Match, Open: Open})
}

func Match(h []byte) bool {
	return len(h) >= 12 && string(h[0:4]) == "RIFF" && string(h[8:12]) == "WAVE"
}

type Decoder struct {
	r       io.ReadSeeker
	info    codec.Info
	float   bool
	bytes   int
	frame   int
	dataOff int64
	pos     int64
	loopS   int64
	loopE   int64
	loop    bool
	raw     []byte
}

func Open(r io.ReadSeeker) (codec.Decoder, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil || !Match(hdr[:]) {
		return nil, errFormat
	}
	d := &Decoder{r: r}
	var haveFmt, haveData bool
	var dataLen int64
	at := int64(12)
	for i := 0; i < maxChunks && at+8 <= size; i++ {
		if _, err := r.Seek(at, io.SeekStart); err != nil {
			return nil, err
		}
		var ck [8]byte
		if _, err := io.ReadFull(r, ck[:]); err != nil {
			break
		}
		n := int64(binary.LittleEndian.Uint32(ck[4:]))
		body := at + 8
		switch string(ck[:4]) {
		case "fmt ":
			if err := d.readFmt(n); err != nil {
				return nil, err
			}
			haveFmt = true
		case "data":
			d.dataOff = body
			dataLen = max(0, min(n, size-body))
			haveData = true
		case "smpl":
			d.readSmpl(n)
		}
		at = body + n + n&1
	}
	if !haveFmt || !haveData {
		return nil, errFormat
	}
	d.info.Frames = dataLen / int64(d.frame)
	if d.loop && (d.loopE > d.info.Frames || d.loopS >= d.loopE) {
		d.loop = false
	}
	if _, err := r.Seek(d.dataOff, io.SeekStart); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Decoder) chunk(n int64) ([]byte, error) {
	if n > maxHeader {
		return nil, errFormat
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(d.r, b); err != nil {
		return nil, errFormat
	}
	return b, nil
}

func (d *Decoder) readFmt(n int64) error {
	if n < 16 {
		return errFormat
	}
	b, err := d.chunk(n)
	if err != nil {
		return err
	}
	format := binary.LittleEndian.Uint16(b[0:])
	ch := int(binary.LittleEndian.Uint16(b[2:]))
	rate := int(binary.LittleEndian.Uint32(b[4:]))
	bits := int(binary.LittleEndian.Uint16(b[14:]))
	if format == fmtExtensible && len(b) >= 26 {
		format = binary.LittleEndian.Uint16(b[24:])
	}
	switch {
	case ch < 1 || ch > 2, rate < 1 || rate > maxRate:
		return errFormat
	case format == fmtPCM && (bits == 8 || bits == 16 || bits == 24 || bits == 32):
	case format == fmtFloat && bits == 32:
		d.float = true
	default:
		return errFormat
	}
	d.bytes = bits / 8
	d.frame = d.bytes * ch
	d.info.SampleRate, d.info.Channels = rate, ch
	return nil
}

func (d *Decoder) readSmpl(n int64) {
	if n < 36+24 {
		return
	}
	b, err := d.chunk(n)
	if err != nil {
		return
	}
	if binary.LittleEndian.Uint32(b[28:]) == 0 {
		return
	}
	s := int64(binary.LittleEndian.Uint32(b[36+8:]))
	e := int64(binary.LittleEndian.Uint32(b[36+12:]))
	if e >= s {
		d.loopS, d.loopE, d.loop = s, e+1, true
	}
}

func (d *Decoder) Info() codec.Info {
	return d.info
}

func (d *Decoder) Loop() (int64, int64, bool) {
	return d.loopS, d.loopE, d.loop
}

func (d *Decoder) Read(dst []float32) (int, error) {
	ch := d.info.Channels
	frames := min(int64(len(dst)/ch), d.info.Frames-d.pos)
	if frames <= 0 {
		return 0, io.EOF
	}
	need := int(frames) * d.frame
	if cap(d.raw) < need {
		d.raw = make([]byte, need)
	}
	got, _ := io.ReadFull(d.r, d.raw[:need])
	n := got / d.frame
	if n == 0 {
		return 0, io.EOF
	}
	d.decode(d.raw[:n*d.frame], dst[:n*ch])
	d.pos += int64(n)
	return n, nil
}

func (d *Decoder) decode(raw []byte, out []float32) {
	switch {
	case d.float:
		for i := range out {
			v := math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
			if v != v || v > 1e6 || v < -1e6 {
				v = 0
			}
			out[i] = v
		}
	case d.bytes == 1:
		for i := range out {
			out[i] = (float32(raw[i]) - 128) / 128
		}
	case d.bytes == 2:
		for i := range out {
			out[i] = float32(int16(binary.LittleEndian.Uint16(raw[i*2:]))) / 32768
		}
	case d.bytes == 3:
		for i := range out {
			j := i * 3
			v := int32(uint32(raw[j])<<8|uint32(raw[j+1])<<16|uint32(raw[j+2])<<24) >> 8
			out[i] = float32(v) / 8388608
		}
	default:
		for i := range out {
			out[i] = float32(int32(binary.LittleEndian.Uint32(raw[i*4:]))) / 2147483648
		}
	}
}

func (d *Decoder) SeekFrame(frame int64) error {
	frame = min(max(frame, 0), d.info.Frames)
	if _, err := d.r.Seek(d.dataOff+frame*int64(d.frame), io.SeekStart); err != nil {
		return err
	}
	d.pos = frame
	return nil
}

func (d *Decoder) Close() error {
	if c, ok := d.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
