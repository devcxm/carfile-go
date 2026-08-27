package carfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportRaw(t *testing.T) {
	catalog := Catalog{Renditions: []Rendition{
		{
			AssetName: "document",
			CSI:       CSI{Name: "document.pdf", Payload: Payload{Tag: "RAWD", DeclaredLength: 4, Data: append([]byte("DWAR\x00\x00\x00\x00\x04\x00\x00\x00"), "%PDF"...)}},
		},
		{
			AssetName: "icon",
			CSI:       CSI{Width: 10, Height: 20, ScaleFactor: 200, Payload: Payload{Tag: "CELM", Compression: "lzfse", DeclaredLength: 1, Data: append(make([]byte, 16), 1, 2, 3)}},
		},
	}}
	directory := t.TempDir()
	result, err := catalog.ExportRaw(directory)
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 2 || result.Skipped != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	pdf, err := os.ReadFile(filepath.Join(directory, result.Files[0].File))
	if err != nil {
		t.Fatal(err)
	}
	if string(pdf) != "%PDF" || !result.Files[0].Openable {
		t.Fatalf("unexpected RAWD export: %q, %#v", pdf, result.Files[0])
	}
	compressed, err := os.ReadFile(filepath.Join(directory, result.Files[1].File))
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) != 3 || result.Files[1].Openable {
		t.Fatalf("unexpected CELM export: %v, %#v", compressed, result.Files[1])
	}
}

func TestExportRawMarksCompressedOriginalAsNotOpenable(t *testing.T) {
	stream := []byte{'b', 'v', 'x', '-', 0, 0, 0, 0, 'b', 'v', 'x', '$'}
	payload := append([]byte("DWAR\x01\x00\x00\x00\x0c\x00\x00\x00"), stream...)
	catalog := Catalog{Renditions: []Rendition{{
		AssetName: "symbol", CSI: CSI{Name: "symbol.svg", Payload: Payload{Tag: "RAWD", DeclaredLength: uint32(len(stream)), Data: payload}},
	}}}
	result, err := catalog.ExportRaw(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.Files[0].Openable || result.Files[0].Status != "RAWD wrapper removed; LZFSE data remains compressed" {
		t.Fatalf("unexpected record: %#v", result.Files[0])
	}
}
