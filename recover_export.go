package carfile

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type RecoveryResult struct {
	Directory        string           `json:"directory"`
	CatalogDirectory string           `json:"catalog_directory,omitempty"`
	AssetSets        int              `json:"asset_sets"`
	Written          int              `json:"written"`
	CopiedOriginals  int              `json:"copied_originals"`
	Decoded          int              `json:"decoded"`
	Cropped          int              `json:"cropped"`
	Duplicates       int              `json:"duplicates_skipped"`
	Failed           int              `json:"failed"`
	Files            []RecoveryRecord `json:"files"`
}

type RecoveryRecord struct {
	Index       int    `json:"index"`
	AssetName   string `json:"asset_name"`
	Name        string `json:"name"`
	File        string `json:"file,omitempty"`
	Mode        string `json:"mode,omitempty"`
	TargetIndex *int   `json:"target_index,omitempty"`
	Error       string `json:"error,omitempty"`
}

type recoveryCandidate struct {
	index     int
	rendition Rendition
}

type recoveredContents struct {
	Images     []recoveredImageEntry `json:"images"`
	Info       recoveredInfo         `json:"info"`
	Properties map[string]bool       `json:"properties,omitempty"`
}

type recoveredImageEntry struct {
	Size     string `json:"size,omitempty"`
	Idiom    string `json:"idiom"`
	Filename string `json:"filename"`
	Scale    string `json:"scale"`
}

type recoveredInfo struct {
	Version int    `json:"version"`
	Author  string `json:"author"`
}

// ExportXCAssets recreates a flat, valid Assets.xcassets directory. It
// resolves internal references into packed images and crops every referenced
// subimage back into an individual PNG. Original RAWD data such as SVG and
// JPEG files is copied without re-encoding.
func (c *Catalog) ExportXCAssets(directory string) (RecoveryResult, error) {
	return c.exportLogicalAssets(directory, true)
}

// ExportRecovered is kept for compatibility. New callers should use
// ExportXCAssets or the format-independent Export method.
func (c *Catalog) ExportRecovered(directory string) (RecoveryResult, error) {
	return c.ExportXCAssets(directory)
}

// ExportResources recovers all logical assets into ordinary directories,
// grouped by asset name without generating Asset Catalog metadata.
func (c *Catalog) ExportResources(directory string) (RecoveryResult, error) {
	return c.exportLogicalAssets(directory, false)
}

func (c *Catalog) exportLogicalAssets(directory string, xcassets bool) (RecoveryResult, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return RecoveryResult{}, err
	}
	assetsDirectory := absolute
	if xcassets {
		assetsDirectory = filepath.Join(absolute, "Assets.xcassets")
	}
	if err := os.MkdirAll(assetsDirectory, 0o755); err != nil {
		return RecoveryResult{}, fmt.Errorf("create recovered assets directory: %w", err)
	}
	result := RecoveryResult{Directory: absolute}
	if xcassets {
		result.CatalogDirectory = assetsDirectory
	}

	groups := make(map[string]map[string]recoveryCandidate)
	appIconSets := make(map[string]bool)
	for index, rendition := range c.Renditions {
		if rendition.AssetName == "" || rendition.CSI.Name == "" {
			continue
		}
		if rendition.CSI.Layout == 1010 {
			appIconSets[rendition.AssetName] = true
			continue
		}
		byName := groups[rendition.AssetName]
		if byName == nil {
			byName = make(map[string]recoveryCandidate)
			groups[rendition.AssetName] = byName
		}
		candidate := recoveryCandidate{index: index, rendition: rendition}
		if previous, ok := byName[rendition.CSI.Name]; ok {
			result.Duplicates++
			if recoveryCandidateScore(candidate) <= recoveryCandidateScore(previous) {
				continue
			}
		}
		byName[rendition.CSI.Name] = candidate
	}

	assetNames := make([]string, 0, len(groups))
	for name := range groups {
		assetNames = append(assetNames, name)
	}
	sort.Strings(assetNames)
	decodedTargets := make(map[int]image.Image)
	setNameCounts := make(map[string]int)
	createdCatalogGroups := make(map[string]bool)
	for _, assetName := range assetNames {
		extension := ""
		if xcassets {
			extension = ".imageset"
			if appIconSets[assetName] {
				extension = ".appiconset"
			}
		}
		setName := recoveredPathName(assetName, "asset") + extension
		relativeSet := setName
		foldedSetName := strings.ToLower(setName)
		setNameCounts[foldedSetName]++
		if setNameCounts[foldedSetName] > 1 {
			groupName := fmt.Sprintf("_CaseCollisions_%d", setNameCounts[foldedSetName])
			relativeSet = filepath.Join(groupName, setName)
			if !createdCatalogGroups[groupName] {
				groupDirectory := filepath.Join(assetsDirectory, groupName)
				if err := os.MkdirAll(groupDirectory, 0o755); err != nil {
					return result, fmt.Errorf("create catalog group %s: %w", groupName, err)
				}
				if xcassets {
					groupContents := struct {
						Info recoveredInfo `json:"info"`
					}{Info: recoveredInfo{Version: 1, Author: "carfile-go"}}
					if err := writeIndentedJSON(filepath.Join(groupDirectory, "Contents.json"), groupContents); err != nil {
						return result, fmt.Errorf("write catalog group %s: %w", groupName, err)
					}
				}
				createdCatalogGroups[groupName] = true
			}
		}
		setDirectory := filepath.Join(assetsDirectory, relativeSet)
		if err := os.MkdirAll(setDirectory, 0o755); err != nil {
			return result, fmt.Errorf("create %s: %w", setName, err)
		}

		candidates := make([]recoveryCandidate, 0, len(groups[assetName]))
		for _, candidate := range groups[assetName] {
			candidates = append(candidates, candidate)
		}
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].index < candidates[j].index
		})
		contents := recoveredContents{Info: recoveredInfo{Version: 1, Author: "carfile-go"}}
		for _, candidate := range candidates {
			r := candidate.rendition
			name := recoveredPathName(r.CSI.Name, fmt.Sprintf("rendition-%d.png", candidate.index))
			record := RecoveryRecord{Index: candidate.index, AssetName: assetName, Name: r.CSI.Name}
			path := filepath.Join(setDirectory, name)

			switch {
			case r.CSI.Payload.Tag == "RAWD":
				data, _, _, _ := exportablePayload(r.CSI)
				if len(data) == 0 {
					record.Error = "RAWD rendition contains no recoverable data"
				} else if err := os.WriteFile(path, data, 0o644); err != nil {
					return result, fmt.Errorf("write %s: %w", name, err)
				} else {
					record.Mode = "copied original payload"
					result.CopiedOriginals++
				}

			case r.CSI.Layout == 1003:
				img, targetIndex, err := c.decodeInternalReference(candidate.index, decodedTargets)
				if err != nil {
					record.Error = err.Error()
				} else if err := writePNG(path, img); err != nil {
					return result, fmt.Errorf("write %s: %w", name, err)
				} else {
					record.Mode = "cropped from packed image"
					record.TargetIndex = &targetIndex
					result.Cropped++
				}

			case r.CSI.Payload.CompressionType != nil:
				img, err := DecodeRenditionImage(r)
				if err != nil {
					record.Error = err.Error()
				} else if err := writePNG(path, img); err != nil {
					return result, fmt.Errorf("write %s: %w", name, err)
				} else {
					record.Mode = "decoded standalone bitmap"
					result.Decoded++
				}

			default:
				record.Error = fmt.Sprintf("unsupported rendition layout %s", r.CSI.LayoutName)
			}

			if record.Error != "" {
				result.Failed++
				result.Files = append(result.Files, record)
				continue
			}
			relativeFile, err := filepath.Rel(absolute, path)
			if err != nil {
				return result, fmt.Errorf("make relative path for %s: %w", name, err)
			}
			record.File = filepath.ToSlash(relativeFile)
			result.Written++
			result.Files = append(result.Files, record)
			contents.Images = append(contents.Images, recoveredContentsEntry(r, name, appIconSets[assetName]))
			if r.CSI.Flags.PreservedVectorRepresentation {
				contents.Properties = map[string]bool{"preserves-vector-representation": true}
			}
		}
		if xcassets {
			if err := writeIndentedJSON(filepath.Join(setDirectory, "Contents.json"), contents); err != nil {
				return result, fmt.Errorf("write %s Contents.json: %w", assetName, err)
			}
		}
		result.AssetSets++
	}

	if xcassets {
		rootContents := struct {
			Info recoveredInfo `json:"info"`
		}{Info: recoveredInfo{Version: 1, Author: "carfile-go"}}
		if err := writeIndentedJSON(filepath.Join(assetsDirectory, "Contents.json"), rootContents); err != nil {
			return result, fmt.Errorf("write root Contents.json: %w", err)
		}
	}
	if err := writeIndentedJSON(filepath.Join(absolute, "manifest.json"), result); err != nil {
		return result, fmt.Errorf("write recovery manifest: %w", err)
	}
	return result, nil
}

func recoveryCandidateScore(candidate recoveryCandidate) int {
	csi := candidate.rendition.CSI
	score := 0
	switch {
	case csi.Payload.Tag == "RAWD":
		score = 300
	case csi.Layout == 1003:
		score = 200
	case csi.Payload.CompressionType != nil:
		score = 100
	}
	if expected := scaleFromFileName(csi.Name); expected != 0 && csi.ScaleFactor == expected*100 {
		score += 50
	}
	return score
}

func scaleFromFileName(name string) uint32 {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	pos := strings.LastIndex(base, "@")
	if pos < 0 || !strings.HasSuffix(base, "x") {
		return 0
	}
	value, err := strconv.ParseUint(base[pos+1:len(base)-1], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(value)
}

func (c *Catalog) decodeInternalReference(index int, cache map[int]image.Image) (image.Image, int, error) {
	r := c.Renditions[index]
	var link *TLV
	for i := range r.CSI.TLVs {
		if r.CSI.TLVs[i].Type == 1010 {
			link = &r.CSI.TLVs[i]
			break
		}
	}
	if link == nil || link.LinkedRect == nil || len(link.LinkedKey) == 0 {
		return nil, 0, fmt.Errorf("internal reference has no parsed link")
	}
	targetIndex, err := c.findLinkedRendition(link.LinkedKey)
	if err != nil {
		return nil, 0, err
	}
	target := cache[targetIndex]
	if target == nil {
		target, err = DecodeRenditionImage(c.Renditions[targetIndex])
		if err != nil {
			return nil, 0, fmt.Errorf("decode packed rendition %d: %w", targetIndex, err)
		}
		cache[targetIndex] = target
	}
	cropped, err := cropPixelRect(target, *link.LinkedRect)
	if err != nil {
		return nil, 0, fmt.Errorf("crop packed rendition %d: %w", targetIndex, err)
	}
	if cropped.Bounds().Dx() != int(r.CSI.Width) || cropped.Bounds().Dy() != int(r.CSI.Height) {
		return nil, 0, fmt.Errorf("linked rectangle is %dx%d, rendition is %dx%d", cropped.Bounds().Dx(), cropped.Bounds().Dy(), r.CSI.Width, r.CSI.Height)
	}
	return cropped, targetIndex, nil
}

func (c *Catalog) findLinkedRendition(tokens []AttributeValue) (int, error) {
	match := -1
	for index, rendition := range c.Renditions {
		if !keyMatchesTokens(rendition.Key, tokens) {
			continue
		}
		if match >= 0 {
			return 0, fmt.Errorf("internal link key matches multiple renditions (%d and %d)", match, index)
		}
		match = index
	}
	if match < 0 {
		return 0, fmt.Errorf("internal link target was not found")
	}
	return match, nil
}

func keyMatchesTokens(key, tokens []AttributeValue) bool {
	want := make(map[AttributeType]uint16, len(tokens))
	for _, token := range tokens {
		want[token.Type] = token.Value
	}
	for _, attribute := range key {
		if attribute.Value != want[attribute.Type] {
			return false
		}
		delete(want, attribute.Type)
	}
	return len(want) == 0
}

func cropPixelRect(source image.Image, rect PixelRect) (image.Image, error) {
	if rect.Width == 0 || rect.Height == 0 {
		return nil, fmt.Errorf("empty linked rectangle")
	}
	// Internal-link coordinates use Core Graphics' lower-left origin, while
	// Go images use an upper-left origin.
	x0 := source.Bounds().Min.X + int(rect.X)
	y0 := source.Bounds().Max.Y - int(rect.Y) - int(rect.Height)
	bounds := image.Rect(x0, y0, x0+int(rect.Width), y0+int(rect.Height))
	if !bounds.In(source.Bounds()) {
		return nil, fmt.Errorf("rectangle %v exceeds image bounds %v", bounds, source.Bounds())
	}
	result := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(result, result.Bounds(), source, bounds.Min, draw.Src)
	return result, nil
}

func recoveredContentsEntry(r Rendition, name string, appIcon bool) recoveredImageEntry {
	scale := attributeValue(r.Key, 12)
	if scale == 0 && r.CSI.ScaleFactor != 0 {
		scale = uint16(r.CSI.ScaleFactor / 100)
	}
	if scale == 0 {
		scale = 1
	}
	entry := recoveredImageEntry{
		Idiom: idiomName(attributeValue(r.Key, 15)), Filename: name,
		Scale: fmt.Sprintf("%dx", scale),
	}
	if appIcon {
		entry.Size = fmt.Sprintf("%sx%s", logicalDimension(r.CSI.Width, scale), logicalDimension(r.CSI.Height, scale))
	}
	return entry
}

func attributeValue(key []AttributeValue, typ AttributeType) uint16 {
	for _, attribute := range key {
		if attribute.Type == typ {
			return attribute.Value
		}
	}
	return 0
}

func idiomName(value uint16) string {
	names := map[uint16]string{0: "universal", 1: "iphone", 2: "ipad", 3: "tv", 4: "car", 5: "watch", 6: "ios-marketing", 7: "mac", 8: "vision"}
	if name, ok := names[value]; ok {
		return name
	}
	return "universal"
}

func logicalDimension(pixels uint32, scale uint16) string {
	value := float64(pixels) / float64(scale)
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func recoveredPathName(name, fallback string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return fallback
	}
	return name
}

func writePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(file, img)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func writeIndentedJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
