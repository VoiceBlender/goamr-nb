package amrnb

import "testing"

// TestLExtractComp checks L_Extract / L_Comp are inverse to within the 1-LSB
// precision the split allows.
func TestLExtractComp(t *testing.T) {
	for _, L := range []int32{0, 1, -1, 0x12345678, -0x12345678, 0x7fffffff, minInt32 + 1, 100000, -100000} {
		hi, lo := L_Extract(L)
		back := L_Comp(hi, lo)
		if d := back - L; d < -2 || d > 2 {
			t.Errorf("L_Comp(L_Extract(%#x))=%#x (diff %d)", L, back, d)
		}
	}
}

// TestMpy3216 checks mpy32_16(L_Extract(L), n) tracks (L*n)>>15 (the fixed-point
// product semantics) within a couple of LSBs of truncation error.
func TestMpy3216(t *testing.T) {
	Ls := []int32{0x12345678, -0x12345678, 1 << 20, -(1 << 20), 0x40000000, 1000000}
	ns := []int16{1, -1, 1000, -1000, 16384, 32767}
	for _, L := range Ls {
		hi, lo := L_Extract(L)
		for _, n := range ns {
			got := int64(mpy32_16(hi, lo, n))
			want := (int64(L) * int64(n)) >> 15
			if d := got - want; d < -4 || d > 4 {
				t.Errorf("mpy32_16(%#x,%d)=%d, want ~%d (diff %d)", L, n, got, want, d)
			}
		}
	}
}

// makeCode builds a sparse innovation vector with the given pulse amplitude.
func makeCode(amp int16) []int16 {
	c := make([]int16, cL_SUBFR)
	c[5] = amp
	c[17] = -amp
	c[30] = amp
	c[38] = -amp
	return c
}

// gcode0 reconstructs the predicted gain factor from gc_pred's exponent and
// fraction outputs.
func gcode0(r gcPredResult) int32 { return pow2(r.expGcode0, r.fracGcode0) }

// TestGcPredEnergyMonotonic checks the predicted code gain falls as the
// innovation energy rises (the predictor normalizes toward a target energy).
func TestGcPredEnergyMonotonic(t *testing.T) {
	for _, mode := range []int{dMR475, dMR59, dMR67, dMR74, dMR795, dMR102} {
		var prev int32 = 1 << 30
		for _, amp := range []int16{500, 1500, 4000, 8000} {
			var st gcPredState
			st.reset()
			r := st.gcPred(mode, makeCode(amp))
			g := gcode0(r)
			if g > prev {
				t.Errorf("mode %d: gcode0 not decreasing with energy: amp=%d g=%d prev=%d", mode, amp, g, prev)
			}
			prev = g
		}
	}
}

// TestGcPredMR122 exercises the 12.2k predictor path and checks determinism and
// that the predicted gain is positive and finite.
func TestGcPredMR122(t *testing.T) {
	var st gcPredState
	st.reset()
	r := st.gcPred(dMR122, makeCode(3000))
	g := gcode0(r)
	if g <= 0 {
		t.Errorf("MR122 gcode0 = %d, want > 0", g)
	}
	var st2 gcPredState
	st2.reset()
	r2 := st2.gcPred(dMR122, makeCode(3000))
	if r2 != r {
		t.Errorf("MR122 gc_pred non-deterministic: %+v != %+v", r2, r)
	}
}

// TestGcPredUpdate verifies the predictor memory shifts correctly and changes
// the prediction across frames.
func TestGcPredUpdate(t *testing.T) {
	var st gcPredState
	st.reset()
	code := makeCode(2000)
	r1 := st.gcPred(dMR59, code)
	st.update(100, 2000) // push a high quantized energy into the history
	r2 := st.gcPred(dMR59, code)
	if r1 == r2 {
		t.Errorf("predictor memory had no effect: %+v == %+v", r1, r2)
	}
	// memory ordering: newest entry is index 0
	if st.pastQuaEn[0] != 2000 || st.pastQuaEnMR122[0] != 100 {
		t.Errorf("update ordering wrong: past[0]=%d mr122[0]=%d", st.pastQuaEn[0], st.pastQuaEnMR122[0])
	}
}
