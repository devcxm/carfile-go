// Package lzvn decodes Apple's raw LZVN instruction streams.
package lzvn

import (
	"encoding/binary"
	"fmt"
)

// Decode expands a raw LZVN instruction stream to exactly outputSize
// bytes. Raw streams end with the eight-byte LZVN end marker.
func Decode(src []byte, outputSize int) ([]byte, error) {
	if outputSize < 0 {
		return nil, fmt.Errorf("negative LZVN output size %d", outputSize)
	}
	dst := make([]byte, 0, outputSize)
	offset := 0
	distance := 0
	for {
		if offset >= len(src) {
			return nil, fmt.Errorf("LZVN stream ends before end marker")
		}
		opcode := src[offset]
		switch {
		case opcode == 0x06:
			if len(src)-offset < 8 {
				return nil, fmt.Errorf("truncated LZVN end marker at offset %d", offset)
			}
			offset += 8
			if offset != len(src) {
				return nil, fmt.Errorf("LZVN stream has %d trailing bytes", len(src)-offset)
			}
			if len(dst) != outputSize {
				return nil, fmt.Errorf("LZVN decoded %d bytes, expected %d", len(dst), outputSize)
			}
			return dst, nil

		case opcode == 0x0e || opcode == 0x16:
			offset++

		case opcode == 0xe0:
			if len(src)-offset < 2 {
				return nil, fmt.Errorf("truncated large literal at offset %d", offset)
			}
			length := int(src[offset+1]) + 16
			var err error
			offset, err = appendLZVNLiteral(dst, src, offset, 2, length)
			if err != nil {
				return nil, err
			}
			dst = append(dst, src[offset-length:offset]...)
			if len(dst) > outputSize {
				return nil, fmt.Errorf("large literal exceeds output size")
			}

		case opcode >= 0xe1 && opcode <= 0xef:
			length := int(opcode & 0x0f)
			literalStart := offset + 1
			if literalStart+length >= len(src) {
				return nil, fmt.Errorf("truncated literal at offset %d", offset)
			}
			dst = append(dst, src[literalStart:literalStart+length]...)
			offset = literalStart + length
			if len(dst) > outputSize {
				return nil, fmt.Errorf("literal exceeds output size")
			}

		case opcode == 0xf0:
			if len(src)-offset < 2 {
				return nil, fmt.Errorf("truncated large match at offset %d", offset)
			}
			length := int(src[offset+1]) + 16
			offset += 2
			if err := appendLZVNMatch(&dst, distance, length, outputSize); err != nil {
				return nil, fmt.Errorf("large match at offset %d: %w", offset-2, err)
			}

		case opcode >= 0xf1:
			offset++
			if err := appendLZVNMatch(&dst, distance, int(opcode&0x0f), outputSize); err != nil {
				return nil, fmt.Errorf("match at offset %d: %w", offset-1, err)
			}

		case opcode >= 0xa0 && opcode <= 0xbf:
			if len(src)-offset < 3 {
				return nil, fmt.Errorf("truncated medium-distance opcode at offset %d", offset)
			}
			packed := binary.LittleEndian.Uint16(src[offset+1 : offset+3])
			literalLength := int((opcode >> 3) & 3)
			matchLength := int((uint16(opcode&7)<<2)|(packed&3)) + 3
			distance = int((packed >> 2) & 0x3fff)
			var err error
			dst, offset, err = appendLZVNLiteralAndMatch(dst, src, offset, 3, literalLength, matchLength, distance, outputSize)
			if err != nil {
				return nil, err
			}

		case isLZVNCombinedOpcode(opcode):
			kind := opcode & 7
			literalLength := int(opcode >> 6)
			matchLength := int((opcode>>3)&7) + 3
			opcodeBytes := 1
			switch kind {
			case 0, 1, 2, 3, 4, 5:
				if len(src)-offset < 2 {
					return nil, fmt.Errorf("truncated small-distance opcode at offset %d", offset)
				}
				opcodeBytes = 2
				distance = int(opcode&7)<<8 | int(src[offset+1])
			case 6:
				// Reuse the previous distance.
			case 7:
				if len(src)-offset < 3 {
					return nil, fmt.Errorf("truncated large-distance opcode at offset %d", offset)
				}
				opcodeBytes = 3
				distance = int(binary.LittleEndian.Uint16(src[offset+1 : offset+3]))
			}
			var err error
			dst, offset, err = appendLZVNLiteralAndMatch(dst, src, offset, opcodeBytes, literalLength, matchLength, distance, outputSize)
			if err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("undefined LZVN opcode 0x%02x at offset %d", opcode, offset)
		}
	}
}

func isLZVNCombinedOpcode(opcode byte) bool {
	validRange := opcode <= 0x6f || opcode >= 0x80 && opcode <= 0x9f || opcode >= 0xc0 && opcode <= 0xcf
	if !validRange {
		return false
	}
	if opcode&7 == 6 && opcode < 0x46 {
		return false
	}
	return opcode != 0x06 && opcode != 0x0e && opcode != 0x16
}

func appendLZVNLiteral(dst []byte, src []byte, offset, opcodeBytes, length int) (int, error) {
	start := offset + opcodeBytes
	end := start + length
	if start < offset || end < start || end >= len(src) {
		return offset, fmt.Errorf("truncated LZVN literal at offset %d", offset)
	}
	return end, nil
}

func appendLZVNLiteralAndMatch(dst []byte, src []byte, offset, opcodeBytes, literalLength, matchLength, distance, outputSize int) ([]byte, int, error) {
	literalStart := offset + opcodeBytes
	literalEnd := literalStart + literalLength
	if literalStart < offset || literalEnd < literalStart || literalEnd >= len(src) {
		return nil, offset, fmt.Errorf("truncated LZVN literal/match at offset %d", offset)
	}
	if len(dst)+literalLength > outputSize {
		return nil, offset, fmt.Errorf("literal/match at offset %d exceeds output size", offset)
	}
	dst = append(dst, src[literalStart:literalEnd]...)
	if err := appendLZVNMatch(&dst, distance, matchLength, outputSize); err != nil {
		return nil, offset, fmt.Errorf("literal/match at offset %d: %w", offset, err)
	}
	return dst, literalEnd, nil
}

func appendLZVNMatch(dst *[]byte, distance, length, outputSize int) error {
	if distance <= 0 || distance > len(*dst) {
		return fmt.Errorf("invalid distance %d at output offset %d", distance, len(*dst))
	}
	if length < 0 || len(*dst)+length > outputSize {
		return fmt.Errorf("%d-byte match exceeds output size %d", length, outputSize)
	}
	for i := 0; i < length; i++ {
		*dst = append(*dst, (*dst)[len(*dst)-distance])
	}
	return nil
}
