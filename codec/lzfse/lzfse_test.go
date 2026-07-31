package lzfse

import (
	"encoding/binary"
	"testing"
)

func TestDecodeLZFSERawBlock(t *testing.T) {
	stream := make([]byte, 8)
	binary.LittleEndian.PutUint32(stream, lzfseRawMagic)
	binary.LittleEndian.PutUint32(stream[4:], 5)
	stream = append(stream, "hello"...)
	stream = append(stream, 'b', 'v', 'x', '$')

	got, err := Decode(stream)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeLZFSERejectsTruncatedRawBlock(t *testing.T) {
	stream := make([]byte, 8)
	binary.LittleEndian.PutUint32(stream, lzfseRawMagic)
	binary.LittleEndian.PutUint32(stream[4:], 20)
	if _, err := Decode(stream); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFSETableRequiresAllStates(t *testing.T) {
	if _, err := makeFSESymbolTable(4, []uint16{1, 2}); err == nil {
		t.Fatal("expected an error")
	}
	table, err := makeFSESymbolTable(4, []uint16{1, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(table) != 4 {
		t.Fatalf("got %d states", len(table))
	}
}
