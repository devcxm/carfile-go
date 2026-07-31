package carfile

import "testing"

func TestExportReportsProgressForSelectedRenditions(t *testing.T) {
	raw := func(value byte) Payload {
		data := append([]byte("DWAR\x00\x00\x00\x00\x01\x00\x00\x00"), value)
		return Payload{Tag: "RAWD", DeclaredLength: 1, Data: data}
	}
	catalog := Catalog{Renditions: []Rendition{
		{AssetName: "First", CSI: CSI{Name: "first.bin", Payload: raw(1)}},
		{AssetName: "Second", CSI: CSI{Name: "second.bin", Payload: raw(2)}},
	}}
	var events []Progress
	result, err := catalog.Export(ExtractOptions{
		Format:          FormatRaw,
		OutputDirectory: t.TempDir(),
		Includes:        []string{"Second"},
		Progress: func(event Progress) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 1 || len(events) != 1 {
		t.Fatalf("result = %#v, events = %#v", result, events)
	}
	event := events[0]
	if event.Current != 1 || event.Total != 1 || event.RenditionIndex != 1 || event.AssetName != "Second" || event.FileName != "second.bin" {
		t.Fatalf("event = %#v", event)
	}
}
