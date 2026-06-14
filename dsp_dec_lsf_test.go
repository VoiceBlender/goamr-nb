package amrnb

import "testing"

// assertValidLSP checks an LSP vector is strictly descending (cosine domain)
// and that converting it to LP coefficients yields the a[0]=4096 invariant — a
// strong signal the decoded LSFs were valid and ordered.
func assertValidLSP(t *testing.T, tag string, lsp []int16) {
	t.Helper()
	for i := 0; i < cM-1; i++ {
		if lsp[i] <= lsp[i+1] {
			t.Errorf("%s: lsp not strictly descending at %d (%d <= %d)", tag, i, lsp[i], lsp[i+1])
		}
	}
	a := make([]int16, cMP1)
	lspAz(lsp, a)
	if a[0] != 4096 {
		t.Errorf("%s: lspAz a[0]=%d, want 4096", tag, a[0])
	}
}

func TestDPlsf3Decode(t *testing.T) {
	cases := []struct {
		name    string
		mode    int
		indices []int16
	}{
		{"MR475", dMR475, []int16{40, 80, 60}},
		{"MR515", dMR515, []int16{30, 70, 50}},
		{"MR59", dMR59, []int16{40, 80, 120}},
		{"MR67", dMR67, []int16{50, 100, 200}},
		{"MR74", dMR74, []int16{60, 150, 250}},
		{"MR795", dMR795, []int16{100, 200, 300}},
		{"MR102", dMR102, []int16{40, 80, 120}},
	}
	for _, c := range cases {
		var st dPlsfState
		st.reset()
		lsp := make([]int16, cM)
		st.dPlsf3(c.mode, 0, c.indices, lsp)
		assertValidLSP(t, c.name, lsp)
	}
}

func TestDPlsf5Decode(t *testing.T) {
	var st dPlsfState
	st.reset()
	lsp1 := make([]int16, cM)
	lsp2 := make([]int16, cM)
	st.dPlsf5(0, []int16{10, 20, 30, 40, 5}, lsp1, lsp2)
	assertValidLSP(t, "MR122/lsp1", lsp1)
	assertValidLSP(t, "MR122/lsp2", lsp2)
}

// TestDPlsfDeterministic verifies a fresh predictor state plus identical
// indices reproduces identical output.
func TestDPlsfDeterministic(t *testing.T) {
	run := func() []int16 {
		var st dPlsfState
		st.reset()
		lsp := make([]int16, cM)
		st.dPlsf3(dMR74, 0, []int16{60, 150, 250}, lsp)
		return lsp
	}
	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at %d: %d != %d", i, a[i], b[i])
		}
	}
}

// TestDPlsfStateEvolves runs several frames through the predictor and checks
// every frame stays valid (the past-residual memory must remain consistent).
func TestDPlsfStateEvolves(t *testing.T) {
	var st dPlsfState
	st.reset()
	seq := [][]int16{{40, 80, 120}, {50, 90, 130}, {30, 70, 110}, {45, 85, 125}}
	for f, idx := range seq {
		lsp := make([]int16, cM)
		st.dPlsf3(dMR59, 0, idx, lsp)
		assertValidLSP(t, "frame"+itoa(f), lsp)
	}
}

// TestDPlsf3BadFrame exercises the concealment path (bfi=1): it must still
// produce a valid, ordered LSP set from predictor memory alone.
func TestDPlsf3BadFrame(t *testing.T) {
	var st dPlsfState
	st.reset()
	// prime the predictor with a good frame first
	lsp := make([]int16, cM)
	st.dPlsf3(dMR59, 0, []int16{40, 80, 120}, lsp)
	// now a bad frame
	st.dPlsf3(dMR59, 1, []int16{0, 0, 0}, lsp)
	assertValidLSP(t, "MR59/bfi", lsp)
}
