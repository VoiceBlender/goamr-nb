package amrnb

// AMR-NB decoder frame driver, ported from opencore-amrnb dec_amr.cpp
// (Decoder_amr) and sp_dec.cpp (GSMFrameDecode). It orchestrates the
// already-ported building blocks for one 20 ms frame: LSF dequantization and
// LSP interpolation, then four subframes of pitch-lag decode, adaptive
// codebook, fixed codebook with pitch sharpening, gain decode, excitation
// assembly and LP synthesis; finally the formant post-filter and output
// high-pass.
//
// This implements the active-speech (good-frame) path. Error concealment
// (bad-frame / DTX), background-noise adaptation, the gain-averaging
// (Cb_gain_average) and adaptive phase dispersion are not yet ported, so the
// excitation is synthesized directly from the total excitation (equivalent to
// phase dispersion disabled) and the code gain is used unmixed.

const (
	cSHARPMAX      = 13017                  // max pitch sharpening
	cExcOff        = cPIT_MAX + cL_INTERPOL // 154: excitation history length
	cPIT_MIN_MR122 = 18
)

// decoderStateDSP holds the persistent decoder synthesis state across frames.
type decoderStateDSP struct {
	oldExc [cL_SUBFR + cPIT_MAX + cL_INTERPOL]int16 // excitation w/ history
	lspOld [cM]int16                                // previous-frame LSPs
	memSyn [cM]int16                                // synthesis filter memory
	sharp  int16                                    // pitch sharpening (prev gain_pit)
	oldT0  int16                                    // previous integer pitch lag
	lsf    dPlsfState                               // LSF predictor state
	pred   gcPredState                              // gain predictor state
	pst    postFilterState                          // formant post-filter state
	post   postProcessState                         // output HP filter state
}

func (st *decoderStateDSP) reset() {
	for i := range st.oldExc {
		st.oldExc[i] = 0
	}
	copy(st.lspOld[:], lsp_init_data[:cM])
	for i := range st.memSyn {
		st.memSyn[i] = 0
	}
	st.sharp = 0
	st.oldT0 = 40
	st.lsf.reset()
	st.pred.reset()
	st.pst.reset()
	st.post = postProcessState{}
}

// sharpenCode applies pitch sharpening to the innovation: code[i] += pitSharp ·
// code[i-T0] for i in [T0, L_SUBFR).
func sharpenCode(code []int16, T0 int, pitSharp int16) {
	for i := T0; i < cL_SUBFR; i++ {
		code[i] = add(code[i], mult(code[i-T0], pitSharp))
	}
}

// decodeAmr decodes one frame's parameters into 160 raw synthesis samples and
// returns the four per-subframe LP coefficient sets for the post-filter.
func (st *decoderStateDSP) decodeAmr(mode int, params []int16, synth []int16) []int16 {
	var lspMid, lspNew [cM]int16
	at := make([]int16, cNB_SUBFR*cMP1)

	pi := 0
	if mode == dMR122 {
		st.lsf.dPlsf5(0, params[pi:pi+5], lspMid[:], lspNew[:])
		pi += 5
		intLpc1and3(st.lspOld[:], lspMid[:], lspNew[:], at)
	} else {
		st.lsf.dPlsf3(mode, 0, params[pi:pi+3], lspNew[:])
		pi += 3
		intLpc1to3(st.lspOld[:], lspNew[:], at)
	}
	copy(st.lspOld[:], lspNew[:])

	var code [cL_SUBFR]int16
	exc := st.oldExc[cExcOff:] // current-subframe excitation window (40 samples)

	azOff := 0
	subfrNr := -1
	evenSubfr := int16(0)
	var indexMR475 int16 // carried across the MR475 subframe pair

	for iSubfr := 0; iSubfr < cL_FRAME; iSubfr += cL_SUBFR {
		subfrNr++
		evenSubfr = 1 - evenSubfr
		az := at[azOff:]

		pitFlag := int16(iSubfr)
		if iSubfr == cL_FRAME_BY2 && mode != dMR475 && mode != dMR515 {
			pitFlag = 0
		}

		// ---- pitch lag + adaptive codebook ----
		index := params[pi]
		pi++
		var T0, T0frac int16
		if mode != dMR122 {
			flag4 := int16(0)
			if mode == dMR475 || mode == dMR515 || mode == dMR59 || mode == dMR67 {
				flag4 = 1
			}
			deltaLow, deltaRange := int16(5), int16(9)
			if mode == dMR795 {
				deltaLow, deltaRange = 10, 19
			}
			t0min := sub(st.oldT0, deltaLow)
			if t0min < cPIT_MIN {
				t0min = cPIT_MIN
			}
			t0max := add(t0min, deltaRange)
			if t0max > cPIT_MAX {
				t0max = cPIT_MAX
				t0min = sub(t0max, deltaRange)
			}
			T0, T0frac = decLag3(index, t0min, t0max, pitFlag, st.oldT0, flag4)
			predLt(st.oldExc[:], cExcOff, T0, T0frac, cL_SUBFR, 1)
		} else {
			T0, T0frac = decLag6(index, cPIT_MIN_MR122, cPIT_MAX, pitFlag, st.oldT0)
			predLt(st.oldExc[:], cExcOff, T0, T0frac, cL_SUBFR, 0)
		}

		// ---- fixed codebook + pitch sharpening ----
		var gainPit, gainCode int16
		switch mode {
		case dMR475, dMR515:
			posIdx := params[pi]
			pi++
			signs := params[pi]
			pi++
			decode2i40_9bits(int16(subfrNr), signs, posIdx, code[:])
			sharpenCode(code[:], int(T0), shl(st.sharp, 1))
		case dMR59:
			posIdx := params[pi]
			pi++
			signs := params[pi]
			pi++
			decode2i40_11bits(signs, posIdx, code[:])
			sharpenCode(code[:], int(T0), shl(st.sharp, 1))
		case dMR67:
			posIdx := params[pi]
			pi++
			signs := params[pi]
			pi++
			decode3i40_14bits(signs, posIdx, code[:])
			sharpenCode(code[:], int(T0), shl(st.sharp, 1))
		case dMR74, dMR795:
			posIdx := params[pi]
			pi++
			signs := params[pi]
			pi++
			decode4i40_17bits(signs, posIdx, code[:])
			sharpenCode(code[:], int(T0), shl(st.sharp, 1))
		case dMR102:
			dec8i40_31bits(params[pi:pi+7], code[:])
			pi += 7
			sharpenCode(code[:], int(T0), shl(st.sharp, 1))
		case dMR122:
			gpIdx := params[pi]
			pi++
			gainPit = dGainPitch(mode, gpIdx)
			dec10i40_35bits(params[pi:pi+10], code[:])
			pi += 10
			sharpenCode(code[:], int(T0), shl(gainPit, 1))
		}

		// ---- gain decode ----
		switch mode {
		case dMR475:
			if evenSubfr != 0 {
				indexMR475 = params[pi]
				pi++
			}
			gainPit, gainCode = st.pred.decGain(mode, indexMR475, code[:], evenSubfr)
		case dMR515, dMR59, dMR67, dMR74, dMR102:
			idx := params[pi]
			pi++
			gainPit, gainCode = st.pred.decGain(mode, idx, code[:], evenSubfr)
		case dMR795:
			gpIdx := params[pi]
			pi++
			gainPit = dGainPitch(mode, gpIdx)
			gcIdx := params[pi]
			pi++
			gainCode = st.pred.dGainCode(mode, gcIdx, code[:])
		case dMR122:
			gcIdx := params[pi]
			pi++
			gainCode = st.pred.dGainCode(mode, gcIdx, code[:])
		}

		// store pitch sharpening for next subframe (not in MR475 even subframe)
		if mode != dMR475 || evenSubfr == 0 {
			st.sharp = gainPit
			if st.sharp > cSHARPMAX {
				st.sharp = cSHARPMAX
			}
		}

		// ---- total excitation: gain_pit·adaptive + gain_code·code ----
		var pitchFac, tmpShift int16
		if mode <= dMR102 {
			pitchFac = gainPit
			tmpShift = 1
		} else {
			pitchFac = shr(gainPit, 1)
			tmpShift = 2
		}
		for i := 0; i < cL_SUBFR; i++ {
			L := L_mult(exc[i], pitchFac)
			L = L_mac(L, code[i], gainCode)
			L = L_shl(L, tmpShift)
			exc[i] = round_(L)
		}

		// ---- LP synthesis ----
		synFilt(az, exc[:cL_SUBFR], synth[iSubfr:iSubfr+cL_SUBFR], cL_SUBFR, st.memSyn[:], false)
		copy(st.memSyn[:], synth[iSubfr+cL_SUBFR-cM:iSubfr+cL_SUBFR])

		// slide the excitation buffer left by one subframe
		copy(st.oldExc[:cExcOff], st.oldExc[cL_SUBFR:cL_SUBFR+cExcOff])

		st.oldT0 = T0
		azOff += cMP1
	}

	return at
}

// decodeSpeechFrame decodes one good speech frame end-to-end into 160 PCM
// samples: parameter decode, post-filter, output high-pass, and the 13-bit
// output truncation. Mirrors GSMFrameDecode for the speech path.
func (st *decoderStateDSP) decodeSpeechFrame(mode int, params []int16, synth []int16) {
	az := st.decodeAmr(mode, params, synth)
	st.pst.postFilter(mode, synth, az)
	st.post.process(synth, cL_FRAME)
	for i := 0; i < cL_FRAME; i++ {
		synth[i] &^= 7 // truncate to 13 bits
	}
}
