package amrnb

// AMR-NB decoder frame driver, ported from opencore-amrnb dec_amr.cpp
// (Decoder_amr, Decoder_amr_reset) and sp_dec.cpp (GSMFrameDecode). Per 20 ms
// frame it runs either comfort-noise generation (DTX) or speech decoding:
// LSF dequantization and interpolation, then four subframes of pitch lag,
// adaptive codebook, fixed codebook with pitch sharpening, gains (concealed
// for bad frames), code-gain averaging, phase dispersion, excitation control
// and LP synthesis; then background-noise detection and the DTX history
// update. GSMFrameDecode adds the post-filter, the output high-pass and the
// 13-bit truncation.

const (
	cSHARPMAX      = 13017                  // max pitch sharpening
	cExcOff        = cPIT_MAX + cL_INTERPOL // 154: excitation history length
	cPIT_MIN_MR122 = 18
)

// decoderStateDSP holds the decoder state across frames. Mirrors
// Decoder_amrState.
type decoderStateDSP struct {
	oldExc            [cL_SUBFR + cPIT_MAX + cL_INTERPOL]int16 // excitation w/ history
	lspOld            [cM]int16
	memSyn            [cM]int16
	sharp             int16
	oldT0             int16
	prevBF            int16
	prevPDF           int16
	state             int16 // concealment state 0..6
	excEnergyHist     [cEXC_ENERGY_HIST]int16
	T0LagBuff         int16
	inBackgroundNoise int16
	voicedHangover    int16
	ltpGainHistory    [cLTP_GAIN_HIST_LEN]int16
	bgn               bgnScdState
	nodataSeed        int16
	cbAver            cbGainAverageState
	lspMeanSave       [cM]int16 // lsp_avg state
	lsf               dPlsfState
	ecP               ecGainPitchState
	ecC               ecGainCodeState
	pred              gcPredState
	phd               phDispState
	dtx               dtxDecState
}

// reset mirrors Decoder_amr_reset. A DTX reset (dtx true) keeps the
// synthesis memory, the LSPs, the LSF and energy averages, the gain predictor
// and the comfort-noise state.
func (st *decoderStateDSP) reset(dtx bool) {
	for i := 0; i < cExcOff; i++ {
		st.oldExc[i] = 0
	}
	if !dtx {
		st.memSyn = [cM]int16{}
		st.lspOld = dtxInitLsp
		st.excEnergyHist = [cEXC_ENERGY_HIST]int16{}
		copy(st.lspMeanSave[:], mean_lsf_5[:cM])
		st.pred.reset()
		st.dtx.reset()
	}
	st.sharp = 0
	st.oldT0 = 40
	st.prevBF = 0
	st.prevPDF = 0
	st.state = 0
	st.T0LagBuff = 40
	st.inBackgroundNoise = 0
	st.voicedHangover = 0
	st.ltpGainHistory = [cLTP_GAIN_HIST_LEN]int16{}
	st.cbAver.reset()
	st.lsf.reset()
	st.ecP.reset()
	st.ecC.reset()
	st.bgn.reset()
	st.nodataSeed = 21845
	st.phd.reset()
}

// decodeAmr decodes one frame of parameters parm into 160 synthesis samples
// and the four subframe LP filters at (4 x 11). Mirrors Decoder_amr.
func (st *decoderStateDSP) decodeAmr(mode int, parm []int16, frameType int, synth, at []int16) {
	newDTXState := st.dtx.rxDtxHandler(frameType)
	if newDTXState != dtxSpeech {
		st.reset(true)
		st.dtx.decode(st.memSyn[:], &st.lsf, &st.pred, &st.cbAver, newDTXState, mode, parm, synth, at)
		lsfLsp(st.lsf.pastLsfQ[:], st.lspOld[:], cM)
		lspAvg(st.lspMeanSave[:], st.lsf.pastLsfQ[:])
		st.dtx.dtxGlobalState = newDTXState
		return
	}

	var bfi, pdfi int16
	switch frameType {
	case cRX_SPEECH_BAD, cRX_NO_DATA, cRX_ONSET:
		bfi = 1
		if frameType != cRX_SPEECH_BAD {
			buildCNParam(&st.nodataSeed, bitno[mode], parm)
		}
	case cRX_SPEECH_DEGRADED:
		pdfi = 1
	}
	switch {
	case bfi != 0:
		st.state++
	case st.state == 6:
		st.state = 5
	default:
		st.state = 0
	}
	if st.state > 6 {
		st.state = 6
	}
	switch st.dtx.dtxGlobalState {
	case dtxActive:
		st.state = 5
		st.prevBF = 0
	case dtxMute:
		st.state = 5
		st.prevBF = 1
	}

	prevLsf := st.lsf.pastLsfQ
	var lspMid, lspNew [cM]int16
	pi := 0
	if mode != dMR122 {
		st.lsf.dPlsf3(mode, bfi, parm[:3], lspNew[:])
		pi = 3
		intLpc1to3(st.lspOld[:], lspNew[:], at)
	} else {
		st.lsf.dPlsf5(bfi, parm[:5], lspMid[:], lspNew[:])
		pi = 5
		intLpc1and3(st.lspOld[:], lspMid[:], lspNew[:], at)
	}
	st.lspOld = lspNew

	lowMode := mode == dMR475 || mode == dMR515 || mode == dMR59
	exc := st.oldExc[cExcOff : cExcOff+cL_SUBFR]
	var code, excp, excEnhanced, lsfI [cL_SUBFR]int16
	var indexMR475 int16
	var T0, T0frac int16
	evenSubfr := int16(0)
	subfrNr := int16(-1)

	for iSubfr := 0; iSubfr < cL_FRAME; iSubfr += cL_SUBFR {
		subfrNr++
		evenSubfr = 1 - evenSubfr
		az := at[iSubfr/cL_SUBFR*cMP1:]

		pitFlag := int16(iSubfr)
		if iSubfr == cL_FRAME_BY2 && mode != dMR475 && mode != dMR515 {
			pitFlag = 0
		}

		// ---- pitch lag and adaptive codebook ----
		index := parm[pi]
		pi++
		if mode != dMR122 {
			flag4 := int16(0)
			if lowMode || mode == dMR67 {
				flag4 = 1
			}
			deltaLow, deltaRange := int16(5), int16(9)
			if mode == dMR795 {
				deltaLow, deltaRange = 10, 19
			}
			t0min := st.oldT0 - deltaLow
			if t0min < cPIT_MIN {
				t0min = cPIT_MIN
			}
			t0max := t0min + deltaRange
			if t0max > cPIT_MAX {
				t0max = cPIT_MAX
				t0min = t0max - deltaRange
			}
			T0, T0frac = decLag3(index, t0min, t0max, pitFlag, st.oldT0, flag4)
			st.T0LagBuff = T0
			if bfi != 0 {
				if st.oldT0 < cPIT_MAX {
					st.oldT0++
				}
				T0 = st.oldT0
				T0frac = 0
				if st.inBackgroundNoise != 0 && st.voicedHangover > 4 && lowMode {
					T0 = st.T0LagBuff
				}
			}
			predLt(st.oldExc[:], cExcOff, T0, T0frac, cL_SUBFR, 1)
		} else {
			T0, T0frac = decLag6(index, cPIT_MIN_MR122, cPIT_MAX, pitFlag, T0)
			if !(bfi == 0 && (pitFlag == 0 || index < 61)) {
				st.T0LagBuff = T0
				T0 = st.oldT0
				T0frac = 0
			}
			predLt(st.oldExc[:], cExcOff, T0, T0frac, cL_SUBFR, 0)
		}

		// ---- fixed codebook and pitch sharpening ----
		var gainPit, gainCode, pitSharp int16
		switch mode {
		case dMR475, dMR515:
			decode2i40_9bits(subfrNr, parm[pi+1], parm[pi], code[:])
			pi += 2
			pitSharp = shl(st.sharp, 1)
		case dMR59:
			decode2i40_11bits(parm[pi+1], parm[pi], code[:])
			pi += 2
			pitSharp = shl(st.sharp, 1)
		case dMR67:
			decode3i40_14bits(parm[pi+1], parm[pi], code[:])
			pi += 2
			pitSharp = shl(st.sharp, 1)
		case dMR74, dMR795:
			decode4i40_17bits(parm[pi+1], parm[pi], code[:])
			pi += 2
			pitSharp = shl(st.sharp, 1)
		case dMR102:
			dec8i40_31bits(parm[pi:pi+7], code[:])
			pi += 7
			pitSharp = shl(st.sharp, 1)
		default: // MR122
			index = parm[pi]
			pi++
			if bfi != 0 {
				gainPit = st.ecP.conceal(st.state)
			} else {
				gainPit = dGainPitch(mode, index)
			}
			st.ecP.update(bfi, st.prevBF, &gainPit)
			dec10i40_35bits(parm[pi:pi+10], code[:])
			pi += 10
			pitSharp = shl(gainPit, 1)
		}
		sharpenCode(code[:], int(T0), pitSharp)

		// ---- gains ----
		switch {
		case mode == dMR475:
			if evenSubfr != 0 {
				indexMR475 = parm[pi]
				pi++
			}
			if bfi == 0 {
				gainPit, gainCode = st.pred.decGain(mode, indexMR475, code[:], evenSubfr)
			} else {
				gainPit = st.ecP.conceal(st.state)
				gainCode = st.ecC.conceal(&st.pred, st.state)
			}
			st.ecP.update(bfi, st.prevBF, &gainPit)
			st.ecC.update(bfi, st.prevBF, &gainCode)
			pitSharp = min(gainPit, cSHARPMAX)
		case mode <= dMR74 || mode == dMR102:
			index = parm[pi]
			pi++
			if bfi == 0 {
				gainPit, gainCode = st.pred.decGain(mode, index, code[:], evenSubfr)
			} else {
				gainPit = st.ecP.conceal(st.state)
				gainCode = st.ecC.conceal(&st.pred, st.state)
			}
			st.ecP.update(bfi, st.prevBF, &gainPit)
			st.ecC.update(bfi, st.prevBF, &gainCode)
			pitSharp = min(gainPit, cSHARPMAX)
			if mode == dMR102 && st.oldT0 > cL_SUBFR+5 {
				pitSharp >>= 2
			}
		case mode == dMR795:
			index = parm[pi]
			pi++
			if bfi != 0 {
				gainPit = st.ecP.conceal(st.state)
			} else {
				gainPit = dGainPitch(mode, index)
			}
			st.ecP.update(bfi, st.prevBF, &gainPit)
			index = parm[pi]
			pi++
			if bfi == 0 {
				gainCode = st.pred.dGainCode(mode, index, code[:])
			} else {
				gainCode = st.ecC.conceal(&st.pred, st.state)
			}
			st.ecC.update(bfi, st.prevBF, &gainCode)
			pitSharp = min(gainPit, cSHARPMAX)
		default: // MR122
			index = parm[pi]
			pi++
			if bfi == 0 {
				gainCode = st.pred.dGainCode(mode, index, code[:])
			} else {
				gainCode = st.ecC.conceal(&st.pred, st.state)
			}
			st.ecC.update(bfi, st.prevBF, &gainCode)
			pitSharp = gainPit
		}

		// store pitch sharpening for the next subframe (not after the first
		// MR475 subframe of a pair)
		if mode != dMR475 || evenSubfr == 0 {
			st.sharp = min(gainPit, cSHARPMAX)
		}

		// pitch enhancer for strongly voiced subframes
		pitSharp = shl(pitSharp, 1)
		if pitSharp > 16384 {
			for i := 0; i < cL_SUBFR; i++ {
				L := L_mult(mult(exc[i], pitSharp), gainPit)
				if mode == dMR122 {
					L >>= 1
				}
				excp[i] = round_(L)
			}
		}

		if bfi == 0 {
			copy(st.ltpGainHistory[:8], st.ltpGainHistory[1:])
			st.ltpGainHistory[8] = gainPit
		}

		// limit the pitch gain of concealed frames in background noise
		if (st.prevBF != 0 || bfi != 0) && st.inBackgroundNoise != 0 && lowMode {
			if gainPit > 12288 {
				gainPit = (gainPit-12288)>>1 + 12288
			}
			if gainPit > 14745 {
				gainPit = 14745
			}
		}

		// ---- code-gain averaging ----
		intLsf(prevLsf[:], st.lsf.pastLsfQ[:], iSubfr, lsfI[:])
		gainCodeMix := st.cbAver.average(mode, gainCode, lsfI[:cM], st.lspMeanSave[:], bfi, st.prevBF,
			pdfi, st.prevPDF, st.inBackgroundNoise, st.voicedHangover)
		if mode > dMR67 && mode != dMR102 {
			gainCodeMix = gainCode
		}

		// ---- total excitation ----
		var pitchFac, tmpShift int16
		if mode <= dMR102 {
			pitchFac = gainPit
			tmpShift = 1
		} else {
			pitchFac = gainPit >> 1
			tmpShift = 2
		}
		for i := 0; i < cL_SUBFR; i++ {
			excEnhanced[i] = exc[i]
			L := L_mult(exc[i], pitchFac)
			L = L_mac(L, code[i], gainCode)
			exc[i] = round_(L_shl(L, tmpShift))
		}

		// ---- phase dispersion ----
		st.phd.lockFull = 0
		if lowMode && st.voicedHangover > 3 && st.inBackgroundNoise != 0 && bfi != 0 {
			st.phd.lockFull = 1
		}
		st.phd.disperse(mode, excEnhanced[:], gainCodeMix, gainPit, code[:], pitchFac, tmpShift)

		// ---- excitation energy control ----
		var L int32
		for i := 0; i < cL_SUBFR; i++ {
			L = L_mac(L, excEnhanced[i], excEnhanced[i])
		}
		L >>= 1
		L, e := sqrtLExp(L)
		L = L_shr(L, e>>1+15)
		excEnergy := int16(L >> 2)
		if lowMode && st.voicedHangover > 5 && st.inBackgroundNoise != 0 && st.state < 4 &&
			((pdfi != 0 && st.prevPDF != 0) || bfi != 0 || st.prevBF != 0) {
			careful := int16(0)
			if pdfi != 0 && bfi == 0 {
				careful = 1
			}
			exCtrl(excEnhanced[:], excEnergy, st.excEnergyHist[:], st.voicedHangover, st.prevBF, careful)
		}
		if !(st.inBackgroundNoise != 0 && (bfi != 0 || st.prevBF != 0) && st.state < 4) {
			copy(st.excEnergyHist[:8], st.excEnergyHist[1:])
			st.excEnergyHist[8] = excEnergy
		}

		// ---- LP synthesis ----
		// (Syn_filt never reports overflow in opencore-amrnb, so the
		// reference's rescaled re-synthesis is dead code and not ported.)
		out := synth[iSubfr : iSubfr+cL_SUBFR]
		if pitSharp > 16384 {
			for i := 0; i < cL_SUBFR; i++ {
				excp[i] = add(excp[i], excEnhanced[i])
			}
			agc2(excEnhanced[:], excp[:], cL_SUBFR)
			synFilt(az, excp[:], out, cL_SUBFR, st.memSyn[:], true)
		} else {
			synFilt(az, excEnhanced[:], out, cL_SUBFR, st.memSyn[:], true)
		}

		copy(st.oldExc[:cExcOff], st.oldExc[cL_SUBFR:cL_SUBFR+cExcOff])
		st.oldT0 = T0
	}

	st.inBackgroundNoise = st.bgn.detect(st.ltpGainHistory[:], synth, &st.voicedHangover)
	st.dtx.activityUpdate(st.lsf.pastLsfQ[:], synth)
	st.prevBF = bfi
	st.prevPDF = pdfi
	lspAvg(st.lspMeanSave[:], st.lsf.pastLsfQ[:])
	st.dtx.dtxGlobalState = newDTXState
}

// sharpenCode applies pitch sharpening to the innovation: code[i] += pitSharp ·
// code[i-T0] for i in [T0, L_SUBFR).
func sharpenCode(code []int16, T0 int, pitSharp int16) {
	for i := T0; i < cL_SUBFR; i++ {
		code[i] = add(code[i], mult(code[i-T0], pitSharp))
	}
}
