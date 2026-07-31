package carfile

import (
	"carfile-go/codec/deepmap2"
	"carfile-go/codec/kcbc"
	"carfile-go/codec/lzfse"
	"fmt"
	"image"
)

// DecodeRenditionImage converts a supported compressed pixel rendition into a
// standard-library image. No CoreUI APIs or external codecs are used.
func DecodeRenditionImage(rendition Rendition) (image.Image, error) {
	csi := rendition.CSI
	if csi.Payload.Tag != "CELM" || len(csi.Payload.Data) < 16 || csi.Payload.CompressionType == nil {
		return nil, fmt.Errorf("rendition has no compressed CELM bitmap")
	}
	data := csi.Payload.Data[16:]
	switch *csi.Payload.CompressionType {
	case 4:
		components, err := pixelComponents(csi.PixelFormat)
		if err != nil {
			return nil, err
		}
		var pixels []byte
		if len(data) >= 4 && string(data[:4]) == "KCBC" {
			pixels, err = kcbc.Decode(data, csi.Width, csi.Height, components)
		} else {
			pixels, err = lzfse.Decode(data)
			if err == nil {
				pixels, err = stripBitmapRowPadding(pixels, csi.Width, csi.Height, components)
			}
		}
		if err != nil {
			return nil, err
		}
		return imageFromPixels(pixels, csi.Width, csi.Height, csi.PixelFormat, false)

	case 11:
		bitmap, err := deepmap2.Decode(data)
		if err != nil {
			return nil, err
		}
		if uint32(bitmap.Width) != csi.Width || uint32(bitmap.Height) != csi.Height {
			return nil, fmt.Errorf("dmp2 geometry %dx%d differs from CSI geometry %dx%d", bitmap.Width, bitmap.Height, csi.Width, csi.Height)
		}
		expectedComponents, err := pixelComponents(csi.PixelFormat)
		if err != nil {
			return nil, err
		}
		if int(bitmap.Components) != expectedComponents {
			return nil, fmt.Errorf("dmp2 has %d components, %s needs %d", bitmap.Components, csi.PixelFormat, expectedComponents)
		}
		// Deepmap2 reconstructs the byte layout used by CoreUI (BGRA for
		// the canonical ARGB pixel format), just like the lossless path.
		return imageFromPixels(bitmap.Pixels, csi.Width, csi.Height, csi.PixelFormat, false)

	default:
		return nil, fmt.Errorf("unsupported bitmap compression %d (%s)", *csi.Payload.CompressionType, csi.Payload.Compression)
	}
}

func pixelComponents(format string) (int, error) {
	switch format {
	case "ARGB":
		return 4, nil
	case "GA8 ":
		return 2, nil
	default:
		return 0, fmt.Errorf("unsupported pixel format %q", format)
	}
}

func stripBitmapRowPadding(pixels []byte, width, height uint32, components int) ([]byte, error) {
	if width == 0 || height == 0 {
		return nil, fmt.Errorf("invalid bitmap geometry %dx%d", width, height)
	}
	tightRow := uint64(width) * uint64(components)
	expected := tightRow * uint64(height)
	if expected > uint64(len(pixels)) {
		return nil, fmt.Errorf("bitmap has %d bytes, expected at least %d", len(pixels), expected)
	}
	if expected == uint64(len(pixels)) {
		return pixels, nil
	}
	if len(pixels)%int(height) != 0 {
		return nil, fmt.Errorf("bitmap has %d bytes across %d rows", len(pixels), height)
	}
	sourceRow := len(pixels) / int(height)
	if sourceRow < int(tightRow) {
		return nil, fmt.Errorf("bitmap row is %d bytes, expected at least %d", sourceRow, tightRow)
	}
	tight := make([]byte, 0, int(expected))
	for row := 0; row < int(height); row++ {
		start := row * sourceRow
		tight = append(tight, pixels[start:start+int(tightRow)]...)
	}
	return tight, nil
}

func imageFromPixels(pixels []byte, width, height uint32, format string, rgbaOrder bool) (image.Image, error) {
	components, err := pixelComponents(format)
	if err != nil {
		return nil, err
	}
	expected := uint64(width) * uint64(height) * uint64(components)
	if uint64(len(pixels)) != expected {
		return nil, fmt.Errorf("%s bitmap has %d bytes, expected %d", format, len(pixels), expected)
	}
	rect := image.Rect(0, 0, int(width), int(height))
	result := image.NewRGBA(rect)
	switch format {
	case "ARGB":
		for source, dest := 0, 0; source < len(pixels); source, dest = source+4, dest+4 {
			if rgbaOrder {
				copy(result.Pix[dest:dest+4], pixels[source:source+4])
			} else {
				result.Pix[dest+0] = pixels[source+2]
				result.Pix[dest+1] = pixels[source+1]
				result.Pix[dest+2] = pixels[source+0]
				result.Pix[dest+3] = pixels[source+3]
			}
		}
	case "GA8 ":
		for source, dest := 0, 0; source < len(pixels); source, dest = source+2, dest+4 {
			gray, alpha := pixels[source], pixels[source+1]
			result.Pix[dest+0] = gray
			result.Pix[dest+1] = gray
			result.Pix[dest+2] = gray
			result.Pix[dest+3] = alpha
		}
	}
	return result, nil
}
