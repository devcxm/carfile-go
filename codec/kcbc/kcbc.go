// Package kcbc decodes CoreUI's chunked bitmap container.
package kcbc

import (
	"encoding/binary"
	"fmt"

	"github.com/devcxm/carfile-go/codec/lzfse"
)

// Decode expands a sequence of independently compressed horizontal image
// slices. The returned pixels are tightly packed; alignment bytes at the end
// of source rows are discarded.
func Decode(src []byte, width, height uint32, bytesPerPixel int) ([]byte, error) {
	if width == 0 || height == 0 || bytesPerPixel <= 0 {
		return nil, fmt.Errorf("invalid bitmap geometry %dx%d at %d bytes per pixel", width, height, bytesPerPixel)
	}
	tightRowBytes64 := uint64(width) * uint64(bytesPerPixel)
	totalBytes64 := tightRowBytes64 * uint64(height)
	if tightRowBytes64 > uint64(maxInt()) || totalBytes64 > uint64(maxInt()) {
		return nil, fmt.Errorf("bitmap geometry is too large")
	}
	tightRowBytes := int(tightRowBytes64)
	dst := make([]byte, 0, int(totalBytes64))
	offset := 0
	rows := uint32(0)
	for rows < height {
		if len(src)-offset < 20 {
			return nil, fmt.Errorf("KCBC chunk %d header is truncated", rows)
		}
		if string(src[offset:offset+4]) != "KCBC" {
			return nil, fmt.Errorf("KCBC chunk at offset %d has magic %q", offset, src[offset:offset+4])
		}
		chunkHeight := binary.LittleEndian.Uint32(src[offset+12 : offset+16])
		compressedBytes := binary.LittleEndian.Uint32(src[offset+16 : offset+20])
		if chunkHeight == 0 || chunkHeight > height-rows {
			return nil, fmt.Errorf("KCBC chunk at offset %d has invalid height %d", offset, chunkHeight)
		}
		payloadStart := uint64(offset + 20)
		payloadEnd := payloadStart + uint64(compressedBytes)
		if payloadEnd > uint64(len(src)) {
			return nil, fmt.Errorf("KCBC chunk at offset %d needs %d compressed bytes", offset, compressedBytes)
		}
		decoded, err := lzfse.Decode(src[int(payloadStart):int(payloadEnd)])
		if err != nil {
			return nil, fmt.Errorf("KCBC chunk at row %d: %w", rows, err)
		}
		if len(decoded)%int(chunkHeight) != 0 {
			return nil, fmt.Errorf("KCBC chunk at row %d expands to %d bytes for %d rows", rows, len(decoded), chunkHeight)
		}
		sourceRowBytes := len(decoded) / int(chunkHeight)
		if sourceRowBytes < tightRowBytes {
			return nil, fmt.Errorf("KCBC chunk at row %d has %d-byte rows, need at least %d", rows, sourceRowBytes, tightRowBytes)
		}
		for row := 0; row < int(chunkHeight); row++ {
			start := row * sourceRowBytes
			dst = append(dst, decoded[start:start+tightRowBytes]...)
		}
		rows += chunkHeight
		offset = int(payloadEnd)
	}
	if offset != len(src) {
		return nil, fmt.Errorf("KCBC payload has %d trailing bytes", len(src)-offset)
	}
	return dst, nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
