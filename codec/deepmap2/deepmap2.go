// Package deepmap2 decodes CoreUI Deepmap2 bitmap payloads.
package deepmap2

import (
	"encoding/binary"
	"fmt"

	"github.com/devcxm/carfile-go/codec/lzfse"
	"github.com/devcxm/carfile-go/codec/lzvn"
)

const (
	deepmap2Default  = 2
	deepmap2Lossless = 3
	deepmap2Palette  = 4
)

// Bitmap is the decoded, tightly packed pixel buffer stored in a dmp2
// payload. Components is normally 4 (BGRA) or 2 (gray and alpha).
type Bitmap struct {
	Width             uint16
	Height            uint16
	PixelFormat       uint8
	Components        uint8
	BytesPerComponent uint8
	Method            uint8
	Pixels            []byte
}

// DecodeDefaultData reconstructs pixels from an already-decompressed default
// Deepmap tile. Legacy Deepmap uses the same predictor transform as dmp2.
func DecodeDefaultData(src []byte, width, height uint16, pixelFormat uint8) (Bitmap, error) {
	var result Bitmap
	layout, err := layoutForPixelFormat(pixelFormat)
	if err != nil {
		return result, err
	}
	pixelCount := int(width) * int(height)
	residualComponents := layout.components
	alphaBytes := 0
	if layout.hasAlpha {
		residualComponents--
		alphaBytes = pixelCount
	}
	needed := alphaBytes + int(height) + 2*residualComponents*pixelCount
	if len(src) < needed {
		return result, fmt.Errorf("default Deepmap data has %d bytes, needs %d", len(src), needed)
	}
	pixels, err := decodeDeepmap2DefaultPixels(src[:needed], int(width), int(height), layout)
	if err != nil {
		return result, err
	}
	return Bitmap{
		Width: width, Height: height, PixelFormat: pixelFormat,
		Components: uint8(layout.components), BytesPerComponent: uint8(layout.bytesPerComponent),
		Method: deepmap2Default, Pixels: pixels,
	}, nil
}

// DecodePaletteData expands an already-decompressed palette tile. Palette
// entries use the full pixel stride; components after paletteComponents are
// stored as separate planes before the one-byte palette indices.
func DecodePaletteData(palette, planesAndIndices []byte, width, height uint16, pixelFormat uint8, paletteComponents int) (Bitmap, error) {
	var result Bitmap
	layout, err := layoutForPixelFormat(pixelFormat)
	if err != nil {
		return result, err
	}
	paletteStride := layout.bytesPerPixel()
	if len(palette) == 0 || len(palette)%paletteStride != 0 {
		return result, fmt.Errorf("dmp2 palette has %d bytes for stride %d", len(palette), paletteStride)
	}
	paletteCount := len(palette) / paletteStride
	if paletteCount > 256 {
		return result, fmt.Errorf("invalid dmp2 palette size %d", paletteCount)
	}
	if paletteComponents <= 0 || paletteComponents > layout.components {
		return result, fmt.Errorf("dmp2 palette has %d components, pixel format %d has %d", paletteComponents, pixelFormat, layout.components)
	}
	pixelCount := int(width) * int(height)
	separateComponents := layout.components - paletteComponents
	planeBytes := pixelCount * separateComponents * layout.bytesPerComponent
	expected := planeBytes + pixelCount
	if len(planesAndIndices) != expected {
		return result, fmt.Errorf("decoded %d palette plane bytes, expected %d", len(planesAndIndices), expected)
	}
	indices := planesAndIndices[planeBytes:]
	pixels := make([]byte, pixelCount*paletteStride)
	for i, index := range indices {
		if int(index) >= paletteCount {
			return result, fmt.Errorf("dmp2 palette index %d at pixel %d exceeds %d colors", index, i, paletteCount)
		}
		copy(pixels[i*paletteStride:(i+1)*paletteStride], palette[int(index)*paletteStride:(int(index)+1)*paletteStride])
		for component := 0; component < separateComponents; component++ {
			source := component*pixelCount*layout.bytesPerComponent + i*layout.bytesPerComponent
			destComponent := paletteComponents + component
			dest := i*paletteStride + destComponent*layout.bytesPerComponent
			copy(pixels[dest:dest+layout.bytesPerComponent], planesAndIndices[source:source+layout.bytesPerComponent])
		}
	}
	return Bitmap{
		Width: width, Height: height, PixelFormat: pixelFormat,
		Components: uint8(layout.components), BytesPerComponent: uint8(layout.bytesPerComponent),
		Method: deepmap2Palette, Pixels: pixels,
	}, nil
}

// Decode decodes the data after a CELM wrapper. It supports the default,
// lossless, and palette encodings emitted by current versions of actool.
func Decode(src []byte) (Bitmap, error) {
	return decodePortable(src)
}

func decodePortable(src []byte) (Bitmap, error) {
	var result Bitmap
	if len(src) >= 4 && string(src[:4]) == "KCBC" {
		return decodeKCBCDeepmap2(src)
	}
	if len(src) < 32 {
		return result, fmt.Errorf("Deepmap2 payload is truncated: have %d bytes", len(src))
	}
	outerVersion := binary.LittleEndian.Uint32(src[0:4])
	outerEncoding := binary.LittleEndian.Uint32(src[4:8])
	outerLength := binary.LittleEndian.Uint64(src[8:16])
	if outerVersion != 1 {
		return result, fmt.Errorf("unsupported Deepmap2 container version %d", outerVersion)
	}
	if outerLength > uint64(len(src)-16) {
		return result, fmt.Errorf("Deepmap2 container declares %d bytes, has %d", outerLength, len(src)-16)
	}
	containerEnd := 16 + int(outerLength)
	if string(src[16:20]) != "dmp2" {
		return result, fmt.Errorf("invalid Deepmap2 magic %q", src[16:20])
	}
	result.Method = src[20]
	blobVersion := src[21]
	result.PixelFormat = src[23]
	layout, err := layoutForPixelFormat(result.PixelFormat)
	if err != nil {
		return result, err
	}
	result.Components = uint8(layout.components)
	result.BytesPerComponent = uint8(layout.bytesPerComponent)
	result.Width = binary.LittleEndian.Uint16(src[24:26])
	result.Height = binary.LittleEndian.Uint16(src[26:28])
	if blobVersion != 1 {
		return result, fmt.Errorf("unsupported dmp2 blob version %d", blobVersion)
	}
	if result.Width == 0 || result.Height == 0 {
		return result, fmt.Errorf("invalid dmp2 geometry %dx%d", result.Width, result.Height)
	}
	pixelBytes := uint64(result.Width) * uint64(result.Height) * uint64(layout.bytesPerPixel())
	if pixelBytes > uint64(maxInt()) {
		return result, fmt.Errorf("dmp2 bitmap is too large")
	}
	descriptor := binary.LittleEndian.Uint32(src[28:32])
	payload := src[32:containerEnd]

	switch result.Method {
	case deepmap2Default:
		pixels, totalRows, err := decodeDefaultSegments(payload, descriptor, int(result.Width), layout, int(result.Height))
		if err != nil {
			return result, err
		}
		if totalRows > 0xffff {
			return result, fmt.Errorf("dmp2 default height %d exceeds format limit", totalRows)
		}
		result.Height = uint16(totalRows)
		result.Pixels = pixels
		return result, nil

	case deepmap2Lossless:
		result.Pixels, err = decodeCompressedSegments(payload, descriptor, "lossless", int(pixelBytes))
		if err != nil {
			return result, err
		}
		rowBytes := uint64(result.Width) * uint64(layout.bytesPerPixel())
		if uint64(len(result.Pixels))%rowBytes != 0 {
			return result, fmt.Errorf("dmp2 lossless expands to a partial row")
		}
		totalRows := uint64(len(result.Pixels)) / rowBytes
		if totalRows == 0 || totalRows > 0xffff {
			return result, fmt.Errorf("dmp2 lossless height %d is invalid", totalRows)
		}
		result.Height = uint16(totalRows)
		return result, nil

	case deepmap2Palette:
		paletteCount := int(descriptor & 0xffff)
		paletteComponents := int(descriptor >> 16)
		if paletteCount == 0 || paletteCount > 256 {
			return result, fmt.Errorf("invalid dmp2 palette size %d", paletteCount)
		}
		if paletteComponents == 0 || paletteComponents > layout.components {
			return result, fmt.Errorf("dmp2 palette has %d components, pixel format %d has %d", paletteComponents, result.PixelFormat, layout.components)
		}
		paletteStride := layout.bytesPerPixel()
		paletteBytes := paletteCount * paletteStride
		if paletteBytes+4 > len(payload) {
			return result, fmt.Errorf("dmp2 palette and index header need %d bytes, have %d", paletteBytes+4, len(payload))
		}
		encodedBytes := binary.LittleEndian.Uint32(payload[paletteBytes : paletteBytes+4])
		encodedStart := paletteBytes + 4
		encodedEnd := uint64(encodedStart) + uint64(encodedBytes)
		if encodedEnd > uint64(len(payload)) {
			return result, fmt.Errorf("dmp2 palette index stream needs %d bytes, has %d", encodedBytes, len(payload)-encodedStart)
		}
		pixelCount := int(result.Width) * int(result.Height)
		separateComponents := layout.components - paletteComponents
		planeBytes := pixelCount * separateComponents * layout.bytesPerComponent
		decodedBytes := planeBytes + pixelCount
		encodedIndices := payload[encodedStart:int(encodedEnd)]
		var planesAndIndices []byte
		if len(encodedIndices) >= 4 && (string(encodedIndices[:4]) == "bvx2" || string(encodedIndices[:4]) == "bvx-" || string(encodedIndices[:4]) == "bvxn") {
			planesAndIndices, err = lzfse.Decode(encodedIndices)
			if err == nil && len(planesAndIndices) != decodedBytes {
				err = fmt.Errorf("decoded %d palette plane bytes, expected %d", len(planesAndIndices), decodedBytes)
			}
		} else {
			planesAndIndices, err = lzvn.Decode(encodedIndices, decodedBytes)
		}
		if err != nil {
			return result, fmt.Errorf("dmp2 palette indices: %w", err)
		}
		return DecodePaletteData(payload[:paletteBytes], planesAndIndices, result.Width, result.Height, result.PixelFormat, paletteComponents)

	default:
		return result, fmt.Errorf("unsupported dmp2 encoding method %d (outer encoding %d)", result.Method, outerEncoding)
	}
}

// DecodeWithGeometry decodes a Deepmap2 payload. The geometry parameters are
// retained for API compatibility; the portable decoder derives the effective
// dimensions from the payload and its decoded segments.
func DecodeWithGeometry(src []byte, width, height uint16) (Bitmap, error) {
	return Decode(src)
}

type pixelLayout struct {
	components        int
	bytesPerComponent int
	hasAlpha          bool
}

func (l pixelLayout) bytesPerPixel() int {
	return l.components * l.bytesPerComponent
}

func layoutForPixelFormat(format uint8) (pixelLayout, error) {
	switch format {
	case 1:
		return pixelLayout{components: 1, bytesPerComponent: 1}, nil
	case 2:
		return pixelLayout{components: 2, bytesPerComponent: 1, hasAlpha: true}, nil
	case 3:
		return pixelLayout{components: 3, bytesPerComponent: 1}, nil
	case 4:
		return pixelLayout{components: 4, bytesPerComponent: 1, hasAlpha: true}, nil
	case 20:
		return pixelLayout{components: 4, bytesPerComponent: 2, hasAlpha: true}, nil
	default:
		return pixelLayout{}, fmt.Errorf("unsupported dmp2 pixel format %d", format)
	}
}

func decodeCompressedSegments(src []byte, firstLength uint32, method string, expectedBytes int) ([]byte, error) {
	remaining := src
	compressedBytes := firstLength
	var decodedAll []byte
	for segment := 0; ; segment++ {
		if compressedBytes == 0 || uint64(compressedBytes) > uint64(len(remaining)) {
			return nil, fmt.Errorf("dmp2 %s segment %d needs %d bytes, has %d", method, segment, compressedBytes, len(remaining))
		}
		encoded := remaining[:compressedBytes]
		decoded, err := decodeCompressedSegment(encoded, expectedBytes-len(decodedAll))
		if err != nil {
			return nil, fmt.Errorf("dmp2 %s segment %d: %w", method, segment, err)
		}
		decodedAll = append(decodedAll, decoded...)
		remaining = remaining[compressedBytes:]
		if len(remaining) == 0 {
			return decodedAll, nil
		}
		if len(remaining) < 4 {
			return nil, fmt.Errorf("dmp2 %s segment length is truncated", method)
		}
		compressedBytes = binary.LittleEndian.Uint32(remaining[:4])
		remaining = remaining[4:]
	}
}

func decodeCompressedSegment(src []byte, expectedBytes int) ([]byte, error) {
	if len(src) >= 4 && (string(src[:4]) == "bvx2" || string(src[:4]) == "bvx-" || string(src[:4]) == "bvxn" || string(src[:4]) == "bvx1") {
		return lzfse.Decode(src)
	}
	if expectedBytes <= 0 {
		return nil, fmt.Errorf("raw LZVN segment has no output-size hint")
	}
	return lzvn.DecodeUpTo(src, expectedBytes)
}

func decodeDefaultSegments(src []byte, firstLength uint32, width int, layout pixelLayout, firstHeight int) ([]byte, int, error) {
	remaining := src
	compressedBytes := firstLength
	var pixels []byte
	totalRows := 0
	for segment := 0; ; segment++ {
		if compressedBytes == 0 || uint64(compressedBytes) > uint64(len(remaining)) {
			return nil, 0, fmt.Errorf("dmp2 default segment %d needs %d bytes, has %d", segment, compressedBytes, len(remaining))
		}
		encoded := remaining[:compressedBytes]
		expected := defaultEncodedBytes(width, firstHeight, layout)
		decoded, err := decodeCompressedSegment(encoded, expected)
		if err != nil {
			return nil, 0, fmt.Errorf("dmp2 default segment %d: %w", segment, err)
		}
		rows, err := defaultRowCount(len(decoded), width, layout)
		if err != nil {
			return nil, 0, fmt.Errorf("dmp2 default segment %d: %w", segment, err)
		}
		segmentPixels, err := decodeDeepmap2DefaultPixels(decoded, width, rows, layout)
		if err != nil {
			return nil, 0, fmt.Errorf("dmp2 default segment %d: %w", segment, err)
		}
		pixels = append(pixels, segmentPixels...)
		totalRows += rows
		remaining = remaining[compressedBytes:]
		if len(remaining) == 0 {
			return pixels, totalRows, nil
		}
		if len(remaining) < 4 {
			return nil, 0, fmt.Errorf("dmp2 default segment length is truncated")
		}
		compressedBytes = binary.LittleEndian.Uint32(remaining[:4])
		remaining = remaining[4:]
	}
}

func defaultEncodedBytes(width, height int, layout pixelLayout) int {
	residualComponents := layout.components
	alphaBytes := 0
	if layout.hasAlpha {
		residualComponents--
		alphaBytes = width * height
	}
	needed := alphaBytes + height + 2*residualComponents*width*height
	return (needed + 15) &^ 15
}

func defaultRowCount(decodedBytes, width int, layout pixelLayout) (int, error) {
	residualComponents := layout.components
	alphaBytesPerRow := 0
	if layout.hasAlpha {
		residualComponents--
		alphaBytesPerRow = width
	}
	bytesPerRow := alphaBytesPerRow + 1 + 2*residualComponents*width
	rows := decodedBytes / bytesPerRow
	if rows == 0 || decodedBytes-rows*bytesPerRow >= 16 {
		return 0, fmt.Errorf("cannot derive row count from %d decoded bytes", decodedBytes)
	}
	return rows, nil
}

func decodeDeepmap2DefaultPixels(src []byte, width, height int, layout pixelLayout) ([]byte, error) {
	pixelCount := width * height
	residualComponents := layout.components
	alphaBytes := 0
	if layout.hasAlpha {
		residualComponents--
		alphaBytes = pixelCount
	}
	needed := alphaBytes + height + 2*residualComponents*pixelCount
	if len(src) < needed || len(src)-needed >= 16 {
		return nil, fmt.Errorf("dmp2 default data has %d bytes, expected %d plus less than 16 bytes of alignment", len(src), needed)
	}
	alpha := src[:alphaBytes]
	predictors := src[alphaBytes : alphaBytes+height]
	firstPlane := src[alphaBytes+height : alphaBytes+height+residualComponents*pixelCount]
	secondPlane := src[alphaBytes+height+residualComponents*pixelCount : needed]
	previous := make([]int16, width*3)
	current := make([]int16, width*3)
	pixels := make([]byte, pixelCount*layout.bytesPerPixel())
	for row := 0; row < height; row++ {
		for i := range current {
			current[i] = 0
		}
		for column := 0; column < width; column++ {
			for component := 0; component < residualComponents; component++ {
				index := (row*width+column)*residualComponents + component
				// Deepmap stores the high-byte plane first and the low/sign-byte
				// plane second to improve the following entropy compression.
				encoded := uint16(secondPlane[index]) | uint16(firstPlane[index])<<8
				value := int16(encoded >> 1)
				if encoded&1 != 0 {
					value = -value
				}
				current[column*3+component] = value
			}
		}
		unpredictDeepmapRow(current, previous, predictors[row])
		for column := 0; column < width; column++ {
			source := column * 3
			dest := (row*width + column) * layout.bytesPerPixel()
			if residualComponents == 1 {
				pixels[dest] = byte(current[source])
				if layout.hasAlpha {
					pixels[dest+1] = alpha[row*width+column]
				}
				continue
			}
			y := current[source]
			co := int16(uint16(current[source+1]) << 1)
			cg := int16(uint16(current[source+2]) << 1)
			temporary := int16(int32(y) - int32(cg)/2)
			green := int16(int32(temporary) + int32(cg))
			blue := int16(int32(temporary) - int32(co)/2)
			red := int16(int32(blue) + int32(co))
			if layout.bytesPerComponent == 1 {
				pixels[dest+0] = byte(red)
				pixels[dest+1] = byte(green)
				pixels[dest+2] = byte(blue)
				if layout.hasAlpha {
					pixels[dest+3] = alpha[row*width+column]
				}
			} else {
				binary.LittleEndian.PutUint16(pixels[dest+0:dest+2], scaleWide(blue))
				binary.LittleEndian.PutUint16(pixels[dest+2:dest+4], scaleWide(green))
				binary.LittleEndian.PutUint16(pixels[dest+4:dest+6], scaleWide(red))
				wideAlpha := (uint32(alpha[row*width+column])*10000 + 127) / 255
				binary.LittleEndian.PutUint16(pixels[dest+6:dest+8], uint16(wideAlpha))
			}
		}
		previous, current = current, previous
	}
	return pixels, nil
}

func scaleWide(value int16) uint16 {
	if value < 0 {
		return 0
	}
	if value >= 512 {
		return 10000
	}
	return uint16((uint32(value)*10000 + 256) / 512)
}

func unpredictDeepmapRow(row, previous []int16, predictor byte) {
	add := func(a, b int16) int16 { return int16(int32(a) + int32(b)) }
	switch predictor {
	case 0:
		return
	case 1:
		for component := 0; component < 3; component++ {
			row[component] = add(row[component], previous[component])
		}
		for i := 3; i < len(row); i += 3 {
			leftY := int(row[i-3])
			upY := int(previous[i])
			upLeftY := int(previous[i-3])
			predict := row[i-3 : i]
			if absInt(upY-upLeftY) > absInt(leftY-upLeftY) {
				predict = previous[i : i+3]
			}
			for component := 0; component < 3; component++ {
				row[i+component] = add(row[i+component], predict[component])
			}
		}
	case 2:
		for i := 3; i < len(row); i++ {
			row[i] = add(row[i], row[i-3])
		}
	case 3:
		for i := range row {
			row[i] = add(row[i], previous[i])
		}
	case 4:
		for component := 0; component < 3; component++ {
			row[component] = add(row[component], previous[component])
		}
		for i := 3; i < len(row); i++ {
			sum := int32(row[i-3]) + int32(previous[i]) + 1
			if sum < 0 {
				sum++
			}
			row[i] = add(row[i], int16(sum>>1))
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func decodeKCBCDeepmap2(src []byte) (Bitmap, error) {
	var result Bitmap
	var totalHeight uint64
	for offset, segment := 0, 0; offset < len(src); segment++ {
		if len(src)-offset < 20 {
			return result, fmt.Errorf("truncated KCBC Deepmap2 header at offset %d", offset)
		}
		if string(src[offset:offset+4]) != "KCBC" {
			return result, fmt.Errorf("invalid KCBC Deepmap2 magic at offset %d", offset)
		}
		chunkRows := binary.LittleEndian.Uint32(src[offset+12 : offset+16])
		length := binary.LittleEndian.Uint32(src[offset+16 : offset+20])
		end := uint64(offset+20) + uint64(length)
		if end > uint64(len(src)) {
			return result, fmt.Errorf("KCBC Deepmap2 segment %d needs %d bytes", segment, length)
		}
		payload := src[offset+20 : int(end)]
		width := uint16(0)
		if len(payload) >= 28 && string(payload[16:20]) == "dmp2" {
			width = binary.LittleEndian.Uint16(payload[24:26])
		}
		decoded, err := DecodeWithGeometry(payload, width, uint16(chunkRows))
		if err != nil {
			return result, fmt.Errorf("KCBC Deepmap2 segment %d: %w", segment, err)
		}
		if uint32(decoded.Height) != chunkRows {
			return result, fmt.Errorf("KCBC Deepmap2 segment %d has %d rows, chunk declares %d", segment, decoded.Height, chunkRows)
		}
		if segment == 0 {
			result = decoded
		} else {
			if decoded.Width != result.Width || decoded.Components != result.Components {
				return result, fmt.Errorf("KCBC Deepmap2 segment %d has incompatible geometry", segment)
			}
			result.Pixels = append(result.Pixels, decoded.Pixels...)
		}
		totalHeight += uint64(decoded.Height)
		offset = int(end)
	}
	if totalHeight > 0xffff {
		return result, fmt.Errorf("KCBC Deepmap2 height %d exceeds format limit", totalHeight)
	}
	result.Height = uint16(totalHeight)
	return result, nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
