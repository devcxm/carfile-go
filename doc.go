// Package carfile parses and extracts Apple's compiled Asset Catalog files.
//
// Most formats are decoded in pure Go. On Darwin with cgo enabled, Deepmap
// variants use the system Accelerate framework for CoreUI-compatible output.
// High-level callers normally use ExtractFile. Callers that need metadata or
// repeated exports can use Open followed by methods on Catalog. Specialized
// compression formats are also exposed as importable packages under codec/.
package carfile
