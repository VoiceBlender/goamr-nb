package amrnb

import "testing"

// TestCode8i40RoundTrip checks the MR102 8-pulse search round-trips through the
// decoder's dec8i40_31bits (decompressCode) to the same innovation vector — the
// strongest validation of search10and8i40 + build8Code + compressCode against
// decompressCode.
func TestCode8i40RoundTrip(t *testing.T) {
	h := makeH1()
	for trial := 0; trial < 8; trial++ {
		x := make([]int16, cL_CODE)
		cn := make([]int16, cL_CODE)
		seed := uint32(0x3000 + trial*1777)
		for i := range x {
			seed = seed*1664525 + 1013904223
			x[i] = int16(int32(seed>>20) - 2048)
			seed = seed*1664525 + 1013904223
			cn[i] = int16(int32(seed>>20) - 2048)
		}
		hc := append([]int16(nil), h...)
		cod := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		indx := make([]int16, 7)
		code8i40_31bits(x, cn, hc, cod, y, indx)

		dec := make([]int16, cL_CODE)
		dec8i40_31bits(indx, dec)
		for i := range cod {
			if cod[i] != dec[i] {
				t.Fatalf("trial %d: enc cod[%d]=%d != dec %d", trial, i, cod[i], dec[i])
			}
		}
	}
}

// TestCode8i40Deterministic confirms repeatability.
func TestCode8i40Deterministic(t *testing.T) {
	h := makeH1()
	x := make([]int16, cL_CODE)
	cn := make([]int16, cL_CODE)
	for i := range x {
		x[i] = int16(1400 * sinApprox(float64(i)*0.31))
		cn[i] = int16(1100 * sinApprox(float64(i)*0.23))
	}
	run := func() []int16 {
		hc := append([]int16(nil), h...)
		cod := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		indx := make([]int16, 7)
		code8i40_31bits(x, cn, hc, cod, y, indx)
		return indx
	}
	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("code8i40_31bits non-deterministic at %d", i)
		}
	}
}
