package rle

import (
	"encoding/binary"
	"testing"
)

func TestDecodeRepeatLiteralAndEmptyRow(t *testing.T) {
	src := make([]byte, 24)
	binary.LittleEndian.PutUint32(src[0:4], 4)
	binary.LittleEndian.PutUint32(src[4:8], 3)
	binary.LittleEndian.PutUint32(src[8:12], 2)
	binary.LittleEndian.PutUint32(src[12:16], 20)
	binary.LittleEndian.PutUint32(src[16:20], 36)
	binary.LittleEndian.PutUint32(src[20:24], 0x80000002)
	src = append(src, 1, 2, 3, 4)
	var count [4]byte
	binary.LittleEndian.PutUint32(count[:], 1)
	src = append(src, count[:]...)
	src = append(src, 5, 6, 7, 8)

	pixels, err := Decode(src, 3, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		1, 2, 3, 4, 1, 2, 3, 4, 5, 6, 7, 8,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
	if string(pixels) != string(want) {
		t.Fatalf("got %v, want %v", pixels, want)
	}
}

func TestDecodeRejectsUnknownPacketType(t *testing.T) {
	src := make([]byte, 20)
	binary.LittleEndian.PutUint32(src[0:4], 4)
	binary.LittleEndian.PutUint32(src[4:8], 1)
	binary.LittleEndian.PutUint32(src[8:12], 1)
	binary.LittleEndian.PutUint32(src[12:16], 16)
	binary.LittleEndian.PutUint32(src[16:20], 0x01000001)

	if _, err := Decode(src, 1, 1, 4); err == nil {
		t.Fatal("Decode succeeded with an unknown packet type")
	}
}
