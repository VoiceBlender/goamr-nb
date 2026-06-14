package amrnb

// Adaptive formant post-filter, ported from opencore-amrnb pstfilt.cpp. For each
// subframe it builds the weighted LP filters A(z/γ3) and A(z/γ4), forms the
// residual through A(z/γ3), derives a spectral-tilt compensation coefficient
// from the truncated impulse response, applies it via preemphasis, synthesizes
// through 1/A(z/γ4), and finally matches the energy back to the synthesis with
// AGC. Uses the building blocks weightAi/residu/synFilt/preemphasis/agc; this is
// the orchestration that chains them. Bit-exact with the reference.

const (
	cL_H = 22    // truncated impulse-response length
	cMU  = 26214 // tilt compensation factor (0.8)
)

// γ bandwidth-expansion factors (pstfilt.cpp).
var gamma3MR122 = [cM]int16{22938, 16057, 11240, 7868, 5508, 3856, 2699, 1889, 1322, 925}
var gamma3 = [cM]int16{18022, 9912, 5451, 2998, 1649, 907, 499, 274, 151, 83}
var gamma4MR122 = [cM]int16{24576, 18432, 13824, 10368, 7776, 5832, 4374, 3281, 2461, 1846}
var gamma4 = [cM]int16{22938, 16057, 11240, 7868, 5508, 3856, 2699, 1889, 1322, 925}

// postFilterState holds the formant post-filter memory across frames.
type postFilterState struct {
	res2      [cL_SUBFR]int16
	memSynPst [cM]int16
	preemph   int16
	agc       agcState
	synthBuf  [cM + cL_FRAME]int16 // M history + current frame
}

func (st *postFilterState) reset() {
	*st = postFilterState{}
}

// postFilter post-filters one 160-sample synthesis frame in place. az4 holds the
// four per-subframe LP coefficient sets (4*cMP1). Mirrors Post_Filter.
func (st *postFilterState) postFilter(mode int, syn []int16, az4 []int16) {
	var ap3, ap4 [cMP1]int16
	var h [cL_H]int16

	synWork := st.synthBuf[cM:] // syn_work[i] == synthBuf[M+i]
	copy(synWork[:cL_FRAME], syn[:cL_FRAME])

	azOff := 0
	for iSubfr := 0; iSubfr < cL_FRAME; iSubfr += cL_SUBFR {
		az := az4[azOff:]
		if mode == dMR122 || mode == dMR102 {
			weightAi(az, gamma3MR122[:], ap3[:])
			weightAi(az, gamma4MR122[:], ap4[:])
		} else {
			weightAi(az, gamma3[:], ap3[:])
			weightAi(az, gamma4[:], ap4[:])
		}

		// Residual through A(z/γ3); syn_work[iSubfr] == synthBuf[M+iSubfr].
		residu(ap3[:], st.synthBuf[:], cM+iSubfr, cL_SUBFR, st.res2[:])

		// Truncated impulse response of A(z/γ3)/A(z/γ4).
		copy(h[:cMP1], ap3[:cMP1])
		for i := cMP1; i < cL_H; i++ {
			h[i] = 0
		}
		synFilt(ap4[:], h[:], h[:], cL_H, h[cMP1:], false)

		// Tilt compensation coefficient from the impulse-response autocorr.
		var lTmp int32
		for i := cL_H - 1; i >= 0; i-- {
			t2 := int32(h[i]) * int32(h[i])
			if t2 == 0x40000000 {
				break // reference sets overflow and stops without adding
			}
			lTmp = L_add(lTmp, t2<<1)
		}
		temp1 := int16(lTmp >> 16)

		lTmp = 0
		for i := cL_H - 2; i >= 0; i-- {
			t2 := int32(h[i]) * int32(h[i+1])
			if t2 == 0x40000000 {
				break
			}
			lTmp = L_add(lTmp, t2<<1)
		}
		temp2 := int16(lTmp >> 16)

		if temp2 <= 0 {
			temp2 = 0
		} else {
			lt := (int32(temp2) * cMU) >> 15
			if lt&0x00010000 != 0 {
				lt |= int32(-0x10000)
			}
			temp2 = int16(lt)
			temp2 = div_s(temp2, temp1)
		}

		preemphasis(&st.preemph, st.res2[:], temp2, cL_SUBFR)

		synFilt(ap4[:], st.res2[:], syn[iSubfr:], cL_SUBFR, st.memSynPst[:], true)

		st.agc.agc(synWork[iSubfr:], syn[iSubfr:], cAGC_FAC, cL_SUBFR)

		azOff += cMP1
	}

	// Carry the last M samples as history for the next frame.
	copy(st.synthBuf[:cM], synWork[cL_FRAME-cM:cL_FRAME])
}
