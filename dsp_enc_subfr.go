package amrnb

// Per-subframe codebook dispatch and post-processing, ported from opencore-amrnb
// cbsearch.cpp (cbsearch) and spstproc.cpp (subframePostProc). cbsearch routes
// to the mode's algebraic codebook search (and applies the MR102/MR122 pitch
// sharpening around it); subframePostProc forms the total excitation, runs the
// local synthesis, and updates the synthesis/error/weighting filter memories
// for the next subframe. Bit-exact with the reference.

// cbsearch runs the fixed-codebook search for the mode, writing the innovation
// code and filtered code y and the analysis indices into anap (returning the
// count). Mirrors cbsearch.
func cbsearch(x, h []int16, T0, pitchSharp, gainPit int16, res2, code, y []int16, mode, subNr int, anap []int16) int {
	switch {
	case mode == dMR475 || mode == dMR515:
		idx, sgn := code2i40_9bits(int16(subNr), x, h, T0, pitchSharp, code, y)
		anap[0], anap[1] = idx, sgn
		return 2
	case mode == dMR59:
		idx, sgn := code2i40_11bits(x, h, T0, pitchSharp, code, y)
		anap[0], anap[1] = idx, sgn
		return 2
	case mode == dMR67:
		idx, sgn := code3i40_14bits(x, h, T0, pitchSharp, code, y)
		anap[0], anap[1] = idx, sgn
		return 2
	case mode == dMR74 || mode == dMR795:
		idx, sgn := code4i40_17bits(x, h, T0, pitchSharp, code, y)
		anap[0], anap[1] = idx, sgn
		return 2
	case mode == dMR102:
		ps := shl(pitchSharp, 1)
		for i := T0; i < cL_SUBFR; i++ {
			h[i] = add(h[i], mult(h[i-T0], ps))
		}
		code8i40_31bits(x, res2, h, code, y, anap[0:7])
		for i := T0; i < cL_SUBFR; i++ {
			code[i] = add(code[i], mult(code[i-T0], ps))
		}
		return 7
	default: // MR122
		ps := shl(gainPit, 1)
		for i := T0; i < cL_SUBFR; i++ {
			h[i] = add(h[i], int16((int32(h[i-T0])*int32(ps))>>15))
		}
		code10i40_35bits(x, res2, h, code, y, anap[0:10])
		for i := T0; i < cL_SUBFR; i++ {
			code[i] = add(code[i], mult(code[i-T0], ps))
		}
		return 10
	}
}

// subframePostProc builds the total excitation (gain_pit·adaptive +
// gain_code·code), runs the local synthesis (updating mem_syn), and refreshes
// the error and weighting-filter memories. sharp is updated with the clamped
// pitch gain. Mirrors subframePostProc.
func subframePostProc(speech []int16, speechOff, mode, iSubfr int, gainPit, gainCode int16, aq, synth, xn, code, y1, y2, memSyn, memErr, memW0, exc []int16, excOff int, sharp *int16) {
	var tempShift, kShift, pitchFac int16
	if mode != dMR122 {
		tempShift, kShift, pitchFac = 1, 16-2-1, gainPit
	} else {
		tempShift, kShift, pitchFac = 2, 16-4-1, gainPit>>1
	}

	if gainPit < cSHARPMAX {
		*sharp = gainPit
	} else {
		*sharp = cSHARPMAX
	}

	base := excOff + iSubfr
	for i := 0; i < cL_SUBFR; i++ {
		L := (int32(exc[base+i]) * int32(pitchFac)) << 1
		L += (int32(code[i]) * int32(gainCode)) << 1
		L <<= uint(tempShift)
		exc[base+i] = int16((L + 0x8000) >> 16)
	}

	synFilt(aq, exc[base:base+cL_SUBFR], synth[iSubfr:iSubfr+cL_SUBFR], cL_SUBFR, memSyn, true)

	for i, j := cL_SUBFR-cM, 0; i < cL_SUBFR; i, j = i+1, j+1 {
		memErr[j] = speech[speechOff+iSubfr+i] - synth[iSubfr+i]
		temp := int16((int32(y1[i]) * int32(gainPit)) >> 14)
		temp += int16((int32(y2[i]) * int32(gainCode)) >> uint(kShift))
		memW0[j] = xn[i] - temp
	}
}
