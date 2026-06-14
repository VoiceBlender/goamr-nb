package amrnb

import "testing"

// TestCode4i40RoundTrip checks the MR74/MR795 4-pulse search round-trips through
// the decoder's decode4i40_17bits (Gray-decoded) to the same innovation vector.
func TestCode4i40RoundTrip(t *testing.T) {
	h := makeH1()
	for trial := 0; trial < 8; trial++ {
		x := make([]int16, cL_CODE)
		seed := uint32(0x2100 + trial*1511)
		for i := range x {
			seed = seed*1664525 + 1013904223
			x[i] = int16(int32(seed>>20) - 2048)
		}
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		index, sign := code4i40_17bits(x, hc, cL_CODE, 0, code, y)

		dec := make([]int16, cL_CODE)
		decode4i40_17bits(sign, index, dec)
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
		if nz != 4 {
			t.Errorf("trial %d: %d pulses, want 4", trial, nz)
		}
	}
}

// TestCode4i40Deterministic confirms repeatability.
func TestCode4i40Deterministic(t *testing.T) {
	h := makeH1()
	x := make([]int16, cL_CODE)
	for i := range x {
		x[i] = int16(1450 * sinApprox(float64(i)*0.29))
	}
	run := func() int16 {
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		idx, _ := code4i40_17bits(x, hc, cL_CODE, 0, code, y)
		return idx
	}
	if run() != run() {
		t.Error("code4i40_17bits non-deterministic")
	}
}
