# carfile-go

`carfile-go` is a pure Go library and CLI for parsing and extracting Apple's compiled Asset Catalog (`Assets.car`) format. It has no cgo or platform-specific runtime dependencies.

## CLI

Download the archive for your platform and architecture from the GitHub
Release. Darwin, Linux, and Windows packages are published for both amd64 and
arm64. Each release includes a checksum manifest and signed GitHub build
provenance. Verify both before running the binary:

```sh
grep ' carfile_0.8.0_darwin_arm64.tar.gz$' checksums.txt | shasum -a 256 -c -
gh attestation verify carfile_0.8.0_darwin_arm64.tar.gz \
  --repo devcxm/carfile-go
```

Build the command:

```sh
go build -o carfile ./cmd/carfile
```

Running it with only an input file recovers every logical resource into an `<name>-extracted` directory beside the input:

```sh
carfile Assets.car
```

Options:

```text
Usage:
  carfile [options] <Assets.car>

Options:
  -o, --output DIR       Output directory
  -f, --format FORMAT    resources (default), xcassets, raw, png, or json
  -i, --include PATTERN  Include asset/file glob; may be repeated
  -q, --quiet            Disable progress output
  -v, --version          Print version
  -h, --help             Show help
```

Examples:

```sh
carfile Assets.car
carfile -o output Assets.car
carfile -i AppIcon Assets.car
carfile -i 'myBannerImage_*' -i '*@2x.png' Assets.car
carfile --format xcassets --output restored Assets.car
carfile -f raw -o payloads Assets.car
carfile -f json -o metadata Assets.car
```

### Output formats

| Format | Output |
| --- | --- |
| `resources` | All logical resources. Single Data resources keep their original file name; rendition families are grouped by asset name. Packed atlas entries are cropped into individual files. This is the default. |
| `xcassets` | A flat, compilable `Assets.xcassets` with generated `Contents.json` files. |
| `raw` | Physical CAR payloads with wrappers removed where possible. Compressed data stays compressed. |
| `png` | Every directly stored compressed bitmap as PNG, including packed atlas images. |
| `json` | Parsed CAR metadata in `catalog.json`. |

Every extraction directory includes a machine-readable manifest except the JSON format, whose output is already self-describing.

### Selective extraction

`--include`/`-i` accepts Go-style glob patterns and can be repeated. Patterns are ORed and are matched against the logical asset name, rendition filename, and `asset/file` path. Filtering happens before bitmap decompression and PNG encoding.

```sh
# Both @2x and @3x renditions from one logical asset
carfile -i myBannerImage_de Assets.car

# One exact rendition
carfile -i myBannerImage_de@2x.png Assets.car

# Several asset families
carfile -i 'HomePage_*' -i 'AppIcon' Assets.car

# Precise asset/file selection
carfile -i 'AppIcon/Icon-iPhone-60@3x.png' Assets.car
```

The same filter is available to library callers through `ExtractOptions.Includes`. Include filters apply to `resources`, `xcassets`, `raw`, and `png`; JSON output always describes the complete catalog.

For a logical image stored inside a packed atlas, use `resources` or `xcassets`; these formats resolve the internal link and crop the requested image. The `png` format intentionally operates on directly stored physical bitmap renditions, while `raw` operates on physical payloads.

### Progress

The CLI displays the current percentage, item count, asset name, and rendition filename while extracting. Interactive terminals reuse one line; redirected output uses one event per line. Use `--quiet`/`-q` to disable progress.

Library callers can receive the same synchronous, serial progress events:

```go
result, err := carfile.ExtractFile("Assets.car", carfile.ExtractOptions{
    Format:          carfile.FormatResources,
    OutputDirectory: "output",
    Progress: func(event carfile.Progress) {
        log.Printf("%d/%d %s/%s", event.Current, event.Total, event.AssetName, event.FileName)
    },
})
```

## Library

The module root is a regular importable Go package; the CLI is isolated under `cmd/carfile`.

```go
package main

import (
    "log"

    carfile "github.com/devcxm/carfile-go"
)

func main() {
    result, err := carfile.ExtractFile("Assets.car", carfile.ExtractOptions{
        Format:          carfile.FormatXCAssets,
        OutputDirectory: "restored",
        Includes:        []string{"AppIcon", "myBannerImage_*"},
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("wrote %d files to %s", result.Written, result.OutputDirectory)
}
```

For parsing without immediately exporting:

```go
catalog, err := carfile.Open("Assets.car")
if err != nil {
    return err
}

result, err := catalog.Export(carfile.ExtractOptions{
    Format:          carfile.FormatResources,
    OutputDirectory: "output",
})
```

Individual codecs are independently importable:

```go
import (
	"github.com/devcxm/carfile-go/codec/deepmap"
	"github.com/devcxm/carfile-go/codec/deepmap2"
	"github.com/devcxm/carfile-go/codec/kcbc"
	"github.com/devcxm/carfile-go/codec/lzfse"
	"github.com/devcxm/carfile-go/codec/lzvn"
	"github.com/devcxm/carfile-go/codec/palette"
	"github.com/devcxm/carfile-go/codec/rle"
)
```

## Supported formats

See [the implemented Assets.car format model](docs/assets-car-format.md) for
binary layouts, compatibility levels, current codec matrices, and the
extension procedure. A successful logical `resources` extraction does not by
itself imply that every physical rendition is supported; use `png` when
checking physical bitmap-codec coverage.

The parser reads:

- BOMStore headers, block indices, variables, and linked B+ tree leaves;
- `CARHEADER`, `EXTENDED_METADATA`, and `KEYFORMAT`;
- `APPEARANCEKEYS`, `FACETKEYS`, and `RENDITIONS`;
- CSI headers, TLV metadata, internal links, and `RAWD`, `CELM`, `COLR`, and named-gradient payloads.

The decoder supports:

- LZFSE `bvx2`, raw `bvx-`, and embedded LZVN `bvxn` streams;
- raw LZVN instruction streams;
- uncompressed, RLE, gzip, and LZFSE bitmaps, including KCBC horizontal chunks and row-padding removal;
- row-oriented RLE and quantized `palette-img` bitmaps;
- legacy Deepmap and Deepmap2 default, lossless, and palette encodings;
- ARGB/BGRA, GA8, and wide-gamut RGBW pixels;
- packed-image links, including lower-left coordinate conversion and atlas cropping.

Named colors support RGB and grayscale component sets. Named gradients are
recovered as semantic JSON; icon image stack and icon group descriptors are
retained as metadata alongside their independently recovered images.

Original RAWD files such as SVG and JPEG are copied byte-for-byte. Compiled bitmaps are re-encoded as PNG; their original PNG compression, ancillary metadata, and source group hierarchy are not present in the CAR and cannot be reconstructed exactly.
