package carfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type ExportResult struct {
	Directory string         `json:"directory"`
	Written   int            `json:"written"`
	Skipped   int            `json:"skipped"`
	Files     []ExportRecord `json:"files"`
}

type ExportRecord struct {
	Index         int              `json:"index"`
	AssetName     string           `json:"asset_name,omitempty"`
	RenditionName string           `json:"rendition_name,omitempty"`
	Layout        string           `json:"layout"`
	PixelFormat   string           `json:"pixel_format,omitempty"`
	Compression   string           `json:"compression,omitempty"`
	File          string           `json:"file,omitempty"`
	Openable      bool             `json:"openable"`
	Status        string           `json:"status"`
	LinkedKey     []AttributeValue `json:"linked_key,omitempty"`
}

// ExportRaw writes rendition payloads without invoking CoreUI or external
// decompressors. Standard RAWD files are unwrapped; CELM files have their
// 16-byte wrapper removed but remain compressed.
func (c *Catalog) ExportRaw(directory string) (ExportResult, error) {
	return c.exportRaw(directory, renditionMatcher{}, nil)
}

func (c *Catalog) exportRaw(directory string, matcher renditionMatcher, progress func(Progress)) (ExportResult, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return ExportResult{}, err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return ExportResult{}, fmt.Errorf("create export directory: %w", err)
	}

	total := 0
	for _, rendition := range c.Renditions {
		if matcher.matches(rendition) {
			total++
		}
	}
	current := 0
	result := ExportResult{Directory: absolute, Files: make([]ExportRecord, 0, total)}
	for index, rendition := range c.Renditions {
		if !matcher.matches(rendition) {
			continue
		}
		current++
		reportProgress(progress, current, total, index, rendition)
		record := ExportRecord{
			Index: index, AssetName: rendition.AssetName, RenditionName: rendition.CSI.Name,
			Layout: rendition.CSI.LayoutName, PixelFormat: rendition.CSI.PixelFormat,
			Compression: rendition.CSI.Payload.Compression,
		}
		for _, tlv := range rendition.CSI.TLVs {
			if tlv.Type == 1010 && len(tlv.LinkedKey) != 0 {
				record.LinkedKey = tlv.LinkedKey
				break
			}
		}

		data, extension, openable, status := exportablePayload(rendition.CSI)
		record.Openable = openable
		record.Status = status
		if len(data) == 0 {
			result.Skipped++
			result.Files = append(result.Files, record)
			continue
		}

		base := rendition.AssetName
		if base == "" {
			base = rendition.CSI.Name
		}
		base = strings.TrimSuffix(base, filepath.Ext(base))
		base = safeFileName(base)
		if base == "" {
			base = "rendition"
		}
		scale := ""
		if rendition.CSI.ScaleFactor != 0 {
			scale = fmt.Sprintf("@%gx", float64(rendition.CSI.ScaleFactor)/100)
		}
		name := fmt.Sprintf("%04d_%s_%dx%d%s%s", index, base, rendition.CSI.Width, rendition.CSI.Height, scale, extension)
		path := filepath.Join(absolute, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return result, fmt.Errorf("write %s: %w", name, err)
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
		return result, fmt.Errorf("write manifest: %w", err)
	}
	return result, nil
}

func exportablePayload(csi CSI) ([]byte, string, bool, string) {
	raw := csi.Payload.Data
	switch csi.Payload.Tag {
	case "RAWD":
		if len(raw) < 12 {
			return nil, "", false, "truncated RAWD payload"
		}
		data := raw[12:]
		if csi.Payload.DeclaredLength <= uint32(len(data)) {
			data = data[:csi.Payload.DeclaredLength]
		}
		extension := originalExtension(csi)
		if isLZFSEStream(data) {
			return data, extension, false, "RAWD wrapper removed; LZFSE data remains compressed"
		}
		return data, extension, isDirectlyOpenable(extension), "unwrapped RAWD data"
	case "CELM":
		if len(raw) < 16 {
			return nil, "", false, "truncated CELM payload"
		}
		data := raw[16:]
		extension := ".celm"
		if csi.Payload.Compression != "" {
			extension = "." + safeFileName(csi.Payload.Compression)
		}
		return data, extension, false, "CELM wrapper removed; data remains compressed"
	case "COLR":
		if len(raw) != 0 {
			return raw, ".colr", false, "raw named-color payload"
		}
	default:
		if len(raw) != 0 {
			extension := ".bin"
			if csi.Payload.Tag != "" {
				extension = "." + strings.ToLower(safeFileName(csi.Payload.Tag))
			}
			return raw, extension, false, "raw unsupported payload"
		}
	}
	if csi.Layout == 1003 {
		return nil, "", false, "internal reference; no standalone payload"
	}
	return nil, "", false, "no payload"
}

func isLZFSEStream(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	switch string(data[:4]) {
	case "bvx-", "bvx1", "bvx2", "bvxn":
		return true
	default:
		return false
	}
}

func originalExtension(csi CSI) string {
	if extension := strings.ToLower(filepath.Ext(csi.Name)); extension != "" {
		return extension
	}
	switch csi.PixelFormat {
	case "JPEG":
		return ".jpg"
	case "HEIF":
		return ".heic"
	case "PDF ", "PDF":
		return ".pdf"
	}
	for _, tlv := range csi.TLVs {
		if tlv.Type != 1005 {
			continue
		}
		switch strings.ToLower(tlv.Text) {
		case "com.adobe.pdf":
			return ".pdf"
		case "public.svg-image", "public.svg":
			return ".svg"
		case "public.jpeg":
			return ".jpg"
		case "public.png":
			return ".png"
		case "public.plain-text":
			return ".txt"
		}
	}
	return ".data"
}

func isDirectlyOpenable(extension string) bool {
	switch extension {
	case ".png", ".jpg", ".jpeg", ".heic", ".heif", ".pdf", ".svg", ".txt", ".json", ".plist":
		return true
	default:
		return false
	}
}

func safeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, name)
}
