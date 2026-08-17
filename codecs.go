package carfile

import (
	"github.com/devcxm/carfile-go/codec/deepmap"
	"github.com/devcxm/carfile-go/codec/deepmap2"
	"github.com/devcxm/carfile-go/codec/kcbc"
	"github.com/devcxm/carfile-go/codec/lzfse"
	"github.com/devcxm/carfile-go/codec/lzvn"
	"github.com/devcxm/carfile-go/codec/palette"
	"github.com/devcxm/carfile-go/codec/rle"
)

// Deepmap2Bitmap is kept as a root-package alias for callers that do not need
// to import the specialized codec package directly.
type Deepmap2Bitmap = deepmap2.Bitmap

// DecodeLZFSE decodes an Apple LZFSE stream.
func DecodeLZFSE(src []byte) ([]byte, error) {
	return lzfse.Decode(src)
}

// DecodeLZVN decodes a raw Apple LZVN stream to outputSize bytes.
func DecodeLZVN(src []byte, outputSize int) ([]byte, error) {
	return lzvn.Decode(src, outputSize)
}

// DecodeKCBC decodes a CoreUI chunked bitmap payload.
func DecodeKCBC(src []byte, width, height uint32, bytesPerPixel int) ([]byte, error) {
	return kcbc.Decode(src, width, height, bytesPerPixel)
}

// DecodeRLE decodes a CoreUI row-oriented run-length encoded bitmap.
func DecodeRLE(src []byte, width, height uint32, bytesPerPixel int) ([]byte, error) {
	return rle.Decode(src, width, height, bytesPerPixel)
}

// DecodePaletteImage decodes a CoreUI palette-img bitmap payload.
func DecodePaletteImage(src []byte, width, height uint32, pixelFormat string) ([]byte, error) {
	return palette.Decode(src, width, height, pixelFormat)
}

// DecodeDeepmap decodes a legacy CoreUI Deepmap bitmap payload.
func DecodeDeepmap(src []byte, width, height uint32) (Deepmap2Bitmap, error) {
	return deepmap.Decode(src, width, height)
}

// DecodeDeepmap2 decodes a CoreUI Deepmap2 bitmap payload.
func DecodeDeepmap2(src []byte) (Deepmap2Bitmap, error) {
	return deepmap2.Decode(src)
}

// DecodeDeepmap2WithGeometry decodes a CoreUI Deepmap2 payload using the
// complete rendition geometry, including chunked streams.
func DecodeDeepmap2WithGeometry(src []byte, width, height uint16) (Deepmap2Bitmap, error) {
	return deepmap2.DecodeWithGeometry(src, width, height)
}
