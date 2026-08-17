// Package rle decodes CoreUI row-oriented run-length encoded bitmaps.
package rle

import (
	"encoding/binary"
	"fmt"
)

// Decode expands a CoreUI RLE payload into tightly packed pixels.
func Decode(src []byte, width, height uint32, bytesPerPixel int) ([]byte, error) {
	if width == 0 || height == 0 || bytesPerPixel <= 0 {
		return nil, fmt.Errorf("invalid RLE geometry %dx%d at %d bytes per pixel", width, height, bytesPerPixel)
	}
	headerBytes := uint64(12) + uint64(height)*4
	if headerBytes > uint64(len(src)) {
		return nil, fmt.Errorf("RLE row table is truncated")
	}
	components := binary.LittleEndian.Uint32(src[0:4])
	if components != uint32(bytesPerPixel) {
		return nil, fmt.Errorf("RLE declares %d-byte pixels, expected %d", components, bytesPerPixel)
	}
	if encodedWidth, encodedHeight := binary.LittleEndian.Uint32(src[4:8]), binary.LittleEndian.Uint32(src[8:12]); encodedWidth != width || encodedHeight != height {
		return nil, fmt.Errorf("RLE geometry %dx%d differs from CSI geometry %dx%d", encodedWidth, encodedHeight, width, height)
	}
	pixelBytes := uint64(width) * uint64(height) * uint64(bytesPerPixel)
	if pixelBytes > uint64(maxInt()) {
		return nil, fmt.Errorf("RLE bitmap is too large")
	}
	pixels := make([]byte, int(pixelBytes))
	previousOffset := uint32(headerBytes)
	for row := uint32(0); row < height; row++ {
		start := binary.LittleEndian.Uint32(src[12+row*4 : 16+row*4])
		end := uint32(len(src))
		if row+1 < height {
			end = binary.LittleEndian.Uint32(src[16+row*4 : 20+row*4])
		}
		if start < uint32(headerBytes) || start < previousOffset || end < start || uint64(end) > uint64(len(src)) {
			return nil, fmt.Errorf("RLE row %d has invalid byte range %d...%d", row, start, end)
		}
		previousOffset = start
		source := int(start)
		rowEnd := int(end)
		written := uint32(0)
		rowStart := int(row*width) * bytesPerPixel
		for source < rowEnd {
			if rowEnd-source < 4 {
				return nil, fmt.Errorf("RLE row %d packet header is truncated", row)
			}
			control := binary.LittleEndian.Uint32(src[source : source+4])
			source += 4
			packetType := byte(control >> 24)
			if packetType != 0 && packetType != 0x80 {
				return nil, fmt.Errorf("RLE row %d has unsupported packet type %#x", row, packetType)
			}
			count := control & 0x00ffffff
			if count == 0 || count > width-written {
				return nil, fmt.Errorf("RLE row %d packet writes %d pixels after %d of %d", row, count, written, width)
			}
			dest := rowStart + int(written)*bytesPerPixel
			if packetType == 0x80 {
				if rowEnd-source < bytesPerPixel {
					return nil, fmt.Errorf("RLE row %d repeat pixel is truncated", row)
				}
				pixel := src[source : source+bytesPerPixel]
				source += bytesPerPixel
				for i := uint32(0); i < count; i++ {
					copy(pixels[dest+int(i)*bytesPerPixel:], pixel)
				}
			} else {
				bytes := uint64(count) * uint64(bytesPerPixel)
				if bytes > uint64(rowEnd-source) {
					return nil, fmt.Errorf("RLE row %d literal packet is truncated", row)
				}
				copy(pixels[dest:dest+int(bytes)], src[source:source+int(bytes)])
				source += int(bytes)
			}
			written += count
		}
	}
	return pixels, nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
