package amrnb

import "testing"

// analysisA runs the LP chain on a seeded signal and returns A(z).
func analysisA(seed int) []int16 {
	x := make([]int16, cL_WINDOW)
	for i := range x {
		f := float64(i)
		x[i] = int16(7000*sinApprox(f*(0.18+float64(seed)*0.02)) + 2500*sinApprox(f*0.5))
	}
	rh := make([]int16, cMP1)
	rl := make([]int16, cMP1)
	autocorr(x, cM, rh, rl, window_200_40)
	lagWindow(cM, rh, rl)
	var lev levinsonState
	a := make([]int16, cMP1)
	rc := make([]int16, 4)
	lev.levinson(rh, rl, a, rc)
	return a
}

func allA0Are4096(t *testing.T, tag string, az []int16) {
	t.Helper()
	for sf := 0; sf < cNB_SUBFR; sf++ {
		if az[sf*cMP1] != 4096 {
			t.Errorf("%s subframe %d: a[0]=%d, want 4096", tag, sf, az[sf*cMP1])
		}
	}
}

// TestLspWrapper3 checks the non-MR122 path: valid interpolated filters and an
// LSF index that the decoder reconstructs into the same quantized filters.
func TestLspWrapper3(t *testing.T) {
	var enc lspEncState
	enc.reset()
	var dec dPlsfState
	dec.reset()

	az := make([]int16, cNB_SUBFR*cMP1)
	azQ := make([]int16, cNB_SUBFR*cMP1)
	copy(az[3*cMP1:], analysisA(1)) // A_new at the 4th subframe

	anap := make([]int16, 5)
	n := enc.lsp(dMR59, az, azQ, anap)
	if n != 3 {
		t.Fatalf("expected 3 indices, got %d", n)
	}
	allA0Are4096(t, "az", az)
	allA0Are4096(t, "azQ", azQ)

	// reconstruct azQ from the indices via the decoder + interpolation
	lspQ := make([]int16, cM)
	dec.dPlsf3(dMR59, 0, anap[:3], lspQ)
	wantAzQ := make([]int16, cNB_SUBFR*cMP1)
	intLpc1to3(lsp_init_data, lspQ, wantAzQ)
	for i := range azQ {
		if azQ[i] != wantAzQ[i] {
			t.Fatalf("azQ[%d]=%d != decoder-reconstructed %d", i, azQ[i], wantAzQ[i])
		}
	}
}

// TestLspWrapper5 checks the MR122 path (mid + new LSP, 5 indices).
func TestLspWrapper5(t *testing.T) {
	var enc lspEncState
	enc.reset()
	var dec dPlsfState
	dec.reset()

	az := make([]int16, cNB_SUBFR*cMP1)
	azQ := make([]int16, cNB_SUBFR*cMP1)
	copy(az[cMP1:], analysisA(1))   // A_mid at the 2nd subframe
	copy(az[3*cMP1:], analysisA(3)) // A_new at the 4th subframe

	anap := make([]int16, 5)
	n := enc.lsp(dMR122, az, azQ, anap)
	if n != 5 {
		t.Fatalf("expected 5 indices, got %d", n)
	}
	allA0Are4096(t, "az", az)
	allA0Are4096(t, "azQ", azQ)

	lspMidQ := make([]int16, cM)
	lspNewQ := make([]int16, cM)
	dec.dPlsf5(0, anap[:5], lspMidQ, lspNewQ)
	wantAzQ := make([]int16, cNB_SUBFR*cMP1)
	intLpc1and3(lsp_init_data, lspMidQ, lspNewQ, wantAzQ)
	for i := range azQ {
		if azQ[i] != wantAzQ[i] {
			t.Fatalf("MR122 azQ[%d]=%d != decoder-reconstructed %d", i, azQ[i], wantAzQ[i])
		}
	}
}
