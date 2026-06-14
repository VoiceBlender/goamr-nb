package amrnb

import "testing"

// TestClLtpMatchedTarget checks the closed-loop LTP: when the target equals the
// adaptive codebook at a known lag (unit-impulse filter), the pitch gain is near
// unity and the codebook-search target xn2 is largely cancelled.
func TestClLtpMatchedTarget(t *testing.T) {
	const excOff = 200
	exc := make([]int16, excOff+cL_SUBFR)
	seed := uint32(0xABC)
	for i := range exc {
		seed = seed*1664525 + 1013904223
		exc[i] = int16(int32(seed>>20) - 2048)
	}
	h1 := make([]int16, cL_SUBFR)
	h1[0] = 4096 // unit impulse

	const T = 50
	xn := make([]int16, cL_SUBFR)
	for j := 0; j < cL_SUBFR; j++ {
		xn[j] = exc[excOff-T+j]
	}
	Top := []int16{T, T}
	res2 := make([]int16, cL_SUBFR)
	xn2 := make([]int16, cL_SUBFR)
	yl := make([]int16, cL_SUBFR)

	var st pitchFrState
	r := st.clLtp(dMR59, 0, Top, h1, exc, excOff, res2, xn, xn2, yl)

	if r.T0 != T {
		t.Errorf("lag %d, want %d", r.T0, T)
	}
	if r.gainPit < 14000 {
		t.Errorf("matched target gain_pit %d, want near unity (16384)", r.gainPit)
	}
	// xn2 (residual after pitch) should be much smaller than xn.
	var exn, exn2 float64
	for j := 0; j < cL_SUBFR; j++ {
		exn += float64(xn[j]) * float64(xn[j])
		exn2 += float64(xn2[j]) * float64(xn2[j])
	}
	if exn2 > exn/4 {
		t.Errorf("xn2 not cancelled: exn=%.0f exn2=%.0f", exn, exn2)
	}
}

// TestClLtpMR122Index checks the MR122 path quantizes the pitch gain and the
// index round-trips through dGainPitch.
func TestClLtpMR122Index(t *testing.T) {
	const excOff = 200
	exc := make([]int16, excOff+cL_SUBFR)
	for i := range exc {
		exc[i] = int16(2000 * sinApprox(float64(i)*0.2))
	}
	h1 := make([]int16, cL_SUBFR)
	h1[0] = 4096
	xn := make([]int16, cL_SUBFR)
	for j := 0; j < cL_SUBFR; j++ {
		xn[j] = exc[excOff-60+j]
	}
	Top := []int16{60, 60}
	res2 := make([]int16, cL_SUBFR)
	xn2 := make([]int16, cL_SUBFR)
	yl := make([]int16, cL_SUBFR)

	var st pitchFrState
	r := st.clLtp(dMR122, 0, Top, h1, exc, excOff, res2, xn, xn2, yl)
	if !r.hasGainPitIndex {
		t.Fatal("MR122 should produce a gain_pit index")
	}
	if got := dGainPitch(dMR122, r.gainPitIndex); got != r.gainPit {
		t.Errorf("MR122 gain_pit %d != dGainPitch(idx=%d)=%d", r.gainPit, r.gainPitIndex, got)
	}
}
