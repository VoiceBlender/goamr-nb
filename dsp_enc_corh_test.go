package amrnb

import "testing"

// makeH1 returns a decaying impulse-response-like vector.
func makeH1() []int16 {
	h := make([]int16, cL_CODE)
	v := 4096.0
	for i := range h {
		h[i] = int16(v * sinApprox(float64(i)*0.3+0.4))
		v *= 0.88
	}
	h[0] = 4096
	return h
}

// TestCorHSymmetry checks the autocorrelation matrix is symmetric — the most
// sensitive test of the pointer-walk translation: any off-by-one would break it.
func TestCorHSymmetry(t *testing.T) {
	const L = cL_CODE
	h := makeH1()
	sign := make([]int16, cL_CODE)
	for i := range sign {
		if i%3 == 0 {
			sign[i] = -32767
		} else {
			sign[i] = 32767
		}
	}
	rr := make([]int16, L*L)
	corH(h, sign, rr)

	for i := 0; i < L; i++ {
		for j := 0; j < L; j++ {
			if rr[i*L+j] != rr[j*L+i] {
				t.Fatalf("rr not symmetric at (%d,%d): %d != %d", i, j, rr[i*L+j], rr[j*L+i])
			}
		}
	}
}

// TestCorHDiagonal checks the diagonal is the truncated energy of the weighted
// impulse response: non-negative and non-increasing with index (rr[0][0] is the
// full energy, rr[L-1][L-1] only the first tap).
func TestCorHDiagonal(t *testing.T) {
	const L = cL_CODE
	h := makeH1()
	sign := make([]int16, cL_CODE)
	for i := range sign {
		sign[i] = 32767
	}
	rr := make([]int16, L*L)
	corH(h, sign, rr)

	for i := 0; i < L; i++ {
		if rr[i*L+i] < 0 {
			t.Errorf("diagonal rr[%d][%d]=%d < 0", i, i, rr[i*L+i])
		}
		if i > 0 && rr[i*L+i] > rr[(i-1)*L+(i-1)] {
			t.Errorf("diagonal not non-increasing at %d: %d > %d", i, rr[i*L+i], rr[(i-1)*L+(i-1)])
		}
	}
	if rr[0] == 0 {
		t.Error("rr[0][0] (total energy) is zero")
	}
}

// TestCorHDeterministic confirms repeatability.
func TestCorHDeterministic(t *testing.T) {
	const L = cL_CODE
	h := makeH1()
	sign := make([]int16, cL_CODE)
	for i := range sign {
		sign[i] = 32767
	}
	a := make([]int16, L*L)
	b := make([]int16, L*L)
	corH(h, sign, a)
	corH(h, sign, b)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("corH non-deterministic at %d", i)
		}
	}
}
