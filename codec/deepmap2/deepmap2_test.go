package deepmap2

import (
	"encoding/binary"
	"testing"
)

func TestDecodeDeepmap2Raw(t *testing.T) {
	src := make([]byte, 28)
	binary.LittleEndian.PutUint32(src[0:4], 1)
	binary.LittleEndian.PutUint32(src[4:8], 2)
	binary.LittleEndian.PutUint64(src[8:16], 14)
	copy(src[16:20], "dmp2")
	src[20] = 1
	src[21] = 1
	src[22] = 10
	src[23] = 2
	binary.LittleEndian.PutUint16(src[24:26], 1)
	binary.LittleEndian.PutUint16(src[26:28], 1)
	src = append(src, 0, 0)

	got, err := Decode(src)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 1 || got.Height != 1 || got.PixelFormat != 2 || got.Method != 1 || string(got.Pixels) != "\x00\x00" {
		t.Fatalf("got %#v", got)
	}
}

func TestDecodeDeepmap2Palette(t *testing.T) {
	indices := []byte{0xe4, 0, 1, 1, 0, 0x06, 0, 0, 0, 0, 0, 0, 0}
	payload := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	length := make([]byte, 4)
	binary.LittleEndian.PutUint32(length, uint32(len(indices)))
	payload = append(payload, length...)
	payload = append(payload, indices...)
	src := deepmap2Fixture(deepmap2Palette, 2, 2, 4, uint32(2|(4<<16)), payload)

	got, err := Decode(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{10, 20, 30, 40, 50, 60, 70, 80, 50, 60, 70, 80, 10, 20, 30, 40}
	if string(got.Pixels) != string(want) {
		t.Fatalf("got %v, want %v", got.Pixels, want)
	}
}

func TestDecodeDeepmap2PaletteWithSeparateAlpha(t *testing.T) {
	planes := rawLZFSEStream([]byte{255, 128, 0, 1})
	payload := []byte{
		10, 20, 30, 0,
		40, 50, 60, 0,
	}
	length := make([]byte, 4)
	binary.LittleEndian.PutUint32(length, uint32(len(planes)))
	payload = append(payload, length...)
	payload = append(payload, planes...)
	src := deepmap2Fixture(deepmap2Palette, 2, 1, 4, uint32(2|(3<<16)), payload)

	got, err := Decode(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{10, 20, 30, 255, 40, 50, 60, 128}
	if string(got.Pixels) != string(want) {
		t.Fatalf("got %v, want %v", got.Pixels, want)
	}
}

func TestDecodeDeepmap2Lossless(t *testing.T) {
	pixels := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	compressed := rawLZFSEStream(pixels)
	src := deepmap2Fixture(deepmap2Lossless, 2, 1, 4, uint32(len(compressed)), compressed)
	got, err := Decode(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Pixels) != string(pixels) {
		t.Fatalf("got %v", got.Pixels)
	}
}

func TestDecodeDeepmap2Default(t *testing.T) {
	intermediate := []byte{
		255, 255, // alpha
		0,                // no row predictor
		0, 0, 0, 0, 0, 0, // high-byte plane
		20, 0, 0, 40, 0, 0, // low/sign-byte plane: Y residuals 10 and 20
		0, // 16-byte alignment
	}
	compressed := rawLZFSEStream(intermediate)
	src := deepmap2Fixture(deepmap2Default, 2, 1, 4, uint32(len(compressed)), compressed)
	got, err := Decode(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{10, 10, 10, 255, 20, 20, 20, 255}
	if string(got.Pixels) != string(want) {
		t.Fatalf("got %v, want %v", got.Pixels, want)
	}
}

func TestDecodeDeepmap2DefaultRGBW(t *testing.T) {
	intermediate := []byte{
		255, 255, // alpha
		0,                // no row predictor
		2, 1, 0, 2, 1, 0, // high-byte plane
		0x10, 0xfd, 0x14, 0x10, 0xfd, 0x14, // low/sign-byte plane: Y=264, Co=-254, Cg=10
		0, // 16-byte alignment
	}
	compressed := rawLZFSEStream(intermediate)
	src := deepmap2Fixture(deepmap2Default, 2, 1, 20, uint32(len(compressed)), compressed)
	got, err := decodePortable(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []uint16{9922, 5352, 0, 10000}
	for component, value := range want {
		offset := component * 2
		if actual := binary.LittleEndian.Uint16(got.Pixels[offset : offset+2]); actual != value {
			t.Fatalf("component %d is %d, want %d", component, actual, value)
		}
	}
}

func deepmap2Fixture(method byte, width, height uint16, components byte, descriptor uint32, payload []byte) []byte {
	src := make([]byte, 32)
	binary.LittleEndian.PutUint32(src[0:4], 1)
	binary.LittleEndian.PutUint32(src[4:8], 4)
	binary.LittleEndian.PutUint64(src[8:16], uint64(16+len(payload)))
	copy(src[16:20], "dmp2")
	src[20] = method
	src[21] = 1
	src[22] = 10
	src[23] = components
	binary.LittleEndian.PutUint16(src[24:26], width)
	binary.LittleEndian.PutUint16(src[26:28], height)
	binary.LittleEndian.PutUint32(src[28:32], descriptor)
	return append(src, payload...)
}

func rawLZFSEStream(raw []byte) []byte {
	stream := make([]byte, 8)
	copy(stream, "bvx-")
	binary.LittleEndian.PutUint32(stream[4:], uint32(len(raw)))
	stream = append(stream, raw...)
	return append(stream, 'b', 'v', 'x', '$')
}
