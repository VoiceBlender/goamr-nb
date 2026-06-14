package amrnb

import (
	"math"
	"testing"
)

// TestLog2Absolute checks log2 returns exponent + fraction/2^15 ≈ log2(L_x)
// for positive inputs spanning the 32-bit range.
func TestLog2Absolute(t *testing.T) {
	for _, x := range []int32{1, 2, 3, 1024, 1536, 100000, 1 << 20, 0x40000000, 0x7fffffff} {
		e, f := log2(x)
		got := float64(e) + float64(f)/32768.0
		want := math.Log2(float64(x))
		if math.Abs(got-want) > 1e-3 {
			t.Errorf("log2(%d): got %.5f, want %.5f", x, got, want)
		}
	}
}

// TestPow2Absolute checks pow2(exponent, fraction) ≈ 2^(exponent+fraction/2^15).
func TestPow2Absolute(t *testing.T) {
	for _, e := range []int16{0, 1, 5, 10, 14, 20, 30} {
		for _, f := range []int16{0, 4096, 8192, 16384, 24576, 32767} {
			got := float64(pow2(e, f))
			want := math.Pow(2, float64(e)+float64(f)/32768.0)
			// pow2 returns an integer Q0 value, so small magnitudes quantize
			// hard (this is the reference behaviour). Require relative accuracy
			// only once the result is large enough to carry it.
			if math.Abs(got-want) <= 1 {
				continue
			}
			rel := math.Abs(got-want) / want
			if rel > 2e-3 {
				t.Errorf("pow2(%d,%d): got %.0f, want %.0f (rel %.4f)", e, f, got, want, rel)
			}
		}
	}
}

// TestLog2Pow2RoundTrip composes the two: pow2(log2(x)) ≈ x.
func TestLog2Pow2RoundTrip(t *testing.T) {
	for _, x := range []int32{16, 1000, 65536, 1 << 24, 0x20000000} {
		e, f := log2(x)
		back := float64(pow2(e, f))
		rel := math.Abs(back-float64(x)) / float64(x)
		if rel > 3e-3 {
			t.Errorf("pow2(log2(%d))=%.0f, rel err %.4f", x, back, rel)
		}
	}
}

// TestInvSqrtRatio validates invSqrt up to its fixed scale: the ratio
// invSqrt(a)/invSqrt(b) must track sqrt(b/a), independent of Q-format.
func TestInvSqrtRatio(t *testing.T) {
	xs := []int32{1 << 20, 1 << 24, 100000000, 1 << 28, 0x40000000, 0x7fffffff}
	for i := 0; i < len(xs); i++ {
		for j := i + 1; j < len(xs); j++ {
			a, b := xs[i], xs[j]
			ya, yb := float64(invSqrt(a)), float64(invSqrt(b))
			got := ya / yb
			want := math.Sqrt(float64(b) / float64(a))
			rel := math.Abs(got-want) / want
			if rel > 5e-3 {
				t.Errorf("invSqrt ratio a=%d b=%d: got %.5f, want %.5f", a, b, got, want)
			}
		}
	}
	if invSqrt(0) != 0x3fffffff {
		t.Errorf("invSqrt(0) = %#x, want 0x3fffffff", invSqrt(0))
	}
}

// TestSqrtLExpRatio validates sqrt_l_exp by reconstructing the denormalized
// value (result * 2^(-exp/2)) and checking its ratio tracks sqrt(a/b).
func TestSqrtLExpRatio(t *testing.T) {
	val := func(x int32) float64 {
		r, e := sqrtLExp(x)
		return float64(r) * math.Pow(2, -float64(e)/2.0)
	}
	xs := []int32{1 << 20, 1 << 24, 100000000, 1 << 28, 0x40000000, 0x7fffffff}
	for i := 0; i < len(xs); i++ {
		for j := i + 1; j < len(xs); j++ {
			a, b := xs[i], xs[j]
			got := val(a) / val(b)
			want := math.Sqrt(float64(a) / float64(b))
			rel := math.Abs(got-want) / want
			if rel > 5e-3 {
				t.Errorf("sqrtLExp ratio a=%d b=%d: got %.5f, want %.5f", a, b, got, want)
			}
		}
	}
	if r, e := sqrtLExp(0); r != 0 || e != 0 {
		t.Errorf("sqrtLExp(0) = (%d,%d), want (0,0)", r, e)
	}
}

// TestRoundingShifts checks the rounding behaviour of shr_r / L_shr_r.
func TestRoundingShifts(t *testing.T) {
	// 0b110 >> 1 = 3 with the dropped bit (the low 1) set -> rounds to 3? The
	// dropped MSB is bit (var2-1)=bit0 = 0 here, so no round: 6>>1=3.
	if got := shr_r(6, 1); got != 3 {
		t.Errorf("shr_r(6,1)=%d, want 3", got)
	}
	// 3 >> 1: shr=1, dropped bit0=1 -> +1 = 2.
	if got := shr_r(3, 1); got != 2 {
		t.Errorf("shr_r(3,1)=%d, want 2", got)
	}
	if got := shr_r(100, 20); got != 0 {
		t.Errorf("shr_r oversized: %d, want 0", got)
	}
	// L_shr_r: 3>>1 rounds up to 2.
	if got := L_shr_r(3, 1); got != 2 {
		t.Errorf("L_shr_r(3,1)=%d, want 2", got)
	}
	if got := L_shr_r(1<<20, 40); got != 0 {
		t.Errorf("L_shr_r oversized: %d, want 0", got)
	}
}
