//go:build darwin && cgo

package deepmap2

/*
#include <dlfcn.h>
#include <stddef.h>
#include <stdint.h>

typedef struct {
	void *data;
	size_t height;
	size_t width;
	size_t rowBytes;
} carfile_dmp2_vimage_buffer;

typedef size_t (*carfile_dmp2_decode_fn)(carfile_dmp2_vimage_buffer *, uint32_t, const void *, size_t, void *);

static size_t carfile_decode_dmp2(
	const uint8_t *src, size_t srcSize, uint8_t *dst,
	size_t width, size_t height, size_t rowBytes, uint32_t pixelFormat
) {
	void *handle = dlopen("/System/Library/Frameworks/Accelerate.framework/Accelerate", RTLD_LAZY | RTLD_LOCAL);
	if (handle == NULL) return 0;
	carfile_dmp2_decode_fn decode = (carfile_dmp2_decode_fn)dlsym(handle, "vImageDeepmap2Decode");
	if (decode == NULL) {
		dlclose(handle);
		return 0;
	}
	carfile_dmp2_vimage_buffer output = {dst, height, width, rowBytes};
	size_t result = decode(&output, pixelFormat, src, srcSize, NULL);
	dlclose(handle);
	return result;
}
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"
)

func decodeNative(src []byte, overrideWidth, overrideHeight uint16) (Bitmap, bool, error) {
	var bitmap Bitmap
	inner := src
	if len(src) >= 20 && string(src[16:20]) == "dmp2" {
		inner = src[16:]
	}
	if len(inner) < 16 || string(inner[:4]) != "dmp2" {
		return bitmap, false, nil
	}
	pixelFormat := inner[7]
	layout, err := layoutForPixelFormat(pixelFormat)
	if err != nil {
		return bitmap, true, err
	}
	width := binary.LittleEndian.Uint16(inner[8:10])
	height := binary.LittleEndian.Uint16(inner[10:12])
	if overrideWidth != 0 {
		width = overrideWidth
	}
	if overrideHeight != 0 {
		height = overrideHeight
	}
	outputBytes := uint64(width) * uint64(height) * uint64(layout.bytesPerPixel())
	if outputBytes == 0 || outputBytes > uint64(maxInt()) {
		return bitmap, true, fmt.Errorf("native dmp2 bitmap is too large")
	}
	pixels := make([]byte, int(outputBytes))
	decoded := C.carfile_decode_dmp2(
		(*C.uint8_t)(unsafe.Pointer(&inner[0])), C.size_t(len(inner)),
		(*C.uint8_t)(unsafe.Pointer(&pixels[0])),
		C.size_t(width), C.size_t(height), C.size_t(int(width)*layout.bytesPerPixel()),
		C.uint32_t(pixelFormat),
	)
	if uint64(decoded) != outputBytes {
		return bitmap, true, fmt.Errorf("Accelerate vImageDeepmap2 decoded %d bytes, expected %d", uint64(decoded), outputBytes)
	}
	if pixelFormat == 20 {
		for offset := 0; offset < len(pixels); offset += 8 {
			var components [4]uint16
			for component := range components {
				half := binary.LittleEndian.Uint16(pixels[offset+component*2 : offset+component*2+2])
				value := halfToFloat32(half)
				if value < 0 {
					value = 0
				} else if value > 1 {
					value = 1
				}
				components[component] = uint16(math.Round(float64(value * 10000)))
			}
			// vImage produces RGBA half floats. Normalize to the BGRA-like
			// component order used by the other CoreUI bitmap paths.
			binary.LittleEndian.PutUint16(pixels[offset:offset+2], components[2])
			binary.LittleEndian.PutUint16(pixels[offset+2:offset+4], components[1])
			binary.LittleEndian.PutUint16(pixels[offset+4:offset+6], components[0])
			binary.LittleEndian.PutUint16(pixels[offset+6:offset+8], components[3])
		}
	}
	return Bitmap{
		Width: width, Height: height, PixelFormat: pixelFormat,
		Components: uint8(layout.components), BytesPerComponent: uint8(layout.bytesPerComponent),
		Pixels: pixels,
	}, true, nil
}

func halfToFloat32(value uint16) float32 {
	sign := uint32(value&0x8000) << 16
	exponent := int32(value>>10) & 0x1f
	fraction := uint32(value & 0x03ff)
	var bits uint32
	switch exponent {
	case 0:
		if fraction == 0 {
			bits = sign
		} else {
			exponent = -14
			for fraction&0x0400 == 0 {
				fraction <<= 1
				exponent--
			}
			fraction &= 0x03ff
			bits = sign | (uint32(exponent+127) << 23) | (fraction << 13)
		}
	case 31:
		bits = sign | 0x7f800000 | (fraction << 13)
	default:
		bits = sign | (uint32(exponent+112) << 23) | (fraction << 13)
	}
	return math.Float32frombits(bits)
}
