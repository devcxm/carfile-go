package kcbc

import (
	"encoding/binary"
	"testing"
)

func TestDecodeKCBCStripsRowPadding(t *testing.T) {
	first := rawLZFSEStream([]byte{1, 2, 3, 4, 99, 99, 5, 6, 7, 8, 99, 99})
	second := rawLZFSEStream([]byte{9, 10, 11, 12, 88, 88})
	input := append(kcbcChunk(2, first), kcbcChunk(1, second)...)

	got, err := Decode(input, 2, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	if string(got) != string(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDecodeKCBCRejectsMissingRows(t *testing.T) {
	input := kcbcChunk(1, rawLZFSEStream([]byte{1, 2, 3, 4}))
	if _, err := Decode(input, 1, 2, 4); err == nil {
		t.Fatal("expected an error")
	}
}

func rawLZFSEStream(raw []byte) []byte {
	stream := make([]byte, 8)
	copy(stream, "bvx-")
	binary.LittleEndian.PutUint32(stream[4:], uint32(len(raw)))
	stream = append(stream, raw...)
	return append(stream, 'b', 'v', 'x', '$')
}

func kcbcChunk(height uint32, compressed []byte) []byte {
	header := make([]byte, 20)
	copy(header, "KCBC")
	binary.LittleEndian.PutUint32(header[12:], height)
	binary.LittleEndian.PutUint32(header[16:], uint32(len(compressed)))
	return append(header, compressed...)
}
