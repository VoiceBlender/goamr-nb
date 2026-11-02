package amrnb

// Closed-loop long-term-prediction (adaptive codebook) step, ported from
// opencore-amrnb cl_ltp.cpp. clLtp runs the fractional pitch search (pitchFr),
// builds the adaptive codebook (predLt), filters it (convolve), computes the
// pitch gain (gPitch) with mode-specific clamping (and MR122 quantization), then
// forms the codebook-search target xn2 = xn - gain·y1 and updates res2. When
// the LSPs show a resonance (lspFlag, from check_lsp) and the pitch gain has
// been high, the tone stabilizer limits the pitch gain to GP_CLIP.

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
func (st *pitchFrState) clLtp(ton *tonStabState, lspFlag int16, mode, frameOffset int, Top, h1, exc []int16, excOff int, res2, xn, xn2, yl []int16) clLtpResult {
	var r clLtpResult
	resu3 := int16(0)
	r.T0, r.T0frac, resu3, r.pitchIndex = st.pitchFr(mode, Top, exc, excOff, xn, h1, cL_SUBFR, frameOffset)

	predLt(exc, excOff, r.T0, r.T0frac, cL_SUBFR, resu3)
	convolve(exc[excOff:], h1, yl, cL_SUBFR)

	r.gainPit = gPitch(mode, xn, yl, r.gCoeff[:], cL_SUBFR)
	r.gpLimit = maxInt16
	gpcFlag := false
	if lspFlag != 0 && r.gainPit > cGP_CLIP {
		gpcFlag = ton.checkGpClipping(r.gainPit)
	}

	if mode == dMR475 || mode == dMR515 {
		if r.gainPit > 13926 {
			r.gainPit = 13926
		}
		if gpcFlag {
			r.gpLimit = cGP_CLIP
		}
	} else {
		if gpcFlag {
			r.gpLimit = cGP_CLIP
			r.gainPit = cGP_CLIP
		}
		if mode == dMR122 {
			var cand, cind [3]int16
			r.gainPitIndex = qGainPitch(dMR122, r.gpLimit, &r.gainPit, cand[:], cind[:])
			r.hasGainPitIndex = true
		}
	}

	g := r.gainPit
	for i := 0; i < cL_SUBFR; i++ {
		xn2[i] = xn[i] - int16((int32(yl[i])*int32(g))>>14)
		res2[i] -= int16((int32(exc[excOff+i]) * int32(g)) >> 14)
	}
	return r
}

// tonStabState is the tone stabilizer, which keeps the pitch gain from
// building up on stationary tones. Mirrors tonStabState (ton_stab.cpp).
type tonStabState struct {
	count int16
	gp    [cN_FRAME]int16
}

const cN_FRAME = 7 // old pitch gains in the average

func (st *tonStabState) reset() { *st = tonStabState{} }

// checkLsp reports (1) whether the LSPs have shown a resonance for 12
// frames in a row. Mirrors check_lsp.
func (st *tonStabState) checkLsp(lsp []int16) int16 {
	distMin1 := int16(maxInt16)
	for i := 3; i < cM-2; i++ {
		if d := lsp[i] - lsp[i+1]; d < distMin1 {
			distMin1 = d
		}
	}
	distMin2 := int16(maxInt16)
	for i := 1; i < 3; i++ {
		if d := lsp[i] - lsp[i+1]; d < distMin2 {
			distMin2 = d
		}
	}
	distTh := int16(1100)
	if lsp[1] > 32000 {
		distTh = 600
	} else if lsp[1] > 30500 {
		distTh = 800
	}
	if distMin1 < 1500 || distMin2 < distTh {
		st.count++
	} else {
		st.count = 0
	}
	if st.count >= 12 {
		st.count = 12
		return 1
	}
	return 0
}

// checkGpClipping reports whether the recent pitch gains sum above GP_CLIP.
// Mirrors check_gp_clipping.
func (st *tonStabState) checkGpClipping(gPitch int16) bool {
	sum := shr(gPitch, 3)
	for _, g := range st.gp {
		sum = add(sum, g)
	}
	return sum > cGP_CLIP
}

// updateGpClipping records a subframe's pitch gain. Mirrors
// update_gp_clipping.
func (st *tonStabState) updateGpClipping(gPitch int16) {
	copy(st.gp[:cN_FRAME-1], st.gp[1:])
	st.gp[cN_FRAME-1] = gPitch >> 3
}
