package carfile

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

const csiFixedPrefixSize = 180

// Catalog is a parsed compiled Asset Catalog.
type Catalog struct {
	BOM         BOMInfo           `json:"bom"`
	Header      CARHeader         `json:"header"`
	Metadata    *ExtendedMetadata `json:"extended_metadata,omitempty"`
	KeyFormat   KeyFormat         `json:"key_format"`
	Appearances []Appearance      `json:"appearances,omitempty"`
	Facets      []Facet           `json:"facets"`
	Renditions  []Rendition       `json:"renditions"`
}

type CARHeader struct {
	Tag                 string `json:"tag"`
	CoreUIVersion       uint32 `json:"coreui_version"`
	StorageVersion      uint32 `json:"storage_version"`
	StorageTimestamp    uint32 `json:"storage_timestamp"`
	RenditionCount      uint32 `json:"rendition_count"`
	MainVersion         string `json:"main_version"`
	AssetStorageVersion string `json:"asset_storage_version"`
	UUID                string `json:"uuid"`
	AssociatedChecksum  uint32 `json:"associated_checksum"`
	SchemaVersion       uint32 `json:"schema_version"`
	ColorSpaceID        uint32 `json:"color_space_id"`
	KeySemantics        uint32 `json:"key_semantics"`
}

type ExtendedMetadata struct {
	Tag                       string `json:"tag"`
	ThinningArguments         string `json:"thinning_arguments,omitempty"`
	DeploymentPlatformVersion string `json:"deployment_platform_version,omitempty"`
	DeploymentPlatform        string `json:"deployment_platform,omitempty"`
	AuthoringTool             string `json:"authoring_tool,omitempty"`
}

type KeyFormat struct {
	Tag            string          `json:"tag"`
	Version        uint32          `json:"version"`
	Attributes     []AttributeType `json:"attributes"`
	AttributeNames []string        `json:"attribute_names"`
}

type AttributeType uint32

type AttributeValue struct {
	Type  AttributeType `json:"type"`
	Name  string        `json:"name"`
	Value uint16        `json:"value"`
}

type Appearance struct {
	Name string `json:"name"`
	ID   uint16 `json:"id"`
}

type Facet struct {
	Name       string           `json:"name"`
	HotSpotX   uint16           `json:"hot_spot_x,omitempty"`
	HotSpotY   uint16           `json:"hot_spot_y,omitempty"`
	Attributes []AttributeValue `json:"attributes"`
}

type Rendition struct {
	AssetName string           `json:"asset_name,omitempty"`
	Key       []AttributeValue `json:"key"`
	CSI       CSI              `json:"csi"`
}

type CSI struct {
	Tag              string         `json:"tag"`
	Version          uint32         `json:"version"`
	Flags            RenditionFlags `json:"flags"`
	Width            uint32         `json:"width"`
	Height           uint32         `json:"height"`
	ScaleFactor      uint32         `json:"scale_factor"`
	PixelFormat      string         `json:"pixel_format,omitempty"`
	ColorSpaceID     uint8          `json:"color_space_id"`
	ModificationTime uint32         `json:"modification_time"`
	Layout           uint16         `json:"layout"`
	LayoutName       string         `json:"layout_name"`
	Name             string         `json:"name,omitempty"`
	BitmapLengths    []uint32       `json:"bitmap_lengths,omitempty"`
	TLVs             []TLV          `json:"tlvs,omitempty"`
	Payload          Payload        `json:"payload"`
}

type RenditionFlags struct {
	Raw                           uint32 `json:"raw"`
	HeaderFlaggedFPO              bool   `json:"header_flagged_fpo"`
	ExcludedFromContrastFilter    bool   `json:"excluded_from_contrast_filter"`
	VectorBased                   bool   `json:"vector_based"`
	Opaque                        bool   `json:"opaque"`
	BitmapEncoding                uint8  `json:"bitmap_encoding"`
	OptOutOfThinning              bool   `json:"opt_out_of_thinning"`
	Flippable                     bool   `json:"flippable"`
	Tintable                      bool   `json:"tintable"`
	PreservedVectorRepresentation bool   `json:"preserved_vector_representation"`
}

type TLV struct {
	Type         uint32           `json:"type"`
	Name         string           `json:"name"`
	Length       uint32           `json:"length"`
	Text         string           `json:"text,omitempty"`
	Hex          string           `json:"hex,omitempty"`
	LinkedRect   *PixelRect       `json:"linked_rect,omitempty"`
	LinkedLayout *uint16          `json:"linked_layout,omitempty"`
	LinkedKey    []AttributeValue `json:"linked_key,omitempty"`
}

type PixelRect struct {
	X      uint32 `json:"x"`
	Y      uint32 `json:"y"`
	Width  uint32 `json:"width"`
	Height uint32 `json:"height"`
}

type Payload struct {
	Tag             string    `json:"tag,omitempty"`
	Version         uint32    `json:"version,omitempty"`
	Length          int       `json:"length"`
	DeclaredLength  uint32    `json:"declared_length,omitempty"`
	CompressionType *uint32   `json:"compression_type,omitempty"`
	Compression     string    `json:"compression,omitempty"`
	ColorSpaceID    *uint32   `json:"color_space_id,omitempty"`
	ColorComponents []float64 `json:"color_components,omitempty"`
	Gradient        *Gradient `json:"gradient,omitempty"`
	Data            []byte    `json:"-"`
}

type Gradient struct {
	Type  uint32         `json:"type"`
	Start [2]float32     `json:"start"`
	End   [2]float32     `json:"end"`
	Stops []GradientStop `json:"stops"`
}

type GradientStop struct {
	Location  float32 `json:"location"`
	ColorName string  `json:"color_name"`
}

// Open parses the CAR file at path.
func Open(path string) (*Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Parse(f, stat.Size())
}

// Parse parses a CAR file from a random-access reader.
func Parse(r io.ReaderAt, size int64) (*Catalog, error) {
	bom, err := ParseBOM(r, size)
	if err != nil {
		return nil, err
	}

	rawHeader, err := bom.NamedBlock("CARHEADER")
	if err != nil {
		return nil, err
	}
	header, order, err := parseCARHeader(rawHeader)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{BOM: bom.Info(), Header: header}

	if raw, err := bom.NamedBlock("EXTENDED_METADATA"); err == nil {
		metadata, err := parseExtendedMetadata(raw, order)
		if err != nil {
			return nil, err
		}
		catalog.Metadata = &metadata
	}

	rawKeyFormat, err := bom.NamedBlock("KEYFORMAT")
	if err != nil {
		return nil, err
	}
	catalog.KeyFormat, err = parseKeyFormat(rawKeyFormat, order)
	if err != nil {
		return nil, err
	}

	if _, ok := catalog.BOM.Variables["APPEARANCEKEYS"]; ok {
		entries, err := bom.TreeEntries("APPEARANCEKEYS")
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if len(entry.Value) < 2 {
				return nil, errors.New("CAR: appearance value is shorter than 2 bytes")
			}
			catalog.Appearances = append(catalog.Appearances, Appearance{Name: cString(entry.Key), ID: order.Uint16(entry.Value[:2])})
		}
	}

	facetEntries, err := bom.TreeEntries("FACETKEYS")
	if err != nil {
		return nil, err
	}
	for _, entry := range facetEntries {
		facet, err := parseFacet(cString(entry.Key), entry.Value, order)
		if err != nil {
			return nil, err
		}
		catalog.Facets = append(catalog.Facets, facet)
	}

	nameByIdentifier := make(map[uint16]string, len(catalog.Facets))
	for _, facet := range catalog.Facets {
		for _, attribute := range facet.Attributes {
			if attribute.Type == 17 {
				nameByIdentifier[attribute.Value] = facet.Name
			}
		}
	}

	renditionEntries, err := bom.TreeEntries("RENDITIONS")
	if err != nil {
		return nil, err
	}
	for i, entry := range renditionEntries {
		key, err := parseRenditionKey(entry.Key, catalog.KeyFormat.Attributes, order)
		if err != nil {
			return nil, fmt.Errorf("CAR: rendition %d key: %w", i, err)
		}
		csi, err := parseCSI(entry.Value, catalog.KeyFormat.Attributes, order)
		if err != nil {
			return nil, fmt.Errorf("CAR: rendition %d: %w", i, err)
		}
		rendition := Rendition{Key: key, CSI: csi}
		for _, attribute := range key {
			if attribute.Type == 17 {
				rendition.AssetName = nameByIdentifier[attribute.Value]
				break
			}
		}
		catalog.Renditions = append(catalog.Renditions, rendition)
	}
	if header.RenditionCount != 0 && uint32(len(catalog.Renditions)) != header.RenditionCount {
		return nil, fmt.Errorf("CAR: header declares %d renditions but tree contains %d", header.RenditionCount, len(catalog.Renditions))
	}
	return catalog, nil
}

func parseCARHeader(raw []byte) (CARHeader, binary.ByteOrder, error) {
	if len(raw) < 436 {
		return CARHeader{}, nil, fmt.Errorf("CAR: CARHEADER is only %d bytes", len(raw))
	}
	var order binary.ByteOrder
	switch string(raw[:4]) {
	case "RATC":
		order = binary.LittleEndian
	case "CTAR":
		order = binary.BigEndian
	default:
		return CARHeader{}, nil, fmt.Errorf("CAR: invalid CARHEADER magic %q", raw[:4])
	}
	h := CARHeader{
		Tag:                 canonicalFourCC(raw[:4], order),
		CoreUIVersion:       order.Uint32(raw[4:8]),
		StorageVersion:      order.Uint32(raw[8:12]),
		StorageTimestamp:    order.Uint32(raw[12:16]),
		RenditionCount:      order.Uint32(raw[16:20]),
		MainVersion:         cString(raw[20:148]),
		AssetStorageVersion: cString(raw[148:404]),
		UUID:                formatUUID(raw[404:420]),
		AssociatedChecksum:  order.Uint32(raw[420:424]),
		SchemaVersion:       order.Uint32(raw[424:428]),
		ColorSpaceID:        order.Uint32(raw[428:432]),
		KeySemantics:        order.Uint32(raw[432:436]),
	}
	return h, order, nil
}

func parseExtendedMetadata(raw []byte, order binary.ByteOrder) (ExtendedMetadata, error) {
	if len(raw) < 1028 {
		return ExtendedMetadata{}, fmt.Errorf("CAR: EXTENDED_METADATA is only %d bytes", len(raw))
	}
	return ExtendedMetadata{
		Tag:                       canonicalFourCC(raw[:4], order),
		ThinningArguments:         cString(raw[4:260]),
		DeploymentPlatformVersion: cString(raw[260:516]),
		DeploymentPlatform:        cString(raw[516:772]),
		AuthoringTool:             cString(raw[772:1028]),
	}, nil
}

func parseKeyFormat(raw []byte, order binary.ByteOrder) (KeyFormat, error) {
	if len(raw) < 12 {
		return KeyFormat{}, fmt.Errorf("CAR: KEYFORMAT is only %d bytes", len(raw))
	}
	count := order.Uint32(raw[8:12])
	if uint64(count) > uint64((len(raw)-12)/4) {
		return KeyFormat{}, fmt.Errorf("CAR: KEYFORMAT declares %d attributes but only %d fit", count, (len(raw)-12)/4)
	}
	format := KeyFormat{
		Tag: canonicalFourCC(raw[:4], order), Version: order.Uint32(raw[4:8]),
		Attributes: make([]AttributeType, count), AttributeNames: make([]string, count),
	}
	for i := range format.Attributes {
		format.Attributes[i] = AttributeType(order.Uint32(raw[12+i*4 : 16+i*4]))
		format.AttributeNames[i] = format.Attributes[i].String()
	}
	return format, nil
}

func parseFacet(name string, raw []byte, order binary.ByteOrder) (Facet, error) {
	if len(raw) < 6 {
		return Facet{}, fmt.Errorf("CAR: facet %q value is only %d bytes", name, len(raw))
	}
	count := int(order.Uint16(raw[4:6]))
	if count > (len(raw)-6)/4 {
		return Facet{}, fmt.Errorf("CAR: facet %q declares %d attributes but only %d fit", name, count, (len(raw)-6)/4)
	}
	facet := Facet{Name: name, HotSpotX: order.Uint16(raw[:2]), HotSpotY: order.Uint16(raw[2:4]), Attributes: make([]AttributeValue, count)}
	for i := range facet.Attributes {
		pos := 6 + i*4
		typ := AttributeType(order.Uint16(raw[pos : pos+2]))
		facet.Attributes[i] = AttributeValue{Type: typ, Name: typ.String(), Value: order.Uint16(raw[pos+2 : pos+4])}
	}
	return facet, nil
}

func parseRenditionKey(raw []byte, types []AttributeType, order binary.ByteOrder) ([]AttributeValue, error) {
	need := len(types) * 2
	if len(raw) < need {
		return nil, fmt.Errorf("requires %d bytes for %d attributes, got %d", need, len(types), len(raw))
	}
	key := make([]AttributeValue, len(types))
	for i, typ := range types {
		value := order.Uint16(raw[i*2 : i*2+2])
		key[i] = AttributeValue{Type: typ, Name: typ.String(), Value: value}
	}
	return key, nil
}

func parseCSI(raw []byte, keyTypes []AttributeType, order binary.ByteOrder) (CSI, error) {
	if len(raw) < csiFixedPrefixSize {
		return CSI{}, fmt.Errorf("CSI block is only %d bytes", len(raw))
	}
	flags := order.Uint32(raw[8:12])
	bitmapCount := order.Uint32(raw[172:176])
	if bitmapCount > uint32((len(raw)-csiFixedPrefixSize)/4) {
		return CSI{}, fmt.Errorf("CSI declares %d bitmap lengths but only %d fit", bitmapCount, (len(raw)-csiFixedPrefixSize)/4)
	}
	bitmapBytes := int(bitmapCount) * 4
	tlvLength := order.Uint32(raw[168:172])
	tlvStart := csiFixedPrefixSize + bitmapBytes
	if uint64(tlvLength) > uint64(len(raw)-tlvStart) {
		return CSI{}, fmt.Errorf("CSI TLV length %d exceeds remaining %d bytes", tlvLength, len(raw)-tlvStart)
	}
	payloadStart := tlvStart + int(tlvLength)

	csi := CSI{
		Tag:              canonicalFourCC(raw[:4], order),
		Version:          order.Uint32(raw[4:8]),
		Flags:            parseRenditionFlags(flags),
		Width:            order.Uint32(raw[12:16]),
		Height:           order.Uint32(raw[16:20]),
		ScaleFactor:      order.Uint32(raw[20:24]),
		PixelFormat:      canonicalFourCC(raw[24:28], order),
		ColorSpaceID:     uint8(order.Uint32(raw[28:32]) & 0xf),
		ModificationTime: order.Uint32(raw[32:36]),
		Layout:           order.Uint16(raw[36:38]),
		Name:             cString(raw[40:168]),
		BitmapLengths:    make([]uint32, bitmapCount),
	}
	csi.LayoutName = layoutName(csi.Layout)
	for i := range csi.BitmapLengths {
		pos := csiFixedPrefixSize + i*4
		csi.BitmapLengths[i] = order.Uint32(raw[pos : pos+4])
	}
	var err error
	csi.TLVs, err = parseTLVs(raw[tlvStart:payloadStart], keyTypes, order)
	if err != nil {
		return CSI{}, err
	}
	csi.Payload = parsePayload(raw[payloadStart:], order)
	return csi, nil
}

func parseRenditionFlags(raw uint32) RenditionFlags {
	return RenditionFlags{
		Raw: raw, HeaderFlaggedFPO: raw&1 != 0, ExcludedFromContrastFilter: raw&(1<<1) != 0,
		VectorBased: raw&(1<<2) != 0, Opaque: raw&(1<<3) != 0, BitmapEncoding: uint8((raw >> 4) & 0xf),
		OptOutOfThinning: raw&(1<<8) != 0, Flippable: raw&(1<<9) != 0, Tintable: raw&(1<<10) != 0,
		PreservedVectorRepresentation: raw&(1<<11) != 0,
	}
}

func parseTLVs(raw []byte, keyTypes []AttributeType, order binary.ByteOrder) ([]TLV, error) {
	var result []TLV
	for pos := 0; pos < len(raw); {
		if len(raw)-pos < 8 {
			if validTLVTrailer(raw[pos:], order) {
				break
			}
			return nil, fmt.Errorf("CSI TLV has %d trailing bytes: %x", len(raw)-pos, raw[pos:])
		}
		typ := order.Uint32(raw[pos : pos+4])
		length := order.Uint32(raw[pos+4 : pos+8])
		pos += 8
		if uint64(length) > uint64(len(raw)-pos) && length >= 8 && uint64(length-8) == uint64(len(raw)-pos) {
			// Some newer TLVs store their full record size instead of only
			// the value size used by older records.
			length -= 8
		}
		if uint64(length) > uint64(len(raw)-pos) && length&0x7ff == 0 && uint64(length>>11) == uint64(len(raw)-pos) {
			// Some catalogs encode the value size shifted left by 11 bits.
			length >>= 11
		}
		if uint64(length) > uint64(len(raw)-pos) {
			return nil, fmt.Errorf("CSI TLV type %d length %d exceeds remaining %d bytes: %x", typ, length, len(raw)-pos, raw[pos-8:])
		}
		value := raw[pos : pos+int(length)]
		pos += int(length)
		tlv := TLV{Type: typ, Name: tlvName(typ), Length: length}
		if typ == 1005 || typ == 1008 {
			tlv.Text = embeddedText(value)
		}
		if typ == 1010 && len(value) >= 30 {
			tlv.LinkedRect = &PixelRect{
				X: order.Uint32(value[8:12]), Y: order.Uint32(value[12:16]),
				Width: order.Uint32(value[16:20]), Height: order.Uint32(value[20:24]),
			}
			layout := order.Uint16(value[24:26])
			tlv.LinkedLayout = &layout
			keyLength := order.Uint32(value[26:30])
			if keyLength <= uint32(len(value)-30) {
				linkedKey, err := parseRenditionKeyTokens(value[30:30+int(keyLength)], order)
				if err == nil {
					tlv.LinkedKey = linkedKey
				}
			}
		}
		if tlv.Text == "" && len(value) != 0 {
			limit := len(value)
			if limit > 64 {
				limit = 64
			}
			tlv.Hex = hex.EncodeToString(value[:limit])
		}
		result = append(result, tlv)
	}
	return result, nil
}

func parseRenditionKeyTokens(raw []byte, order binary.ByteOrder) ([]AttributeValue, error) {
	if len(raw)%4 != 0 {
		return nil, fmt.Errorf("rendition key token list is %d bytes, want a multiple of 4", len(raw))
	}
	result := make([]AttributeValue, 0, len(raw)/4)
	for pos := 0; pos < len(raw); pos += 4 {
		typ := AttributeType(order.Uint16(raw[pos : pos+2]))
		value := order.Uint16(raw[pos+2 : pos+4])
		if typ == 0 && value == 0 {
			break
		}
		result = append(result, AttributeValue{Type: typ, Name: typ.String(), Value: value})
	}
	return result, nil
}

// parsePayload dispatches only on the storage wrapper tag. Layout semantics,
// bitmap compression, and decoded pixel interpretation belong to later layers.
func parsePayload(raw []byte, order binary.ByteOrder) Payload {
	payload := Payload{Length: len(raw), Data: raw}
	if len(raw) < 4 {
		return payload
	}
	payload.Tag = canonicalFourCC(raw[:4], order)
	if len(raw) >= 8 {
		payload.Version = order.Uint32(raw[4:8])
	}
	switch payload.Tag {
	case "RAWD":
		if len(raw) >= 12 {
			payload.DeclaredLength = order.Uint32(raw[8:12])
		}
	case "CELM":
		if len(raw) >= 16 {
			compression := order.Uint32(raw[8:12])
			payload.CompressionType = &compression
			payload.Compression = compressionName(compression)
			payload.DeclaredLength = order.Uint32(raw[12:16])
		}
	case "COLR":
		if len(raw) >= 16 {
			colorSpace := order.Uint32(raw[8:12])
			payload.ColorSpaceID = &colorSpace
			count := order.Uint32(raw[12:16])
			if count <= uint32((len(raw)-16)/8) {
				payload.ColorComponents = make([]float64, count)
				for i := range payload.ColorComponents {
					bits := order.Uint64(raw[16+i*8 : 24+i*8])
					payload.ColorComponents[i] = math.Float64frombits(bits)
				}
			}
		}
	case "ARGG":
		payload.Gradient = parseGradient(raw, order)
	}
	return payload
}

func parseGradient(raw []byte, order binary.ByteOrder) *Gradient {
	if len(raw) < 32 {
		return nil
	}
	count := order.Uint32(raw[4:8])
	if uint64(count) > uint64((len(raw)-32)/8) {
		return nil
	}
	gradient := &Gradient{
		Type: order.Uint32(raw[8:12]),
		Start: [2]float32{
			math.Float32frombits(order.Uint32(raw[16:20])),
			math.Float32frombits(order.Uint32(raw[20:24])),
		},
		End: [2]float32{
			math.Float32frombits(order.Uint32(raw[24:28])),
			math.Float32frombits(order.Uint32(raw[28:32])),
		},
		Stops: make([]GradientStop, 0, count),
	}
	pos := 32
	for i := uint32(0); i < count; i++ {
		if len(raw)-pos < 8 {
			return nil
		}
		location := math.Float32frombits(order.Uint32(raw[pos : pos+4]))
		nameLength := order.Uint32(raw[pos+4 : pos+8])
		pos += 8
		if uint64(nameLength) > uint64(len(raw)-pos) {
			return nil
		}
		gradient.Stops = append(gradient.Stops, GradientStop{
			Location: location, ColorName: cString(raw[pos : pos+int(nameLength)]),
		})
		pos += int(nameLength)
	}
	return gradient
}

func allZero(raw []byte) bool {
	for _, value := range raw {
		if value != 0 {
			return false
		}
	}
	return true
}

func validTLVTrailer(raw []byte, order binary.ByteOrder) bool {
	if allZero(raw) {
		return true
	}
	return len(raw) == 4 && order.Uint32(raw) == 3
}

func (t AttributeType) String() string {
	names := map[AttributeType]string{
		0: "Theme Look", 1: "Element", 2: "Part", 3: "Size", 4: "Direction", 5: "Placeholder",
		6: "Value", 7: "Theme Appearance", 8: "Dimension 1", 9: "Dimension 2", 10: "State", 11: "Layer",
		12: "Scale", 13: "Localization", 14: "Presentation State", 15: "Idiom", 16: "Subtype", 17: "Identifier",
		18: "Previous Value", 19: "Previous State", 20: "Horizontal Size Class", 21: "Vertical Size Class",
		22: "Memory Level Class", 23: "Graphics Feature Set Class", 24: "Display Gamut", 25: "Deployment Target",
		26: "Localization", 27: "Graphics Fall Back", 28: "Performance Class",
	}
	if name, ok := names[t]; ok {
		return name
	}
	return fmt.Sprintf("Unknown %d", t)
}

func layoutName(layout uint16) string {
	names := map[uint16]string{
		7: "Text Effect", 9: "Vector", 10: "One Part Fixed Size", 11: "One Part Tile", 12: "One Part Scale",
		20: "Three Part Horizontal Tile", 21: "Three Part Horizontal Scale", 22: "Three Part Horizontal Uniform",
		23: "Three Part Vertical Tile", 24: "Three Part Vertical Scale", 25: "Three Part Vertical Uniform",
		30: "Nine Part Tile", 31: "Nine Part Scale", 32: "Nine Part Horizontal Tile Vertical Tile",
		33: "Nine Part Horizontal Tile Vertical Scale", 34: "Nine Part Horizontal Scale Vertical Tile", 40: "Many Part",
		50: "Animation Filmstrip", 1000: "Data", 1001: "External Link", 1002: "Layer Stack", 1003: "Internal Reference",
		1004: "Packed Image", 1005: "Named Content", 1006: "Thinning Placeholder", 1007: "Texture",
		1008: "Texture Image", 1009: "Color", 1010: "Multisize Image Set", 1011: "Layer Reference",
		1012: "Content Rendition", 1013: "Recognition Object", 1019: "Icon Image Stack",
		1020: "Icon Group", 1021: "Named Gradient",
	}
	if name, ok := names[layout]; ok {
		return name
	}
	return fmt.Sprintf("Unknown %d", layout)
}

func tlvName(typ uint32) string {
	names := map[uint32]string{
		1001: "Slices", 1003: "Metrics", 1004: "Blend Mode and Opacity", 1005: "UTI", 1006: "EXIF Orientation",
		1007: "Unknown 1007", 1008: "External Tags", 1009: "Frame", 1010: "Internal Link", 1012: "Layer Stack",
	}
	if name, ok := names[typ]; ok {
		return name
	}
	return fmt.Sprintf("Unknown %d", typ)
}

func compressionName(kind uint32) string {
	names := []string{"uncompressed", "rle", "zip", "lzvn", "lzfse", "jpeg-lzfse", "blurred", "astc", "palette-img", "hevc", "deepmap-lzfse", "deepmap2", "dxtc"}
	if int(kind) < len(names) {
		return names[kind]
	}
	return fmt.Sprintf("unknown-%d", kind)
}

func canonicalFourCC(raw []byte, order binary.ByteOrder) string {
	if len(raw) < 4 || raw[0]|raw[1]|raw[2]|raw[3] == 0 {
		return ""
	}
	text := string(raw[:4])
	if order != binary.LittleEndian {
		return text
	}
	known := map[string]string{
		"RATC": "CTAR", "ISTC": "CTSI", "DWAR": "RAWD", "MLEC": "CELM", "RLOC": "COLR", "tmfk": "kfmt",
		"BGRA": "ARGB", "ATAD": "DATA", "GEPJ": "JPEG", "FIEH": "HEIF", "WBGR": "RGBW", " 8AG": "GA8 ",
		"61AG": "GA16", "5BGR": "RGB5", "MSIS": "SISM", "KLNI": "INLK",
	}
	if canonical, ok := known[text]; ok {
		return canonical
	}
	return text
}

func cString(raw []byte) string {
	if i := strings.IndexByte(string(raw), 0); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(string(raw))
}

func embeddedText(raw []byte) string {
	best := ""
	for start := 0; start < len(raw); {
		for start < len(raw) && (raw[start] < 0x20 || raw[start] > 0x7e) {
			start++
		}
		end := start
		for end < len(raw) && raw[end] >= 0x20 && raw[end] <= 0x7e {
			end++
		}
		if end-start >= 3 && end-start > len(best) {
			best = string(raw[start:end])
		}
		start = end + 1
	}
	return best
}

func formatUUID(raw []byte) string {
	if len(raw) < 16 {
		return ""
	}
	text := hex.EncodeToString(raw[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", text[:8], text[8:12], text[12:16], text[16:20], text[20:32])
}
