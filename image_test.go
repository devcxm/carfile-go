package carfile

import (
	"encoding/binary"
	"testing"
)

func TestImageFromBGRA(t *testing.T) {
	img, err := imageFromPixels([]byte{3, 2, 1, 4}, 1, 1, "ARGB", false)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 1 || g>>8 != 2 || b>>8 != 3 || a>>8 != 4 {
		t.Fatalf("got RGBA %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestImageFromWideBGRA(t *testing.T) {
	pixels := make([]byte, 8)
	binary.LittleEndian.PutUint16(pixels[0:2], 2500)
	binary.LittleEndian.PutUint16(pixels[2:4], 5000)
	binary.LittleEndian.PutUint16(pixels[4:6], 10000)
	binary.LittleEndian.PutUint16(pixels[6:8], 10000)
	img, err := imageFromPixels(pixels, 1, 1, "RGBW", false)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 128 || b>>8 != 64 || a>>8 != 255 {
		t.Fatalf("got RGBA %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestDecodeDeepmap2UsesBGRAByteOrder(t *testing.T) {
	stream := make([]byte, 8)
	copy(stream[0:4], "bvx-")
	binary.LittleEndian.PutUint32(stream[4:8], 4)
	stream = append(stream, 3, 2, 1, 255)
	stream = append(stream, 'b', 'v', 'x', '$')

	deepmap := make([]byte, 32+len(stream))
	binary.LittleEndian.PutUint32(deepmap[0:4], 1)
	binary.LittleEndian.PutUint64(deepmap[8:16], uint64(len(deepmap)-16))
	copy(deepmap[16:20], "dmp2")
	deepmap[20] = 3
	deepmap[21] = 1
	deepmap[23] = 4
	binary.LittleEndian.PutUint16(deepmap[24:26], 1)
	binary.LittleEndian.PutUint16(deepmap[26:28], 1)
	binary.LittleEndian.PutUint32(deepmap[28:32], uint32(len(stream)))
	copy(deepmap[32:], stream)

	kind := uint32(11)
	rendition := Rendition{CSI: CSI{
		Width: 1, Height: 1, PixelFormat: "ARGB",
		Payload: Payload{Tag: "CELM", CompressionType: &kind, Data: append(make([]byte, 16), deepmap...)},
	}}
	img, err := DecodeRenditionImage(rendition)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 1 || g>>8 != 2 || b>>8 != 3 || a>>8 != 255 {
		t.Fatalf("got RGBA %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
}
