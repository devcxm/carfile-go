// Package carfile parses and extracts Apple's compiled Asset Catalog files.
//
// Asset formats are decoded in pure Go without cgo or platform-specific
// runtime dependencies.
// High-level callers normally use ExtractFile. Callers that need metadata or
// repeated exports can use Open followed by methods on Catalog. Specialized
// compression formats are also exposed as importable packages under codec/.
package carfile
