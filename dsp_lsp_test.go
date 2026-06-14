package amrnb

import "testing"

// descending LSP sets used as fixtures (lspLsf requires descending order).
func sampleLSPs() [][]int16 {
	a := make([]int16, cM)
	copy(a, lsp_init_data)
	// a second, perturbed descending set
	b := []int16{31000, 24000, 19000, 12000, 5000, -2000, -10000, -17000, -23000, -29000}
	return [][]int16{a, b}
}

// TestLspAzA0 checks the order-10 LSP->A(z) conversion always yields a[0]=4096
// (the fixed Q12 leading coefficient) and a full 11-tap vector.
func TestLspAzA0(t *testing.T) {
	for k, lsp := range sampleLSPs() {
		a := make([]int16, cMP1)
		lspAz(lsp, a)
		if a[0] != 4096 {
			t.Errorf("set %d: a[0]=%d, want 4096", k, a[0])
		}
	}
}

// TestLspLsfRoundTrip converts LSP->LSF->LSP and checks the result returns
// close to the original (the table interpolation is lossy to a few units), and
// that the intermediate LSFs are ascending and in the normalized (0,0.5) range.
func TestLspLsfRoundTrip(t *testing.T) {
	for k, lsp := range sampleLSPs() {
		lsf := make([]int16, cM)
		lspLsf(lsp, lsf, cM)
		// LSFs must be strictly ascending and within (0, 16384] (0..0.5 in Q15).
		for i := 0; i < cM; i++ {
			if lsf[i] <= 0 || lsf[i] >= 16384 {
				t.Errorf("set %d: lsf[%d]=%d out of (0,16384)", k, i, lsf[i])
			}
			if i > 0 && lsf[i] <= lsf[i-1] {
				t.Errorf("set %d: lsf not ascending at %d (%d <= %d)", k, i, lsf[i], lsf[i-1])
			}
		}
		back := make([]int16, cM)
		lsfLsp(lsf, back, cM)
		for i := 0; i < cM; i++ {
			d := int(back[i]) - int(lsp[i])
			if d < -16 || d > 16 {
				t.Errorf("set %d: lsp[%d] round-trip %d -> %d (diff %d)", k, i, lsp[i], back[i], d)
			}
		}
	}
}

// TestIntLpcShape checks both interpolators fill 4*cMP1 coefficients with the
// a[0]=4096 invariant at each subframe boundary.
func TestIntLpcShape(t *testing.T) {
	sets := sampleLSPs()
	old, mid, new_ := sets[0], sets[1], sets[0]

	az := make([]int16, cNB_SUBFR*cMP1)
	intLpc1and3(old, mid, new_, az)
	for sf := 0; sf < cNB_SUBFR; sf++ {
		if az[sf*cMP1] != 4096 {
			t.Errorf("1and3 subframe %d: a[0]=%d, want 4096", sf, az[sf*cMP1])
		}
	}

	az2 := make([]int16, cNB_SUBFR*cMP1)
	intLpc1to3(old, new_, az2)
	for sf := 0; sf < cNB_SUBFR; sf++ {
		if az2[sf*cMP1] != 4096 {
			t.Errorf("1to3 subframe %d: a[0]=%d, want 4096", sf, az2[sf*cMP1])
		}
	}
	// Subframe 4 of 1to3 is Lsp_Az(lsp_new) directly; verify it equals a direct
	// conversion of lsp_new.
	direct := make([]int16, cMP1)
	lspAz(new_, direct)
	for i := 0; i < cMP1; i++ {
		if az2[3*cMP1+i] != direct[i] {
			t.Fatalf("1to3 subframe 4 mismatch at %d: %d != %d", i, az2[3*cMP1+i], direct[i])
		}
	}
}
