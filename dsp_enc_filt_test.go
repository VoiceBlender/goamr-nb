package amrnb

import "testing"

// TestConvolveImpulse: convolving with a unit impulse (h[0]=4096 Q12) reproduces
// the input.
func TestConvolveImpulse(t *testing.T) {
	h := make([]int16, cL_SUBFR)
	h[0] = 4096 // 1.0 in Q12
	x := make([]int16, cL_SUBFR)
	for i := range x {
		x[i] = int16(1000 * sinApprox(float64(i)*0.3))
	}
	y := make([]int16, cL_SUBFR)
	convolve(x, h, y, cL_SUBFR)
	for i := range x {
		if y[i] != x[i] {
			t.Fatalf("impulse convolve at %d: %d != %d", i, y[i], x[i])
		}
	}
}

// TestConvolveKnown checks a small hand-computed lower-triangular convolution.
func TestConvolveKnown(t *testing.T) {
	// x = {a, b, c}, h = {p, q, r}; y[n] = (sum_{i<=n} x[i]h[n-i]) >> 12
	x := []int16{8192, 4096, 2048} // 2.0, 1.0, 0.5 in Q12
	h := []int16{4096, 8192, 2048} // 1.0, 2.0, 0.5 in Q12
	y := make([]int16, 3)
	convolve(x, h, y, 3)
	want := []int16{
		int16((int32(8192) * 4096) >> 12),                                     // x0 h0
		int16((int32(8192)*8192 + int32(4096)*4096) >> 12),                    // x0 h1 + x1 h0
		int16((int32(8192)*2048 + int32(4096)*8192 + int32(2048)*4096) >> 12), // x0 h2 + x1 h1 + x2 h0
	}
	for i := range y {
		if y[i] != want[i] {
			t.Errorf("convolve[%d]=%d, want %d", i, y[i], want[i])
		}
	}
}

// TestConvolveLinear checks the convolution scales linearly with the input.
func TestConvolveLinear(t *testing.T) {
	h := make([]int16, cL_SUBFR)
	for i := 0; i < 8; i++ {
		h[i] = int16(2000 - 200*i)
	}
	mk := func(scale int16) []int16 {
		x := make([]int16, cL_SUBFR)
		for i := range x {
			x[i] = scale * int16(1+i%3)
		}
		y := make([]int16, cL_SUBFR)
		convolve(x, h, y, cL_SUBFR)
		return y
	}
	y1 := mk(100)
	y2 := mk(200)
	for i := range y1 {
		if d := int(y2[i]) - 2*int(y1[i]); d < -2 || d > 2 {
			t.Errorf("nonlinear at %d: y1=%d y2=%d", i, y1[i], y2[i])
		}
	}
}
