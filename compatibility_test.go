package carfile

import (
	"encoding/binary"
	"strings"
	"testing"
)

type compatibilityLevel string

const (
	compatibilityDecoded      compatibilityLevel = "decoded"
	compatibilityPassThrough  compatibilityLevel = "pass-through"
	compatibilityMetadataOnly compatibilityLevel = "metadata-only"
	compatibilityUnsupported  compatibilityLevel = "unsupported"
)

func TestCompressionCompatibilityMatrix(t *testing.T) {
	cases := []struct {
		id    uint32
		name  string
		level compatibilityLevel
	}{
		{0, "uncompressed", compatibilityDecoded},
		{1, "rle", compatibilityDecoded},
		{2, "zip", compatibilityDecoded},
		{3, "lzvn", compatibilityUnsupported},
		{4, "lzfse", compatibilityDecoded},
		{5, "jpeg-lzfse", compatibilityUnsupported},
		{6, "blurred", compatibilityUnsupported},
		{7, "astc", compatibilityUnsupported},
		{8, "palette-img", compatibilityDecoded},
		{9, "hevc", compatibilityUnsupported},
		{10, "deepmap-lzfse", compatibilityDecoded},
		{11, "deepmap2", compatibilityDecoded},
		{12, "dxtc", compatibilityUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compressionName(tc.id); got != tc.name {
				t.Fatalf("compression %d = %q, want %q", tc.id, got, tc.name)
			}
			kind := tc.id
			_, err := DecodeRenditionImage(Rendition{CSI: CSI{
				Width: 1, Height: 1, PixelFormat: "ARGB",
				Payload: Payload{Tag: "CELM", CompressionType: &kind, Data: make([]byte, 16)},
			}})
			unsupported := err != nil && strings.Contains(err.Error(), "unsupported bitmap compression")
			if tc.level == compatibilityDecoded && unsupported {
				t.Fatalf("documented decoder is not dispatched: %v", err)
			}
			if tc.level == compatibilityUnsupported && !unsupported {
				t.Fatalf("documented unsupported compression entered a decoder: %v", err)
			}
		})
	}
}

func TestPixelFormatCompatibilityMatrix(t *testing.T) {
	cases := []struct {
		format       string
		storageBytes int
		level        compatibilityLevel
	}{
		{"ARGB", 4, compatibilityDecoded},
		{"GA8 ", 2, compatibilityDecoded},
		{"RGBW", 8, compatibilityDecoded},
		{"GA16", 0, compatibilityUnsupported},
		{"RGB5", 0, compatibilityUnsupported},
	}
	for _, tc := range cases {
		t.Run(strings.TrimSpace(tc.format), func(t *testing.T) {
			got, err := pixelStorageBytes(tc.format)
			if tc.level == compatibilityDecoded {
				if err != nil || got != tc.storageBytes {
					t.Fatalf("pixel format %q = %d, %v", tc.format, got, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("unsupported pixel format %q reports %d bytes", tc.format, got)
			}
		})
	}
}

func TestPayloadCompatibilityMatrix(t *testing.T) {
	le := binary.LittleEndian
	cases := []struct {
		tag   string
		raw   []byte
		level compatibilityLevel
		check func(Payload) bool
	}{
		{"RAWD", append([]byte("DWAR"), make([]byte, 8)...), compatibilityPassThrough, func(p Payload) bool { return p.Tag == "RAWD" }},
		{"CELM", append([]byte("MLEC"), make([]byte, 12)...), compatibilityDecoded, func(p Payload) bool { return p.CompressionType != nil }},
		{"COLR", append([]byte("RLOC"), make([]byte, 12)...), compatibilityDecoded, func(p Payload) bool { return p.ColorSpaceID != nil }},
		{"ARGG", append([]byte("ARGG"), make([]byte, 28)...), compatibilityMetadataOnly, func(p Payload) bool { return p.Gradient != nil }},
		{"SISM", append([]byte("MSIS"), make([]byte, 4)...), compatibilityUnsupported, func(p Payload) bool {
			return p.Tag == "SISM" && p.CompressionType == nil && p.ColorSpaceID == nil && p.Gradient == nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.tag+"_"+string(tc.level), func(t *testing.T) {
			if payload := parsePayload(tc.raw, le); !tc.check(payload) {
				t.Fatalf("unexpected payload: %#v", payload)
			}
		})
	}
}

func TestLayoutCompatibilityRegistry(t *testing.T) {
	known := map[uint16]string{
		7: "Text Effect", 9: "Vector", 10: "One Part Fixed Size", 11: "One Part Tile", 12: "One Part Scale",
		20: "Three Part Horizontal Tile", 21: "Three Part Horizontal Scale", 22: "Three Part Horizontal Uniform",
		23: "Three Part Vertical Tile", 24: "Three Part Vertical Scale", 25: "Three Part Vertical Uniform",
		30: "Nine Part Tile", 31: "Nine Part Scale", 32: "Nine Part Horizontal Tile Vertical Tile",
		33: "Nine Part Horizontal Tile Vertical Scale", 34: "Nine Part Horizontal Scale Vertical Tile",
		40: "Many Part", 50: "Animation Filmstrip", 1000: "Data", 1001: "External Link",
		1002: "Layer Stack", 1003: "Internal Reference", 1004: "Packed Image", 1005: "Named Content",
		1006: "Thinning Placeholder", 1007: "Texture", 1008: "Texture Image", 1009: "Color",
		1010: "Multisize Image Set", 1011: "Layer Reference", 1012: "Content Rendition",
		1013: "Recognition Object", 1019: "Icon Image Stack", 1020: "Icon Group", 1021: "Named Gradient",
	}
	for id, want := range known {
		if got := layoutName(id); got != want {
			t.Errorf("layout %d = %q, want %q", id, got, want)
		}
	}
	if got := layoutName(1017); got != "Unknown 1017" {
		t.Fatalf("unverified layout 1017 unexpectedly has semantic name %q", got)
	}
}
