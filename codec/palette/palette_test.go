package palette

import (
	"encoding/binary"
	"testing"
)

func TestDecodeARGB(t *testing.T) {
	decoded := make([]byte, 10)
	binary.LittleEndian.PutUint32(decoded[0:4], magic)
	binary.LittleEndian.PutUint32(decoded[4:8], 1)
	binary.LittleEndian.PutUint16(decoded[8:10], 2)
	decoded = append(decoded,
		1, 2, 3, 4,
		5, 6, 7, 8,
		0x00, 0x40, // two one-bit indices, MSB first: 0, 1
	)

	pixels, err := decodeExpanded(decoded, 2, 1, "ARGB")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{4, 3, 2, 1, 8, 7, 6, 5}
	if string(pixels) != string(want) {
		t.Fatalf("got %v, want %v", pixels, want)
	}
}

func TestDecodeSingleColorNeedsNoIndexData(t *testing.T) {
	decoded := make([]byte, 10)
	binary.LittleEndian.PutUint32(decoded[0:4], magic)
	binary.LittleEndian.PutUint32(decoded[4:8], 1)
	binary.LittleEndian.PutUint16(decoded[8:10], 1)
	decoded = append(decoded, 9, 8, 7, 6)
	decoded = append(decoded, 0, 0, 0, 0)

	pixels, err := decodeExpanded(decoded, 3, 2, "ARGB")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		6, 7, 8, 9, 6, 7, 8, 9, 6, 7, 8, 9,
		6, 7, 8, 9, 6, 7, 8, 9, 6, 7, 8, 9,
	}
	if string(pixels) != string(want) {
		t.Fatalf("got %v, want %v", pixels, want)
	}
}
