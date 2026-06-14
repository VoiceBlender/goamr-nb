package amrnb

import "testing"

// TestReorderBitsArePermutations checks every per-mode IETF reorder table is a
// bijection over 0..modeBits[mode]-1 and that bitno widths sum to modeBits.
// This is the cheap pure-Go guard that the generated sort tables are
// self-consistent before any C-reference comparison.
func TestReorderBitsArePermutations(t *testing.T) {
	for m := Mode0475; m < numSpeechModes; m++ {
		nb := modeBits[m]
		ro := reorderBits[m]
		if len(ro) != nb {
			t.Fatalf("mode %d: reorderBits len %d, want %d", m, len(ro), nb)
		}
		seen := make([]bool, nb)
		for k, v := range ro {
			if v < 0 || int(v) >= nb {
				t.Fatalf("mode %d: reorderBits[%d]=%d out of range [0,%d)", m, k, v, nb)
			}
			if seen[v] {
				t.Fatalf("mode %d: reorderBits value %d repeated", m, v)
			}
			seen[v] = true
		}
		sum := 0
		for _, w := range bitno[m] {
			sum += int(w)
		}
		if sum != nb {
			t.Errorf("mode %d: bitno sum %d, want modeBits %d", m, sum, nb)
		}
	}
}

// TestPackUnsortRoundTrip proves packFrame and mimeUnsort are exact inverses
// for every speech mode over deterministic parameter arrays whose values span
// each parameter's bitno range. Bit-exact with the opencore-amrnb
// prm2bits/ets_to_ietf and wmf_to_ets/bits2prm chain by construction.
func TestPackUnsortRoundTrip(t *testing.T) {
	seed := uint32(0xC0FFEE)
	next := func() uint32 {
		seed = seed*1664525 + 1013904223
		return seed >> 8
	}
	for m := Mode0475; m < numSpeechModes; m++ {
		bn := bitno[m]
		for iter := 0; iter < 64; iter++ {
			prm := make([]int16, len(bn))
			for i, w := range bn {
				mask := int16((1 << uint(w)) - 1)
				prm[i] = int16(next()) & mask
			}
			data := packFrame(prm, m)
			if want := m.Bytes(); len(data) != want {
				t.Fatalf("mode %d: packed %d bytes, want %d", m, len(data), want)
			}
			got := mimeUnsort(data, m)
			if len(got) != len(prm) {
				t.Fatalf("mode %d: unsorted %d params, want %d", m, len(got), len(prm))
			}
			for i := range prm {
				if got[i] != prm[i] {
					t.Fatalf("mode %d iter %d: param %d round-trip got %d, want %d", m, iter, i, got[i], prm[i])
				}
			}
		}
	}
}

// TestEtsRoundTrip checks the intermediate ETS serial layer (prm2bits/bits2prm)
// independently of the byte reorder.
func TestEtsRoundTrip(t *testing.T) {
	for m := Mode0475; m < numSpeechModes; m++ {
		bn := bitno[m]
		prm := make([]int16, len(bn))
		for i, w := range bn {
			prm[i] = int16((1 << uint(w)) - 1) // all-ones in range
		}
		ets := prm2bits(m, prm)
		if len(ets) != modeBits[m] {
			t.Fatalf("mode %d: ets len %d, want %d", m, len(ets), modeBits[m])
		}
		got := bits2prm(m, ets)
		for i := range prm {
			if got[i] != prm[i] {
				t.Errorf("mode %d: ets param %d got %d, want %d", m, i, got[i], prm[i])
			}
		}
	}
}
