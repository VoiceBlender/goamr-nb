package amrnb

import "testing"

func energyF(s []int16) float64 {
	var e float64
	for _, v := range s {
		e += float64(v) * float64(v)
	}
	return e
}

// TestEnergyNew checks energyNew matches the (sum of squares >> 4) definition
// for a non-saturating signal.
func TestEnergyNew(t *testing.T) {
	s := make([]int16, cL_SUBFR)
	for i := range s {
		s[i] = int16(1000 * sinApprox(float64(i)*0.3))
	}
	var ref int32
	for _, v := range s {
		ref = L_mac(ref, v, v)
	}
	ref >>= 4
	if got := energyNew(s, cL_SUBFR); got != ref {
		t.Errorf("energyNew=%d, want %d", got, ref)
	}
}

// baseSig returns a deterministic subframe scaled by k.
func baseSig(k float64) []int16 {
	s := make([]int16, cL_SUBFR)
	for i := range s {
		s[i] = int16(k * 3000 * sinApprox(float64(i)*0.35))
	}
	return s
}

// runAGC repeatedly applies agc (one subframe per iteration, fresh sigOut copy)
// and returns the final output-energy / target-input-energy ratio.
func runAGC(t *testing.T, inScale float64, iters int) float64 {
	t.Helper()
	sigIn := baseSig(inScale)
	eIn := energyF(sigIn)
	var st agcState
	var eOut float64
	for it := 0; it < iters; it++ {
		out := baseSig(1.0)
		st.agc(sigIn, out, cAGC_FAC, cL_SUBFR)
		eOut = energyF(out)
	}
	return eOut / eIn
}

// TestAGCConvergence checks the AGC drives the post-filter output energy toward
// the synthesis (input) energy over successive subframes, for both
// amplification (louder input) and unity cases.
func TestAGCConvergence(t *testing.T) {
	if r := runAGC(t, 2.0, 60); r < 0.8 || r > 1.25 {
		t.Errorf("amplify case: out/in energy ratio %.3f, want ~1", r)
	}
	if r := runAGC(t, 1.0, 60); r < 0.8 || r > 1.25 {
		t.Errorf("unity case: out/in energy ratio %.3f, want ~1", r)
	}
	if r := runAGC(t, 0.5, 60); r < 0.8 || r > 1.25 {
		t.Errorf("attenuate case: out/in energy ratio %.3f, want ~1", r)
	}
}

// TestAGCZeroOutput checks the zero-energy guard resets past_gain.
func TestAGCZeroOutput(t *testing.T) {
	var st agcState
	st.pastGain = 5000
	zero := make([]int16, cL_SUBFR)
	in := baseSig(1.0)
	st.agc(in, zero, cAGC_FAC, cL_SUBFR)
	if st.pastGain != 0 {
		t.Errorf("zero output: pastGain=%d, want 0", st.pastGain)
	}
}

// TestAGCDeterministic confirms repeatability.
func TestAGCDeterministic(t *testing.T) {
	run := func() int16 {
		var st agcState
		out := baseSig(1.0)
		st.agc(baseSig(2.0), out, cAGC_FAC, cL_SUBFR)
		return out[20]
	}
	if run() != run() {
		t.Errorf("agc non-deterministic")
	}
}
