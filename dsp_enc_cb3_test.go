package amrnb

import "testing"

// TestCode3i40RoundTrip checks the MR67 3-pulse search round-trips through the
// decoder's decode3i40_14bits to the same innovation vector.
func TestCode3i40RoundTrip(t *testing.T) {
	h := makeH1()
	for trial := 0; trial < 8; trial++ {
		x := make([]int16, cL_CODE)
		seed := uint32(0x1500 + trial*1013)
		for i := range x {
			seed = seed*1664525 + 1013904223
			x[i] = int16(int32(seed>>20) - 2048)
		}
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		index, sign := code3i40_14bits(x, hc, cL_CODE, 0, code, y)

		dec := make([]int16, cL_CODE)
		decode3i40_14bits(sign, index, dec)
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
		if nz != 3 {
			t.Errorf("trial %d: %d pulses, want 3", trial, nz)
		}
	}
}

// TestCode3i40Deterministic confirms repeatability.
func TestCode3i40Deterministic(t *testing.T) {
	h := makeH1()
	x := make([]int16, cL_CODE)
	for i := range x {
		x[i] = int16(1400 * sinApprox(float64(i)*0.33))
	}
	run := func() int16 {
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		idx, _ := code3i40_14bits(x, hc, cL_CODE, 0, code, y)
		return idx
	}
	if run() != run() {
		t.Error("code3i40_14bits non-deterministic")
	}
}
