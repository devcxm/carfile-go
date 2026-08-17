//go:build !darwin || !cgo

package deepmap2

func decodeNative([]byte, uint16, uint16) (Bitmap, bool, error) {
	return Bitmap{}, false, nil
}
