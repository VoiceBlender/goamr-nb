package amrnb

import "testing"

// TestPitchOlFindsPeriod checks the open-loop pitch estimator locks onto the
// fundamental period (or an integer multiple of it) of a strongly periodic
// signal, for several periods within the pitch range.
func TestPitchOlFindsPeriod(t *testing.T) {
	const (
		pitMin = cPIT_MIN
		pitMax = cPIT_MAX
		Lframe = 80
		off    = pitMax
	)
	for _, P := range []int{30, 40, 60, 80} {
		buf := make([]int16, off+Lframe)
		for n := range buf {
			// one strong harmonic at the fundamental period P
			ph := float64((n % P)) / float64(P)
			buf[n] = int16(9000*sinApprox(2*3.14159265*ph) + 2500*sinApprox(2*3.14159265*ph*2))
		}
		lag := int(pitchOl(dMR59, buf, off, pitMin, pitMax, Lframe))

		// the chosen lag should be a near-multiple of the true period
		ok := false
		for k := 1; k*P <= pitMax+1; k++ {
			if lag >= k*P-2 && lag <= k*P+2 {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("period %d: open-loop lag %d is not a multiple of the period", P, lag)
		}
		if lag < pitMin || lag > pitMax {
			t.Errorf("period %d: lag %d out of range [%d,%d]", P, lag, pitMin, pitMax)
		}
	}
}

// TestPitchOlDeterministic confirms repeatability and the MR122 scaling path.
func TestPitchOlDeterministic(t *testing.T) {
	const (
		pitMax = cPIT_MAX
		Lframe = 80
		off    = pitMax
	)
	buf := make([]int16, off+Lframe)
	for n := range buf {
		buf[n] = int16(6000 * sinApprox(float64(n)*0.13))
	}
	a := pitchOl(dMR122, buf, off, cPIT_MIN_MR122, pitMax, Lframe)
	b := pitchOl(dMR122, buf, off, cPIT_MIN_MR122, pitMax, Lframe)
	if a != b {
		t.Errorf("pitchOl non-deterministic: %d != %d", a, b)
	}
	if a < cPIT_MIN_MR122 || a > pitMax {
		t.Errorf("MR122 lag %d out of range", a)
	}
}
