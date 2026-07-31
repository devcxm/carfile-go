package carfile

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

type ImageExportResult struct {
	Directory string              `json:"directory"`
	Written   int                 `json:"written"`
	Failed    int                 `json:"failed"`
	Files     []ImageExportRecord `json:"files"`
}

type ImageExportRecord struct {
	Index       int    `json:"index"`
	AssetName   string `json:"asset_name,omitempty"`
	Name        string `json:"name,omitempty"`
	Compression string `json:"compression"`
	File        string `json:"file,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ExportImages decodes compressed bitmap renditions and writes PNG files plus
// a manifest describing both successful and unsupported renditions.
func (c *Catalog) ExportImages(directory string) (ImageExportResult, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return ImageExportResult{}, err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return ImageExportResult{}, fmt.Errorf("create image export directory: %w", err)
	}
	result := ImageExportResult{Directory: absolute}
	for index, rendition := range c.Renditions {
		if rendition.CSI.Payload.CompressionType == nil {
			continue
		}
		record := ImageExportRecord{
			Index: index, AssetName: rendition.AssetName, Name: rendition.CSI.Name,
			Compression: rendition.CSI.Payload.Compression,
		}
		img, err := DecodeRenditionImage(rendition)
		if err != nil {
			record.Error = err.Error()
			result.Failed++
			result.Files = append(result.Files, record)
			continue
		}
		base := rendition.AssetName
		if base == "" {
			base = rendition.CSI.Name
		}
		base = safeFileName(strings.TrimSuffix(base, filepath.Ext(base)))
		if base == "" {
			base = "rendition"
		}
		scale := ""
		if rendition.CSI.ScaleFactor != 0 {
			scale = fmt.Sprintf("@%gx", float64(rendition.CSI.ScaleFactor)/100)
		}
		name := fmt.Sprintf("%04d_%s_%dx%d%s.png", index, base, rendition.CSI.Width, rendition.CSI.Height, scale)
		file, err := os.Create(filepath.Join(absolute, name))
		if err != nil {
			return result, fmt.Errorf("create %s: %w", name, err)
		}
		encodeErr := png.Encode(file, img)
		closeErr := file.Close()
		if encodeErr != nil {
			return result, fmt.Errorf("encode %s: %w", name, encodeErr)
		}
		if closeErr != nil {
			return result, fmt.Errorf("close %s: %w", name, closeErr)
		}
		record.File = name
		result.Written++
		result.Files = append(result.Files, record)
	}
	manifest, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return result, err
	}
	manifest = append(manifest, '\n')
	if err := os.WriteFile(filepath.Join(absolute, "manifest.json"), manifest, 0o644); err != nil {
		return result, fmt.Errorf("write image manifest: %w", err)
	}
	return result, nil
}
