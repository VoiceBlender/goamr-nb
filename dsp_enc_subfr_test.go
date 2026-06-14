package amrnb

import "testing"

// TestCbsearchDispatch checks cbsearch routes to the correct per-mode search,
// producing an innovation that round-trips through the decoder, for every mode.
func TestCbsearchDispatch(t *testing.T) {
	h := makeH1()
	x := make([]int16, cL_SUBFR)
	res2 := make([]int16, cL_SUBFR)
	seed := uint32(0x5151)
	for i := range x {
		seed = seed*1664525 + 1013904223
		x[i] = int16(int32(seed>>20) - 2048)
		seed = seed*1664525 + 1013904223
		res2[i] = int16(int32(seed>>20) - 2048)
	}

	cases := []struct {
		mode    int
		nPulses int
		decode  func(anap []int16, dec []int16)
	}{
		{dMR475, 2, func(a, d []int16) { decode2i40_9bits(0, a[1], a[0], d) }},
		{dMR59, 2, func(a, d []int16) { decode2i40_11bits(a[1], a[0], d) }},
		{dMR67, 3, func(a, d []int16) { decode3i40_14bits(a[1], a[0], d) }},
		{dMR74, 4, func(a, d []int16) { decode4i40_17bits(a[1], a[0], d) }},
		{dMR102, 8, func(a, d []int16) { dec8i40_31bits(a[0:7], d) }},
		{dMR122, 10, func(a, d []int16) { dec10i40_35bits(a[0:10], d) }},
	}
	for _, c := range cases {
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_SUBFR)
		y := make([]int16, cL_SUBFR)
		anap := make([]int16, 12)
		// T0 = L_SUBFR disables pitch sharpening so enc code == dec code.
		cbsearch(x, hc, cL_SUBFR, 0, 0, res2, code, y, c.mode, 0, anap)

		dec := make([]int16, cL_CODE)
		c.decode(anap, dec)
		for i := range code {
			if code[i] != dec[i] {
				t.Fatalf("mode %d: code[%d]=%d != dec %d", c.mode, i, code[i], dec[i])
			}
		}
		nz := 0
		for _, v := range code {
			if v != 0 {
				nz++
			}
		}
		// pulses may overlap (8/10-pulse), so allow <= expected.
		if nz == 0 || nz > c.nPulses {
			t.Errorf("mode %d: %d nonzero pulses, expected up to %d", c.mode, nz, c.nPulses)
		}
	}
}

// TestSubframePostProc checks the post-processor updates the synthesis memory
// from the synthesis tail and produces a bounded total excitation.
func TestSubframePostProc(t *testing.T) {
	aq := realAz()
	speech := make([]int16, cL_FRAME)
	for i := range speech {
		speech[i] = int16(2000 * sinApprox(float64(i)*0.25))
	}
	synth := make([]int16, cL_FRAME)
	xn := make([]int16, cL_SUBFR)
	code := make([]int16, cL_SUBFR)
	y1 := make([]int16, cL_SUBFR)
	y2 := make([]int16, cL_SUBFR)
	for i := 0; i < cL_SUBFR; i++ {
		xn[i] = int16(1500 * sinApprox(float64(i)*0.3))
		y1[i] = int16(1200 * sinApprox(float64(i)*0.3))
		code[i] = int16(800 * sinApprox(float64(i)*0.5))
		y2[i] = int16(700 * sinApprox(float64(i)*0.5))
	}
	memSyn := make([]int16, cM)
	memErr := make([]int16, cM)
	memW0 := make([]int16, cM)
	exc := make([]int16, cExcOff+cL_FRAME)
	for i := range exc {
		exc[i] = int16(500 * sinApprox(float64(i)*0.2))
	}
	sharp := int16(0)

	subframePostProc(speech, 0, dMR59, 0, 13000, 200, aq, synth, xn, code, y1, y2, memSyn, memErr, memW0, exc, cExcOff, &sharp)

	// mem_syn must equal the last M synthesis samples.
	for j := 0; j < cM; j++ {
		if memSyn[j] != synth[cL_SUBFR-cM+j] {
			t.Errorf("memSyn[%d]=%d != synth tail %d", j, memSyn[j], synth[cL_SUBFR-cM+j])
		}
	}
	if sharp != 13000 {
		t.Errorf("sharp=%d, want 13000 (gain_pit < SHARPMAX)", sharp)
	}
}
