package amrnb

import "testing"

// az4Frame builds the four-subframe LP coefficient set from a single A(z).
func az4Frame() []int16 {
	a := realAz()
	az := make([]int16, cNB_SUBFR*cMP1)
	for sf := 0; sf < cNB_SUBFR; sf++ {
		copy(az[sf*cMP1:], a)
	}
	return az
}

// synthFrame builds a deterministic speech-like synthesis frame.
func synthFrame() []int16 {
	s := make([]int16, cL_FRAME)
	for i := range s {
		s[i] = int16(4000*sinApprox(float64(i)*0.18) + 1500*sinApprox(float64(i)*0.5))
	}
	return s
}

// TestPostFilterRuns checks the post-filter produces a bounded, non-silent,
// deterministic frame and that AGC keeps the output energy close to the input.
func TestPostFilterRuns(t *testing.T) {
	az := az4Frame()
	in := synthFrame()

	var st postFilterState
	st.reset()
	out := append([]int16(nil), in...)
	st.postFilter(dMR59, out, az)

	// non-silent
	var e float64
	for _, v := range out {
		e += float64(v) * float64(v)
	}
	if e == 0 {
		t.Fatal("post-filter produced silence")
	}
	// energy roughly preserved by AGC
	ratio := e / energyF(in)
	if ratio < 0.4 || ratio > 2.5 {
		t.Errorf("post-filter energy ratio %.3f out of expected band", ratio)
	}

	// determinism
	var st2 postFilterState
	st2.reset()
	out2 := append([]int16(nil), in...)
	st2.postFilter(dMR59, out2, az)
	for i := range out {
		if out[i] != out2[i] {
			t.Fatalf("non-deterministic at %d: %d != %d", i, out[i], out2[i])
		}
	}
}

// TestPostFilterModes exercises every mode's gamma selection without panicking
// and confirms bounded output.
func TestPostFilterModes(t *testing.T) {
	az := az4Frame()
	for _, mode := range []int{dMR475, dMR515, dMR59, dMR67, dMR74, dMR795, dMR102, dMR122} {
		var st postFilterState
		st.reset()
		out := synthFrame()
		st.postFilter(mode, out, az)
		// bounded (int16 by construction; just confirm not all-zero)
		nonzero := false
		for _, v := range out {
			if v != 0 {
				nonzero = true
				break
			}
		}
		if !nonzero {
			t.Errorf("mode %d: post-filter output all zero", mode)
		}
	}
}

// TestPostFilterStateCarries checks the inter-frame history makes the second
// frame's output depend on the first (filter memory is live).
func TestPostFilterStateCarries(t *testing.T) {
	az := az4Frame()

	var stA postFilterState
	stA.reset()
	f1 := synthFrame()
	stA.postFilter(dMR59, f1, az) // prime state
	f2a := synthFrame()
	stA.postFilter(dMR59, f2a, az)

	var stB postFilterState
	stB.reset()
	f2b := synthFrame()
	stB.postFilter(dMR59, f2b, az) // fresh state on the same frame

	differs := false
	for i := range f2a {
		if f2a[i] != f2b[i] {
			differs = true
			break
		}
	}
	if !differs {
		t.Error("post-filter ignored carried state across frames")
	}
}
