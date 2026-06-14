package amrnb

import "testing"

// TestGPitchRatio checks the adaptive-codebook gain equals <xn,y1>/<y1,y1> in
// Q14: when the target xn is a known scalar multiple of the filtered excitation
// y1, gPitch recovers that scalar (1.0 -> 16384, 0.5 -> 8192), with the 1.2
// saturation and zero-target cases.
func TestGPitchRatio(t *testing.T) {
	y1 := make([]int16, cL_SUBFR)
	for i := range y1 {
		y1[i] = int16(2000 * sinApprox(float64(i)*0.4)) // Q12-ish filtered excitation
	}
	gc := make([]int16, 5)

	// xn = y1  -> gain ~ 1.0 (16384 Q14)
	xn := make([]int16, cL_SUBFR)
	copy(xn, y1)
	if g := gPitch(dMR59, xn, y1, gc, cL_SUBFR); g < 16200 || g > 16550 {
		t.Errorf("xn=y1: gain %d, want ~16384", g)
	}

	// xn = y1/2 -> gain ~ 0.5 (8192)
	for i := range xn {
		xn[i] = y1[i] >> 1
	}
	if g := gPitch(dMR59, xn, y1, gc, cL_SUBFR); g < 8000 || g > 8400 {
		t.Errorf("xn=y1/2: gain %d, want ~8192", g)
	}

	// xn = 2*y1 -> gain saturates at 1.2 (19661)
	for i := range xn {
		xn[i] = shl(y1[i], 1)
	}
	if g := gPitch(dMR59, xn, y1, gc, cL_SUBFR); g != 19661 {
		t.Errorf("xn=2y1: gain %d, want 19661 (saturated)", g)
	}

	// xn = 0 -> gain 0
	for i := range xn {
		xn[i] = 0
	}
	if g := gPitch(dMR59, xn, y1, gc, cL_SUBFR); g != 0 {
		t.Errorf("xn=0: gain %d, want 0", g)
	}
}

// TestGPitchMR122Mask checks MR122 clears the two LSBs of the gain.
func TestGPitchMR122Mask(t *testing.T) {
	y1 := make([]int16, cL_SUBFR)
	xn := make([]int16, cL_SUBFR)
	for i := range y1 {
		y1[i] = int16(1500 * sinApprox(float64(i)*0.35))
		xn[i] = int16(float64(y1[i]) * 0.73)
	}
	gc := make([]int16, 5)
	g := gPitch(dMR122, xn, y1, gc, cL_SUBFR)
	if g&3 != 0 {
		t.Errorf("MR122 gain %d not LSB-masked", g)
	}
}

// TestGPitchDeterministic confirms repeatability and that the correlation
// coefficients are populated.
func TestGPitchDeterministic(t *testing.T) {
	y1 := make([]int16, cL_SUBFR)
	xn := make([]int16, cL_SUBFR)
	for i := range y1 {
		y1[i] = int16(1800 * sinApprox(float64(i)*0.25))
		xn[i] = int16(900 * sinApprox(float64(i)*0.25))
	}
	g1 := make([]int16, 5)
	g2 := make([]int16, 5)
	a := gPitch(dMR74, xn, y1, g1, cL_SUBFR)
	b := gPitch(dMR74, xn, y1, g2, cL_SUBFR)
	if a != b {
		t.Errorf("gPitch non-deterministic: %d != %d", a, b)
	}
	if g1[0] == 0 && g1[2] == 0 {
		t.Errorf("gCoeff not populated: %v", g1)
	}
}
