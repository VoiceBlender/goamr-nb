package amrnb

import "testing"

// TestDecLag3Anchors checks decLag3 against values derived by hand from the
// reference algorithm, for both subframe-coding paths.
func TestDecLag3Anchors(t *testing.T) {
	cases := []struct {
		index, t0Min, t0Max, iSubfr, T0prev, flag4 int16
		wantT0, wantFrac                           int16
	}{
		{0, 20, 143, 0, 0, 0, 19, 1},   // 1st subframe, index 0
		{10, 20, 143, 0, 0, 0, 23, -1}, // 1st subframe, index 10
		{196, 20, 143, 0, 0, 0, 85, -1},
		{200, 20, 143, 0, 0, 0, 88, 0}, // 1st subframe, index >= 197
		{5, 20, 60, 1, 0, 0, 21, 0},    // 2nd subframe, flag4=0
	}
	for _, c := range cases {
		t0, frac := decLag3(c.index, c.t0Min, c.t0Max, c.iSubfr, c.T0prev, c.flag4)
		if t0 != c.wantT0 || frac != c.wantFrac {
			t.Errorf("decLag3(idx=%d,sub=%d)=({%d,%d}), want ({%d,%d})",
				c.index, c.iSubfr, t0, frac, c.wantT0, c.wantFrac)
		}
	}
}

// TestDecLag6Anchors checks decLag6 (MR122 1/6 resolution) against hand-derived
// values.
func TestDecLag6Anchors(t *testing.T) {
	cases := []struct {
		index, pitMin, pitMax, iSubfr, T0in int16
		wantT0, wantFrac                    int16
	}{
		{0, 18, 143, 0, 0, 17, 3},   // 1st subframe, index 0
		{463, 18, 143, 0, 0, 95, 0}, // 1st subframe, index >= 463
	}
	for _, c := range cases {
		t0, frac := decLag6(c.index, c.pitMin, c.pitMax, c.iSubfr, c.T0in)
		if t0 != c.wantT0 || frac != c.wantFrac {
			t.Errorf("decLag6(idx=%d,sub=%d)=({%d,%d}), want ({%d,%d})",
				c.index, c.iSubfr, t0, frac, c.wantT0, c.wantFrac)
		}
	}
}

// TestPredLtDCGain checks the pitch interpolator: a constant past excitation
// must yield a constant output at near-unity DC gain (the FIR is designed for
// it), and the interpolation must be deterministic. A large lag (T0 >= L_subfr)
// is used so reads never overlap the freshly written samples.
func TestPredLtDCGain(t *testing.T) {
	const pos = 80
	const T0 = 60
	const V = int16(4000)
	exc := make([]int16, 200)
	for i := 0; i < pos; i++ {
		exc[i] = V
	}
	predLt(exc, pos, T0, 0, cL_SUBFR, 1)

	first := exc[pos]
	for i := 0; i < cL_SUBFR; i++ {
		if exc[pos+i] != first {
			t.Fatalf("output not constant: exc[%d]=%d != %d", pos+i, exc[pos+i], first)
		}
	}
	// DC gain within ~6% of unity.
	d := int(first) - int(V)
	if d < -int(V)/16 || d > int(V)/16 {
		t.Errorf("DC gain off: out=%d, in=%d (diff %d)", first, V, d)
	}

	// Determinism.
	exc2 := make([]int16, 200)
	for i := 0; i < pos; i++ {
		exc2[i] = V
	}
	predLt(exc2, pos, T0, 0, cL_SUBFR, 1)
	for i := 0; i < cL_SUBFR; i++ {
		if exc2[pos+i] != exc[pos+i] {
			t.Fatalf("non-deterministic at %d", pos+i)
		}
	}

	// Zero past excitation -> zero output (the 0x4000 rounding bias shifts to 0).
	zero := make([]int16, 200)
	predLt(zero, pos, T0, 0, cL_SUBFR, 1)
	for i := 0; i < cL_SUBFR; i++ {
		if zero[pos+i] != 0 {
			t.Fatalf("zero input gave nonzero output at %d: %d", pos+i, zero[pos+i])
		}
	}
}

// TestPredLtLinearity checks the interpolator scales linearly with input level.
func TestPredLtLinearity(t *testing.T) {
	const pos = 80
	const T0 = 60
	run := func(v int16) int16 {
		exc := make([]int16, 200)
		for i := 0; i < pos; i++ {
			exc[i] = v
		}
		predLt(exc, pos, T0, 2, cL_SUBFR, 1) // nonzero frac exercises interpolation
		return exc[pos+10]
	}
	g1 := int(run(1000))
	g2 := int(run(2000))
	if d := g2 - 2*g1; d < -2 || d > 2 {
		t.Errorf("nonlinear: out(1000)=%d out(2000)=%d (2x diff %d)", g1, g2, d)
	}
}
