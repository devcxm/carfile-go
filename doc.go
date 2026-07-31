// Package carfile parses and extracts Apple's compiled Asset Catalog files.
//
// The package is implemented in pure Go and uses only the standard library.
// High-level callers normally use ExtractFile. Callers that need metadata or
// repeated exports can use Open followed by methods on Catalog. Specialized
// compression formats are also exposed as importable packages under codec/.
package carfile
