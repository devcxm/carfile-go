package carfile

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestBOMTreeWalksLinkedLeaves(t *testing.T) {
	raw := make([]byte, 256)
	be := binary.BigEndian
	copy(raw[:8], "BOMStore")
	be.PutUint32(raw[8:12], 1)
	be.PutUint32(raw[12:16], 7)
	be.PutUint32(raw[16:20], 128)
	be.PutUint32(raw[20:24], 68)
	be.PutUint32(raw[24:28], 112)
	be.PutUint32(raw[28:32], 13)

	copy(raw[32:36], "tree")
	be.PutUint32(raw[36:40], 1)
	be.PutUint32(raw[40:44], 2)
	be.PutUint32(raw[44:48], 64)
	be.PutUint32(raw[48:52], 2)

	be.PutUint16(raw[64:66], 1)
	be.PutUint16(raw[66:68], 1)
	be.PutUint32(raw[68:72], 3)
	be.PutUint32(raw[76:80], 4)
	be.PutUint32(raw[80:84], 5)

	be.PutUint16(raw[84:86], 1)
	be.PutUint16(raw[86:88], 1)
	be.PutUint32(raw[92:96], 2)
	be.PutUint32(raw[96:100], 6)
	be.PutUint32(raw[100:104], 7)
	copy(raw[104:112], "v1k1v2k2")

	be.PutUint32(raw[112:116], 1)
	be.PutUint32(raw[116:120], 1)
	raw[120] = 4
	copy(raw[121:125], "TEST")

	be.PutUint32(raw[128:132], 8)
	blocks := []BOMBlock{
		{}, {Offset: 32, Length: 21}, {Offset: 64, Length: 20}, {Offset: 84, Length: 20},
		{Offset: 104, Length: 2}, {Offset: 106, Length: 2}, {Offset: 108, Length: 2}, {Offset: 110, Length: 2},
	}
	for i, block := range blocks {
		pos := 132 + i*8
		be.PutUint32(raw[pos:pos+4], block.Offset)
		be.PutUint32(raw[pos+4:pos+8], block.Length)
	}

	bom, err := ParseBOM(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := bom.TreeEntries("TEST")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || string(entries[0].Key) != "k1" || string(entries[0].Value) != "v1" || string(entries[1].Key) != "k2" || string(entries[1].Value) != "v2" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestParseBOMAcceptsUnusedBlockSentinel(t *testing.T) {
	raw := make([]byte, 128)
	be := binary.BigEndian
	copy(raw[:8], "BOMStore")
	be.PutUint32(raw[8:12], 1)
	be.PutUint32(raw[12:16], 2)
	be.PutUint32(raw[16:20], 64)
	be.PutUint32(raw[20:24], 20)
	be.PutUint32(raw[24:28], 96)
	be.PutUint32(raw[28:32], 4)

	be.PutUint32(raw[64:68], 2)
	be.PutUint32(raw[76:80], ^uint32(0))
	be.PutUint32(raw[80:84], ^uint32(0))

	bom, err := ParseBOM(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bom.Block(1); err == nil {
		t.Fatal("unused block sentinel was readable")
	}
}
