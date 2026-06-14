package amrnb

import "testing"

// TestPreProcessDCRejection checks the encoder input high-pass removes DC.
func TestPreProcessDCRejection(t *testing.T) {
	var st preProcessState
	sig := make([]int16, 400)
	for i := range sig {
		sig[i] = 6000 // DC
	}
	st.process(sig, len(sig))
	var maxTail int16
	for i := 300; i < len(sig); i++ {
		if a := abs16(sig[i]); a > maxTail {
			maxTail = a
		}
	}
	if maxTail > 60 {
		t.Errorf("DC not rejected: tail max |%d|", maxTail)
	}
}

// TestLPAnalysisChain runs PCM -> Autocorr -> Lag_window -> Levinson -> A(z) on
// a well-conditioned speech-like window and checks the result is a stable
// order-10 filter: a[0]=4096 and Az_lsp recovers 10 strictly-descending LSPs.
func TestLPAnalysisChain(t *testing.T) {
	// deterministic voiced-like analysis window (L_WINDOW samples)
	x := make([]int16, cL_WINDOW)
	for i := range x {
		f := float64(i)
		x[i] = int16(8000*sinApprox(f*0.20) + 3000*sinApprox(f*0.55) + 1500*sinApprox(f*0.9))
	}

	rh := make([]int16, cMP1)
	rl := make([]int16, cMP1)
	autocorr(x, cM, rh, rl, window_200_40)
	if rh[0] == 0 {
		t.Fatal("autocorr R[0] is zero")
	}
	lagWindow(cM, rh, rl)

	var lev levinsonState
	a := make([]int16, cMP1)
	rc := make([]int16, 4)
	lev.levinson(rh, rl, a, rc)

	if a[0] != 4096 {
		t.Errorf("a[0]=%d, want 4096", a[0])
	}

	lsp := make([]int16, cM)
	azLsp(a, lsp, lsp_init_data)
	for i := 0; i < cM-1; i++ {
		if lsp[i] <= lsp[i+1] {
			t.Errorf("LSP not strictly descending at %d (%d <= %d) — unstable A(z)?", i, lsp[i], lsp[i+1])
		}
	}
	// LSPs must be within the cosine range
	for i := 0; i < cM; i++ {
		if lsp[i] < -32768 || lsp[i] > 32767 {
			t.Errorf("LSP[%d] out of range: %d", i, lsp[i])
		}
	}
}

// TestLPAnalysisDeterministic confirms repeatability of the chain.
func TestLPAnalysisDeterministic(t *testing.T) {
	x := make([]int16, cL_WINDOW)
	for i := range x {
		x[i] = int16(5000 * sinApprox(float64(i)*0.3))
	}
	run := func() []int16 {
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
	a1, a2 := run(), run()
	for i := range a1 {
		if a1[i] != a2[i] {
			t.Fatalf("non-deterministic at %d", i)
		}
	}
}
