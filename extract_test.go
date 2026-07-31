package carfile

import (
	"path/filepath"
	"testing"
)

func TestParseOutputFormat(t *testing.T) {
	tests := map[string]OutputFormat{
		"": FormatResources, "files": FormatResources, "xcassets": FormatXCAssets,
		"raw": FormatRaw, "images": FormatPNG, "json": FormatJSON,
	}
	for input, want := range tests {
		got, err := ParseOutputFormat(input)
		if err != nil {
			t.Fatalf("ParseOutputFormat(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseOutputFormat(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := ParseOutputFormat("zip"); err == nil {
		t.Fatal("expected invalid format error")
	}
}

func TestDefaultOutputDirectory(t *testing.T) {
	got := DefaultOutputDirectory(filepath.Join("tmp", "Assets.car"))
	want := filepath.Join("tmp", "Assets-extracted")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
