//go:build !darwin || !cgo

package deepmap

import "github.com/devcxm/carfile-go/codec/deepmap2"

func decodeNativeChunk([]byte, uint16, uint16) (deepmap2.Bitmap, bool, error) {
	return deepmap2.Bitmap{}, false, nil
}
