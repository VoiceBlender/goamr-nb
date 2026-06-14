package amrnb

import "testing"

// TestQuaGainRoundTrip checks the general-mode gain quantizer: the index it
// chooses, when dequantized by the decoder's decGain, reproduces the exact same
// pitch and code gains (both read the same VQ table entry and predicted gain).
func TestQuaGainRoundTrip(t *testing.T) {
	h := makeH1()
	for _, mode := range []int{dMR515, dMR59, dMR67, dMR74, dMR102} {
		// adaptive codebook y1 and target xn
		y1 := make([]int16, cL_SUBFR)
		xn := make([]int16, cL_SUBFR)
		for i := range y1 {
			y1[i] = int16(1500 * sinApprox(float64(i)*0.3))
			xn[i] = int16(1100 * sinApprox(float64(i)*0.3+0.2))
		}
		gCoeff := make([]int16, 5)
		gPitch(mode, xn, y1, gCoeff, cL_SUBFR)

		// a fixed-codebook innovation + its filtered version y2
		code := make([]int16, cL_SUBFR)
		code[7] = 8191
		code[18] = -8192
		code[31] = 8191
		y2 := make([]int16, cL_SUBFR)
		convolve(code, h, y2, cL_SUBFR)

		// predicted code gain (fresh predictor, mirrors the decoder)
		var encPred gcPredState
		encPred.reset()
		r := encPred.gcPred(mode, code)

		fc, ec, _, _ := calcFiltEnergies(mode, xn, xn, y1, y2, gCoeff)
		index, gainPit, gainCod, _, _ := quaGain(mode, r.expGcode0, r.fracGcode0, fc[:], ec[:], 32767)

		var decPred gcPredState
		decPred.reset()
		dGainPit, dGainCod := decPred.decGain(mode, index, code, 1)

		if gainPit != dGainPit || gainCod != dGainCod {
			t.Errorf("mode %d: enc gains (%d,%d) != dec (%d,%d) idx=%d",
				mode, gainPit, gainCod, dGainPit, dGainCod, index)
		}
	}
}

// TestQuaGainDeterministic confirms repeatability.
func TestQuaGainDeterministic(t *testing.T) {
	h := makeH1()
	y1 := make([]int16, cL_SUBFR)
	xn := make([]int16, cL_SUBFR)
	for i := range y1 {
		y1[i] = int16(1200 * sinApprox(float64(i)*0.25))
		xn[i] = int16(900 * sinApprox(float64(i)*0.25))
	}
	gCoeff := make([]int16, 5)
	gPitch(dMR59, xn, y1, gCoeff, cL_SUBFR)
	code := make([]int16, cL_SUBFR)
	code[3] = 8191
	code[22] = -8192
	y2 := make([]int16, cL_SUBFR)
	convolve(code, h, y2, cL_SUBFR)
	var pred gcPredState
	pred.reset()
	r := pred.gcPred(dMR59, code)
	fc, ec, _, _ := calcFiltEnergies(dMR59, xn, xn, y1, y2, gCoeff)
	i1, _, _, _, _ := quaGain(dMR59, r.expGcode0, r.fracGcode0, fc[:], ec[:], 32767)
	i2, _, _, _, _ := quaGain(dMR59, r.expGcode0, r.fracGcode0, fc[:], ec[:], 32767)
	if i1 != i2 {
		t.Errorf("quaGain non-deterministic: %d != %d", i1, i2)
	}
}
