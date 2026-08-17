//go:build darwin && cgo

package deepmap2

import (
	"math"
	"testing"
)

func TestHalfToFloat32(t *testing.T) {
	tests := []struct {
		half uint16
		want float32
	}{
		{0x0000, 0},
		{0x3c00, 1},
		{0xc000, -2},
		{0x0001, 1.0 / 16777216.0},
	}
	for _, test := range tests {
		if got := halfToFloat32(test.half); math.Float32bits(got) != math.Float32bits(test.want) {
			t.Errorf("halfToFloat32(%#04x) = %g, want %g", test.half, got, test.want)
		}
	}
}
