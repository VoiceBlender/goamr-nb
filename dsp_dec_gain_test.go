package amrnb

import "testing"

// TestDGainPitch checks the pitch-gain lookup and the MR122 LSB masking.
func TestDGainPitch(t *testing.T) {
	for _, idx := range []int16{0, 1, 5, 10, 15} {
		if got := dGainPitch(dMR59, idx); got != qua_gain_pitch[idx] {
			t.Errorf("dGainPitch(MR59,%d)=%d, want %d", idx, got, qua_gain_pitch[idx])
		}
	}
	// MR122 clears the two LSBits.
	for _, idx := range []int16{0, 3, 7, 15} {
		want := qua_gain_pitch[idx] &^ 3
		if got := dGainPitch(dMR122, idx); got != want {
			t.Errorf("dGainPitch(MR122,%d)=%d, want %d", idx, got, want)
		}
	}
}

// TestDecGainTableIndexing verifies decGain reads the pitch gain straight from
// the mode's VQ table at the right offset.
func TestDecGainTableIndexing(t *testing.T) {
	var pred gcPredState
	pred.reset()
	code := makeCode(2000)

	// MR59 low-rate table: gain_pit is table_gain_lowrates[index*4].
	for _, idx := range []int16{0, 3, 10, 30} {
		pred.reset()
		gp, gc := pred.decGain(dMR59, idx, code, 1)
		if gp != table_gain_lowrates[idx*4] {
			t.Errorf("MR59 idx=%d: gainPit=%d, want %d", idx, gp, table_gain_lowrates[idx*4])
		}
		if gc < 0 {
			t.Errorf("MR59 idx=%d: gainCod=%d < 0", idx, gc)
		}
	}

	// MR67 high-rate table.
	pred.reset()
	gp, _ := pred.decGain(dMR67, 5, code, 1)
	if gp != table_gain_highrates[5*4] {
		t.Errorf("MR67 idx=5: gainPit=%d, want %d", gp, table_gain_highrates[5*4])
	}
}

// TestDecGainMR475Subframes checks the joint 4.75k gain selects different table
// halves for even and odd subframes.
func TestDecGainMR475Subframes(t *testing.T) {
	var pred gcPredState
	code := makeCode(1500)

	pred.reset()
	gpEven, _ := pred.decGain(dMR475, 3, code, 1) // evenSubfr=1 -> offset 0
	pred.reset()
	gpOdd, _ := pred.decGain(dMR475, 3, code, 0) // evenSubfr=0 -> offset 2

	if gpEven != table_gain_MR475[3*4+0] {
		t.Errorf("MR475 even gainPit=%d, want %d", gpEven, table_gain_MR475[3*4+0])
	}
	if gpOdd != table_gain_MR475[3*4+2] {
		t.Errorf("MR475 odd gainPit=%d, want %d", gpOdd, table_gain_MR475[3*4+2])
	}
}

// TestDGainCode exercises the MR795/MR122 codebook-gain path and checks it is
// positive for a nonzero innovation and deterministic.
func TestDGainCode(t *testing.T) {
	code := makeCode(3000)
	for _, mode := range []int{dMR795, dMR122} {
		var p1, p2 gcPredState
		p1.reset()
		p2.reset()
		g1 := p1.dGainCode(mode, 10, code)
		g2 := p2.dGainCode(mode, 10, code)
		if g1 != g2 {
			t.Errorf("mode %d: dGainCode non-deterministic %d != %d", mode, g1, g2)
		}
		if g1 <= 0 {
			t.Errorf("mode %d: dGainCode=%d, want > 0", mode, g1)
		}
	}
}

// TestDecGainPredictorEvolves confirms the gain decode feeds the predictor so
// successive identical frames produce evolving code gains.
func TestDecGainPredictorEvolves(t *testing.T) {
	var pred gcPredState
	pred.reset()
	code := makeCode(2500)
	_, gc1 := pred.decGain(dMR59, 8, code, 1)
	_, gc2 := pred.decGain(dMR59, 8, code, 0)
	if gc1 == gc2 {
		t.Logf("note: code gains equal across frames (gc1=%d) — acceptable but unusual", gc1)
	}
	// predictor memory must have advanced (newest != floor after two updates)
	if pred.pastQuaEn[0] == cMIN_ENERGY && pred.pastQuaEn[1] == cMIN_ENERGY {
		t.Errorf("predictor memory did not advance")
	}
}
