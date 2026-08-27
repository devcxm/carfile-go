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

	"github.com/devcxm/carfile-go/codec/lzfse"
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
	Images     []recoveredImageEntry `json:"images,omitempty"`
	Colors     []recoveredColorEntry `json:"colors,omitempty"`
	Info       recoveredInfo         `json:"info"`
	Properties map[string]bool       `json:"properties,omitempty"`
}

type recoveredColorEntry struct {
	Appearances []recoveredAppearance `json:"appearances,omitempty"`
	Idiom       string                `json:"idiom"`
	Color       recoveredColorValue   `json:"color"`
}

type recoveredAppearance struct {
	Appearance string `json:"appearance"`
	Value      string `json:"value"`
}

type recoveredColorValue struct {
	ColorSpace string                   `json:"color-space"`
	Components recoveredColorComponents `json:"components"`
}

type recoveredColorComponents struct {
	Red   string `json:"red,omitempty"`
	Green string `json:"green,omitempty"`
	Blue  string `json:"blue,omitempty"`
	White string `json:"white,omitempty"`
	Alpha string `json:"alpha"`
}

type recoveredColorResource struct {
	ColorSpaceID uint32    `json:"color_space_id"`
	ColorSpace   string    `json:"color_space"`
	Components   []float64 `json:"components"`
	Appearance   string    `json:"appearance,omitempty"`
	Idiom        string    `json:"idiom"`
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

type recoveredStructuredResource struct {
	Layout      uint16           `json:"layout"`
	LayoutName  string           `json:"layout_name"`
	Name        string           `json:"name"`
	PixelFormat string           `json:"pixel_format,omitempty"`
	Key         []AttributeValue `json:"key,omitempty"`
	TLVs        []TLV            `json:"tlvs,omitempty"`
	Gradient    *Gradient        `json:"gradient,omitempty"`
}

// ExportXCAssets recreates a flat, valid Assets.xcassets directory. It
// resolves internal references into packed images and crops every referenced
// subimage back into an individual PNG. Original RAWD data such as SVG and
// JPEG files is copied without re-encoding.
func (c *Catalog) ExportXCAssets(directory string) (RecoveryResult, error) {
	return c.exportLogicalAssets(directory, true, renditionMatcher{}, nil)
}

// ExportRecovered is kept for compatibility. New callers should use
// ExportXCAssets or the format-independent Export method.
func (c *Catalog) ExportRecovered(directory string) (RecoveryResult, error) {
	return c.ExportXCAssets(directory)
}

// ExportResources recovers all logical assets without generating Asset Catalog
// metadata. Single Data resources keep their original file name; rendition
// families are grouped by asset name.
func (c *Catalog) ExportResources(directory string) (RecoveryResult, error) {
	return c.exportLogicalAssets(directory, false, renditionMatcher{}, nil)
}

func (c *Catalog) exportLogicalAssets(directory string, xcassets bool, matcher renditionMatcher, progress func(Progress)) (RecoveryResult, error) {
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
	for _, rendition := range c.Renditions {
		if rendition.CSI.Layout == 1010 && rendition.AssetName != "" {
			appIconSets[rendition.AssetName] = true
		}
	}
	for index, rendition := range c.Renditions {
		if !matcher.matches(rendition) {
			continue
		}
		if rendition.AssetName == "" || rendition.CSI.Name == "" {
			continue
		}
		if rendition.CSI.Layout == 1010 {
			continue
		}
		byName := groups[rendition.AssetName]
		if byName == nil {
			byName = make(map[string]recoveryCandidate)
			groups[rendition.AssetName] = byName
		}
		candidate := recoveryCandidate{index: index, rendition: rendition}
		identity := recoveryCandidateIdentity(rendition)
		if previous, ok := byName[identity]; ok {
			result.Duplicates++
			if recoveryCandidateScore(candidate) <= recoveryCandidateScore(previous) {
				continue
			}
		}
		byName[identity] = candidate
	}

	assetNames := make([]string, 0, len(groups))
	for name := range groups {
		assetNames = append(assetNames, name)
	}
	sort.Strings(assetNames)
	total := 0
	for _, byName := range groups {
		total += len(byName)
	}
	current := 0
	decodedTargets := make(map[int]image.Image)
	setNameCounts := make(map[string]int)
	createdCatalogGroups := make(map[string]bool)
	for _, assetName := range assetNames {
		colorSet := true
		for _, candidate := range groups[assetName] {
			if candidate.rendition.CSI.Layout != 1009 {
				colorSet = false
				break
			}
		}
		extension := ""
		if xcassets {
			extension = ".imageset"
			if colorSet {
				extension = ".colorset"
			} else if appIconSets[assetName] {
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
		candidates := make([]recoveryCandidate, 0, len(groups[assetName]))
		for _, candidate := range groups[assetName] {
			candidates = append(candidates, candidate)
		}
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].index < candidates[j].index
		})
		setDirectory := filepath.Join(assetsDirectory, relativeSet)
		restoreSingleDataFile := !xcassets && len(candidates) == 1 &&
			candidates[0].rendition.CSI.Layout == 1000 &&
			candidates[0].rendition.CSI.Payload.Tag == "RAWD" &&
			filepath.Ext(setName) != "" && strings.ToLower(setName) != "manifest.json"
		if !restoreSingleDataFile {
			if err := os.MkdirAll(setDirectory, 0o755); err != nil {
				return result, fmt.Errorf("create %s: %w", setName, err)
			}
		}
		contents := recoveredContents{Info: recoveredInfo{Version: 1, Author: "carfile-go"}}
		for _, candidate := range candidates {
			r := candidate.rendition
			current++
			reportProgress(progress, current, total, candidate.index, r)
			name := recoveredPathName(r.CSI.Name, fmt.Sprintf("rendition-%d.png", candidate.index))
			if colorSet {
				name = recoveredColorFileName(r)
			} else if structuredResourceSuffix(r.CSI.Layout) != "" {
				name = strings.TrimSuffix(name, filepath.Ext(name)) + structuredResourceSuffix(r.CSI.Layout)
			}
			record := RecoveryRecord{Index: candidate.index, AssetName: assetName, Name: r.CSI.Name}
			path := filepath.Join(setDirectory, name)
			catalogImage := true
			if restoreSingleDataFile {
				path = setDirectory
			}

			// Recovery dispatches on semantic layout or payload availability.
			// Bitmap compression and pixel layout stay encapsulated in
			// DecodeRenditionImage so new codecs do not multiply layout cases.
			switch {
			case r.CSI.Layout == 1009:
				entry, resource, err := c.recoveredColor(r)
				if err != nil {
					record.Error = err.Error()
				} else if xcassets {
					contents.Colors = append(contents.Colors, entry)
					path = filepath.Join(setDirectory, "Contents.json")
					record.Mode = "decoded named color"
					result.Decoded++
				} else if err := writeIndentedJSON(path, resource); err != nil {
					return result, fmt.Errorf("write %s: %w", name, err)
				} else {
					record.Mode = "decoded named color"
					result.Decoded++
				}

			case r.CSI.Layout == 1021 && r.CSI.Payload.Gradient != nil:
				metadata := recoveredStructuredMetadata(r)
				if err := writeIndentedJSON(path, metadata); err != nil {
					return result, fmt.Errorf("write %s: %w", name, err)
				}
				catalogImage = false
				record.Mode = "decoded named gradient"
				result.Decoded++

			case (r.CSI.Layout == 1019 || r.CSI.Layout == 1020) && r.CSI.Payload.Tag == "RAWD" && r.CSI.Payload.DeclaredLength == 0:
				metadata := recoveredStructuredMetadata(r)
				if err := writeIndentedJSON(path, metadata); err != nil {
					return result, fmt.Errorf("write %s: %w", name, err)
				}
				catalogImage = false
				record.Mode = "decoded structured metadata"
				result.Decoded++

			case r.CSI.Payload.Tag == "RAWD":
				data, _, _, _ := exportablePayload(r.CSI)
				if len(data) == 0 {
					record.Error = "RAWD rendition contains no recoverable data"
				} else if isLZFSEStream(data) {
					data, err = lzfse.Decode(data)
					if err != nil {
						record.Error = fmt.Sprintf("decode compressed RAWD payload: %v", err)
					} else if err := os.WriteFile(path, data, 0o644); err != nil {
						return result, fmt.Errorf("write %s: %w", name, err)
					} else {
						record.Mode = "decoded original payload"
						result.Decoded++
					}
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
			if !colorSet && catalogImage {
				contents.Images = append(contents.Images, recoveredContentsEntry(r, name, appIconSets[assetName]))
			}
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

func recoveryCandidateIdentity(r Rendition) string {
	if r.CSI.Layout != 1009 && (r.CSI.Layout < 1019 || r.CSI.Layout > 1021) {
		return r.CSI.Name
	}
	var identity strings.Builder
	identity.WriteString(r.CSI.Name)
	for _, attribute := range r.Key {
		identity.WriteByte('|')
		identity.WriteString(strconv.FormatUint(uint64(attribute.Type), 10))
		identity.WriteByte('=')
		identity.WriteString(strconv.FormatUint(uint64(attribute.Value), 10))
	}
	return identity.String()
}

func recoveredColorFileName(r Rendition) string {
	name := strings.TrimSuffix(recoveredPathName(r.CSI.Name, "color"), filepath.Ext(r.CSI.Name))
	appearance := attributeValue(r.Key, 7)
	if appearance != 0 {
		name += "-" + recoveredAppearanceFileSuffix(appearance)
	}
	if idiom := attributeValue(r.Key, 15); idiom != 0 {
		name += "-" + idiomName(idiom)
	}
	if scale := attributeValue(r.Key, 12); scale > 1 {
		name += fmt.Sprintf("-%dx", scale)
	}
	return name + ".color.json"
}

func recoveredAppearanceFileSuffix(value uint16) string {
	if value == 1 {
		return "dark"
	}
	return fmt.Sprintf("appearance-%d", value)
}

func (c *Catalog) recoveredColor(r Rendition) (recoveredColorEntry, recoveredColorResource, error) {
	var entry recoveredColorEntry
	var resource recoveredColorResource
	if r.CSI.Payload.Tag != "COLR" || r.CSI.Payload.ColorSpaceID == nil {
		return entry, resource, fmt.Errorf("Color rendition has no parsed COLR payload")
	}
	colorSpace := recoveredColorSpace(*r.CSI.Payload.ColorSpaceID)
	components := r.CSI.Payload.ColorComponents
	var recoveredComponents recoveredColorComponents
	switch len(components) {
	case 2:
		recoveredComponents = recoveredColorComponents{
			White: formatColorComponent(components[0]), Alpha: formatColorComponent(components[1]),
		}
	case 4:
		recoveredComponents = recoveredColorComponents{
			Red: formatColorComponent(components[0]), Green: formatColorComponent(components[1]),
			Blue: formatColorComponent(components[2]), Alpha: formatColorComponent(components[3]),
		}
	default:
		return entry, resource, fmt.Errorf("Color rendition has %d components, expected 2 or 4", len(components))
	}
	entry = recoveredColorEntry{
		Idiom: idiomName(attributeValue(r.Key, 15)),
		Color: recoveredColorValue{
			ColorSpace: colorSpace,
			Components: recoveredComponents,
		},
	}
	appearance := c.appearanceName(attributeValue(r.Key, 7))
	entry.Appearances = recoveredColorAppearances(appearance)
	resource = recoveredColorResource{
		ColorSpaceID: *r.CSI.Payload.ColorSpaceID, ColorSpace: colorSpace,
		Components: append([]float64(nil), components...), Appearance: appearance, Idiom: entry.Idiom,
	}
	return entry, resource, nil
}

func recoveredColorSpace(id uint32) string {
	switch id {
	case 1:
		return "srgb"
	case 2:
		return "gray-gamma-22"
	case 3:
		return "display-p3"
	case 4:
		return "extended-srgb"
	case 6:
		return "extended-gray"
	default:
		return fmt.Sprintf("coreui-%d", id)
	}
}

func (c *Catalog) appearanceName(id uint16) string {
	if id == 0 {
		return ""
	}
	for _, appearance := range c.Appearances {
		if appearance.ID == id {
			return appearance.Name
		}
	}
	return fmt.Sprintf("UIAppearance%d", id)
}

func recoveredColorAppearances(name string) []recoveredAppearance {
	switch name {
	case "", "UIAppearanceAny":
		return nil
	case "UIAppearanceDark", "NSAppearanceNameDarkAqua":
		return []recoveredAppearance{{Appearance: "luminosity", Value: "dark"}}
	case "UIAppearanceLight", "NSAppearanceNameAqua":
		return []recoveredAppearance{{Appearance: "luminosity", Value: "light"}}
	case "UIAppearanceHighContrast":
		return []recoveredAppearance{{Appearance: "contrast", Value: "high"}}
	case "UIAppearanceDarkHighContrast", "NSAppearanceNameAccessibilityHighContrastDarkAqua":
		return []recoveredAppearance{{Appearance: "luminosity", Value: "dark"}, {Appearance: "contrast", Value: "high"}}
	case "UIAppearanceLightHighContrast", "NSAppearanceNameAccessibilityHighContrastAqua":
		return []recoveredAppearance{{Appearance: "luminosity", Value: "light"}, {Appearance: "contrast", Value: "high"}}
	default:
		return []recoveredAppearance{{Appearance: "luminosity", Value: strings.ToLower(strings.TrimPrefix(name, "UIAppearance"))}}
	}
}

func structuredResourceSuffix(layout uint16) string {
	switch layout {
	case 1019:
		return ".iconstack.json"
	case 1020:
		return ".icongroup.json"
	case 1021:
		return ".gradient.json"
	default:
		return ""
	}
}

func recoveredStructuredMetadata(r Rendition) recoveredStructuredResource {
	return recoveredStructuredResource{
		Layout: r.CSI.Layout, LayoutName: r.CSI.LayoutName, Name: r.CSI.Name,
		PixelFormat: r.CSI.PixelFormat, Key: r.Key, TLVs: r.CSI.TLVs, Gradient: r.CSI.Payload.Gradient,
	}
}

func formatColorComponent(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
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
