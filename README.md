# carfile-go

`carfile-go` is a pure-Go library and CLI for parsing and extracting Apple's compiled Asset Catalog (`Assets.car`) format. It uses only the Go standard library and does not call CoreUI, `assetutil`, cgo, or third-party codecs at runtime.

## CLI

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
  -v, --version          Print version
  -h, --help             Show help
```

Examples:

```sh
carfile Assets.car
carfile -o output Assets.car
carfile --format xcassets --output restored Assets.car
carfile -f raw -o payloads Assets.car
carfile -f json -o metadata Assets.car
```

### Output formats

| Format | Output |
| --- | --- |
| `resources` | All logical resources, grouped by asset name. Packed atlas entries are cropped into individual files. This is the default. |
| `xcassets` | A flat, compilable `Assets.xcassets` with generated `Contents.json` files. |
| `raw` | Physical CAR payloads with wrappers removed where possible. Compressed data stays compressed. |
| `png` | Every directly stored compressed bitmap as PNG, including packed atlas images. |
| `json` | Parsed CAR metadata in `catalog.json`. |

Every extraction directory includes a machine-readable manifest except the JSON format, whose output is already self-describing.

## Library

The module root is a regular importable Go package; the CLI is isolated under `cmd/carfile`.

```go
package main

import (
    "log"

    carfile "carfile-go"
)

func main() {
    result, err := carfile.ExtractFile("Assets.car", carfile.ExtractOptions{
        Format:          carfile.FormatXCAssets,
        OutputDirectory: "restored",
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
    "carfile-go/codec/deepmap2"
    "carfile-go/codec/kcbc"
    "carfile-go/codec/lzfse"
    "carfile-go/codec/lzvn"
)
```

Before publishing the project, replace the temporary `carfile-go` module path in `go.mod` with the final repository URL. The package structure and public interfaces do not otherwise need to change.

## Supported formats

The parser reads:

- BOMStore headers, block indices, variables, and linked B+ tree leaves;
- `CARHEADER`, `EXTENDED_METADATA`, and `KEYFORMAT`;
- `APPEARANCEKEYS`, `FACETKEYS`, and `RENDITIONS`;
- CSI headers, TLV metadata, internal links, and common `RAWD`, `CELM`, and `COLR` payloads.

The decoder supports:

- LZFSE `bvx2`, raw `bvx-`, and embedded LZVN `bvxn` streams;
- raw LZVN instruction streams;
- KCBC horizontal bitmap chunks and row-padding removal;
- Deepmap2 default, lossless, and palette encodings;
- ARGB/BGRA and GA8 pixels;
- packed-image links, including lower-left coordinate conversion and atlas cropping.

Original RAWD files such as SVG and JPEG are copied byte-for-byte. Compiled bitmaps are re-encoded as PNG; their original PNG compression, ancillary metadata, and source group hierarchy are not present in the CAR and cannot be reconstructed exactly.

## References and acknowledgements

This project benefited from the following public research and reference implementations:

- [Timac — Reverse engineering the `.car` file format](https://blog.timac.org/2018/1018-reverse-engineering-the-car-file-format/), the foundational walkthrough of BOM and compiled Asset Catalog structures.
- [DBG.RE — A Deep Dive into Apple's `.car` File Format](https://dbg.re/posts/car-file-format/), especially the CSI, internal-link, compression, and KCBC format analysis.
- [Apple — LZFSE reference implementation](https://github.com/lzfse/lzfse), used as the authoritative algorithm reference for the pure-Go LZFSE and LZVN decoders.

Many thanks to the authors and maintainers for publishing their research and source code. `carfile-go` is an independent pure-Go implementation and does not copy or invoke Apple's private CoreUI framework.
