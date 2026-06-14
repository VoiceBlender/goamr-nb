package amrnb

import "testing"

// TestCorHX2Peak checks the parameterized correlation peaks at the true delay,
// like corHX, for both the MR102 (4-track) and MR122 (5-track) configurations.
func TestCorHX2Peak(t *testing.T) {
	h := make([]int16, cL_CODE)
	for i := 0; i < 12; i++ {
		h[i] = int16(3000 - 220*i)
	}
	for _, cfg := range []struct{ nbTrack, step int16 }{{4, 5}, {5, 5}} {
		for _, D := range []int{0, 5, 13, 27} {
			x := make([]int16, cL_CODE)
			for n := D; n < cL_CODE; n++ {
				x[n] = h[n-D]
			}
			dn := make([]int16, cL_CODE)
			corHX2(h, x, dn, 1, cfg.nbTrack, cfg.step)
			best, pos := int16(-32768), -1
			for i := range dn {
				if dn[i] > best {
					best = dn[i]
					pos = i
				}
			}
			if pos != D {
				t.Errorf("nbTrack=%d D=%d: peak at %d, want %d", cfg.nbTrack, D, pos, D)
			}
		}
	}
}

// TestSetSign12k2 checks the structural outputs: every sign is ±32767, each
// posMax lies in its track, and ipos is a cyclic track ordering (length
// 2*nbTrack) starting from the strongest track.
func TestSetSign12k2(t *testing.T) {
	const nbTrack, step = int16(4), int16(5)
	dn := make([]int16, cL_CODE)
	cn := make([]int16, cL_CODE)
	seed := uint32(0x77)
	for i := range dn {
		seed = seed*1664525 + 1013904223
		dn[i] = int16(int32(seed>>20) - 2048)
		seed = seed*1664525 + 1013904223
		cn[i] = int16(int32(seed>>20) - 2048)
	}
	sign := make([]int16, cL_CODE)
	posMax := make([]int16, nbTrack)
	ipos := make([]int16, 2*nbTrack)
	setSign12k2(dn, cn, sign, posMax, nbTrack, ipos, step)

	for i, s := range sign {
		if s != 32767 && s != -32767 {
			t.Fatalf("sign[%d]=%d, want ±32767", i, s)
		}
	}
	for tr := int16(0); tr < nbTrack; tr++ {
		if posMax[tr]%step != tr {
			t.Errorf("posMax[%d]=%d not in track %d", tr, posMax[tr], tr)
		}
	}
	// ipos: cyclic, the two halves equal, all tracks present once per half.
	for i := int16(0); i < nbTrack; i++ {
		if ipos[i] != ipos[i+nbTrack] {
			t.Errorf("ipos halves differ at %d: %d != %d", i, ipos[i], ipos[i+nbTrack])
		}
	}
	seen := make([]bool, nbTrack)
	for i := int16(0); i < nbTrack; i++ {
		if ipos[i] < 0 || ipos[i] >= nbTrack {
			t.Fatalf("ipos[%d]=%d out of range", i, ipos[i])
		}
		if seen[ipos[i]] {
			t.Errorf("ipos repeats track %d", ipos[i])
		}
		seen[ipos[i]] = true
	}
}

// TestCorHX2Deterministic confirms repeatability.
func TestCorHX2Deterministic(t *testing.T) {
	h := make([]int16, cL_CODE)
	x := make([]int16, cL_CODE)
	for i := range h {
		h[i] = int16(2000 * sinApprox(float64(i)*0.3))
		x[i] = int16(1500 * sinApprox(float64(i)*0.27))
	}
	a := make([]int16, cL_CODE)
	b := make([]int16, cL_CODE)
	corHX2(h, x, a, 1, 4, 5)
	corHX2(h, x, b, 1, 4, 5)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("corHX2 non-deterministic at %d", i)
		}
	}
}
