package amrnb

import "testing"

// analysisLSP derives a realistic descending LSP vector from the LP analysis
// chain on a deterministic signal varied by seed.
func analysisLSP(seed int) []int16 {
	x := make([]int16, cL_WINDOW)
	for i := range x {
		f := float64(i)
		x[i] = int16(7000*sinApprox(f*(0.18+float64(seed)*0.013)) + 2500*sinApprox(f*0.5))
	}
	rh := make([]int16, cMP1)
	rl := make([]int16, cMP1)
	autocorr(x, cM, rh, rl, window_200_40)
	lagWindow(cM, rh, rl)
	var lev levinsonState
	a := make([]int16, cMP1)
	rc := make([]int16, 4)
	lev.levinson(rh, rl, a, rc)
	lsp := make([]int16, cM)
	azLsp(a, lsp, lsp_init_data)
	return lsp
}

// TestQPlsf3RoundTrip is the strong cross-check: the encoder's quantized LSPs
// must exactly equal the decoder's reconstruction of the same indices, for
// every 3-split mode, across several frames with synchronized predictor state.
func TestQPlsf3RoundTrip(t *testing.T) {
	modes := []int{dMR475, dMR515, dMR59, dMR67, dMR74, dMR795, dMR102}
	for _, mode := range modes {
		var enc qPlsfState
		var dec dPlsfState
		enc.reset()
		dec.reset()
		for f := 0; f < 8; f++ {
			lsp := analysisLSP(f)
			lspQEnc := make([]int16, cM)
			indice := enc.qPlsf3(mode, lsp, lspQEnc)

			lspQDec := make([]int16, cM)
			dec.dPlsf3(mode, 0, indice[:], lspQDec)

			for i := 0; i < cM; i++ {
				if lspQEnc[i] != lspQDec[i] {
					t.Fatalf("mode %d frame %d: enc/dec LSP mismatch at %d: %d != %d",
						mode, f, i, lspQEnc[i], lspQDec[i])
				}
			}
			// quantized LSPs must remain valid (strictly descending)
			for i := 0; i < cM-1; i++ {
				if lspQEnc[i] <= lspQEnc[i+1] {
					t.Errorf("mode %d frame %d: quantized LSP not descending at %d", mode, f, i)
				}
			}
		}
	}
}

// TestQPlsf5RoundTrip checks the MR122 5-split encoder against the decoder:
// the two quantized LSP vectors must exactly match dPlsf5's reconstruction of
// the same five indices, across frames with synchronized predictor state.
func TestQPlsf5RoundTrip(t *testing.T) {
	var enc qPlsfState
	var dec dPlsfState
	enc.reset()
	dec.reset()
	for f := 0; f < 8; f++ {
		lsp1 := analysisLSP(f)
		lsp2 := analysisLSP(f + 1)
		e1 := make([]int16, cM)
		e2 := make([]int16, cM)
		indice := enc.qPlsf5(lsp1, lsp2, e1, e2)

		d1 := make([]int16, cM)
		d2 := make([]int16, cM)
		dec.dPlsf5(0, indice[:], d1, d2)

		for i := 0; i < cM; i++ {
			if e1[i] != d1[i] || e2[i] != d2[i] {
				t.Fatalf("frame %d: enc/dec mismatch at %d: lsp1 %d/%d lsp2 %d/%d",
					f, i, e1[i], d1[i], e2[i], d2[i])
			}
		}
		for i := 0; i < cM-1; i++ {
			if e1[i] <= e1[i+1] || e2[i] <= e2[i+1] {
				t.Errorf("frame %d: quantized LSP not descending at %d", f, i)
			}
		}
	}
}

// TestQPlsf3IndexRanges checks the produced indices stay within their codebook
// sizes for each mode.
func TestQPlsf3IndexRanges(t *testing.T) {
	check := func(mode int, lim [3]int16) {
		var enc qPlsfState
		enc.reset()
		lspQ := make([]int16, cM)
		idx := enc.qPlsf3(mode, analysisLSP(2), lspQ)
		for k := 0; k < 3; k++ {
			if idx[k] < 0 || idx[k] >= lim[k] {
				t.Errorf("mode %d: index[%d]=%d out of range [0,%d)", mode, k, idx[k], lim[k])
			}
		}
	}
	check(dMR475, [3]int16{cDICO1_SIZE, cDICO2_SIZE / 2, cMR515_3_SIZE})
	check(dMR795, [3]int16{cMR795_1_SIZE, cDICO2_SIZE, cDICO3_SIZE})
	check(dMR59, [3]int16{cDICO1_SIZE, cDICO2_SIZE, cDICO3_SIZE})
}

// TestQPlsf3Deterministic confirms repeatability.
func TestQPlsf3Deterministic(t *testing.T) {
	run := func() [3]int16 {
		var enc qPlsfState
		enc.reset()
		lspQ := make([]int16, cM)
		return enc.qPlsf3(dMR74, analysisLSP(3), lspQ)
	}
	if run() != run() {
		t.Error("qPlsf3 non-deterministic")
	}
}
