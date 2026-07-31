package carfile

import (
	"carfile-go/codec/deepmap2"
	"carfile-go/codec/kcbc"
	"carfile-go/codec/lzfse"
	"carfile-go/codec/lzvn"
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

// DecodeDeepmap2 decodes a CoreUI Deepmap2 bitmap payload.
func DecodeDeepmap2(src []byte) (Deepmap2Bitmap, error) {
	return deepmap2.Decode(src)
}
