package amrnb

import "testing"

// TestCorHXPeak checks the target↔impulse correlation peaks at the true delay:
// if the target is the impulse response delayed by D, dn[] should be maximal at
// position D (the autocorrelation peak).
func TestCorHXPeak(t *testing.T) {
	// a short decaying impulse response
	h := make([]int16, cL_CODE)
	for i := 0; i < 12; i++ {
		h[i] = int16(3000 - 220*i)
	}
	for _, D := range []int{0, 5, 13, 27} {
		x := make([]int16, cL_CODE)
		for n := D; n < cL_CODE; n++ {
			x[n] = h[n-D]
		}
		dn := make([]int16, cL_CODE)
		corHX(h, x, dn, 1)

		// dn[D] should be the (positive) maximum.
		best, bestPos := int16(-32768), -1
		for i := range dn {
			if dn[i] > best {
				best = dn[i]
				bestPos = i
			}
		}
		if bestPos != D {
			t.Errorf("delay %d: dn peak at %d (val %d), want %d", D, bestPos, best, D)
		}
	}
}

// TestSetSign checks sign extraction, absolute value, and per-track pruning.
func TestSetSign(t *testing.T) {
	dn := make([]int16, cL_CODE)
	for i := range dn {
		// alternate signs, magnitude grows with index within track
		if i%2 == 0 {
			dn[i] = int16(100 + i*10)
		} else {
			dn[i] = int16(-(100 + i*10))
		}
	}
	sign := make([]int16, cL_CODE)
	dn2 := make([]int16, cL_CODE)
	orig := append([]int16(nil), dn...)

	const n = 2
	setSign(dn, sign, dn2, n)

	for i := range dn {
		// sign matches, dn is now |dn|
		if orig[i] >= 0 && sign[i] != 32767 {
			t.Errorf("pos %d: sign %d, want +32767", i, sign[i])
		}
		if orig[i] < 0 && sign[i] != -32767 {
			t.Errorf("pos %d: sign %d, want -32767", i, sign[i])
		}
		if dn[i] != abs16(orig[i]) {
			t.Errorf("pos %d: dn %d, want |%d|", i, dn[i], orig[i])
		}
	}

	// each track keeps exactly n non-negative entries in dn2.
	for tr := 0; tr < cNB_TRACK; tr++ {
		kept := 0
		for j := tr; j < cL_CODE; j += cSTEP {
			if dn2[j] >= 0 {
				kept++
			}
		}
		if kept != n {
			t.Errorf("track %d kept %d positions, want %d", tr, kept, n)
		}
	}
}

// TestCorHXDeterministic confirms repeatability and bounded output.
func TestCorHXDeterministic(t *testing.T) {
	h := make([]int16, cL_CODE)
	x := make([]int16, cL_CODE)
	for i := range h {
		h[i] = int16(2000 * sinApprox(float64(i)*0.3))
		x[i] = int16(1500 * sinApprox(float64(i)*0.27))
	}
	a := make([]int16, cL_CODE)
	b := make([]int16, cL_CODE)
	corHX(h, x, a, 1)
	corHX(h, x, b, 1)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("corHX non-deterministic at %d", i)
		}
	}
}
