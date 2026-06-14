package amrnb

// Open-loop pitch estimation, ported from opencore-amrnb pitch_ol.cpp
// (Pitch_ol, Lag_max) and calc_cor.cpp (comp_corr). pitchOl scales the weighted
// speech, computes its autocorrelation over the pitch-lag range, finds the best
// lag in three sub-ranges, and selects among them favouring longer lags unless
// a shorter lag's normalized correlation is significantly stronger. The DTX/VAD
// tone-detection side path is omitted (dtx=0). Bit-exact with the reference for
// the active-speech path.

const cTHRESHOLD = 27853 // 0.85, the open-loop lag-selection threshold

// lagMax finds the lag in [lagMin, lagMax] with maximum correlation and returns
// it together with its normalized correlation. Mirrors Lag_max (dtx=0 path).
func lagMax(corr []int32, scaled []int16, pitMax int, scalFac, scalFlag int16, Lframe, lagMax, lagMin int) (int16, int16) {
	max := int32(minInt32)
	pMax := int16(lagMax)
	for i := lagMax; i >= lagMin; i-- {
		if corr[i] >= max {
			max = corr[i]
			pMax = int16(i)
		}
	}

	var t0 int32
	base := pitMax - int(pMax)
	for m := 0; m < Lframe; m++ {
		v := int32(scaled[base+m])
		t0 += v * v
	}
	t0 <<= 1
	t0 = invSqrt(t0)
	if scalFlag != 0 {
		if t0 > 0x3fffffff {
			t0 = maxInt32
		} else {
			t0 <<= 1
		}
	}

	maxH := int16(max >> 16)
	maxL := int16((max >> 1) - (int32(maxH) << 15))
	enerH := int16(t0 >> 16)
	enerL := int16((t0 >> 1) - (int32(enerH) << 15))
	t0 = mpy_32(maxH, maxL, enerH, enerL)

	var corMax int16
	if scalFlag != 0 {
		t0 = L_shr(t0, scalFac)
		if t0 > 0x0000FFFF {
			corMax = maxInt16
		} else if t0 < int32(-0x10000) {
			corMax = minInt16
		} else {
			corMax = int16(t0 >> 1)
		}
	} else {
		corMax = int16(t0)
	}
	return pMax, corMax
}

// pitchOl returns the open-loop pitch lag for the weighted speech in
// signal[off-pitMax : off+Lframe], using the given pitch-lag bounds and frame
// length. Mirrors Pitch_ol (dtx=0). signal[off] is the current frame start.
func pitchOl(mode int, signal []int16, off, pitMin, pitMax, Lframe int) int16 {
	// energy of signal[-pitMax .. Lframe-1]
	var t0 int32
	overflow := false
	for i := -pitMax; i < Lframe; i++ {
		v := int32(signal[off+i])
		t0 += (v * v) << 1
		if t0 < 0 {
			t0 = maxInt32
			overflow = true
			break
		}
	}

	n := pitMax + Lframe
	var scaledArr [cPIT_MAX + cL_FRAME]int16 // stack: max pitMax+Lframe
	scaled := scaledArr[:n]
	var scalFac int16
	switch {
	case overflow: // t0 == MAX_32
		for k := 0; k < n; k++ {
			scaled[k] = int16(int32(signal[off-pitMax+k]) >> 3)
		}
		scalFac = 3
	case t0 < 1048576:
		for k := 0; k < n; k++ {
			scaled[k] = int16(int32(signal[off-pitMax+k]) << 3)
		}
		scalFac = -3
	default:
		for k := 0; k < n; k++ {
			scaled[k] = signal[off-pitMax+k]
		}
		scalFac = 0
	}

	// correlation by lag: corr[lag] = 2 * sum_m scaled[pitMax+m]·scaled[pitMax+m-lag].
	// With the fixed window coef = scaled[pitMax:pitMax+Lframe] this is one sliding
	// FIR over scaled: dst[pitMax-lag] = sum_m scaled[pitMax-lag+m]·coef[m] equals
	// corr[lag]>>1. firRaw (AVX2) vectorises it; the wrapping int32 accumulation
	// makes the lane-parallel order bit-identical.
	var corrArr [cPIT_MAX + 1]int32 // stack
	corr := corrArr[:pitMax+1]
	var dstArr [cPIT_MAX + 1]int32
	dst := dstArr[:pitMax-pitMin+1]
	firRaw(dst, scaled, scaled[pitMax:pitMax+Lframe])
	for lag := pitMin; lag <= pitMax; lag++ {
		corr[lag] = dst[pitMax-lag] << 1
	}

	scalFlag := int16(0)
	if mode == dMR122 {
		scalFlag = 1
	}

	j := pitMin << 2
	pMax1, max1 := lagMax(corr, scaled, pitMax, scalFac, scalFlag, Lframe, pitMax, j)
	i := j - 1
	j = pitMin << 1
	pMax2, max2 := lagMax(corr, scaled, pitMax, scalFac, scalFlag, Lframe, i, j)
	i = j - 1
	pMax3, max3 := lagMax(corr, scaled, pitMax, scalFac, scalFlag, Lframe, i, pitMin)

	if int16((int32(max1)*cTHRESHOLD)>>15) < max2 {
		max1 = max2
		pMax1 = pMax2
	}
	if int16((int32(max1)*cTHRESHOLD)>>15) < max3 {
		pMax1 = pMax3
	}
	return pMax1
}
