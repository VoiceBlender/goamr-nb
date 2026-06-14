package amrnb

import "testing"

// TestCode2i40_11RoundTrip checks the MR59 2-pulse search round-trips through
// the decoder's decode2i40_11bits to the same innovation vector.
func TestCode2i40_11RoundTrip(t *testing.T) {
	h := makeH1()
	for trial := 0; trial < 6; trial++ {
		x := make([]int16, cL_CODE)
		seed := uint32(0x900 + trial*877)
		for i := range x {
			seed = seed*1664525 + 1013904223
			x[i] = int16(int32(seed>>20) - 2048)
		}
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		index, sign := code2i40_11bits(x, hc, cL_CODE, 0, code, y)

		dec := make([]int16, cL_CODE)
		decode2i40_11bits(sign, index, dec)
		for i := range code {
			if code[i] != dec[i] {
				t.Fatalf("trial %d: enc code[%d]=%d != dec %d (idx=%d sign=%d)",
					trial, i, code[i], dec[i], index, sign)
			}
		}

		nz := 0
		for _, v := range code {
			if v != 0 {
				nz++
			}
		}
		if nz != 2 {
			t.Errorf("trial %d: %d pulses, want 2", trial, nz)
		}
	}
}

// TestCode2i40_11Deterministic confirms repeatability.
func TestCode2i40_11Deterministic(t *testing.T) {
	h := makeH1()
	x := make([]int16, cL_CODE)
	for i := range x {
		x[i] = int16(1300 * sinApprox(float64(i)*0.37))
	}
	run := func() int16 {
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		idx, _ := code2i40_11bits(x, hc, cL_CODE, 0, code, y)
		return idx
	}
	if run() != run() {
		t.Error("code2i40_11bits non-deterministic")
	}
}
