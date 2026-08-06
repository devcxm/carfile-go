package main

import (
	"bytes"
	"strings"
	"testing"

	carfile "github.com/devcxm/carfile-go"
)

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "carfile ") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--format") || !strings.Contains(stdout.String(), "xcassets") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestInvalidFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--format", "zip", "Assets.car"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stderr.String(), "unsupported output format") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRepeatedIncludeFlag(t *testing.T) {
	var values stringListFlag
	if err := values.Set("AppIcon"); err != nil {
		t.Fatal(err)
	}
	if err := values.Set("*@2x.png"); err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != "AppIcon" || values[1] != "*@2x.png" {
		t.Fatalf("values = %q", values)
	}
}

func TestProgressPrinter(t *testing.T) {
	var output bytes.Buffer
	printer := &progressPrinter{writer: &output}
	printer.report(carfile.Progress{
		Current: 1, Total: 2, RenditionIndex: 4,
		AssetName: "AppIcon", FileName: "Icon@2x.png",
	})
	if got, want := output.String(), "[ 50% 1/2] AppIcon/Icon@2x.png\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
