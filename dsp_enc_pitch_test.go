package amrnb

import "testing"

// TestEncLag3FirstSubframe checks encLag3/decLag3 are inverse over the full
// 1/3-resolution grid for the 1st/3rd subframe (absolute coding).
func TestEncLag3FirstSubframe(t *testing.T) {
	for T0 := int16(20); T0 <= cPIT_MAX; T0++ {
		fracs := []int16{0}
		if T0 <= 84 {
			fracs = []int16{-1, 0, 1}
		}
		for _, frac := range fracs {
			idx := encLag3(T0, frac, 0, cPIT_MIN, cPIT_MAX, 0, 0)
			gT0, gFrac := decLag3(idx, cPIT_MIN, cPIT_MAX, 0, 0, 0)
			if gT0 != T0 || gFrac != frac {
				t.Fatalf("1st subfr T0=%d frac=%d -> idx=%d -> (%d,%d)", T0, frac, idx, gT0, gFrac)
			}
		}
	}
}

// TestEncLag3SecondSubframe checks the relative (flag4=0) 1/3 grid round-trips.
func TestEncLag3SecondSubframe(t *testing.T) {
	const t0min, t0max = 40, 49 // t0max = t0min + 9
	for T0 := int16(t0min); T0 <= t0max; T0++ {
		for _, frac := range []int16{-1, 0, 1} {
			idx := encLag3(T0, frac, 0, t0min, t0max, 1, 0)
			gT0, gFrac := decLag3(idx, t0min, t0max, 1, 0, 0)
			if gT0 != T0 || gFrac != frac {
				t.Fatalf("2nd subfr T0=%d frac=%d -> idx=%d -> (%d,%d)", T0, frac, idx, gT0, gFrac)
			}
		}
	}
}

// TestEncLag3Flag4 checks the 4-bit (flag4=1) relative scheme round-trips over
// its transmittable index space.
func TestEncLag3Flag4(t *testing.T) {
	const t0prev, t0min, t0max = 40, 35, 44
	for idx := int16(0); idx < 16; idx++ {
		gT0, gFrac := decLag3(idx, t0min, t0max, 1, t0prev, 1)
		idx2 := encLag3(gT0, gFrac, t0prev, t0min, t0max, 1, 1)
		if idx2 != idx {
			t.Fatalf("flag4 idx=%d -> (%d,%d) -> idx=%d", idx, gT0, gFrac, idx2)
		}
	}
}

// TestEncLag6FirstSubframe checks encLag6/decLag6 invert over the 1/6 grid for
// the MR122 1st/3rd subframe.
func TestEncLag6FirstSubframe(t *testing.T) {
	for T0 := int16(cPIT_MIN_MR122); T0 <= cPIT_MAX; T0++ {
		fracs := []int16{0}
		if T0 <= 94 {
			fracs = []int16{-2, -1, 0, 1, 2, 3}
		}
		for _, frac := range fracs {
			idx := encLag6(T0, frac, 0, 0)
			gT0, gFrac := decLag6(idx, cPIT_MIN_MR122, cPIT_MAX, 0, 0)
			if gT0 != T0 || gFrac != frac {
				t.Fatalf("MR122 1st T0=%d frac=%d -> idx=%d -> (%d,%d)", T0, frac, idx, gT0, gFrac)
			}
		}
	}
}

// TestEncLag6SecondSubframe checks the relative MR122 grid round-trips. The
// decoder derives its search base T0_min from the previous subframe's lag
// (oldT0); the encoder is handed that same t0min, so they must agree.
func TestEncLag6SecondSubframe(t *testing.T) {
	const oldT0 = 45
	t0min := int16(oldT0 - 5) // = decLag6's internal T0_min
	for T0 := t0min; T0 <= t0min+9; T0++ {
		for _, frac := range []int16{-2, -1, 0, 1, 2, 3} {
			idx := encLag6(T0, frac, t0min, 1)
			gT0, gFrac := decLag6(idx, cPIT_MIN_MR122, cPIT_MAX, 1, oldT0)
			if gT0 != T0 || gFrac != frac {
				t.Fatalf("MR122 2nd T0=%d frac=%d -> idx=%d -> (%d,%d)", T0, frac, idx, gT0, gFrac)
			}
		}
	}
}
