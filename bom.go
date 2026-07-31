package carfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
)

const bomHeaderSize = 32

// BOMHeader describes the big-endian BOMStore container header.
type BOMHeader struct {
	Version        uint32 `json:"version"`
	NumberOfBlocks uint32 `json:"number_of_blocks"`
	IndexOffset    uint32 `json:"index_offset"`
	IndexLength    uint32 `json:"index_length"`
	VarsOffset     uint32 `json:"vars_offset"`
	VarsLength     uint32 `json:"vars_length"`
}

// BOMBlock points to a block in the BOMStore file.
type BOMBlock struct {
	Offset uint32 `json:"offset"`
	Length uint32 `json:"length"`
}

// BOMInfo is the container-level information included in Catalog output.
type BOMInfo struct {
	Header     BOMHeader         `json:"header"`
	BlockCount int               `json:"block_count"`
	Variables  map[string]uint32 `json:"variables"`
}

// BOMTreeEntry contains the raw key and value blocks from a BOM B+ tree.
type BOMTreeEntry struct {
	KeyBlock   uint32
	ValueBlock uint32
	Key        []byte
	Value      []byte
}

type BOM struct {
	r         io.ReaderAt
	size      int64
	Header    BOMHeader
	blocks    []BOMBlock
	variables map[string]uint32
}

// ParseBOM reads a BOMStore without using Apple's private Bom.framework.
func ParseBOM(r io.ReaderAt, size int64) (*BOM, error) {
	if size < bomHeaderSize {
		return nil, fmt.Errorf("BOMStore: file is only %d bytes", size)
	}

	headerBytes := make([]byte, bomHeaderSize)
	if _, err := r.ReadAt(headerBytes, 0); err != nil {
		return nil, fmt.Errorf("BOMStore: read header: %w", err)
	}
	if string(headerBytes[:8]) != "BOMStore" {
		return nil, fmt.Errorf("BOMStore: invalid magic %q", headerBytes[:8])
	}

	be := binary.BigEndian
	header := BOMHeader{
		Version:        be.Uint32(headerBytes[8:12]),
		NumberOfBlocks: be.Uint32(headerBytes[12:16]),
		IndexOffset:    be.Uint32(headerBytes[16:20]),
		IndexLength:    be.Uint32(headerBytes[20:24]),
		VarsOffset:     be.Uint32(headerBytes[24:28]),
		VarsLength:     be.Uint32(headerBytes[28:32]),
	}
	if header.Version != 1 {
		return nil, fmt.Errorf("BOMStore: unsupported version %d", header.Version)
	}

	b := &BOM{r: r, size: size, Header: header}
	index, err := b.readRange(uint64(header.IndexOffset), uint64(header.IndexLength), "block index")
	if err != nil {
		return nil, err
	}
	if len(index) < 4 {
		return nil, errors.New("BOMStore: truncated block index")
	}
	pointerCount := be.Uint32(index[:4])
	if uint64(pointerCount) > uint64((len(index)-4)/8) {
		return nil, fmt.Errorf("BOMStore: block index declares %d pointers but only %d fit", pointerCount, (len(index)-4)/8)
	}
	b.blocks = make([]BOMBlock, pointerCount)
	for i := range b.blocks {
		pos := 4 + i*8
		block := BOMBlock{Offset: be.Uint32(index[pos : pos+4]), Length: be.Uint32(index[pos+4 : pos+8])}
		if block.Length != 0 {
			if _, err := b.checkedRange(uint64(block.Offset), uint64(block.Length)); err != nil {
				return nil, fmt.Errorf("BOMStore: block %d: %w", i, err)
			}
		}
		b.blocks[i] = block
	}
	if len(b.blocks) == 0 || b.blocks[0] != (BOMBlock{}) {
		return nil, errors.New("BOMStore: block zero is not the required null block")
	}

	vars, err := b.readRange(uint64(header.VarsOffset), uint64(header.VarsLength), "variables")
	if err != nil {
		return nil, err
	}
	if len(vars) < 4 {
		return nil, errors.New("BOMStore: truncated variables table")
	}
	variableCount := be.Uint32(vars[:4])
	if uint64(variableCount) > uint64((len(vars)-4)/5) {
		return nil, fmt.Errorf("BOMStore: variables table cannot contain %d entries", variableCount)
	}
	b.variables = make(map[string]uint32, variableCount)
	pos := 4
	for i := uint32(0); i < variableCount; i++ {
		if pos+5 > len(vars) {
			return nil, fmt.Errorf("BOMStore: truncated variable %d", i)
		}
		blockID := be.Uint32(vars[pos : pos+4])
		nameLength := int(vars[pos+4])
		pos += 5
		if pos+nameLength > len(vars) {
			return nil, fmt.Errorf("BOMStore: truncated name for variable %d", i)
		}
		name := string(vars[pos : pos+nameLength])
		pos += nameLength
		if int(blockID) >= len(b.blocks) {
			return nil, fmt.Errorf("BOMStore: variable %q refers to missing block %d", name, blockID)
		}
		b.variables[name] = blockID
	}

	return b, nil
}

func (b *BOM) Info() BOMInfo {
	variables := make(map[string]uint32, len(b.variables))
	for name, block := range b.variables {
		variables[name] = block
	}
	return BOMInfo{Header: b.Header, BlockCount: len(b.blocks), Variables: variables}
}

func (b *BOM) VariableNames() []string {
	names := make([]string, 0, len(b.variables))
	for name := range b.variables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (b *BOM) NamedBlock(name string) ([]byte, error) {
	blockID, ok := b.variables[name]
	if !ok {
		return nil, fmt.Errorf("BOMStore: named block %q not found", name)
	}
	return b.Block(blockID)
}

func (b *BOM) Block(id uint32) ([]byte, error) {
	if uint64(id) >= uint64(len(b.blocks)) {
		return nil, fmt.Errorf("BOMStore: block %d does not exist", id)
	}
	block := b.blocks[id]
	return b.readRange(uint64(block.Offset), uint64(block.Length), fmt.Sprintf("block %d", id))
}

type bomTreeHeader struct {
	Root      uint32
	BlockSize uint32
	PathCount uint32
	Flags     byte
}

type bomTreeNode struct {
	Leaf     bool
	Forward  uint32
	Backward uint32
	Entries  []bomTreeIndex
}

type bomTreeIndex struct {
	ValueBlock uint32
	KeyBlock   uint32
}

// TreeEntries walks every linked leaf in a named BOM B+ tree.
func (b *BOM) TreeEntries(name string) ([]BOMTreeEntry, error) {
	rawHeader, err := b.NamedBlock(name)
	if err != nil {
		return nil, err
	}
	header, err := parseBOMTreeHeader(rawHeader)
	if err != nil {
		return nil, fmt.Errorf("BOMStore: tree %q: %w", name, err)
	}
	if header.Flags&4 != 0 {
		return nil, fmt.Errorf("BOMStore: tree %q uses unsupported inline keys", name)
	}

	nodeID := header.Root
	visited := make(map[uint32]struct{})
	for {
		if _, exists := visited[nodeID]; exists {
			return nil, fmt.Errorf("BOMStore: tree %q contains a branch cycle at block %d", name, nodeID)
		}
		visited[nodeID] = struct{}{}
		node, err := b.readTreeNode(nodeID)
		if err != nil {
			return nil, fmt.Errorf("BOMStore: tree %q: %w", name, err)
		}
		if node.Leaf {
			break
		}
		if len(node.Entries) == 0 {
			return nil, fmt.Errorf("BOMStore: tree %q has an empty branch block %d", name, nodeID)
		}
		nodeID = node.Entries[0].ValueBlock
	}

	capacity := len(b.blocks)
	if uint64(header.PathCount) < uint64(capacity) {
		capacity = int(header.PathCount)
	}
	entries := make([]BOMTreeEntry, 0, capacity)
	visited = make(map[uint32]struct{})
	for nodeID != 0 {
		if _, exists := visited[nodeID]; exists {
			return nil, fmt.Errorf("BOMStore: tree %q contains a leaf cycle at block %d", name, nodeID)
		}
		visited[nodeID] = struct{}{}
		node, err := b.readTreeNode(nodeID)
		if err != nil {
			return nil, fmt.Errorf("BOMStore: tree %q: %w", name, err)
		}
		if !node.Leaf {
			return nil, fmt.Errorf("BOMStore: tree %q leaf chain reached branch block %d", name, nodeID)
		}
		for _, item := range node.Entries {
			key, err := b.Block(item.KeyBlock)
			if err != nil {
				return nil, fmt.Errorf("BOMStore: tree %q key: %w", name, err)
			}
			value, err := b.Block(item.ValueBlock)
			if err != nil {
				return nil, fmt.Errorf("BOMStore: tree %q value: %w", name, err)
			}
			entries = append(entries, BOMTreeEntry{KeyBlock: item.KeyBlock, ValueBlock: item.ValueBlock, Key: key, Value: value})
		}
		nodeID = node.Forward
	}
	if header.PathCount != 0 && uint32(len(entries)) != header.PathCount {
		return nil, fmt.Errorf("BOMStore: tree %q declares %d entries but yielded %d", name, header.PathCount, len(entries))
	}
	return entries, nil
}

func parseBOMTreeHeader(raw []byte) (bomTreeHeader, error) {
	if len(raw) < 21 {
		return bomTreeHeader{}, fmt.Errorf("tree header is only %d bytes", len(raw))
	}
	if string(raw[:4]) != "tree" {
		return bomTreeHeader{}, fmt.Errorf("invalid tree magic %q", raw[:4])
	}
	be := binary.BigEndian
	if version := be.Uint32(raw[4:8]); version != 1 {
		return bomTreeHeader{}, fmt.Errorf("unsupported tree version %d", version)
	}
	return bomTreeHeader{
		Root:      be.Uint32(raw[8:12]),
		BlockSize: be.Uint32(raw[12:16]),
		PathCount: be.Uint32(raw[16:20]),
		Flags:     raw[20],
	}, nil
}

func (b *BOM) readTreeNode(id uint32) (bomTreeNode, error) {
	raw, err := b.Block(id)
	if err != nil {
		return bomTreeNode{}, err
	}
	if len(raw) < 12 {
		return bomTreeNode{}, fmt.Errorf("tree node block %d is only %d bytes", id, len(raw))
	}
	be := binary.BigEndian
	leafValue := be.Uint16(raw[:2])
	if leafValue > 1 {
		return bomTreeNode{}, fmt.Errorf("tree node block %d has invalid leaf flag %d", id, leafValue)
	}
	count := int(be.Uint16(raw[2:4]))
	if count > (len(raw)-12)/8 {
		return bomTreeNode{}, fmt.Errorf("tree node block %d declares %d entries but only %d fit", id, count, (len(raw)-12)/8)
	}
	node := bomTreeNode{
		Leaf:     leafValue == 1,
		Forward:  be.Uint32(raw[4:8]),
		Backward: be.Uint32(raw[8:12]),
		Entries:  make([]bomTreeIndex, count),
	}
	for i := range node.Entries {
		pos := 12 + i*8
		node.Entries[i] = bomTreeIndex{ValueBlock: be.Uint32(raw[pos : pos+4]), KeyBlock: be.Uint32(raw[pos+4 : pos+8])}
	}
	return node, nil
}

func (b *BOM) checkedRange(offset, length uint64) (int64, error) {
	if offset > uint64(b.size) || length > uint64(b.size)-offset {
		return 0, fmt.Errorf("range [%d,%d) exceeds %d-byte file", offset, offset+length, b.size)
	}
	return int64(offset), nil
}

func (b *BOM) readRange(offset, length uint64, label string) ([]byte, error) {
	start, err := b.checkedRange(offset, length)
	if err != nil {
		return nil, fmt.Errorf("BOMStore: %s: %w", label, err)
	}
	if length > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("BOMStore: %s is too large", label)
	}
	data := make([]byte, int(length))
	if length == 0 {
		return data, nil
	}
	if _, err := b.r.ReadAt(data, start); err != nil {
		return nil, fmt.Errorf("BOMStore: read %s: %w", label, err)
	}
	return data, nil
}
