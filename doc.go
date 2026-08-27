// Package carfile parses and extracts Apple's compiled Asset Catalog files.
//
// Asset formats are decoded in pure Go without cgo or platform-specific
// runtime dependencies.
//
// A rendition is interpreted through independent layers: layout describes its
// semantic role, the payload tag describes its storage wrapper, compression
// describes byte decoding, and pixel format describes the decoded channel
// layout. New compatibility should normally extend only the layer that
// introduced an unknown value. The complete implemented format model and
// extension procedure are documented in docs/assets-car-format.md.
//
// High-level callers normally use ExtractFile. Callers that need metadata or
// repeated exports can use Open followed by methods on Catalog. Specialized
// compression formats are also exposed as importable packages under codec/.
package carfile
