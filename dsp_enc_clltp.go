package amrnb

// Closed-loop long-term-prediction (adaptive codebook) step, ported from
// opencore-amrnb cl_ltp.cpp. clLtp runs the fractional pitch search (pitchFr),
// builds the adaptive codebook (predLt), filters it (convolve), computes the
// pitch gain (gPitch) with mode-specific clamping (and MR122 quantization), then
// forms the codebook-search target xn2 = xn - gain·y1 and updates res2. The
// tone-stabilization gain clip (gpc_flag) is not applied (active-speech path).
// Bit-exact with the reference for that path.

const cGP_CLIP = 15565 // 0.95 Q14 pitch-gain clip

type clLtpResult struct {
	T0, T0frac      int16
	gainPit         int16
	gpLimit         int16
	gCoeff          [5]int16
	pitchIndex      int16
	gainPitIndex    int16 // MR122 only
	hasGainPitIndex bool
}

// clLtp performs the adaptive-codebook analysis for one subframe. exc[excOff] is
// the current subframe; predLt writes it in place. xn2 (codebook target) and
// res2 (residual) are written. Mirrors cl_ltp.
func (st *pitchFrState) clLtp(mode, frameOffset int, Top, h1, exc []int16, excOff int, res2, xn, xn2, yl []int16) clLtpResult {
	var r clLtpResult
	resu3 := int16(0)
	r.T0, r.T0frac, resu3, r.pitchIndex = st.pitchFr(mode, Top, exc, excOff, xn, h1, cL_SUBFR, frameOffset)

	predLt(exc, excOff, r.T0, r.T0frac, cL_SUBFR, resu3)
	convolve(exc[excOff:], h1, yl, cL_SUBFR)

	r.gainPit = gPitch(mode, xn, yl, r.gCoeff[:], cL_SUBFR)
	r.gpLimit = maxInt16 // tone-stabilization clip disabled

	if mode == dMR475 || mode == dMR515 {
		if r.gainPit > 13926 {
			r.gainPit = 13926
		}
	} else if mode == dMR122 {
		var cand, cind [3]int16
		r.gainPitIndex = qGainPitch(dMR122, r.gpLimit, &r.gainPit, cand[:], cind[:])
		r.hasGainPitIndex = true
	}

	g := r.gainPit
	for i := 0; i < cL_SUBFR; i++ {
		xn2[i] = xn[i] - int16((int32(yl[i])*int32(g))>>14)
		res2[i] -= int16((int32(exc[excOff+i]) * int32(g)) >> 14)
	}
	return r
}
