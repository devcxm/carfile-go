package carfile

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

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
