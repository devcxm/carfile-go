// Package deepmap decodes the legacy CoreUI deepmap-lzfse container.
package deepmap

import (
	"encoding/binary"
	"fmt"

	"github.com/devcxm/carfile-go/codec/deepmap2"
	"github.com/devcxm/carfile-go/codec/lzfse"
	"github.com/devcxm/carfile-go/codec/lzvn"
)

// Decode expands a KCBC sequence containing legacy dmap payloads. The legacy
// inner bitmap format is converted losslessly to its dmp2 equivalent and then
// decoded by the shared Deepmap implementation.
func Decode(src []byte, width, height uint32) (deepmap2.Bitmap, error) {
	var result deepmap2.Bitmap
	if width == 0 || height == 0 || width > 0xffff || height > 0xffff {
		return result, fmt.Errorf("invalid legacy deepmap geometry %dx%d", width, height)
	}
	rows := uint32(0)
	for offset, chunk := 0, 0; rows < height; chunk++ {
		if len(src)-offset < 20 {
			return result, fmt.Errorf("legacy deepmap KCBC chunk %d header is truncated", chunk)
		}
		if string(src[offset:offset+4]) != "KCBC" {
			return result, fmt.Errorf("legacy deepmap KCBC chunk %d has magic %q", chunk, src[offset:offset+4])
		}
		chunkRows := binary.LittleEndian.Uint32(src[offset+12 : offset+16])
		chunkBytes := binary.LittleEndian.Uint32(src[offset+16 : offset+20])
		if chunkRows == 0 || chunkRows > height-rows {
			return result, fmt.Errorf("legacy deepmap KCBC chunk %d has invalid height %d", chunk, chunkRows)
		}
		start := offset + 20
		end := uint64(start) + uint64(chunkBytes)
		if end > uint64(len(src)) {
			return result, fmt.Errorf("legacy deepmap KCBC chunk %d needs %d bytes", chunk, chunkBytes)
		}
		legacy := src[start:int(end)]
		bitmap, native, nativeErr := decodeNativeChunk(legacy[16:], uint16(width), uint16(chunkRows))
		if !native || nativeErr != nil {
			portable, err := decodePortableChunk(legacy, uint16(width), uint16(chunkRows))
			if err != nil {
				if nativeErr != nil {
					return result, fmt.Errorf("legacy deepmap KCBC chunk %d: native decode: %v; portable decode: %w", chunk, nativeErr, err)
				}
				return result, fmt.Errorf("legacy deepmap KCBC chunk %d: %w", chunk, err)
			}
			bitmap = portable
		}
		if uint32(bitmap.Width) != width || uint32(bitmap.Height) != chunkRows {
			return result, fmt.Errorf("legacy deepmap KCBC chunk %d decoded as %dx%d", chunk, bitmap.Width, bitmap.Height)
		}
		if chunk == 0 {
			result = bitmap
		} else {
			if bitmap.PixelFormat != result.PixelFormat || bitmap.Components != result.Components || bitmap.BytesPerComponent != result.BytesPerComponent {
				return result, fmt.Errorf("legacy deepmap KCBC chunk %d changes pixel format", chunk)
			}
			result.Pixels = append(result.Pixels, bitmap.Pixels...)
		}
		rows += chunkRows
		offset = int(end)
		if rows == height && offset != len(src) {
			return result, fmt.Errorf("legacy deepmap has %d trailing bytes", len(src)-offset)
		}
	}
	result.Height = uint16(height)
	return result, nil
}

func decodePortableChunk(src []byte, width, height uint16) (deepmap2.Bitmap, error) {
	var result deepmap2.Bitmap
	if len(src) < 28 {
		return result, fmt.Errorf("dmap container is truncated: have %d bytes", len(src))
	}
	declared := binary.LittleEndian.Uint64(src[8:16])
	if declared != uint64(len(src)-16) {
		return result, fmt.Errorf("dmap container declares %d bytes, has %d", declared, len(src)-16)
	}
	if string(src[16:20]) != "dmap" {
		return result, fmt.Errorf("invalid dmap magic %q", src[16:20])
	}
	pixelFormat := src[23]
	components, bytesPerComponent := 0, 1
	switch pixelFormat {
	case 1:
		components = 1
	case 2:
		components = 2
	case 3:
		components = 3
	case 4:
		components = 4
	case 20:
		components, bytesPerComponent = 4, 2
	default:
		return result, fmt.Errorf("unsupported dmap pixel format %d", pixelFormat)
	}
	pixelBytes64 := uint64(width) * uint64(height) * uint64(components) * uint64(bytesPerComponent)
	if pixelBytes64 == 0 || pixelBytes64 > uint64(portableMaxInt()) {
		return result, fmt.Errorf("legacy deepmap bitmap is too large")
	}
	pixelBytes := int(pixelBytes64)
	method := src[20]
	switch method {
	case 1:
		pixels := src[28:]
		if len(pixels) != pixelBytes {
			return result, fmt.Errorf("dmap raw data has %d bytes, expected %d", len(pixels), pixelBytes)
		}
		return deepmap2.Bitmap{
			Width: width, Height: height, PixelFormat: pixelFormat,
			Components: uint8(components), BytesPerComponent: uint8(bytesPerComponent),
			Method: method, Pixels: append([]byte(nil), pixels...),
		}, nil

	case 2:
		residualComponents := components
		hasAlpha := pixelFormat == 2 || pixelFormat == 4 || pixelFormat == 20
		if hasAlpha {
			residualComponents--
		}
		limit64 := uint64(0)
		for y := 0; y < int(height); y += 256 {
			tileHeight := minInt(256, int(height)-y)
			for x := 0; x < int(width); x += 256 {
				tileWidth := minInt(256, int(width)-x)
				pixelCount := tileWidth * tileHeight
				needed := tileHeight + 2*residualComponents*pixelCount
				if hasAlpha {
					needed += pixelCount
				}
				limit64 += uint64((needed + 7) &^ 7)
			}
		}
		if limit64 > uint64(portableMaxInt()) {
			return result, fmt.Errorf("legacy deepmap default buffer is too large")
		}
		segments, err := decodeLegacySegments(src[28:], binary.LittleEndian.Uint32(src[24:28]), int(limit64))
		if err != nil {
			return result, fmt.Errorf("dmap default: %w", err)
		}
		pixels := make([]byte, pixelBytes)
		segment := 0
		for y := 0; y < int(height); y += 256 {
			tileHeight := minInt(256, int(height)-y)
			for x := 0; x < int(width); x += 256 {
				if segment == len(segments) {
					return result, fmt.Errorf("dmap default is missing tile at %d,%d", x, y)
				}
				tileWidth := minInt(256, int(width)-x)
				pixelCount := tileWidth * tileHeight
				needed := tileHeight + 2*residualComponents*pixelCount
				if hasAlpha {
					needed += pixelCount
				}
				expected := (needed + 7) &^ 7
				if len(segments[segment]) != expected {
					return result, fmt.Errorf("dmap default tile at %d,%d decodes %d bytes, expected %d", x, y, len(segments[segment]), expected)
				}
				tile, err := deepmap2.DecodeDefaultData(segments[segment], uint16(tileWidth), uint16(tileHeight), pixelFormat)
				if err != nil {
					return result, fmt.Errorf("dmap default tile at %d,%d: %w", x, y, err)
				}
				stitchLegacyTile(pixels, tile.Pixels, int(width), tileWidth, tileHeight, x, y, components*bytesPerComponent)
				segment++
			}
		}
		if segment != len(segments) {
			return result, fmt.Errorf("dmap default has %d unused tiles", len(segments)-segment)
		}
		return deepmap2.Bitmap{
			Width: width, Height: height, PixelFormat: pixelFormat,
			Components: uint8(components), BytesPerComponent: uint8(bytesPerComponent),
			Method: method, Pixels: pixels,
		}, nil

	case 3:
		segments, err := decodeLegacySegments(src[28:], binary.LittleEndian.Uint32(src[24:28]), pixelBytes)
		if err != nil {
			return result, fmt.Errorf("dmap lossless: %w", err)
		}
		pixels := make([]byte, pixelBytes)
		segment := 0
		for y := 0; y < int(height); y += 256 {
			tileHeight := minInt(256, int(height)-y)
			for x := 0; x < int(width); x += 256 {
				if segment == len(segments) {
					return result, fmt.Errorf("dmap lossless is missing tile at %d,%d", x, y)
				}
				tileWidth := minInt(256, int(width)-x)
				expected := tileWidth * tileHeight * components * bytesPerComponent
				if len(segments[segment]) != expected {
					return result, fmt.Errorf("dmap lossless tile at %d,%d decodes %d bytes, expected %d", x, y, len(segments[segment]), expected)
				}
				stitchLegacyTile(pixels, segments[segment], int(width), tileWidth, tileHeight, x, y, components*bytesPerComponent)
				segment++
			}
		}
		if segment != len(segments) {
			return result, fmt.Errorf("dmap lossless has %d unused tiles", len(segments)-segment)
		}
		return deepmap2.Bitmap{
			Width: width, Height: height, PixelFormat: pixelFormat,
			Components: uint8(components), BytesPerComponent: uint8(bytesPerComponent),
			Method: method, Pixels: pixels,
		}, nil

	case 4:
		descriptor := binary.LittleEndian.Uint32(src[24:28])
		paletteCount := int(descriptor & 0xffff)
		paletteComponents := int(descriptor >> 16)
		if paletteCount == 0 || paletteCount > 256 {
			return result, fmt.Errorf("invalid dmap palette size %d", paletteCount)
		}
		if paletteComponents == 0 {
			paletteComponents = components
			if pixelFormat == 2 || pixelFormat == 4 || pixelFormat == 20 {
				paletteComponents--
			}
		}
		paletteBytes := paletteCount * components * bytesPerComponent
		payload := src[28:]
		if len(payload) < paletteBytes+4 {
			return result, fmt.Errorf("dmap palette needs %d bytes, has %d", paletteBytes+4, len(payload))
		}
		palette := payload[:paletteBytes]
		compressed := payload[paletteBytes:]
		separateComponents := components - paletteComponents
		if separateComponents < 0 {
			return result, fmt.Errorf("dmap palette has %d components, pixel format has %d", paletteComponents, components)
		}
		decodedBytesPerPixel := 1 + separateComponents*bytesPerComponent
		segments, err := decodeLegacySegments(compressed[4:], binary.LittleEndian.Uint32(compressed[:4]), int(width)*int(height)*decodedBytesPerPixel)
		if err != nil {
			return result, fmt.Errorf("dmap palette: %w", err)
		}
		pixels := make([]byte, pixelBytes)
		segment := 0
		for y := 0; y < int(height); y += 256 {
			tileHeight := minInt(256, int(height)-y)
			for x := 0; x < int(width); x += 256 {
				if segment == len(segments) {
					return result, fmt.Errorf("dmap palette is missing tile at %d,%d", x, y)
				}
				tileWidth := minInt(256, int(width)-x)
				expected := tileWidth * tileHeight * decodedBytesPerPixel
				if len(segments[segment]) != expected {
					return result, fmt.Errorf("dmap palette tile at %d,%d decodes %d bytes, expected %d", x, y, len(segments[segment]), expected)
				}
				tile, err := deepmap2.DecodePaletteData(palette, segments[segment], uint16(tileWidth), uint16(tileHeight), pixelFormat, paletteComponents)
				if err != nil {
					return result, fmt.Errorf("dmap palette tile at %d,%d: %w", x, y, err)
				}
				stitchLegacyTile(pixels, tile.Pixels, int(width), tileWidth, tileHeight, x, y, components*bytesPerComponent)
				segment++
			}
		}
		if segment != len(segments) {
			return result, fmt.Errorf("dmap palette has %d unused tiles", len(segments)-segment)
		}
		return deepmap2.Bitmap{
			Width: width, Height: height, PixelFormat: pixelFormat,
			Components: uint8(components), BytesPerComponent: uint8(bytesPerComponent),
			Method: method, Pixels: pixels,
		}, nil

	default:
		return result, fmt.Errorf("unsupported dmap encoding method %d", method)
	}
}

func decodeLegacySegments(src []byte, firstLength uint32, outputLimit int) ([][]byte, error) {
	remaining := src
	compressedBytes := firstLength
	var segments [][]byte
	decodedBytes := 0
	for segment := 0; ; segment++ {
		if compressedBytes == 0 || uint64(compressedBytes) > uint64(len(remaining)) {
			return nil, fmt.Errorf("segment %d needs %d bytes, has %d", segment, compressedBytes, len(remaining))
		}
		encoded := remaining[:compressedBytes]
		var decoded []byte
		var err error
		if len(encoded) >= 4 && (string(encoded[:4]) == "bvx2" || string(encoded[:4]) == "bvx-" || string(encoded[:4]) == "bvxn" || string(encoded[:4]) == "bvx1") {
			decoded, err = lzfse.Decode(encoded)
		} else {
			decoded, err = lzvn.DecodeUpTo(encoded, outputLimit-decodedBytes)
		}
		if err != nil {
			return nil, fmt.Errorf("segment %d: %w", segment, err)
		}
		if decodedBytes > outputLimit-len(decoded) {
			return nil, fmt.Errorf("segments exceed output limit %d", outputLimit)
		}
		segments = append(segments, decoded)
		decodedBytes += len(decoded)
		remaining = remaining[compressedBytes:]
		if len(remaining) == 0 {
			return segments, nil
		}
		if len(remaining) < 4 {
			return nil, fmt.Errorf("segment length is truncated")
		}
		compressedBytes = binary.LittleEndian.Uint32(remaining[:4])
		remaining = remaining[4:]
	}
}

func stitchLegacyTile(dst, tile []byte, width, tileWidth, tileHeight, x, y, bytesPerPixel int) {
	dstRowBytes := width * bytesPerPixel
	tileRowBytes := tileWidth * bytesPerPixel
	for row := 0; row < tileHeight; row++ {
		dstStart := (y+row)*dstRowBytes + x*bytesPerPixel
		tileStart := row * tileRowBytes
		copy(dst[dstStart:dstStart+tileRowBytes], tile[tileStart:tileStart+tileRowBytes])
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func portableMaxInt() int {
	return int(^uint(0) >> 1)
}
