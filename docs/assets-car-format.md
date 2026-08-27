# Assets.car format model

This document describes the binary structures and compatibility contract
implemented by carfile-go. It is complete for the structures the current
parser understands; unverified fields are marked as unknown or reserved.

The format is layered. Support for one layer does not imply support for every
value in another layer:

```text
BOMStore container
  -> named CAR blocks and B+ trees
    -> rendition key + CSI record
      -> layout + flags + TLVs
        -> payload tag
          -> bitmap compression
            -> decoded pixel format
```

A rendition capability is therefore identified by the tuple:

```text
(layout, payload tag, compression, pixel format)
```

When adding support, extend only the layer that introduces the unknown value.

## Compatibility levels

The tables below use four compatibility levels:

- **Decoded**: the structure is interpreted and converted to a portable form.
- **Pass-through**: original bytes are recovered without interpreting their
  internal file format.
- **Metadata-only**: semantic metadata is written, but no standalone image
  exists in that rendition.
- **Unsupported**: metadata or raw bytes may still be visible through `json`
  or `raw`, but the default logical recovery cannot use that rendition alone.

`resources` may succeed when one physical rendition is unsupported if another
rendition represents the same logical resource. `png` reports every physical
CELM bitmap independently and is the authoritative physical-codec check.

## Byte order and FourCC normalization

The outer BOMStore container is big-endian. CAR blocks declare their byte
order through the CARHEADER magic:

- `RATC`: little-endian CAR data, normalized to `CTAR`.
- `CTAR`: big-endian CAR data.

FourCC values in little-endian CAR data can appear byte-reversed. The parser
normalizes known values before dispatch. Examples include `DWAR` to `RAWD`,
`MLEC` to `CELM`, `RLOC` to `COLR`, `BGRA` to `ARGB`, and ` 8AG` to `GA8 `.

Unknown FourCC values are preserved verbatim.

## BOMStore container

### Header

The 32-byte BOMStore header is big-endian:

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 8 | ASCII magic `BOMStore` |
| 8 | 4 | version; supported value is `1` |
| 12 | 4 | declared block count |
| 16 | 4 | block-index offset |
| 20 | 4 | block-index length |
| 24 | 4 | variables-table offset |
| 28 | 4 | variables-table length |

All offset-plus-length ranges are checked against the input file using
overflow-safe arithmetic.

### Block index

The block index begins with a big-endian `uint32` pointer count, followed by
that many entries:

| Size | Field |
| ---: | --- |
| 4 | block offset |
| 4 | block length |

Block zero must be `{0, 0}`. `{0xffffffff, 0xffffffff}` is an unused-block
sentinel: it is valid in the index but cannot be read as a block.

### Variables table

The variables table begins with a big-endian `uint32` entry count. Each entry
contains:

| Size | Field |
| ---: | --- |
| 4 | referenced block ID |
| 1 | variable-name byte length |
| N | variable-name bytes |

Referenced block IDs must exist in the block index.

### B+ tree

A named tree header is at least 21 bytes:

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 4 | ASCII magic `tree` |
| 4 | 4 | big-endian version; supported value is `1` |
| 8 | 4 | root block ID |
| 12 | 4 | block size |
| 16 | 4 | path/entry count |
| 20 | 1 | flags |

Inline-key trees (`flags & 4`) are unsupported. A tree node starts with:

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 2 | leaf flag (`0` or `1`) |
| 2 | 2 | entry count |
| 4 | 4 | forward node ID |
| 8 | 4 | backward node ID |
| 12 | 8 × N | value-block ID, then key-block ID |

Branch traversal follows the first value block until a leaf is reached. Leaf
traversal follows forward links. Cycles and entry-count mismatches are errors.

## Named CAR data

The parser consumes these BOM variables and trees:

- `CARHEADER` (required)
- `EXTENDED_METADATA` (optional)
- `KEYFORMAT` (required)
- `APPEARANCEKEYS` (optional)
- `FACETKEYS` (required)
- `RENDITIONS` (required)

### CARHEADER

CARHEADER is at least 436 bytes. Numeric fields use the CAR byte order:

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 4 | CAR magic and byte-order marker |
| 4 | 4 | format/runtime version |
| 8 | 4 | storage version |
| 12 | 4 | storage timestamp |
| 16 | 4 | rendition count |
| 20 | 128 | main version string |
| 148 | 256 | asset storage version string |
| 404 | 16 | UUID bytes |
| 420 | 4 | associated checksum |
| 424 | 4 | schema version |
| 428 | 4 | color-space ID |
| 432 | 4 | key semantics |

Fixed strings are NUL-terminated within their field and trimmed.

### EXTENDED_METADATA

The optional block is at least 1028 bytes:

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 4 | tag |
| 4 | 256 | thinning arguments |
| 260 | 256 | deployment platform version |
| 516 | 256 | deployment platform |
| 772 | 256 | authoring tool |

### KEYFORMAT and rendition keys

KEYFORMAT contains a tag, version, attribute count, and `uint32` attribute
types. Each rendition key then stores one `uint16` value per declared type.

Recognized attribute IDs are 0 through 28. Unknown IDs are preserved with an
`Unknown N` name. Important attributes used during recovery include:

- 7: appearance
- 12: scale
- 15: idiom
- 17: logical asset identifier
- 24: display gamut

### APPEARANCEKEYS and FACETKEYS

Appearance values begin with a CAR-order `uint16` ID; the tree key is the
appearance name. Facet values contain hot-spot coordinates, an attribute
count, and `(uint16 type, uint16 value)` pairs. Identifier attribute 17 joins
a rendition key to its logical asset name.

## CSI rendition record

A CSI value has a 180-byte fixed prefix followed by bitmap lengths, TLVs, and
the payload:

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 4 | CSI tag (`ISTC` normalizes to `CTSI`) |
| 4 | 4 | version |
| 8 | 4 | flags |
| 12 | 4 | pixel width |
| 16 | 4 | pixel height |
| 20 | 4 | scale factor in hundredths (`200` = 2x) |
| 24 | 4 | pixel-format FourCC |
| 28 | 4 | color-space value; low four bits are exposed in CSI metadata |
| 32 | 4 | modification time |
| 36 | 2 | layout ID |
| 38 | 2 | unknown/reserved |
| 40 | 128 | NUL-terminated rendition name |
| 168 | 4 | total TLV byte length |
| 172 | 4 | bitmap-length count |
| 176 | 4 | unknown/reserved |
| 180 | 4 × N | bitmap lengths |
| ... | TLV length | TLV records |
| ... | remaining | payload |

### Flags

The parser exposes these bits:

| Bit(s) | Meaning |
| --- | --- |
| 0 | header-flagged FPO |
| 1 | excluded from contrast filter |
| 2 | vector based |
| 3 | opaque |
| 4–7 | bitmap encoding nibble |
| 8 | opt out of thinning |
| 9 | flippable |
| 10 | tintable |
| 11 | preserve vector representation |

Unknown flag bits remain available in the raw flags value.

## TLV records

The normal TLV representation is:

| Size | Field |
| ---: | --- |
| 4 | type |
| 4 | value length |
| N | value bytes |

Known types include slices (1001), metrics (1003), blend/opacity (1004), UTI
(1005), EXIF orientation (1006), external tags (1008), frame (1009), internal
link (1010), and layer stack (1012). Unknown types preserve up to the first 64
value bytes as hexadecimal metadata.

Observed compatible length variants are accepted only when their decoded
value length exactly matches the remaining TLV region:

- length includes the eight-byte TLV header;
- value length is stored shifted left by 11 bits.

Zero alignment bytes and a four-byte trailer with value `3` are accepted.
Other incomplete or overlong records remain errors.

### Internal link (type 1010)

For values of at least 30 bytes, the parser reads:

| Offset | Size | Field |
| ---: | ---: | --- |
| 8 | 4 | crop X |
| 12 | 4 | crop Y |
| 16 | 4 | crop width |
| 20 | 4 | crop height |
| 24 | 2 | linked layout |
| 26 | 4 | linked-key byte length |
| 30 | N | `(uint16 type, uint16 value)` key tokens |

Crop coordinates use a lower-left origin. PNG output converts them to Go's
upper-left image origin before cropping the linked packed image.

## Payloads

### RAWD: original data

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 4 | `RAWD` tag |
| 4 | 4 | version |
| 8 | 4 | data length |
| 12 | N | original or LZFSE-compressed bytes |

Original bytes are pass-through resources. File names, pixel-format markers,
and UTI TLVs determine the output extension. LZFSE-wrapped RAWD content is
decoded before recovery. Unknown original file formats are still copied; the
openable flag is only a convenience classification.

### CELM: bitmap data

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 4 | `CELM` tag |
| 4 | 4 | version |
| 8 | 4 | compression ID |
| 12 | 4 | declared data length |
| 16 | N | encoded bitmap bytes |

The compression and pixel-format tables below jointly determine whether the
payload can be converted to PNG.

### COLR: named color

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 4 | `COLR` tag |
| 4 | 4 | version |
| 8 | 4 | color-space ID |
| 12 | 4 | component count |
| 16 | 8 × N | floating-point components |

Two components are white and alpha. Four components are red, green, blue, and
alpha. Known color spaces include sRGB (1), gray gamma 2.2 (2), Display P3
(3), extended sRGB (4), and extended gray (6). Unknown IDs are preserved.

### ARGG: named gradient

| Offset | Size | Field |
| ---: | ---: | --- |
| 0 | 4 | `ARGG` tag |
| 4 | 4 | stop count |
| 8 | 4 | gradient type |
| 12 | 4 | unknown/reserved |
| 16 | 4 | start X (`float32`) |
| 20 | 4 | start Y (`float32`) |
| 24 | 4 | end X (`float32`) |
| 28 | 4 | end Y (`float32`) |
| 32 | variable | gradient stops |

Each stop contains a `float32` location, a `uint32` color-name length, and a
NUL-terminated color-resource name. Resources output semantic
`.gradient.json` metadata.

### Other tags

`SISM` has been observed but has no semantic decoder. Unknown payload tags are
preserved by `raw` output using a tag-derived extension when bytes exist.

## Bitmap compression matrix

The compression ID is independent of layout and pixel format.

| ID | Name | Current level | Notes |
| ---: | --- | --- | --- |
| 0 | uncompressed | Decoded | tight or row-padded pixels |
| 1 | rle | Decoded | row table plus literal/repeat packets |
| 2 | zip | Decoded | direct gzip or KCBC chunks containing gzip |
| 3 | lzvn | Unsupported in CELM | standalone LZVN codec is available |
| 4 | lzfse | Decoded | direct LZFSE or KCBC chunks containing LZFSE |
| 5 | jpeg-lzfse | Unsupported | known physical-codec gap |
| 6 | blurred | Unsupported | raw export remains available |
| 7 | astc | Unsupported | raw export remains available |
| 8 | palette-img | Decoded | quantized palette bitmap |
| 9 | hevc | Unsupported | raw export remains available |
| 10 | deepmap-lzfse | Decoded | legacy dmap, direct or chunked |
| 11 | deepmap2 | Decoded | raw, default, lossless, palette, and chunks |
| 12 | dxtc | Unsupported | raw export remains available |

KCBC is a horizontal chunk container rather than a separate CELM compression
ID. Each chunk declares its row count and compressed byte length. Decoded row
padding is removed while the chunks are assembled.

## Pixel-format matrix

| FourCC | Storage | Current level |
| --- | --- | --- |
| `ARGB` / stored `BGRA` | 8-bit blue, green, red, alpha | Decoded |
| `GA8 ` | 8-bit gray, alpha | Decoded |
| `RGBW` | four little-endian 16-bit wide components | Decoded |
| `GA16` | observed marker | Unsupported for PNG conversion |
| `RGB5` | observed marker | Unsupported for PNG conversion |
| `DATA` | non-bitmap resource marker | Pass-through |
| `JPEG` | original JPEG marker | Pass-through |
| `HEIF` | original HEIF marker | Pass-through |

Deepmap payloads also carry a numeric pixel-format code. Current decoders
handle one-, two-, three-, and four-component 8-bit layouts plus four-component
wide layouts where implemented; final PNG conversion still requires a CSI
pixel format listed as Decoded above.

## Rendition layout matrix

Layout describes semantic use, not compression. Most image layouts use the
generic CELM decoder or RAWD pass-through path.

| ID | Name | Recovery behavior |
| ---: | --- | --- |
| 7 | Text Effect | generic payload behavior |
| 9 | Vector | RAWD pass-through when present |
| 10 | One Part Fixed Size | generic bitmap behavior |
| 11 | One Part Tile | generic bitmap behavior |
| 12 | One Part Scale | generic bitmap behavior |
| 20–22 | Three Part Horizontal variants | generic bitmap behavior |
| 23–25 | Three Part Vertical variants | generic bitmap behavior |
| 30–34 | Nine Part variants | generic bitmap behavior |
| 40 | Many Part | generic payload behavior |
| 50 | Animation Filmstrip | generic bitmap behavior |
| 1000 | Data | RAWD pass-through; a single data asset keeps its file name |
| 1001 | External Link | metadata/raw behavior |
| 1002 | Layer Stack | generic payload behavior |
| 1003 | Internal Reference | linked atlas decode and crop |
| 1004 | Packed Image | generic bitmap decode; supplies internal references |
| 1005 | Named Content | generic payload behavior |
| 1006 | Thinning Placeholder | metadata/raw behavior |
| 1007 | Texture | generic bitmap behavior |
| 1008 | Texture Image | generic bitmap behavior |
| 1009 | Color | decoded COLR color resource |
| 1010 | Multisize Image Set | marks application icon families |
| 1011 | Layer Reference | generic payload behavior |
| 1012 | Content Rendition | generic payload behavior |
| 1013 | Recognition Object | generic payload behavior |
| 1019 | Icon Image Stack | metadata-only JSON when RAWD has no data |
| 1020 | Icon Group | metadata-only JSON when RAWD has no data |
| 1021 | Named Gradient | decoded ARGG metadata JSON |

Layout 1017 has been observed with RAWD vector resources. Its original bytes
are recoverable by the generic RAWD path, but its semantic name remains
unknown. Unknown layouts can still succeed through RAWD pass-through or CELM
bitmap decoding; a layout with neither usable path is unsupported.

## Output behavior

| Output | Contract |
| --- | --- |
| `resources` | recover logical resources, resolve atlas links, decode bitmaps, pass through originals, write structured metadata |
| `xcassets` | create a compilable catalog for representable images and colors; retain additional structured metadata files |
| `raw` | unwrap physical payloads where possible without requiring semantic or bitmap support |
| `png` | decode every physical CELM bitmap independently; unsupported codecs are reported in the manifest |
| `json` | serialize parsed container, key, rendition, TLV, and payload metadata |

Logical recovery groups renditions by asset and rendition identity. When
duplicates exist, directly recoverable original data is preferred over linked
images and compressed bitmap fallbacks. This can make `resources` succeed even
when `png` reports an unsupported physical duplicate.

## Extension procedure

For a new failure, record the capability tuple and extend the narrowest layer:

1. Unknown BOM/CAR structure: update bounds-checked container parsing.
2. Unknown TLV: preserve its bytes first; add semantic fields only when the
   structure is stable.
3. Unknown payload tag: add header parsing and decide decoded, pass-through,
   metadata-only, or unsupported behavior.
4. Unknown compression: add a portable decoder and wire its CELM ID into
   `DecodeRenditionImage`.
5. Unknown pixel format: define storage size, channel order, and conversion.
6. Unknown layout: add a name; add special recovery only when generic RAWD or
   CELM behavior is insufficient.

Every extension must include:

- a minimal unit regression for the affected layer;
- a malformed/truncated-input check where applicable;
- an update to `compatibility_test.go` and the matrices in this document;
- logical `resources` verification and physical `png` verification when the
  change affects bitmaps;
- portable builds with `CGO_ENABLED=0`.

Do not infer support from a successful logical extraction alone. Check the
manifest's failure count for both the logical and physical paths.
