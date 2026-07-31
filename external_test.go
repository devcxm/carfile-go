package carfile_test

import (
	"fmt"

	carfile "carfile-go"
	"carfile-go/codec/lzfse"
)

func ExampleExtractOptions() {
	options := carfile.ExtractOptions{Format: carfile.FormatXCAssets, OutputDirectory: "restored"}
	fmt.Println(options.Format)
	// Output: xcassets
}

func Example_specializedCodecPackage() {
	_, _ = lzfse.Decode([]byte("not an LZFSE stream"))
	fmt.Println("codec is independently importable")
	// Output: codec is independently importable
}
