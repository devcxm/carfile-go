package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	carfile "github.com/devcxm/carfile-go"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("carfile", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var output, formatName string
	var includes stringListFlag
	var quiet, showHelp, showVersion bool
	flags.StringVar(&output, "o", "", "output directory")
	flags.StringVar(&output, "output", "", "output directory")
	flags.StringVar(&formatName, "f", string(carfile.FormatResources), "output format")
	flags.StringVar(&formatName, "format", string(carfile.FormatResources), "output format")
	flags.Var(&includes, "i", "asset or rendition glob to include (repeatable)")
	flags.Var(&includes, "include", "asset or rendition glob to include (repeatable)")
	flags.BoolVar(&quiet, "q", false, "disable progress output")
	flags.BoolVar(&quiet, "quiet", false, "disable progress output")
	flags.BoolVar(&showHelp, "h", false, "show help")
	flags.BoolVar(&showHelp, "help", false, "show help")
	flags.BoolVar(&showVersion, "v", false, "show version")
	flags.BoolVar(&showVersion, "version", false, "show version")
	flags.Usage = func() { printUsage(stderr) }

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if showHelp {
		printUsage(stdout)
		return 0
	}
	if showVersion {
		fmt.Fprintf(stdout, "carfile %s\n", carfile.Version)
		return 0
	}
	if flags.NArg() != 1 {
		printUsage(stderr)
		return 2
	}

	format, err := carfile.ParseOutputFormat(formatName)
	if err != nil {
		fmt.Fprintf(stderr, "carfile: %v\n", err)
		return 2
	}
	var printer *progressPrinter
	var progress func(carfile.Progress)
	if !quiet {
		printer = newProgressPrinter(stderr)
		progress = printer.report
	}
	result, err := carfile.ExtractFile(flags.Arg(0), carfile.ExtractOptions{
		OutputDirectory: output,
		Format:          format,
		Includes:        includes,
		Progress:        progress,
	})
	if printer != nil {
		printer.finish()
	}
	if err != nil {
		fmt.Fprintf(stderr, "carfile: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Extracted %d file(s) to %s using %s format", result.Written, result.OutputDirectory, result.Format)
	if result.Skipped != 0 {
		fmt.Fprintf(stdout, " (%d skipped)", result.Skipped)
	}
	if result.Failed != 0 {
		fmt.Fprintf(stdout, " (%d failed)", result.Failed)
	}
	fmt.Fprintln(stdout)
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  carfile [options] <Assets.car>

By default, all logical resources are recovered beside the input file in an
<name>-extracted directory.

Options:
  -o, --output DIR       Output directory
  -f, --format FORMAT    resources (default), xcassets, raw, png, or json
  -i, --include PATTERN  Include asset/file glob; may be repeated
  -q, --quiet            Disable progress output
  -v, --version          Print version
  -h, --help             Show this help

Examples:
  carfile Assets.car
  carfile -o output Assets.car
  carfile -i AppIcon Assets.car
  carfile -i 'myBannerImage_*' -i '*@2x.png' Assets.car
  carfile --format xcassets --output restored Assets.car
  carfile -f raw -o payloads Assets.car`)
}

type stringListFlag []string

func (values *stringListFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *stringListFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("include pattern cannot be empty")
	}
	*values = append(*values, value)
	return nil
}

type progressPrinter struct {
	writer      io.Writer
	interactive bool
	lastWidth   int
	active      bool
	finished    bool
}

func newProgressPrinter(writer io.Writer) *progressPrinter {
	printer := &progressPrinter{writer: writer}
	if file, ok := writer.(*os.File); ok {
		if info, err := file.Stat(); err == nil {
			printer.interactive = info.Mode()&os.ModeCharDevice != 0
		}
	}
	return printer
}

func (p *progressPrinter) report(event carfile.Progress) {
	name := event.FileName
	if event.AssetName != "" {
		name = event.AssetName + "/" + name
	}
	percent := 100
	if event.Total > 0 {
		percent = event.Current * 100 / event.Total
	}
	line := fmt.Sprintf("[%3d%% %d/%d] %s", percent, event.Current, event.Total, name)
	if p.interactive {
		padding := ""
		if p.lastWidth > len(line) {
			padding = strings.Repeat(" ", p.lastWidth-len(line))
		}
		fmt.Fprintf(p.writer, "\r%s%s", line, padding)
		p.lastWidth = len(line)
		p.active = true
		if event.Current == event.Total {
			fmt.Fprintln(p.writer)
			p.finished = true
		}
		return
	}
	fmt.Fprintln(p.writer, line)
}

func (p *progressPrinter) finish() {
	if p.interactive && p.active && !p.finished {
		fmt.Fprintln(p.writer)
	}
}
