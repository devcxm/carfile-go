package carfile

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestParseCARHeader(t *testing.T) {
	raw := make([]byte, 436)
	copy(raw[:4], "RATC")
	le := binary.LittleEndian
	le.PutUint32(raw[4:8], 975)
	le.PutUint32(raw[8:12], 17)
	le.PutUint32(raw[16:20], 7)
	copy(raw[20:148], "CoreUI test")
	copy(raw[148:404], "Xcode test")
	le.PutUint32(raw[424:428], 2)

	header, order, err := parseCARHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	if order != binary.LittleEndian || header.Tag != "CTAR" || header.CoreUIVersion != 975 || header.RenditionCount != 7 {
		t.Fatalf("unexpected header: %#v", header)
	}
}

func TestParseCSI(t *testing.T) {
	raw := make([]byte, 184+12+16)
	le := binary.LittleEndian
	copy(raw[:4], "ISTC")
	le.PutUint32(raw[4:8], 1)
	le.PutUint32(raw[12:16], 20)
	le.PutUint32(raw[16:20], 30)
	le.PutUint32(raw[20:24], 200)
	copy(raw[24:28], "BGRA")
	le.PutUint16(raw[36:38], 12)
	copy(raw[40:168], "icon.png")
	le.PutUint32(raw[168:172], 12)
	le.PutUint32(raw[172:176], 1)
	le.PutUint32(raw[180:184], 16)
	le.PutUint32(raw[184:188], 1006)
	le.PutUint32(raw[188:192], 4)
	le.PutUint32(raw[192:196], 1)
	copy(raw[196:200], "MLEC")
	le.PutUint32(raw[200:204], 0)
	le.PutUint32(raw[204:208], 4)
	le.PutUint32(raw[208:212], 0)

	csi, err := parseCSI(raw, nil, le)
	if err != nil {
		t.Fatal(err)
	}
	if csi.Tag != "CTSI" || csi.PixelFormat != "ARGB" || csi.LayoutName != "One Part Scale" || csi.Payload.Tag != "CELM" {
		t.Fatalf("unexpected CSI: %#v", csi)
	}
	if len(csi.TLVs) != 1 || csi.TLVs[0].Type != 1006 {
		t.Fatalf("unexpected TLVs: %#v", csi.TLVs)
	}
}

func TestAttributeNames(t *testing.T) {
	if got := AttributeType(17).String(); got != "Identifier" {
		t.Fatalf("AttributeType(17) = %q", got)
	}
	if got := AttributeType(99).String(); got != "Unknown 99" {
		t.Fatalf("AttributeType(99) = %q", got)
	}
	if got := AttributeType(13).String(); got != "Localization" {
		t.Fatalf("AttributeType(13) = %q", got)
	}
}

func TestCanonicalFourCCPreservesLiteralTag(t *testing.T) {
	if got := canonicalFourCC([]byte("META"), binary.LittleEndian); got != "META" {
		t.Fatalf("META = %q", got)
	}
	if got := canonicalFourCC([]byte("BGRA"), binary.LittleEndian); got != "ARGB" {
		t.Fatalf("BGRA = %q", got)
	}
}

func TestParseInternalLinkTLV(t *testing.T) {
	value, err := hex.DecodeString("4b4c4e49000000004a0000004a00000017000000170000000c0014000000010009000200b50008001d000c00020000000000")
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 8+len(value))
	binary.LittleEndian.PutUint32(raw[0:4], 1010)
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(value)))
	copy(raw[8:], value)

	tlvs, err := parseTLVs(raw, nil, binary.LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if len(tlvs) != 1 || tlvs[0].LinkedRect == nil {
		t.Fatalf("unexpected TLV: %#v", tlvs)
	}
	if got := *tlvs[0].LinkedRect; got != (PixelRect{X: 74, Y: 74, Width: 23, Height: 23}) {
		t.Fatalf("linked rect = %#v", got)
	}
	if len(tlvs[0].LinkedKey) != 4 || tlvs[0].LinkedKey[2].Type != 8 || tlvs[0].LinkedKey[2].Value != 29 {
		t.Fatalf("linked key = %#v", tlvs[0].LinkedKey)
	}
}
