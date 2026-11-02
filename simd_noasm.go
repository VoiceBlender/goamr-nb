//go:build !amd64 || purego

package amrnb

// useAVX2 is false: the portable kernels are always used.
const useAVX2 = false

// firRaw falls back to the portable implementation on non-amd64 targets and
// with the purego build tag.
func firRaw(dst []int32, x []int16, coef []int16) { firRawGeneric(dst, x, coef) }

// firDot falls back to the portable implementation on non-amd64 targets and
// with the purego build tag.
func firDot(a, b []int16) int32 { return firDotGeneric(a, b) }
