package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	carfile "carfile-go"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("carfile", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var output, formatName string
	var showHelp, showVersion bool
	flags.StringVar(&output, "o", "", "output directory")
	flags.StringVar(&output, "output", "", "output directory")
	flags.StringVar(&formatName, "f", string(carfile.FormatResources), "output format")
	flags.StringVar(&formatName, "format", string(carfile.FormatResources), "output format")
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
	result, err := carfile.ExtractFile(flags.Arg(0), carfile.ExtractOptions{
		OutputDirectory: output,
		Format:          format,
	})
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
  -v, --version          Print version
  -h, --help             Show this help

Examples:
  carfile Assets.car
  carfile -o output Assets.car
  carfile --format xcassets --output restored Assets.car
  carfile -f raw -o payloads Assets.car`)
}
