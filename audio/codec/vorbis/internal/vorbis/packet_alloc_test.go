package vorbis

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Read enough real Ogg packets to exercise changing short/long Vorbis blocks.
// This parses the fixture container only; the production reader is unchanged.
func fixturePackets(data []byte, want int) ([][]byte, error) {
	var packets [][]byte
	var packet []byte
	for pos := 0; pos < len(data) && len(packets) < want; {
		if pos+27 > len(data) || !bytes.Equal(data[pos:pos+4], []byte("OggS")) {
			return nil, fmt.Errorf("invalid Ogg page at %d", pos)
		}
		count := int(data[pos+26])
		if pos+27+count > len(data) {
			return nil, fmt.Errorf("truncated lacing table")
		}
		laces := data[pos+27 : pos+27+count]
		pos += 27 + count
		for _, lace := range laces {
			n := int(lace)
			if pos+n > len(data) {
				return nil, fmt.Errorf("truncated packet")
			}
			packet = append(packet, data[pos:pos+n]...)
			pos += n
			if lace < 255 {
				packets = append(packets, packet)
				packet = nil
			}
		}
	}
	if len(packets) < want {
		return nil, fmt.Errorf("only %d packets, want %d", len(packets), want)
	}
	return packets, nil
}

func TestStereoVorbisSteadyPacketDecodeDoesNotAllocate(t *testing.T) {
	file := filepath.Join("..", "..", "..", "..", "..", "examples", "platformer", "assets", "audio", "glorious-morning.ogg")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	packets, err := fixturePackets(data, 200)
	if err != nil {
		t.Fatal(err)
	}
	var decoder Decoder
	for _, header := range packets[:3] {
		if err := decoder.ReadHeader(header); err != nil {
			t.Fatal(err)
		}
	}
	if decoder.Channels() != 2 {
		t.Fatal("the fixture must exercise distinct stereo floor state")
	}
	buffer := make([]float32, decoder.BufferSize())
	audio := packets[3:]
	for _, packet := range audio {
		if _, err := decoder.DecodeInto(packet, buffer); err != nil {
			t.Fatal(err)
		}
	}
	i := 0
	var decodeErr error
	allocs := testing.AllocsPerRun(200, func() {
		_, decodeErr = decoder.DecodeInto(audio[i%len(audio)], buffer)
		i++
	})
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if allocs != 0 {
		t.Fatalf("steady stereo packet decode allocates %v times", allocs)
	}
}

func TestFloorZeroKeepsChannelBuffersIndependent(t *testing.T) {
	f := floor0{order: 2, amplitudeBits: 4, bookList: []uint8{0}}
	books := []codebook{{dimensions: 2, entries: huffmanCode{1, 1}, values: []float32{0.25, 0.5}}}
	var left, right floorData
	if !f.Decode(&bitReader{data: []byte{3}}, books, 0, &left) || !f.Decode(&bitReader{data: []byte{7}}, books, 0, &right) {
		t.Fatal("nonzero amplitudes should decode")
	}
	if left.floor0.amplitude != 3 || right.floor0.amplitude != 7 || left.floor0.coefficients[0] != 0.25 || right.floor0.coefficients[1] != 0.5 {
		t.Fatal("floor-zero channel values changed")
	}
	before := &left.floor0.coefficients[0]
	right.floor0.coefficients[0] = 9
	if left.floor0.coefficients[0] != 0.25 {
		t.Fatal("stereo channels shared floor-zero scratch")
	}
	if !f.Decode(&bitReader{data: []byte{5}}, books, 0, &left) || &left.floor0.coefficients[0] != before {
		t.Fatal("floor-zero scratch was not reused")
	}
	if f.Decode(&bitReader{data: []byte{0}}, books, 0, &left) {
		t.Fatal("zero amplitude must be absent")
	}
}

func TestFloorZeroOddOrderStartsWithFiniteGain(t *testing.T) {
	f := floor0{order: 1, rate: 44100, barkMapSize: 32, amplitudeBits: 6, amplitudeOffset: 60}
	data := floorData{floor0: floor0Data{amplitude: 32, coefficients: []float32{math.Pi / 2}}}
	out := make([]float32, 64)
	for i := range out {
		out[i] = 1
	}
	f.Apply(out, &data)
	if math.IsNaN(float64(out[0])) || math.IsInf(float64(out[0]), 0) || out[0] <= 0 || out[0] >= 1 {
		t.Fatalf("odd-order floor-zero DC gain = %v, want finite attenuation", out[0])
	}
}
