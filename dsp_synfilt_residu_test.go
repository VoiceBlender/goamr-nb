package amrnb

import "testing"

// residuScalar is the straightforward scalar reference for residu, kept here as
// an independent oracle for the firRaw-vectorised production version.
func residuScalar(coef, buf []int16, start, inputLen int, residual []int16) {
	for i := 0; i < inputLen; i++ {
		s := int32(0x800)
		for j := 0; j <= cM; j++ {
			s += int32(coef[j]) * int32(buf[start+i-j])
		}
		residual[i] = int16(s >> 12)
	}
}

// TestResiduMatchesScalar fuzzes the firRaw-based residu against the scalar
// oracle over pseudo-random coefficients and inputs, including large-magnitude
// values that exercise the wrapping int32 accumulation.
func TestResiduMatchesScalar(t *testing.T) {
	seed := uint32(0x9E3779B9)
	next := func() int16 {
		seed = seed*1664525 + 1013904223
		return int16(seed >> 16)
	}
	for trial := 0; trial < 500; trial++ {
		coef := make([]int16, cMP1)
		for i := range coef {
			coef[i] = next()
		}
		const start = cM
		buf := make([]int16, start+cL_SUBFR)
		for i := range buf {
			buf[i] = next()
		}
		got := make([]int16, cL_SUBFR)
		want := make([]int16, cL_SUBFR)
		residu(coef, buf, start, cL_SUBFR, got)
		residuScalar(coef, buf, start, cL_SUBFR, want)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("trial %d sample %d: residu=%d scalar=%d", trial, i, got[i], want[i])
			}
		}
	}
}
