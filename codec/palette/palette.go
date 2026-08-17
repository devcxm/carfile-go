// Package palette decodes CoreUI palette-img bitmap payloads.
package palette

import (
	"encoding/binary"
	"fmt"
	"math/bits"

	"github.com/devcxm/carfile-go/codec/lzfse"
)

const magic = 0xcafef00d

// Decode expands an LZFSE-compressed CoreUI quantized image into tightly
// packed pixels. ARGB palette entries are converted from stored ARGB order to
// CoreUI's BGRA byte layout. RGBW pixels use four little-endian uint16 values
// whose nominal range is 0...10000.
func Decode(src []byte, width, height uint32, pixelFormat string) ([]byte, error) {
	expanded, err := lzfse.Decode(src)
	if err != nil {
		return nil, fmt.Errorf("palette-img LZFSE: %w", err)
	}
	return decodeExpanded(expanded, width, height, pixelFormat)
}

func decodeExpanded(src []byte, width, height uint32, pixelFormat string) ([]byte, error) {
	if width == 0 || height == 0 {
		return nil, fmt.Errorf("invalid palette-img geometry %dx%d", width, height)
	}
	if len(src) < 10 {
		return nil, fmt.Errorf("palette-img header is truncated: have %d bytes", len(src))
	}
	if binary.LittleEndian.Uint32(src[0:4]) != magic {
		return nil, fmt.Errorf("invalid palette-img magic %#x", binary.LittleEndian.Uint32(src[0:4]))
	}
	if version := binary.LittleEndian.Uint32(src[4:8]); version >= 2 {
		return nil, fmt.Errorf("unsupported palette-img version %d", version)
	}
	colorCount := int(binary.LittleEndian.Uint16(src[8:10]))
	if colorCount == 0 || colorCount > 4096 {
		return nil, fmt.Errorf("invalid palette-img color count %d", colorCount)
	}
	bytesPerPixel := 0
	switch pixelFormat {
	case "ARGB":
		bytesPerPixel = 4
	case "RGBW":
		bytesPerPixel = 8
	default:
		return nil, fmt.Errorf("palette-img does not support pixel format %q", pixelFormat)
	}
	paletteBytes := colorCount * bytesPerPixel
	if len(src) < 10+paletteBytes {
		return nil, fmt.Errorf("palette-img palette needs %d bytes, has %d", paletteBytes, len(src)-10)
	}
	palette := src[10 : 10+paletteBytes]
	indexBits := bits.Len(uint(colorCount - 1))
	rounded := 1
	for rounded < indexBits {
		rounded <<= 1
	}
	indexBits = rounded
	rowBits := uint64(width) * uint64(indexBits)
	rowBytes := int((rowBits + 15) / 16 * 2)
	indexBytes := uint64(rowBytes) * uint64(height)
	indexStart := 10 + paletteBytes
	if indexBytes > uint64(len(src)-indexStart) {
		return nil, fmt.Errorf("palette-img indices need %d bytes, have %d", indexBytes, len(src)-indexStart)
	}
	if int(indexBytes) != len(src)-indexStart {
		return nil, fmt.Errorf("palette-img has %d trailing decoded bytes", len(src)-indexStart-int(indexBytes))
	}
	pixelBytes := uint64(width) * uint64(height) * uint64(bytesPerPixel)
	if pixelBytes > uint64(maxInt()) {
		return nil, fmt.Errorf("palette-img bitmap is too large")
	}
	pixels := make([]byte, int(pixelBytes))
	indices := src[indexStart:]
	for row := 0; row < int(height); row++ {
		rowData := indices[row*rowBytes : (row+1)*rowBytes]
		bitOffset := 0
		for column := 0; column < int(width); column++ {
			wordOffset := bitOffset / 16 * 2
			shift := 16 - indexBits - bitOffset%16
			if shift < 0 || wordOffset+2 > len(rowData) {
				return nil, fmt.Errorf("palette-img index crosses a 16-bit row word")
			}
			word := binary.LittleEndian.Uint16(rowData[wordOffset : wordOffset+2])
			index := int(word>>shift) & ((1 << indexBits) - 1)
			if index >= colorCount {
				return nil, fmt.Errorf("palette-img index %d at (%d,%d) exceeds %d colors", index, column, row, colorCount)
			}
			dest := (row*int(width) + column) * bytesPerPixel
			entry := palette[index*bytesPerPixel : (index+1)*bytesPerPixel]
			if pixelFormat == "ARGB" {
				pixels[dest+0] = entry[3]
				pixels[dest+1] = entry[2]
				pixels[dest+2] = entry[1]
				pixels[dest+3] = entry[0]
			} else {
				copy(pixels[dest:dest+bytesPerPixel], entry)
			}
			bitOffset += indexBits
		}
	}
	return pixels, nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
