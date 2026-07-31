// Package deepmap2 decodes CoreUI Deepmap2 bitmap payloads.
package deepmap2

import (
	"carfile-go/codec/lzfse"
	"carfile-go/codec/lzvn"
	"encoding/binary"
	"fmt"
)

const (
	deepmap2Default  = 2
	deepmap2Lossless = 3
	deepmap2Palette  = 4
)

// Bitmap is the decoded, tightly packed pixel buffer stored in a dmp2
// payload. Components is normally 4 (BGRA) or 2 (gray and alpha).
type Bitmap struct {
	Width      uint16
	Height     uint16
	Components uint8
	Method     uint8
	Pixels     []byte
}

// Decode decodes the data after a CELM wrapper. It supports the
// lossless and palette encodings emitted by current versions of actool.
func Decode(src []byte) (Bitmap, error) {
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
	result.Components = src[23]
	result.Width = binary.LittleEndian.Uint16(src[24:26])
	result.Height = binary.LittleEndian.Uint16(src[26:28])
	if blobVersion != 1 {
		return result, fmt.Errorf("unsupported dmp2 blob version %d", blobVersion)
	}
	if result.Width == 0 || result.Height == 0 || result.Components == 0 {
		return result, fmt.Errorf("invalid dmp2 geometry %dx%d with %d components", result.Width, result.Height, result.Components)
	}
	pixelBytes := uint64(result.Width) * uint64(result.Height) * uint64(result.Components)
	if pixelBytes > uint64(maxInt()) {
		return result, fmt.Errorf("dmp2 bitmap is too large")
	}
	descriptor := binary.LittleEndian.Uint32(src[28:32])
	payload := src[32:containerEnd]

	switch result.Method {
	case deepmap2Default:
		remaining := payload
		compressedBytes := descriptor
		var totalRows uint64
		for segment := 0; ; segment++ {
			if uint64(compressedBytes) > uint64(len(remaining)) {
				return result, fmt.Errorf("dmp2 default segment %d needs %d bytes, has %d", segment, compressedBytes, len(remaining))
			}
			decoded, err := lzfse.Decode(remaining[:compressedBytes])
			if err != nil {
				return result, fmt.Errorf("dmp2 default segment %d: %w", segment, err)
			}
			rows, err := deepmap2DefaultRowCount(len(decoded), int(result.Width), int(result.Components))
			if err != nil {
				return result, fmt.Errorf("dmp2 default segment %d: %w", segment, err)
			}
			if segment == 0 && rows != int(result.Height) {
				return result, fmt.Errorf("dmp2 first default segment has %d rows, header declares %d", rows, result.Height)
			}
			pixels, err := decodeDeepmap2DefaultPixels(decoded, int(result.Width), rows, int(result.Components))
			if err != nil {
				return result, err
			}
			result.Pixels = append(result.Pixels, pixels...)
			totalRows += uint64(rows)
			remaining = remaining[compressedBytes:]
			if len(remaining) == 0 {
				break
			}
			if len(remaining) < 4 {
				return result, fmt.Errorf("dmp2 default segment length is truncated")
			}
			compressedBytes = binary.LittleEndian.Uint32(remaining[:4])
			remaining = remaining[4:]
		}
		if totalRows > 0xffff {
			return result, fmt.Errorf("dmp2 height %d exceeds format limit", totalRows)
		}
		result.Height = uint16(totalRows)
		return result, nil

	case deepmap2Lossless:
		rowBytes := uint64(result.Width) * uint64(result.Components)
		remaining := payload
		compressedBytes := descriptor
		var totalRows uint64
		for segment := 0; ; segment++ {
			if uint64(compressedBytes) > uint64(len(remaining)) {
				return result, fmt.Errorf("dmp2 lossless segment %d needs %d bytes, has %d", segment, compressedBytes, len(remaining))
			}
			decoded, err := lzfse.Decode(remaining[:compressedBytes])
			if err != nil {
				return result, fmt.Errorf("dmp2 lossless segment %d: %w", segment, err)
			}
			if uint64(len(decoded))%rowBytes != 0 {
				return result, fmt.Errorf("dmp2 lossless segment %d expands to a partial row", segment)
			}
			rows := uint64(len(decoded)) / rowBytes
			if segment == 0 && rows != uint64(result.Height) {
				return result, fmt.Errorf("dmp2 first lossless segment has %d rows, header declares %d", rows, result.Height)
			}
			totalRows += rows
			result.Pixels = append(result.Pixels, decoded...)
			remaining = remaining[compressedBytes:]
			if len(remaining) == 0 {
				break
			}
			if len(remaining) < 4 {
				return result, fmt.Errorf("dmp2 lossless segment length is truncated")
			}
			compressedBytes = binary.LittleEndian.Uint32(remaining[:4])
			remaining = remaining[4:]
		}
		if totalRows > 0xffff {
			return result, fmt.Errorf("dmp2 height %d exceeds format limit", totalRows)
		}
		result.Height = uint16(totalRows)
		return result, nil

	case deepmap2Palette:
		paletteCount := int(descriptor & 0xffff)
		paletteComponents := int(descriptor >> 16)
		if paletteCount == 0 || paletteCount > 256 {
			return result, fmt.Errorf("invalid dmp2 palette size %d", paletteCount)
		}
		if paletteComponents != int(result.Components) {
			return result, fmt.Errorf("dmp2 palette has %d components, header declares %d", paletteComponents, result.Components)
		}
		paletteBytes := paletteCount * paletteComponents
		if paletteBytes+4 > len(payload) {
			return result, fmt.Errorf("dmp2 palette and index header need %d bytes, have %d", paletteBytes+4, len(payload))
		}
		encodedBytes := binary.LittleEndian.Uint32(payload[paletteBytes : paletteBytes+4])
		encodedStart := paletteBytes + 4
		encodedEnd := uint64(encodedStart) + uint64(encodedBytes)
		if encodedEnd > uint64(len(payload)) {
			return result, fmt.Errorf("dmp2 palette index stream needs %d bytes, has %d", encodedBytes, len(payload)-encodedStart)
		}
		indexCount := int(result.Width) * int(result.Height)
		encodedIndices := payload[encodedStart:int(encodedEnd)]
		var indices []byte
		var err error
		if len(encodedIndices) >= 4 && (string(encodedIndices[:4]) == "bvx2" || string(encodedIndices[:4]) == "bvx-" || string(encodedIndices[:4]) == "bvxn") {
			indices, err = lzfse.Decode(encodedIndices)
			if err == nil && len(indices) != indexCount {
				err = fmt.Errorf("decoded %d palette indices, expected %d", len(indices), indexCount)
			}
		} else {
			indices, err = lzvn.Decode(encodedIndices, indexCount)
		}
		if err != nil {
			return result, fmt.Errorf("dmp2 palette indices: %w", err)
		}
		pixels := make([]byte, indexCount*paletteComponents)
		palette := payload[:paletteBytes]
		for i, index := range indices {
			if int(index) >= paletteCount {
				return result, fmt.Errorf("dmp2 palette index %d at pixel %d exceeds %d colors", index, i, paletteCount)
			}
			copy(pixels[i*paletteComponents:], palette[int(index)*paletteComponents:])
		}
		result.Pixels = pixels
		return result, nil

	default:
		return result, fmt.Errorf("unsupported dmp2 encoding method %d (outer encoding %d)", result.Method, outerEncoding)
	}
}

func deepmap2DefaultRowCount(decodedBytes, width, components int) (int, error) {
	if components != 2 && components != 4 {
		return 0, fmt.Errorf("unsupported component count %d", components)
	}
	bytesPerRow := width*(1+2*(components-1)) + 1
	rows := decodedBytes / bytesPerRow
	if rows == 0 || (bytesPerRow*rows+15)&^15 != decodedBytes {
		return 0, fmt.Errorf("cannot derive row count from %d decoded bytes", decodedBytes)
	}
	return rows, nil
}

func decodeDeepmap2DefaultPixels(src []byte, width, height, components int) ([]byte, error) {
	if components != 2 && components != 4 {
		return nil, fmt.Errorf("dmp2 default encoding with %d components is unsupported", components)
	}
	pixelCount := width * height
	residualComponents := components - 1
	needed := pixelCount + height + 2*residualComponents*pixelCount
	if len(src) < needed {
		return nil, fmt.Errorf("dmp2 default data has %d bytes, needs %d", len(src), needed)
	}
	alpha := src[:pixelCount]
	predictors := src[pixelCount : pixelCount+height]
	firstPlane := src[pixelCount+height : pixelCount+height+residualComponents*pixelCount]
	secondPlane := src[pixelCount+height+residualComponents*pixelCount : needed]
	previous := make([]int16, width*3)
	current := make([]int16, width*3)
	pixels := make([]byte, pixelCount*components)
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
			dest := (row*width + column) * components
			if components == 2 {
				pixels[dest] = byte(current[source])
				pixels[dest+1] = alpha[row*width+column]
				continue
			}
			y := current[source]
			co := int16(uint16(current[source+1]) << 1)
			cg := int16(uint16(current[source+2]) << 1)
			temporary := int16(int32(y) - int32(cg)/2)
			green := int16(int32(temporary) + int32(cg))
			blue := int16(int32(temporary) - int32(co)/2)
			red := int16(int32(blue) + int32(co))
			pixels[dest+0] = byte(red)
			pixels[dest+1] = byte(green)
			pixels[dest+2] = byte(blue)
			pixels[dest+3] = alpha[row*width+column]
		}
		previous, current = current, previous
	}
	return pixels, nil
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
		decoded, err := Decode(src[offset+20 : int(end)])
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
