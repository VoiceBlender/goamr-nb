package amrnb

import "testing"

// TestConvolveFIRMatchesGeneric fuzzes the FIR form of convolve against the
// triangular loop, including full-scale input whose int32 sums wrap.
func TestConvolveFIRMatchesGeneric(t *testing.T) {
	s := lcg(11)
	for it := 0; it < 2000; it++ {
		var x, h [cL_SUBFR]int16
		shift := s.next() % 16
		for i := range x {
			x[i] = int16(int32(s.next()>>16) >> shift)
			h[i] = int16(int32(s.next()>>16) >> shift)
		}
		if it%7 == 0 {
			x[0], h[0] = -32768, -32768
		}
		var a, b [cL_SUBFR]int16
		convolveFIR(x[:], h[:], a[:])
		convolveGeneric(x[:], h[:], b[:], cL_SUBFR)
		if a != b {
			t.Fatalf("iteration %d: FIR %v, generic %v", it, a, b)
		}
	}
}
