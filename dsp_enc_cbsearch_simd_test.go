package amrnb

import "testing"

// corHXScalar / corHX2Scalar are the straightforward scalar references kept as
// independent oracles for the firRaw-vectorised production versions.
func corHXScalar(h, x, dn []int16, sf int16) {
	var y32 [cL_CODE]int32
	tot := int32(5)
	for k := 0; k < cNB_TRACK; k++ {
		var max int32
		for i := k; i < cL_CODE; i += cSTEP {
			var s int32
			for j := 0; j < cL_CODE-i; j++ {
				s += (int32(x[i+j]) * int32(h[j])) << 1
			}
			y32[i] = s
			if s < 0 {
				s = -s
			}
			if s > max {
				max = s
			}
		}
		tot += max >> 1
	}
	j := norm_l(tot) - sf
	for i := 0; i < cL_CODE; i++ {
		dn[i] = int16((L_shl(y32[i], j) + 0x8000) >> 16)
	}
}

func corHX2Scalar(h, x, dn []int16, sf, nbTrack, step int16) {
	var y32 [cL_CODE]int32
	tot := int32(cLOG2_OF_32)
	for k := int16(0); k < nbTrack; k++ {
		var max int32
		for i := k; i < cL_CODE; i += step {
			var s int32
			for j := i; j < cL_CODE; j++ {
				s += int32(x[j]) * int32(h[j-i])
			}
			s <<= 1
			y32[i] = s
			if a := L_abs(s); a > max {
				max = a
			}
		}
		tot += max >> 1
	}
	j := norm_l(tot) - sf
	for i := 0; i < cL_CODE; i++ {
		dn[i] = round_(L_shl(y32[i], j))
	}
}

// TestCorHXMatchesScalar fuzzes both vectorised correlations against their
// scalar oracles, with large-magnitude h/x to exercise the wrapping int32
// accumulation that makes the lane-parallel order bit-identical.
func TestCorHXMatchesScalar(t *testing.T) {
	seed := uint32(0xC0FFEE)
	next := func() int16 {
		seed = seed*1664525 + 1013904223
		return int16(seed >> 16)
	}
	for trial := 0; trial < 400; trial++ {
		h := make([]int16, cL_CODE)
		x := make([]int16, cL_CODE)
		for i := range h {
			h[i] = next()
			x[i] = next()
		}
		sf := int16(trial & 1) // 0 or 1

		got := make([]int16, cL_CODE)
		want := make([]int16, cL_CODE)
		corHX(h, x, got, sf)
		corHXScalar(h, x, want, sf)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("corHX trial %d pos %d: got=%d want=%d", trial, i, got[i], want[i])
			}
		}

		for _, tc := range []struct{ nb, step int16 }{{4, 4}, {5, 5}} {
			corHX2(h, x, got, sf, tc.nb, tc.step)
			corHX2Scalar(h, x, want, sf, tc.nb, tc.step)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("corHX2(nb=%d) trial %d pos %d: got=%d want=%d", tc.nb, trial, i, got[i], want[i])
				}
			}
		}
	}
}
