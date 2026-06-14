package amrnb

import "testing"

// TestAzLspRoundTrip converts LSP -> A(z) -> LSP and checks the recovered LSPs
// are close to the originals (the Chebyshev root search is approximate but
// accurate to a few units), and that all 10 roots were found (no fallback).
func TestAzLspRoundTrip(t *testing.T) {
	sets := sampleLSPs() // descending, valid LSP vectors
	// a clearly-different fallback to detect a missed root
	fallback := []int16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	for k, lsp := range sets {
		a := make([]int16, cMP1)
		lspAz(lsp, a)

		out := make([]int16, cM)
		azLsp(a, out, fallback)

		// fallback must not have been used
		usedFallback := true
		for i := 0; i < cM; i++ {
			if out[i] != fallback[i] {
				usedFallback = false
				break
			}
		}
		if usedFallback {
			t.Fatalf("set %d: Az_lsp fell back to old_lsp (missed roots)", k)
		}

		var maxd int
		for i := 0; i < cM; i++ {
			d := int(out[i]) - int(lsp[i])
			if d < 0 {
				d = -d
			}
			if d > maxd {
				maxd = d
			}
		}
		if maxd > 64 {
			t.Errorf("set %d: LSP round-trip max abs error %d, want <= 64", k, maxd)
		}
		// recovered LSPs must remain strictly descending (valid ordering)
		for i := 0; i < cM-1; i++ {
			if out[i] <= out[i+1] {
				t.Errorf("set %d: recovered LSP not descending at %d (%d <= %d)", k, i, out[i], out[i+1])
			}
		}
	}
}

// TestAzLspDeterministic confirms repeatability.
func TestAzLspDeterministic(t *testing.T) {
	a := realAz()
	o1 := make([]int16, cM)
	o2 := make([]int16, cM)
	azLsp(a, o1, lsp_init_data)
	azLsp(a, o2, lsp_init_data)
	for i := range o1 {
		if o1[i] != o2[i] {
			t.Fatalf("non-deterministic at %d", i)
		}
	}
}
