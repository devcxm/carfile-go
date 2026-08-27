package carfile

import (
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestExportResourcesDecodesCompressedRawData(t *testing.T) {
	want := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	stream := make([]byte, 8)
	copy(stream, "bvx-")
	binary.LittleEndian.PutUint32(stream[4:], uint32(len(want)))
	stream = append(stream, want...)
	stream = append(stream, 'b', 'v', 'x', '$')
	payload := make([]byte, 12)
	copy(payload, "DWAR")
	binary.LittleEndian.PutUint32(payload[4:8], 1)
	binary.LittleEndian.PutUint32(payload[8:12], uint32(len(stream)))
	payload = append(payload, stream...)
	catalog := Catalog{Renditions: []Rendition{{
		AssetName: "Symbol.svg",
		CSI: CSI{Layout: 1000, LayoutName: "Data", Name: "CoreStructuredImage", Payload: Payload{
			Tag: "RAWD", DeclaredLength: uint32(len(stream)), Data: payload,
		}},
	}}}

	directory := t.TempDir()
	result, err := catalog.ExportResources(directory)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(directory, "Symbol.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) || result.Written != 1 || result.Decoded != 1 || result.Failed != 0 {
		t.Fatalf("data = %q, result = %#v", got, result)
	}
}

func TestExportResourcesPreservesColorAppearances(t *testing.T) {
	catalog := colorCatalogFixture()
	directory := t.TempDir()
	result, err := catalog.ExportResources(directory)
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 2 || result.Failed != 0 || result.Duplicates != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, name := range []string{"AccentColor.color.json", "AccentColor-dark.color.json"} {
		data, err := os.ReadFile(filepath.Join(directory, "AccentColor", name))
		if err != nil {
			t.Fatal(err)
		}
		var colorResource map[string]any
		if err := json.Unmarshal(data, &colorResource); err != nil {
			t.Fatal(err)
		}
		if len(colorResource["components"].([]any)) != 4 {
			t.Fatalf("%s has invalid components: %s", name, data)
		}
	}
}

func TestExportXCAssetsCreatesColorSet(t *testing.T) {
	catalog := colorCatalogFixture()
	directory := t.TempDir()
	result, err := catalog.ExportXCAssets(directory)
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 2 || result.Failed != 0 || result.Duplicates != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(directory, "Assets.xcassets", "AccentColor.colorset", "Contents.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contents struct {
		Colors []struct {
			Appearances []struct {
				Appearance string `json:"appearance"`
				Value      string `json:"value"`
			} `json:"appearances"`
		} `json:"colors"`
	}
	if err := json.Unmarshal(data, &contents); err != nil {
		t.Fatal(err)
	}
	if len(contents.Colors) != 2 || len(contents.Colors[0].Appearances) != 0 ||
		len(contents.Colors[1].Appearances) != 1 || contents.Colors[1].Appearances[0].Value != "dark" {
		t.Fatalf("unexpected color set: %s", data)
	}
}

func colorCatalogFixture() Catalog {
	colorSpaceSRGB := uint32(1)
	return Catalog{
		Appearances: []Appearance{{Name: "UIAppearanceAny", ID: 0}, {Name: "UIAppearanceDark", ID: 1}},
		Renditions: []Rendition{
			{
				AssetName: "AccentColor",
				Key:       []AttributeValue{{Type: 7, Value: 0}, {Type: 15, Value: 0}},
				CSI: CSI{Layout: 1009, LayoutName: "Color", Name: "AccentColor", Payload: Payload{
					Tag: "COLR", ColorSpaceID: &colorSpaceSRGB, ColorComponents: []float64{0.1, 0.2, 0.3, 1},
				}},
			},
			{
				AssetName: "AccentColor",
				Key:       []AttributeValue{{Type: 7, Value: 1}, {Type: 15, Value: 0}},
				CSI: CSI{Layout: 1009, LayoutName: "Color", Name: "AccentColor", Payload: Payload{
					Tag: "COLR", ColorSpaceID: &colorSpaceSRGB, ColorComponents: []float64{0.9, 0.8, 0.7, 1},
				}},
			},
		},
	}
}

func TestExportResourcesRestoresSingleDataAssetFileName(t *testing.T) {
	data := []byte(`{"continents":[]}`)
	payload := append([]byte("DWAR\x00\x00\x00\x00\x11\x00\x00\x00"), data...)
	catalog := Catalog{Renditions: []Rendition{{
		AssetName: "SupportCountryList.json",
		CSI: CSI{
			Layout:     1000,
			LayoutName: "Data",
			Name:       "CoreStructuredImage",
			Payload:    Payload{Tag: "RAWD", DeclaredLength: uint32(len(data)), Data: payload},
		},
	}}}

	directory := t.TempDir()
	result, err := catalog.ExportResources(directory)
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(directory, "SupportCountryList.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != string(data) {
		t.Fatalf("data = %q, want %q", written, data)
	}
	if len(result.Files) != 1 || result.Files[0].File != "SupportCountryList.json" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestKeyMatchesTokens(t *testing.T) {
	key := []AttributeValue{
		{Type: 7, Value: 0}, {Type: 12, Value: 2}, {Type: 8, Value: 29},
		{Type: 1, Value: 9}, {Type: 2, Value: 181},
	}
	tokens := []AttributeValue{
		{Type: 1, Value: 9}, {Type: 2, Value: 181}, {Type: 8, Value: 29}, {Type: 12, Value: 2},
	}
	if !keyMatchesTokens(key, tokens) {
		t.Fatal("expected sparse tokens to match full key")
	}
	key[0].Value = 1
	if keyMatchesTokens(key, tokens) {
		t.Fatal("unexpected match with a nonzero unlisted attribute")
	}
}

func TestCropPixelRect(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 4, 3))
	source.SetRGBA(2, 2, color.RGBA{R: 1, G: 2, B: 3, A: 4})
	cropped, err := cropPixelRect(source, PixelRect{X: 2, Y: 0, Width: 1, Height: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := color.RGBAModel.Convert(cropped.At(0, 0)).(color.RGBA); got != (color.RGBA{R: 1, G: 2, B: 3, A: 4}) {
		t.Fatalf("cropped pixel = %#v", got)
	}
}

func TestScaleFromFileName(t *testing.T) {
	if got := scaleFromFileName("Icon-iPhone-60@3x.png"); got != 3 {
		t.Fatalf("scale = %d", got)
	}
	if got := scaleFromFileName("icon.png"); got != 0 {
		t.Fatalf("scale = %d", got)
	}
}
