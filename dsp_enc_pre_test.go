package amrnb

import "testing"

// TestSubframePreProcImpulse checks the weighted synthesis impulse response
// starts at 4096 (Q12 unity, since a[0]=aq[0]=4096 and the weighting filters
// preserve the leading tap) and is bounded.
func TestSubframePreProcImpulse(t *testing.T) {
	a := realAz()
	aq := realAz()
	speech := make([]int16, cM+cL_SUBFR)
	for i := range speech {
		speech[i] = int16(2000 * sinApprox(float64(i)*0.3))
	}
	memErr := make([]int16, cM)
	memW0 := make([]int16, cM)

	exc := make([]int16, cL_SUBFR)
	h1 := make([]int16, cL_SUBFR)
	xn := make([]int16, cL_SUBFR)
	res2 := make([]int16, cL_SUBFR)
	errSig := make([]int16, cL_SUBFR)

	subframePreProc(dMR59, a, aq, speech, cM, memErr, memW0, exc, h1, xn, res2, errSig)

	if h1[0] != 4096 {
		t.Errorf("h1[0]=%d, want 4096", h1[0])
	}
	// impulse response should decay: late samples much smaller than the peak.
	var late int16
	for i := 30; i < cL_SUBFR; i++ {
		if a := abs16(h1[i]); a > late {
			late = a
		}
	}
	if late > 4096 {
		t.Errorf("impulse response not decaying: late max |%d|", late)
	}
}

// TestSubframePreProcZero checks that zero input with zero memory yields a zero
// target.
func TestSubframePreProcZero(t *testing.T) {
	a := realAz()
	aq := realAz()
	speech := make([]int16, cM+cL_SUBFR) // all zero
	memErr := make([]int16, cM)
	memW0 := make([]int16, cM)

	exc := make([]int16, cL_SUBFR)
	h1 := make([]int16, cL_SUBFR)
	xn := make([]int16, cL_SUBFR)
	res2 := make([]int16, cL_SUBFR)
	errSig := make([]int16, cL_SUBFR)

	subframePreProc(dMR59, a, aq, speech, cM, memErr, memW0, exc, h1, xn, res2, errSig)

	for i := 0; i < cL_SUBFR; i++ {
		if xn[i] != 0 || exc[i] != 0 || res2[i] != 0 {
			t.Fatalf("zero input gave nonzero output at %d: xn=%d exc=%d res2=%d", i, xn[i], exc[i], res2[i])
		}
	}
}

// TestSubframePreProcNonSilent checks a real speech subframe produces a
// non-silent target and residual, and is deterministic.
func TestSubframePreProcNonSilent(t *testing.T) {
	a := realAz()
	aq := realAz()
	speech := make([]int16, cM+cL_SUBFR)
	for i := range speech {
		speech[i] = int16(4000*sinApprox(float64(i)*0.22) + 1500*sinApprox(float64(i)*0.6))
	}
	memErr := make([]int16, cM)
	memW0 := make([]int16, cM)

	run := func() ([]int16, []int16) {
		exc := make([]int16, cL_SUBFR)
		h1 := make([]int16, cL_SUBFR)
		xn := make([]int16, cL_SUBFR)
		res2 := make([]int16, cL_SUBFR)
		errSig := make([]int16, cL_SUBFR)
		subframePreProc(dMR59, a, aq, speech, cM, memErr, memW0, exc, h1, xn, res2, errSig)
		return xn, res2
	}
	xn, res2 := run()
	var exn, eres float64
	for i := range xn {
		exn += float64(xn[i]) * float64(xn[i])
		eres += float64(res2[i]) * float64(res2[i])
	}
	if exn == 0 || eres == 0 {
		t.Errorf("target/residual silent: exn=%.0f eres=%.0f", exn, eres)
	}
	xn2, _ := run()
	for i := range xn {
		if xn[i] != xn2[i] {
			t.Fatalf("non-deterministic at %d", i)
		}
	}
}
