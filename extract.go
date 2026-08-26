package carfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Version is the library and CLI semantic version.
const Version = "0.6.0"

// OutputFormat selects the representation written by ExtractFile or Export.
type OutputFormat string

const (
	// FormatResources recovers every logical resource into ordinary folders.
	FormatResources OutputFormat = "resources"
	// FormatXCAssets recreates a compilable Assets.xcassets directory.
	FormatXCAssets OutputFormat = "xcassets"
	// FormatRaw writes physical rendition payloads without decoding them.
	FormatRaw OutputFormat = "raw"
	// FormatPNG decodes physical bitmap payloads as PNG files, including atlases.
	FormatPNG OutputFormat = "png"
	// FormatJSON writes the parsed catalog metadata as JSON.
	FormatJSON OutputFormat = "json"
)

// ExtractOptions controls high-level file extraction. An empty Format means
// FormatResources. ExtractFile also derives OutputDirectory when it is empty.
type ExtractOptions struct {
	OutputDirectory string
	Format          OutputFormat
	// Includes limits output to matching asset names, rendition file names,
	// or "asset/file" paths. Patterns use path.Match glob syntax.
	Includes []string
	// Progress is called synchronously before each selected item is decoded or
	// written. Current is one-based and callbacks are never concurrent.
	Progress func(Progress)
}

// Progress describes the item currently being processed by an export.
type Progress struct {
	Current        int    `json:"current"`
	Total          int    `json:"total"`
	RenditionIndex int    `json:"rendition_index"`
	AssetName      string `json:"asset_name,omitempty"`
	FileName       string `json:"file_name"`
}

// ExtractResult is the format-independent summary returned to library and CLI
// callers. Detailed per-file results are stored in each output manifest.
type ExtractResult struct {
	Format          OutputFormat `json:"format"`
	OutputDirectory string       `json:"output_directory"`
	Written         int          `json:"written"`
	Skipped         int          `json:"skipped,omitempty"`
	Failed          int          `json:"failed,omitempty"`
}

// ExtractFile opens a compiled asset catalog and exports it according to
// options. With zero-value options it recovers all logical resources beside
// the input file in a <name>-extracted directory.
func ExtractFile(path string, options ExtractOptions) (ExtractResult, error) {
	if options.OutputDirectory == "" {
		options.OutputDirectory = DefaultOutputDirectory(path)
	}
	catalog, err := Open(path)
	if err != nil {
		return ExtractResult{}, err
	}
	return catalog.Export(options)
}

// Export writes an already parsed catalog using one of the supported formats.
func (c *Catalog) Export(options ExtractOptions) (ExtractResult, error) {
	format := options.Format
	if format == "" {
		format = FormatResources
	}
	if _, err := ParseOutputFormat(string(format)); err != nil {
		return ExtractResult{}, err
	}
	if options.OutputDirectory == "" {
		return ExtractResult{}, fmt.Errorf("output directory is required when exporting a parsed Catalog")
	}
	matcher, err := newRenditionMatcher(options.Includes)
	if err != nil {
		return ExtractResult{}, err
	}
	if format == FormatJSON && len(options.Includes) != 0 {
		return ExtractResult{}, fmt.Errorf("include filters are not supported by the json format")
	}
	if len(options.Includes) != 0 && !matcher.matchesCatalog(c) {
		return ExtractResult{}, fmt.Errorf("no renditions match include patterns %q", options.Includes)
	}

	summary := ExtractResult{Format: format}
	switch format {
	case FormatResources:
		result, err := c.exportLogicalAssets(options.OutputDirectory, false, matcher, options.Progress)
		if err != nil {
			return summary, err
		}
		summary.OutputDirectory = result.Directory
		summary.Written, summary.Failed = result.Written, result.Failed

	case FormatXCAssets:
		result, err := c.exportLogicalAssets(options.OutputDirectory, true, matcher, options.Progress)
		if err != nil {
			return summary, err
		}
		summary.OutputDirectory = result.Directory
		summary.Written, summary.Failed = result.Written, result.Failed

	case FormatRaw:
		result, err := c.exportRaw(options.OutputDirectory, matcher, options.Progress)
		if err != nil {
			return summary, err
		}
		summary.OutputDirectory = result.Directory
		summary.Written, summary.Skipped = result.Written, result.Skipped

	case FormatPNG:
		result, err := c.exportImages(options.OutputDirectory, matcher, options.Progress)
		if err != nil {
			return summary, err
		}
		summary.OutputDirectory = result.Directory
		summary.Written, summary.Failed = result.Written, result.Failed

	case FormatJSON:
		absolute, err := filepath.Abs(options.OutputDirectory)
		if err != nil {
			return summary, err
		}
		if err := os.MkdirAll(absolute, 0o755); err != nil {
			return summary, fmt.Errorf("create JSON output directory: %w", err)
		}
		reportProgress(options.Progress, 1, 1, -1, Rendition{CSI: CSI{Name: "catalog.json"}})
		data, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return summary, err
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(absolute, "catalog.json"), data, 0o644); err != nil {
			return summary, fmt.Errorf("write catalog.json: %w", err)
		}
		summary.OutputDirectory = absolute
		summary.Written = 1
	}
	return summary, nil
}

func reportProgress(callback func(Progress), current, total, index int, rendition Rendition) {
	if callback == nil {
		return
	}
	callback(Progress{
		Current: current, Total: total, RenditionIndex: index,
		AssetName: rendition.AssetName, FileName: rendition.CSI.Name,
	})
}

// ParseOutputFormat validates a user-facing format name and accepts a few
// convenient aliases.
func ParseOutputFormat(value string) (OutputFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "resources", "resource", "files", "all":
		return FormatResources, nil
	case "xcassets", "xcasset":
		return FormatXCAssets, nil
	case "raw":
		return FormatRaw, nil
	case "png", "images", "image":
		return FormatPNG, nil
	case "json":
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unsupported output format %q (want resources, xcassets, raw, png, or json)", value)
	}
}

// DefaultOutputDirectory returns the zero-configuration extraction directory
// used by the CLI and ExtractFile.
func DefaultOutputDirectory(inputPath string) string {
	directory := filepath.Dir(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if base == "" {
		base = "Assets"
	}
	return filepath.Join(directory, base+"-extracted")
}
