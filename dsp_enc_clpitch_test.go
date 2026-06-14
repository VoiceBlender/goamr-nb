package amrnb

import "testing"

// TestPitchFrFindsLag checks the closed-loop search recovers a known pitch lag:
// with a unit-impulse weighting filter, the filtered adaptive excitation equals
// the excitation, so a target built as the excitation delayed by T must peak the
// normalized correlation exactly at lag T.
func TestPitchFrFindsLag(t *testing.T) {
	const excOff = 200
	exc := make([]int16, excOff+cL_SUBFR)
	seed := uint32(0xBEEF)
	for i := range exc {
		seed = seed*1664525 + 1013904223
		exc[i] = int16(int32(seed>>20) - 2048) // pseudo-random, ~±2k (energy fits int32)
	}
	h := make([]int16, cL_SUBFR)
	h[0] = 4096 // unit impulse, Q12

	for _, T := range []int16{40, 50, 75} {
		xn := make([]int16, cL_SUBFR)
		for j := 0; j < cL_SUBFR; j++ {
			xn[j] = exc[excOff-int(T)+j]
		}
		Top := []int16{T, T} // open-loop lag points at the true period

		var st pitchFrState
		T0, frac, _, _ := st.pitchFr(dMR59, Top, exc, excOff, xn, h, cL_SUBFR, 0)
		if T0 != T {
			t.Errorf("T=%d: pitchFr found lag %d frac %d, want %d", T, T0, frac, T)
		}
		if frac != 0 {
			t.Errorf("T=%d: exact integer match should give frac 0, got %d", T, frac)
		}
	}
}

// TestPitchFrEncodesLag checks the returned analysis index decodes (via the
// decoder) back to the same lag, tying the search to the lag coders.
func TestPitchFrEncodesLag(t *testing.T) {
	const excOff = 200
	exc := make([]int16, excOff+cL_SUBFR)
	seed := uint32(0x1234)
	for i := range exc {
		seed = seed*1664525 + 1013904223
		exc[i] = int16(int32(seed>>20) - 2048)
	}
	h := make([]int16, cL_SUBFR)
	h[0] = 4096
	const T = 55
	xn := make([]int16, cL_SUBFR)
	for j := 0; j < cL_SUBFR; j++ {
		xn[j] = exc[excOff-T+j]
	}
	Top := []int16{T, T}

	var st pitchFrState
	T0, frac, _, idx := st.pitchFr(dMR59, Top, exc, excOff, xn, h, cL_SUBFR, 0)

	// 1st subframe (iSubfr=0) is absolute coding -> decLag3 with i_subfr=0.
	dT0, dFrac := decLag3(idx, cPIT_MIN, cPIT_MAX, 0, 0, 0)
	if dT0 != T0 || dFrac != frac {
		t.Errorf("encoded idx=%d -> dec (%d,%d), enc (%d,%d)", idx, dT0, dFrac, T0, frac)
	}
}

// TestPitchFrDeterministic confirms repeatability.
func TestPitchFrDeterministic(t *testing.T) {
	const excOff = 200
	exc := make([]int16, excOff+cL_SUBFR)
	for i := range exc {
		exc[i] = int16(3000 * sinApprox(float64(i)*0.21))
	}
	h := make([]int16, cL_SUBFR)
	h[0] = 4096
	xn := make([]int16, cL_SUBFR)
	for j := 0; j < cL_SUBFR; j++ {
		xn[j] = exc[excOff-60+j]
	}
	Top := []int16{60, 60}
	run := func() int16 {
		var st pitchFrState
		t0, _, _, _ := st.pitchFr(dMR74, Top, exc, excOff, xn, h, cL_SUBFR, 0)
		return t0
	}
	if run() != run() {
		t.Error("pitchFr non-deterministic")
	}
}
