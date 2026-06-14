package amrnb

import "testing"

// TestQGainPitchRoundTrip checks the quantized pitch gain matches the decoder's
// dGainPitch for the chosen index, across modes (incl. the MR122 LSB mask).
func TestQGainPitchRoundTrip(t *testing.T) {
	for _, mode := range []int{dMR59, dMR795, dMR122} {
		for _, g := range []int16{0, 3000, 8200, 12000, 15000, 19000} {
			gain := g
			cand := make([]int16, 3)
			cind := make([]int16, 3)
			idx := qGainPitch(mode, 32767, &gain, cand, cind)
			want := dGainPitch(mode, idx)
			if gain != want {
				t.Errorf("mode %d g=%d: quantized %d != dGainPitch %d (idx=%d)", mode, g, gain, want, idx)
			}
		}
	}
}

// TestQGainCodeRoundTrip checks the MR122 quantized code gain matches the
// decoder's dGainCode for the chosen index (both fresh predictors on the same
// code give the same predicted gain).
func TestQGainCodeRoundTrip(t *testing.T) {
	code := make([]int16, cL_SUBFR)
	code[5] = 4096
	code[12] = -4096
	code[27] = 4096
	for _, g := range []int16{200, 1000, 4000, 12000, 30000} {
		var encPred gcPredState
		encPred.reset()
		r := encPred.gcPred(dMR122, code)

		gain := g
		idx, _, _ := qGainCode(dMR122, r.expGcode0, r.fracGcode0, &gain)

		var decPred gcPredState
		decPred.reset()
		dg := decPred.dGainCode(dMR122, idx, code)
		if gain != dg {
			t.Errorf("g=%d: quantized code gain %d != dGainCode %d (idx=%d)", g, gain, dg, idx)
		}
	}
}

// TestQGainPitchNearest sanity-checks the quantizer picks a table entry no
// farther than any other within the limit.
func TestQGainPitchNearest(t *testing.T) {
	gain := int16(7000)
	cand := make([]int16, 3)
	cind := make([]int16, 3)
	idx := qGainPitch(dMR59, 32767, &gain, cand, cind)
	chosen := abs16(7000 - qua_gain_pitch[idx])
	for i := range qua_gain_pitch {
		if d := abs16(7000 - qua_gain_pitch[i]); d < chosen {
			t.Errorf("index %d closer (%d) than chosen %d (%d)", i, d, idx, chosen)
		}
	}
}
