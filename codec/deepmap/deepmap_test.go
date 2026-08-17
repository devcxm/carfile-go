package deepmap

import (
	"encoding/binary"
	"testing"
)

func TestDecodeLegacyKCBC(t *testing.T) {
	pixels := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	stream := make([]byte, 8)
	copy(stream, "bvx-")
	binary.LittleEndian.PutUint32(stream[4:], uint32(len(pixels)))
	stream = append(stream, pixels...)
	stream = append(stream, 'b', 'v', 'x', '$')

	legacy := make([]byte, 28)
	binary.LittleEndian.PutUint32(legacy[0:4], 5)
	binary.LittleEndian.PutUint32(legacy[4:8], 4)
	binary.LittleEndian.PutUint64(legacy[8:16], uint64(12+len(stream)))
	copy(legacy[16:20], "dmap")
	legacy[20] = 3
	legacy[21] = 1
	legacy[22] = 10
	legacy[23] = 4
	binary.LittleEndian.PutUint32(legacy[24:28], uint32(len(stream)))
	legacy = append(legacy, stream...)

	src := make([]byte, 20)
	copy(src, "KCBC")
	binary.LittleEndian.PutUint32(src[12:16], 1)
	binary.LittleEndian.PutUint32(src[16:20], uint32(len(legacy)))
	src = append(src, legacy...)

	got, err := Decode(src, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.PixelFormat != 4 || string(got.Pixels) != string(pixels) {
		t.Fatalf("got format %d pixels %v", got.PixelFormat, got.Pixels)
	}
}

func TestDecodeLegacyKCBCSplitLZVN(t *testing.T) {
	firstPixels := make([]byte, 256*2*4)
	secondPixels := make([]byte, 2*2*4)
	for i := range firstPixels {
		firstPixels[i] = 1
	}
	for i := range secondPixels {
		secondPixels[i] = 2
	}
	src := legacyKCBCFixture(3, 4, 0, nil, [][]byte{rawLZFSEStream(firstPixels), rawLZFSEStream(secondPixels)}, 2)

	got, err := Decode(src, 258, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 258*2*4)
	for row := 0; row < 2; row++ {
		copy(want[row*258*4:row*258*4+256*4], firstPixels[row*256*4:(row+1)*256*4])
		copy(want[row*258*4+256*4:(row+1)*258*4], secondPixels[row*2*4:(row+1)*2*4])
	}
	if string(got.Pixels) != string(want) {
		t.Fatal("lossless tiles were not stitched by row")
	}
}

func TestDecodeLegacyKCBCSplitDefault(t *testing.T) {
	first := rawLZFSEStream(defaultARGBData(256, 2, 10))
	second := rawLZFSEStream(defaultARGBData(2, 2, 20))
	src := legacyKCBCFixture(2, 4, 0, nil, [][]byte{first, second}, 2)

	got, err := Decode(src, 258, 2)
	if err != nil {
		t.Fatal(err)
	}
	for row := 0; row < 2; row++ {
		for column := 0; column < 258; column++ {
			want := byte(10)
			if column >= 256 {
				want = 20
			}
			pixel := got.Pixels[(row*258+column)*4 : (row*258+column+1)*4]
			if string(pixel) != string([]byte{want, want, want, 255}) {
				t.Fatalf("pixel %d,%d is %v", column, row, pixel)
			}
		}
	}
}

func TestDecodeLegacyKCBCPaletteWithSeparateAlpha(t *testing.T) {
	palette := []byte{
		10, 20, 30, 0,
		40, 50, 60, 0,
	}
	firstData := append(make([]byte, 256*2), make([]byte, 256*2)...)
	for i := 0; i < 256*2; i++ {
		firstData[i] = 255
	}
	secondData := []byte{128, 128, 128, 128, 1, 1, 1, 1}
	src := legacyKCBCFixture(4, 4, 2, palette, [][]byte{rawLZFSEStream(firstData), rawLZFSEStream(secondData)}, 2)

	got, err := Decode(src, 258, 2)
	if err != nil {
		t.Fatal(err)
	}
	for row := 0; row < 2; row++ {
		for column := 0; column < 258; column++ {
			want := []byte{10, 20, 30, 255}
			if column >= 256 {
				want = []byte{40, 50, 60, 128}
			}
			pixel := got.Pixels[(row*258+column)*4 : (row*258+column+1)*4]
			if string(pixel) != string(want) {
				t.Fatalf("pixel %d,%d is %v", column, row, pixel)
			}
		}
	}
}

func legacyKCBCFixture(method, pixelFormat byte, descriptor uint32, prefix []byte, segments [][]byte, rows uint32) []byte {
	payload := append([]byte{}, prefix...)
	if method == 4 {
		for _, segment := range segments {
			length := make([]byte, 4)
			binary.LittleEndian.PutUint32(length, uint32(len(segment)))
			payload = append(payload, length...)
			payload = append(payload, segment...)
		}
	} else {
		descriptor = uint32(len(segments[0]))
		payload = append(payload, segments[0]...)
		for _, segment := range segments[1:] {
			length := make([]byte, 4)
			binary.LittleEndian.PutUint32(length, uint32(len(segment)))
			payload = append(payload, length...)
			payload = append(payload, segment...)
		}
	}
	legacy := make([]byte, 28)
	binary.LittleEndian.PutUint32(legacy[0:4], 5)
	binary.LittleEndian.PutUint32(legacy[4:8], 4)
	binary.LittleEndian.PutUint64(legacy[8:16], uint64(12+len(payload)))
	copy(legacy[16:20], "dmap")
	legacy[20] = method
	legacy[21] = 1
	legacy[22] = 10
	legacy[23] = pixelFormat
	binary.LittleEndian.PutUint32(legacy[24:28], descriptor)
	legacy = append(legacy, payload...)

	src := make([]byte, 20)
	copy(src, "KCBC")
	binary.LittleEndian.PutUint32(src[12:16], rows)
	binary.LittleEndian.PutUint32(src[16:20], uint32(len(legacy)))
	return append(src, legacy...)
}

func rawLZFSEStream(raw []byte) []byte {
	stream := make([]byte, 8)
	copy(stream, "bvx-")
	binary.LittleEndian.PutUint32(stream[4:], uint32(len(raw)))
	stream = append(stream, raw...)
	return append(stream, 'b', 'v', 'x', '$')
}

func defaultARGBData(width, height int, value byte) []byte {
	pixelCount := width * height
	data := make([]byte, pixelCount+height+6*pixelCount)
	for i := 0; i < pixelCount; i++ {
		data[i] = 255
		data[pixelCount+height+3*pixelCount+i*3] = value << 1
	}
	return append(data, make([]byte, (-len(data))&7)...)
}
