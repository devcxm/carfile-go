//go:build darwin && cgo

package deepmap

/*
#include <dlfcn.h>
#include <stddef.h>
#include <stdint.h>

typedef struct {
	const void *data;
	size_t size;
	void *state;
} carfile_deepmap_stream;

typedef struct {
	void *data;
	size_t height;
	size_t width;
	size_t rowBytes;
} carfile_vimage_buffer;

typedef int (*carfile_stream_create_fn)(carfile_deepmap_stream *, carfile_vimage_buffer *, uint32_t, void *);
typedef int (*carfile_stream_process_fn)(carfile_deepmap_stream *);
typedef int (*carfile_stream_release_fn)(carfile_deepmap_stream *);

static int carfile_decode_deepmap(
	const uint8_t *src, size_t srcSize, uint8_t *dst,
	size_t width, size_t height, size_t rowBytes, uint32_t pixelFormat
) {
	void *handle = dlopen("/System/Library/Frameworks/Accelerate.framework/Accelerate", RTLD_LAZY | RTLD_LOCAL);
	if (handle == NULL) return -100;
	carfile_stream_create_fn create = (carfile_stream_create_fn)dlsym(handle, "vImageDeepmapDecodeStreamCreate");
	carfile_stream_process_fn process = (carfile_stream_process_fn)dlsym(handle, "vImageDeepmapDecodeStreamProcess");
	carfile_stream_release_fn release = (carfile_stream_release_fn)dlsym(handle, "vImageDeepmapDecodeStreamRelease");
	if (create == NULL || process == NULL || release == NULL) {
		dlclose(handle);
		return -101;
	}
	carfile_deepmap_stream stream = {0};
	carfile_vimage_buffer output = {dst, height, width, rowBytes};
	int status = create(&stream, &output, pixelFormat, NULL);
	if (status != 0) {
		dlclose(handle);
		return -200 + status;
	}
	stream.data = src;
	stream.size = srcSize;
	while (status == 0) status = process(&stream);
	if (status == 1) status = release(&stream);
	else status = -300 + status;
	dlclose(handle);
	return status;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/devcxm/carfile-go/codec/deepmap2"
)

func decodeNativeChunk(src []byte, width, height uint16) (deepmap2.Bitmap, bool, error) {
	var bitmap deepmap2.Bitmap
	if len(src) < 12 || string(src[:4]) != "dmap" {
		return bitmap, true, fmt.Errorf("native deepmap payload has invalid magic")
	}
	pixelFormat := src[7]
	components, bytesPerComponent := 0, 1
	switch pixelFormat {
	case 1:
		components = 1
	case 2:
		components = 2
	case 3:
		components = 3
	case 4:
		components = 4
	case 20:
		components, bytesPerComponent = 4, 2
	default:
		return bitmap, true, fmt.Errorf("native deepmap pixel format %d is unsupported", pixelFormat)
	}
	outputBytes := uint64(width) * uint64(height) * uint64(components) * uint64(bytesPerComponent)
	if outputBytes == 0 || outputBytes > uint64(maxInt()) {
		return bitmap, true, fmt.Errorf("native deepmap bitmap is too large")
	}
	pixels := make([]byte, int(outputBytes))
	status := C.carfile_decode_deepmap(
		(*C.uint8_t)(unsafe.Pointer(&src[0])), C.size_t(len(src)),
		(*C.uint8_t)(unsafe.Pointer(&pixels[0])),
		C.size_t(width), C.size_t(height), C.size_t(int(width)*components*bytesPerComponent),
		C.uint32_t(pixelFormat),
	)
	if status != 0 {
		return bitmap, true, fmt.Errorf("Accelerate vImageDeepmap decode failed with status %d", int(status))
	}
	return deepmap2.Bitmap{
		Width: width, Height: height, PixelFormat: pixelFormat,
		Components: uint8(components), BytesPerComponent: uint8(bytesPerComponent),
		Pixels: pixels,
	}, true, nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
