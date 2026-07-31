// Package lzfse decodes Apple LZFSE streams without cgo.
package lzfse

import (
	"carfile-go/codec/lzvn"
	"encoding/binary"
	"fmt"
	"math/bits"
)

const (
	lzfseEndMagic        = 0x24787662 // bvx$
	lzfseRawMagic        = 0x2d787662 // bvx-
	lzfseCompressedMagic = 0x32787662 // bvx2
	lzfseV1Magic         = 0x31787662 // bvx1
	lzfseLZVNMagic       = 0x6e787662 // bvxn
)

var (
	lExtraBits = [...]uint8{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 3, 5, 8}
	lBaseValue = [...]int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 20, 28, 60}
	mExtraBits = [...]uint8{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 5, 8, 11}
	mBaseValue = [...]int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 24, 56, 312}
	dExtraBits = [...]uint8{
		0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3,
		4, 4, 4, 4, 5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7,
		8, 8, 8, 8, 9, 9, 9, 9, 10, 10, 10, 10, 11, 11, 11, 11,
		12, 12, 12, 12, 13, 13, 13, 13, 14, 14, 14, 14, 15, 15, 15, 15,
	}
	dBaseValue = [...]int32{
		0, 1, 2, 3, 4, 6, 8, 10, 12, 16, 20, 24, 28, 36, 44, 52,
		60, 76, 92, 108, 124, 156, 188, 220, 252, 316, 380, 444, 508, 636,
		764, 892, 1020, 1276, 1532, 1788, 2044, 2556, 3068, 3580, 4092, 5116,
		6140, 7164, 8188, 10236, 12284, 14332, 16380, 20476, 24572, 28668,
		32764, 40956, 49148, 57340, 65532, 81916, 98300, 114684, 131068,
		163836, 196604, 229372,
	}
)

// Decode expands an Apple LZFSE byte stream without cgo or external
// packages. It currently accepts the block kinds used by compiled asset
// catalogs: uncompressed (bvx-) and compressed-v2 (bvx2) blocks.
func Decode(src []byte) ([]byte, error) {
	var dst []byte
	for offset := 0; ; {
		if len(src)-offset < 4 {
			return nil, fmt.Errorf("LZFSE stream ends before block magic at offset %d", offset)
		}
		magic := binary.LittleEndian.Uint32(src[offset:])
		switch magic {
		case lzfseEndMagic:
			if offset+4 != len(src) {
				return nil, fmt.Errorf("LZFSE stream has %d trailing bytes", len(src)-offset-4)
			}
			return dst, nil

		case lzfseRawMagic:
			if len(src)-offset < 8 {
				return nil, fmt.Errorf("truncated LZFSE raw block header at offset %d", offset)
			}
			rawSize := uint64(binary.LittleEndian.Uint32(src[offset+4:]))
			end := uint64(offset+8) + rawSize
			if end > uint64(len(src)) {
				return nil, fmt.Errorf("LZFSE raw block at offset %d needs %d bytes", offset, rawSize)
			}
			dst = append(dst, src[offset+8:int(end)]...)
			offset = int(end)

		case lzfseCompressedMagic:
			var consumed int
			var err error
			dst, consumed, err = decodeLZFSEV2Block(src[offset:], dst)
			if err != nil {
				return nil, fmt.Errorf("LZFSE block at offset %d: %w", offset, err)
			}
			offset += consumed

		case lzfseV1Magic:
			return nil, fmt.Errorf("LZFSE v1 block at offset %d is not supported", offset)
		case lzfseLZVNMagic:
			if len(src)-offset < 12 {
				return nil, fmt.Errorf("truncated LZVN block header at offset %d", offset)
			}
			rawBytes := binary.LittleEndian.Uint32(src[offset+4:])
			payloadBytes := binary.LittleEndian.Uint32(src[offset+8:])
			end := uint64(offset+12) + uint64(payloadBytes)
			if end > uint64(len(src)) {
				return nil, fmt.Errorf("LZVN block at offset %d needs %d payload bytes", offset, payloadBytes)
			}
			decoded, err := lzvn.Decode(src[offset+12:int(end)], int(rawBytes))
			if err != nil {
				return nil, fmt.Errorf("LZVN block at offset %d: %w", offset, err)
			}
			dst = append(dst, decoded...)
			offset = int(end)
		default:
			return nil, fmt.Errorf("unknown LZFSE block magic %q at offset %d", src[offset:offset+4], offset)
		}
	}
}

type lzfseV2Header struct {
	rawBytes            uint32
	literalCount        uint32
	literalPayloadBytes uint32
	matchCount          uint32
	literalBits         int
	literalState        [4]uint16
	lmdPayloadBytes     uint32
	lmdBits             int
	lState              uint16
	mState              uint16
	dState              uint16
	headerBytes         uint32
	lFreq               [20]uint16
	mFreq               [20]uint16
	dFreq               [64]uint16
	literalFreq         [256]uint16
}

func decodeLZFSEV2Block(src []byte, decoded []byte) ([]byte, int, error) {
	header, err := parseLZFSEV2Header(src)
	if err != nil {
		return nil, 0, err
	}
	payloadBytes := uint64(header.literalPayloadBytes) + uint64(header.lmdPayloadBytes)
	blockBytes := uint64(header.headerBytes) + payloadBytes
	if blockBytes > uint64(len(src)) {
		return nil, 0, fmt.Errorf("header and payload need %d bytes, have %d", blockBytes, len(src))
	}

	literalStart := int(header.headerBytes)
	literalEnd := literalStart + int(header.literalPayloadBytes)
	lmdEnd := literalEnd + int(header.lmdPayloadBytes)

	literalTable, err := makeFSESymbolTable(1024, header.literalFreq[:])
	if err != nil {
		return nil, 0, fmt.Errorf("literal table: %w", err)
	}
	lTable, err := makeFSEValueTable(64, header.lFreq[:], lExtraBits[:], lBaseValue[:])
	if err != nil {
		return nil, 0, fmt.Errorf("L table: %w", err)
	}
	mTable, err := makeFSEValueTable(64, header.mFreq[:], mExtraBits[:], mBaseValue[:])
	if err != nil {
		return nil, 0, fmt.Errorf("M table: %w", err)
	}
	dTable, err := makeFSEValueTable(256, header.dFreq[:], dExtraBits[:], dBaseValue[:])
	if err != nil {
		return nil, 0, fmt.Errorf("D table: %w", err)
	}

	// Apple's decoder permits the wide refill load to overlap the block header.
	// Only the requested low bits are retained, but using the block boundary here
	// is required for short literal payloads.
	literalStream, err := newReverseBitStream(src, 0, literalEnd, header.literalBits)
	if err != nil {
		return nil, 0, fmt.Errorf("literal bitstream: %w", err)
	}
	literals := make([]byte, header.literalCount)
	states := header.literalState
	for i := 0; i < len(literals); i += 4 {
		if err := literalStream.refill(); err != nil {
			return nil, 0, fmt.Errorf("literal %d: %w", i, err)
		}
		for lane := 0; lane < 4; lane++ {
			value, next, err := decodeFSESymbol(literalStream, literalTable, states[lane])
			if err != nil {
				return nil, 0, fmt.Errorf("literal %d: %w", i+lane, err)
			}
			literals[i+lane] = value
			states[lane] = next
		}
	}

	lmdStream, err := newReverseBitStream(src, literalEnd, lmdEnd, header.lmdBits)
	if err != nil {
		return nil, 0, fmt.Errorf("LMD bitstream: %w", err)
	}
	lState, mState, dState := header.lState, header.mState, header.dState
	blockStart := len(decoded)
	if uint64(blockStart)+uint64(header.rawBytes) > uint64(maxInt()) {
		return nil, 0, fmt.Errorf("decoded stream is too large")
	}
	if cap(decoded)-len(decoded) < int(header.rawBytes) {
		grown := make([]byte, len(decoded), len(decoded)+int(header.rawBytes))
		copy(grown, decoded)
		decoded = grown
	}
	literalOffset := 0
	distance := int32(-1)
	for match := uint32(0); match < header.matchCount; match++ {
		if err := lmdStream.refill(); err != nil {
			return nil, 0, fmt.Errorf("match %d: %w", match, err)
		}
		l, nextL, err := decodeFSEValue(lmdStream, lTable, lState)
		if err != nil {
			return nil, 0, fmt.Errorf("match %d L: %w", match, err)
		}
		m, nextM, err := decodeFSEValue(lmdStream, mTable, mState)
		if err != nil {
			return nil, 0, fmt.Errorf("match %d M: %w", match, err)
		}
		newDistance, nextD, err := decodeFSEValue(lmdStream, dTable, dState)
		if err != nil {
			return nil, 0, fmt.Errorf("match %d D: %w", match, err)
		}
		lState, mState, dState = nextL, nextM, nextD
		if newDistance != 0 {
			distance = newDistance
		}

		literalEnd := literalOffset + int(l)
		if literalEnd < literalOffset || literalEnd > len(literals) {
			return nil, 0, fmt.Errorf("match %d consumes literals through %d, only %d decoded", match, literalEnd, len(literals))
		}
		if uint64(len(decoded)-blockStart)+uint64(l)+uint64(m) > uint64(header.rawBytes) {
			return nil, 0, fmt.Errorf("match %d exceeds declared output size %d", match, header.rawBytes)
		}
		decoded = append(decoded, literals[literalOffset:literalEnd]...)
		literalOffset = literalEnd
		if distance <= 0 || int64(distance) > int64(len(decoded)) {
			return nil, 0, fmt.Errorf("match %d has invalid distance %d at output offset %d", match, distance, len(decoded))
		}
		for i := int32(0); i < m; i++ {
			decoded = append(decoded, decoded[len(decoded)-int(distance)])
		}
	}
	if len(decoded)-blockStart != int(header.rawBytes) {
		return nil, 0, fmt.Errorf("decoded %d bytes, expected %d", len(decoded)-blockStart, header.rawBytes)
	}
	return decoded, int(blockBytes), nil
}

func parseLZFSEV2Header(src []byte) (lzfseV2Header, error) {
	var h lzfseV2Header
	if len(src) < 32 {
		return h, fmt.Errorf("truncated v2 header: have %d bytes", len(src))
	}
	if binary.LittleEndian.Uint32(src) != lzfseCompressedMagic {
		return h, fmt.Errorf("not a bvx2 block")
	}
	h.rawBytes = binary.LittleEndian.Uint32(src[4:])
	v0 := binary.LittleEndian.Uint64(src[8:])
	v1 := binary.LittleEndian.Uint64(src[16:])
	v2 := binary.LittleEndian.Uint64(src[24:])
	h.literalCount = uint32(field(v0, 0, 20))
	h.literalPayloadBytes = uint32(field(v0, 20, 20))
	h.matchCount = uint32(field(v0, 40, 20))
	h.literalBits = int(field(v0, 60, 3)) - 7
	for i := range h.literalState {
		h.literalState[i] = uint16(field(v1, i*10, 10))
	}
	h.lmdPayloadBytes = uint32(field(v1, 40, 20))
	h.lmdBits = int(field(v1, 60, 3)) - 7
	h.headerBytes = uint32(field(v2, 0, 32))
	h.lState = uint16(field(v2, 32, 10))
	h.mState = uint16(field(v2, 42, 10))
	h.dState = uint16(field(v2, 52, 10))

	if h.headerBytes < 32 || uint64(h.headerBytes) > uint64(len(src)) {
		return h, fmt.Errorf("invalid v2 header size %d", h.headerBytes)
	}
	if h.literalCount > 40000 || h.literalCount%4 != 0 {
		return h, fmt.Errorf("invalid literal count %d", h.literalCount)
	}
	if h.matchCount > 10000 {
		return h, fmt.Errorf("invalid match count %d", h.matchCount)
	}
	for i, state := range h.literalState {
		if state >= 1024 {
			return h, fmt.Errorf("invalid literal state %d in lane %d", state, i)
		}
	}
	if h.lState >= 64 || h.mState >= 64 || h.dState >= 256 {
		return h, fmt.Errorf("invalid LMD states %d/%d/%d", h.lState, h.mState, h.dState)
	}

	freqs := make([]uint16, 20+20+64+256)
	if err := decodeLZFSEFreqs(src[32:h.headerBytes], freqs); err != nil {
		return h, err
	}
	copy(h.lFreq[:], freqs[:20])
	copy(h.mFreq[:], freqs[20:40])
	copy(h.dFreq[:], freqs[40:104])
	copy(h.literalFreq[:], freqs[104:])
	return h, nil
}

func decodeLZFSEFreqs(src []byte, dst []uint16) error {
	nbitsTable := [...]uint8{2, 3, 2, 5, 2, 3, 2, 8, 2, 3, 2, 5, 2, 3, 2, 14, 2, 3, 2, 5, 2, 3, 2, 8, 2, 3, 2, 5, 2, 3, 2, 14}
	valueTable := [...]int8{0, 2, 1, 4, 0, 3, 1, -1, 0, 2, 1, 5, 0, 3, 1, -1, 0, 2, 1, 6, 0, 3, 1, -1, 0, 2, 1, 7, 0, 3, 1, -1}
	var accum uint64
	available := 0
	offset := 0
	for i := range dst {
		for offset < len(src) && available <= 56 {
			accum |= uint64(src[offset]) << available
			available += 8
			offset++
		}
		code := accum & 31
		n := int(nbitsTable[code])
		if n > available {
			return fmt.Errorf("frequency table ends at symbol %d", i)
		}
		var value int
		switch n {
		case 8:
			value = 8 + int((accum>>4)&15)
		case 14:
			value = 24 + int((accum>>4)&1023)
		default:
			value = int(valueTable[code])
		}
		if value < 0 {
			return fmt.Errorf("invalid frequency code at symbol %d", i)
		}
		dst[i] = uint16(value)
		accum >>= n
		available -= n
	}
	if offset != len(src) || available >= 8 {
		return fmt.Errorf("frequency table does not end at header boundary")
	}
	return nil
}

type fseSymbolEntry struct {
	bits   uint8
	symbol byte
	delta  int
}

type fseValueEntry struct {
	totalBits uint8
	valueBits uint8
	delta     int
	base      int32
}

func makeFSESymbolTable(states int, freq []uint16) ([]fseSymbolEntry, error) {
	if err := checkFSEFrequencies(states, freq); err != nil {
		return nil, err
	}
	table := make([]fseSymbolEntry, states)
	offset := 0
	for symbol, frequency := range freq {
		f := int(frequency)
		if f == 0 {
			continue
		}
		k := bits.Len(uint(states)) - bits.Len(uint(f))
		j0 := ((2 * states) >> k) - f
		for j := 0; j < f; j++ {
			entry := fseSymbolEntry{symbol: byte(symbol)}
			if j < j0 {
				entry.bits = uint8(k)
				entry.delta = ((f + j) << k) - states
			} else {
				entry.bits = uint8(k - 1)
				entry.delta = (j - j0) << (k - 1)
			}
			table[offset] = entry
			offset++
		}
	}
	return table, nil
}

func makeFSEValueTable(states int, freq []uint16, valueBits []uint8, base []int32) ([]fseValueEntry, error) {
	if len(freq) != len(valueBits) || len(freq) != len(base) {
		return nil, fmt.Errorf("inconsistent value table lengths")
	}
	if err := checkFSEFrequencies(states, freq); err != nil {
		return nil, err
	}
	table := make([]fseValueEntry, states)
	offset := 0
	for symbol, frequency := range freq {
		f := int(frequency)
		if f == 0 {
			continue
		}
		k := bits.Len(uint(states)) - bits.Len(uint(f))
		j0 := ((2 * states) >> k) - f
		for j := 0; j < f; j++ {
			entry := fseValueEntry{valueBits: valueBits[symbol], base: base[symbol]}
			if j < j0 {
				entry.totalBits = uint8(k) + entry.valueBits
				entry.delta = ((f + j) << k) - states
			} else {
				entry.totalBits = uint8(k-1) + entry.valueBits
				entry.delta = (j - j0) << (k - 1)
			}
			table[offset] = entry
			offset++
		}
	}
	return table, nil
}

func checkFSEFrequencies(states int, freq []uint16) error {
	sum := 0
	for _, frequency := range freq {
		sum += int(frequency)
	}
	if sum != states {
		return fmt.Errorf("frequencies sum to %d, expected %d", sum, states)
	}
	return nil
}

func decodeFSESymbol(stream *reverseBitStream, table []fseSymbolEntry, state uint16) (byte, uint16, error) {
	if int(state) >= len(table) {
		return 0, 0, fmt.Errorf("state %d is outside table", state)
	}
	entry := table[state]
	value, err := stream.pull(int(entry.bits))
	if err != nil {
		return 0, 0, err
	}
	next := entry.delta + int(value)
	if next < 0 || next >= len(table) {
		return 0, 0, fmt.Errorf("next state %d is outside table", next)
	}
	return entry.symbol, uint16(next), nil
}

func decodeFSEValue(stream *reverseBitStream, table []fseValueEntry, state uint16) (int32, uint16, error) {
	if int(state) >= len(table) {
		return 0, 0, fmt.Errorf("state %d is outside table", state)
	}
	entry := table[state]
	encoded, err := stream.pull(int(entry.totalBits))
	if err != nil {
		return 0, 0, err
	}
	next := entry.delta + int(encoded>>entry.valueBits)
	if next < 0 || next >= len(table) {
		return 0, 0, fmt.Errorf("next state %d is outside table", next)
	}
	var mask uint64
	if entry.valueBits != 0 {
		mask = (uint64(1) << entry.valueBits) - 1
	}
	return entry.base + int32(encoded&mask), uint16(next), nil
}

type reverseBitStream struct {
	src      []byte
	start    int
	position int
	accum    uint64
	bits     int
}

func newReverseBitStream(src []byte, start, end, encodedBits int) (*reverseBitStream, error) {
	if start < 0 || end < start || end > len(src) {
		return nil, fmt.Errorf("invalid byte range [%d,%d)", start, end)
	}
	stream := &reverseBitStream{src: src, start: start}
	if encodedBits != 0 {
		if end-start < 8 {
			return nil, fmt.Errorf("need 8 bytes, have %d", end-start)
		}
		stream.position = end - 8
		stream.accum = binary.LittleEndian.Uint64(src[stream.position:end])
		stream.bits = encodedBits + 64
	} else {
		if end-start < 7 {
			return nil, fmt.Errorf("need 7 bytes, have %d", end-start)
		}
		stream.position = end - 7
		for i := 0; i < 7; i++ {
			stream.accum |= uint64(src[stream.position+i]) << (8 * i)
		}
		stream.bits = 56
	}
	if stream.bits < 56 || stream.bits >= 64 || stream.accum>>stream.bits != 0 {
		return nil, fmt.Errorf("invalid initial accumulator with %d bits", stream.bits)
	}
	return stream, nil
}

func (s *reverseBitStream) refill() error {
	addBits := (63 - s.bits) & -8
	if addBits == 0 {
		return nil
	}
	position := s.position - addBits/8
	if position < s.start {
		return fmt.Errorf("bitstream underflow")
	}
	incoming := binary.LittleEndian.Uint64(s.src[position : position+8])
	s.accum = (s.accum << addBits) | (incoming & lowMask(addBits))
	s.bits += addBits
	s.position = position
	return nil
}

func (s *reverseBitStream) pull(count int) (uint64, error) {
	if count < 0 || count > s.bits {
		return 0, fmt.Errorf("need %d bits, have %d", count, s.bits)
	}
	s.bits -= count
	value := s.accum >> s.bits
	s.accum &= lowMask(s.bits)
	return value, nil
}

func lowMask(count int) uint64 {
	if count == 64 {
		return ^uint64(0)
	}
	if count == 0 {
		return 0
	}
	return (uint64(1) << count) - 1
}

func field(value uint64, offset, width int) uint64 {
	if width == 64 {
		return value
	}
	return (value >> offset) & ((uint64(1) << width) - 1)
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
