package amrnb

import "testing"

// TestCode10i40RoundTrip checks the MR122 10-pulse search round-trips through
// the decoder's dec10i40_35bits (Gray-decoded) to the same innovation vector.
func TestCode10i40RoundTrip(t *testing.T) {
	h := makeH1()
	for trial := 0; trial < 8; trial++ {
		x := make([]int16, cL_CODE)
		cn := make([]int16, cL_CODE)
		seed := uint32(0x4000 + trial*2003)
		for i := range x {
			seed = seed*1664525 + 1013904223
			x[i] = int16(int32(seed>>20) - 2048)
			seed = seed*1664525 + 1013904223
			cn[i] = int16(int32(seed>>20) - 2048)
		}
		hc := append([]int16(nil), h...)
		cod := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		indx := make([]int16, 10)
		code10i40_35bits(x, cn, hc, cod, y, indx)

		dec := make([]int16, cL_CODE)
		dec10i40_35bits(indx, dec)
		for i := range cod {
			if cod[i] != dec[i] {
				t.Fatalf("trial %d: enc cod[%d]=%d != dec %d", trial, i, cod[i], dec[i])
			}
		}
	}
}

// TestCode10i40Deterministic confirms repeatability.
func TestCode10i40Deterministic(t *testing.T) {
	h := makeH1()
	x := make([]int16, cL_CODE)
	cn := make([]int16, cL_CODE)
	for i := range x {
		x[i] = int16(1350 * sinApprox(float64(i)*0.27))
		cn[i] = int16(1000 * sinApprox(float64(i)*0.19))
	}
	run := func() []int16 {
		hc := append([]int16(nil), h...)
		cod := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		indx := make([]int16, 10)
		code10i40_35bits(x, cn, hc, cod, y, indx)
		return indx
	}
	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("code10i40_35bits non-deterministic at %d", i)
		}
	}
}
