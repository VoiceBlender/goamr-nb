package amrnb

// Weighted open-loop pitch analysis for MR102, ported from opencore-amrnb
// p_ol_wgh.cpp (Pitch_ol_wgh, Lag_max) and calc_cor.cpp (comp_corr). Unlike the
// plain Pitch_ol used by the other modes, MR102 weights the lag correlations by
// a fixed curve and, when the previous frame tracked pitch well, by a window
// centred on the running median lag — then adapts whether to keep weighting.
// It also emits a per-half-frame gain flag that gates the median update and the
// closed-loop lag history. Bit-exact with the reference (non-DTX path).

// pitchOLWghtState is the weighted open-loop pitch state. Mirrors
// pitchOLWghtState.
type pitchOLWghtState struct {
	oldT0Med int16 // running median of past lags
	adaW     int16 // adaptive weighting factor, Q15
	wghtFlg  int16 // whether the median-window weighting is applied
}

func (st *pitchOLWghtState) reset() {
	st.oldT0Med = 40
	st.adaW = 0
	st.wghtFlg = 0
}

// compCorr computes corr[-i] = 2·Σ_j sig[j]·sig[j-i] (wrapping int32) for every
// lag i in [pitMin, pitMax], where corr is indexed corr[corrOff-i]. Mirrors
// comp_corr.
func compCorr(sig []int16, sigOff, Lframe, pitMax, pitMin int, corr []int32, corrOff int) {
	// corr[corrOff-i] = 2·sum_j sig[sigOff+j]·sig[sigOff+j-i]. With the fixed
	// window coef = sig[sigOff:sigOff+Lframe] over x = sig[sigOff-pitMax:], this
	// is one sliding FIR: dst[pitMax-i] = sum_j sig[sigOff-i+j]·coef[j] equals
	// corr[corrOff-i]>>1. firRaw (AVX2) vectorises it, bit-identically (wrapping).
	var dstArr [cPIT_MAX + 1]int32
	dst := dstArr[:pitMax-pitMin+1]
	firRaw(dst, sig[sigOff-pitMax:], sig[sigOff:sigOff+Lframe])
	for i := pitMin; i <= pitMax; i++ {
		corr[corrOff-i] = dst[pitMax-i] << 1
	}
}

// lagMaxWght finds the best (most weighted-correlated) lag and the open-loop
// gain flag. Mirrors the weighted Lag_max (non-DTX path: cor_max is unused).
func lagMaxWght(corr []int32, corrOff int, sig []int16, sigOff int, Lframe, lagMax, lagMin, oldLag, wghtFlg int16) (pMax, gainFlg int16) {
	wwi := 250                             // &corrweight[250]
	wei := 123 + int(lagMax) - int(oldLag) // &corrweight[123 + lag_max - old_lag]
	max := int32(-2147483648)              // MIN_32
	pMax = lagMax
	for i := lagMax; i >= lagMin; i-- {
		hi, lo := L_Extract(corr[corrOff-int(i)])
		t0 := mpy32_16(hi, lo, corrweight[wwi])
		wwi--
		if wghtFlg > 0 {
			hi, lo = L_Extract(t0)
			t0 = mpy32_16(hi, lo, corrweight[wei])
			wei--
		}
		if t0 >= max {
			max = t0
			pMax = i
		}
	}

	var t0, t1 int32
	for j := 0; j < int(Lframe); j++ {
		a := sig[sigOff+j]
		b := sig[sigOff+j-int(pMax)]
		t0 = L_mac(t0, a, b)
		t1 = L_mac(t1, b, b)
	}
	temp := round_(t1)
	t1 = L_msu(t0, temp, 13107) // t0 - round(t1)*0.4
	gainFlg = round_(t1)
	return
}

// pitchOlWgh runs the MR102 weighted open-loop pitch over one half-frame of the
// weighted speech (sig[sigOff-pitMax : sigOff+Lframe]), updating the state and
// the gain flag, and maintaining the median-lag history. Mirrors Pitch_ol_wgh
// (non-DTX). idx selects the half-frame (0 or 1).
func (st *pitchOLWghtState) pitchOlWgh(sig []int16, sigOff, pitMin, pitMax, Lframe int, oldLags, olGainFlg []int16, idx int) int16 {
	// energy of the (history + frame) signal, to choose the scaling
	var t0 int32
	for i := -pitMax; i < Lframe; i++ {
		t0 = L_mac(t0, sig[sigOff+i], sig[sigOff+i])
	}

	// scaled signal occupies scal[pitMax-pitMax : pitMax+Lframe]; use a buffer
	// with the same [pitMax | Lframe] layout as the input.
	var scalArr [cPIT_MAX + cL_FRAME]int16 // stack
	scal := scalArr[:pitMax+Lframe]
	scalOff := pitMax
	switch {
	case t0 == maxInt32:
		for i := -pitMax; i < Lframe; i++ {
			scal[scalOff+i] = shr(sig[sigOff+i], 3)
		}
	case t0 < 1048576:
		for i := -pitMax; i < Lframe; i++ {
			scal[scalOff+i] = shl(sig[sigOff+i], 3)
		}
	default:
		for i := -pitMax; i < Lframe; i++ {
			scal[scalOff+i] = sig[sigOff+i]
		}
	}

	var corrArr [cPIT_MAX + 1]int32 // stack
	corr := corrArr[:pitMax+1]
	corrOff := pitMax
	compCorr(scal, scalOff, Lframe, pitMax, pitMin, corr, corrOff)

	pMax1, gainFlg := lagMaxWght(corr, corrOff, scal, scalOff, int16(Lframe),
		int16(pitMax), int16(pitMin), st.oldT0Med, st.wghtFlg)
	olGainFlg[idx] = gainFlg

	if olGainFlg[idx] > 0 {
		for i := 4; i > 0; i-- {
			oldLags[i] = oldLags[i-1]
		}
		oldLags[0] = pMax1
		st.oldT0Med = gmedN(oldLags, 5)
		st.adaW = 32767
	} else {
		st.oldT0Med = pMax1
		st.adaW = mult(st.adaW, 29491) // ada_w *= 0.9
	}

	if st.adaW < 9830 { // < 0.3
		st.wghtFlg = 0
	} else {
		st.wghtFlg = 1
	}
	return pMax1
}
