package amrnb

// Main encoder driver, ported from opencore-amrnb cod_amr.cpp (cod_amr) and the
// per-frame wrapper sp_enc.cpp (Speech_Encode_Frame). It ties together the LP
// analysis (lpc), LSP quantization (lsp), perceptual weighting + open-loop pitch
// (pre_big/ol_ltp), and the four-subframe analysis-by-synthesis loop
// (subframePreProc -> clLtp -> cbsearch -> gainQuant -> subframePostProc),
// producing the parameter array consumed by prm2bits/packFrame.
//
// Scope: the active-speech path for all eight modes — MR515/MR59/MR67/MR74/MR102
// via Qua_gain, MR122 via q_gain_code, MR475 via the joint two-subframe VQ
// (mr475Loop), and MR795 via the gain-adapted quantizer. MR102 uses the weighted
// open-loop pitch (pitchOlWgh). DTX/VAD is always-active. All eight modes are
// bit-exact with the opencore-amrnb reference encoder.

// buffer offsets within oldSpeech (length cL_TOTAL).
const (
	cSpeechOff  = cL_TOTAL - cL_FRAME - cL_NEXT // 120: present frame start
	cPWindowOff = cL_TOTAL - cL_WINDOW          // 80:  LPC analysis window
	cPWin122Off = cPWindowOff - cL_NEXT         // 40:  EFR (MR122) window, no lookahead
	cNewSpchOff = cL_TOTAL - cL_FRAME           // 160: new speech
)

// coderState holds the persistent encoder analysis state across frames,
// mirroring cod_amrState plus the Pre_Process state.
type coderState struct {
	oldSpeech [cL_TOTAL]int16            // [history | present | lookahead]
	oldWsp    [cPIT_MAX + cL_FRAME]int16 // weighted speech with pitch history
	oldExc    [cExcOff + cL_FRAME]int16  // excitation with pitch+interp history

	pre     preProcessState
	lspS    lspEncState
	pred    gcPredState
	predUnq gcPredState    // "unquantized" gain predictor (MR475 only)
	adapt   gainAdaptState // gain adaptor (MR795 only)
	pit     pitchFrState
	pitWght pitchOLWghtState // weighted open-loop pitch (MR102 only)
	lev     levinsonState

	oldLags   [5]int16  // closed-loop lag history (MR102 weighted OLP)
	olGainFlg [2]int16  // per-half-frame open-loop gain flag (MR102)
	prmBuf    [64]int16 // reused parameter array (max params = 57, MR122)

	memSyn [cM]int16
	memW0  [cM]int16
	memErr [cM]int16
	memW   [cM]int16 // weighting-filter memory for pre_big
	sharp  int16

	// MR475 joint-pair quantizer state carried from the even to the odd subframe
	sf0ExpGcode0, sf0FracGcode0     int16
	sf0FracCoeff, sf0ExpCoeff       [5]int16
	sf0ExpTargetEn, sf0FracTargetEn int16
}

func (st *coderState) reset() {
	*st = coderState{}
	st.lspS.reset()
	st.pred.reset()
	st.pit.reset()
	st.pitWght.reset()
	for i := range st.oldLags {
		st.oldLags[i] = 40
	}
	st.sharp = 0 // SHARPMIN
}

// lpcAnalysis runs the windowed autocorrelation + Levinson chain, filling the
// per-window LP filters in az: az[3*MP1] (A_new) always, and az[MP1] (A_mid) for
// MR122. Mirrors lpc.
func (st *coderState) lpcAnalysis(mode int, az []int16) {
	var rh, rl [cMP1]int16
	var rc [4]int16
	if mode == dMR122 {
		autocorr(st.oldSpeech[cPWin122Off:], cM, rh[:], rl[:], window_160_80)
		lagWindow(cM, rh[:], rl[:])
		st.lev.levinson(rh[:], rl[:], az[cMP1:], rc[:])

		autocorr(st.oldSpeech[cPWin122Off:], cM, rh[:], rl[:], window_232_8)
		lagWindow(cM, rh[:], rl[:])
		st.lev.levinson(rh[:], rl[:], az[3*cMP1:], rc[:])
		return
	}
	autocorr(st.oldSpeech[cPWindowOff:], cM, rh[:], rl[:], window_200_40)
	lagWindow(cM, rh[:], rl[:])
	st.lev.levinson(rh[:], rl[:], az[3*cMP1:], rc[:])
}

// preBig computes the perceptually weighted speech wsp for the two subframes of
// one half-frame (frameOffset 0 or L_FRAME_BY2), updating the weighting-filter
// memory memW. Mirrors pre_big.
func (st *coderState) preBig(mode, frameOffset int, az []int16) {
	g1 := gamma1[:]
	if mode > dMR795 { // MR102, MR122
		g1 = gamma1_12k2[:]
	}
	aOffset := 0
	if frameOffset > 0 {
		aOffset = cMP1 * 2
	}
	fo := frameOffset
	for i := 0; i < 2; i++ {
		var ap1, ap2 [cMP1]int16
		weightAi(az[aOffset:], g1, ap1[:])
		weightAi(az[aOffset:], gamma2[:], ap2[:])
		residu(ap1[:], st.oldSpeech[:], cSpeechOff+fo, cL_SUBFR, st.oldWsp[cPIT_MAX+fo:])
		w := st.oldWsp[cPIT_MAX+fo : cPIT_MAX+fo+cL_SUBFR]
		synFilt(ap2[:], w, w, cL_SUBFR, st.memW[:], true)
		aOffset += cMP1
		fo += cL_SUBFR
	}
}

// gainQuant quantizes the subframe gains for the non-MR475 modes, writing the
// quantization index/indices into prm, returning the quantized gains and the
// number of indices written, and advancing the predictor. res/exc are the LP
// residual and adaptive-codebook excitation (used only by MR795). Mirrors the
// non-MR475 branch of gainQuant.
func (st *coderState) gainQuant(mode int, r clLtpResult, xn, xn2, y1, y2, code, res, exc, prm []int16) (gainPit, gainCode int16, nIdx int) {
	gp := st.pred.gcPred(mode, code)
	var qe122, qe int16
	switch {
	case mode == dMR122:
		gainPit = r.gainPit // already quantized in cl_ltp
		gainCode = gCode(xn2, y2)
		var idx int16
		idx, qe122, qe = qGainCode(mode, gp.expGcode0, gp.fracGcode0, &gainCode)
		prm[0] = idx
		nIdx = 1
	case mode == dMR795:
		fc, ec, cgFrac, cgExp := calcFiltEnergies(mode, xn, xn2, y1, y2, r.gCoeff[:])
		gainPit = r.gainPit
		var gpIdx, gcIdx int16
		gainCode, qe122, qe, gpIdx, gcIdx = st.mr795GainQuant(res, exc, code, fc[:], ec[:],
			gp.expEn, gp.fracEn, gp.expGcode0, gp.fracGcode0, cgFrac, cgExp, r.gpLimit, &gainPit)
		prm[0], prm[1] = gpIdx, gcIdx
		nIdx = 2
	default:
		fc, ec, _, _ := calcFiltEnergies(mode, xn, xn2, y1, y2, r.gCoeff[:])
		var idx int16
		idx, gainPit, gainCode, qe122, qe = quaGain(mode, gp.expGcode0, gp.fracGcode0, fc[:], ec[:], r.gpLimit)
		prm[0] = idx
		nIdx = 1
	}
	st.pred.update(qe122, qe)
	return
}

// codAmr encodes one pre-processed 160-sample frame, returning the parameter
// array (LSF indices followed by the per-subframe pitch/codebook/gain indices,
// in prm2bits order). Mirrors cod_amr for the supported modes.
func (st *coderState) codAmr(mode int, newSpeech []int16) ([]int16, error) {
	copy(st.oldSpeech[cNewSpchOff:], newSpeech[:cL_FRAME])

	az := make([]int16, cNB_SUBFR*cMP1)
	azQ := make([]int16, cNB_SUBFR*cMP1)
	st.lpcAnalysis(mode, az)

	prm := st.prmBuf[:len(bitno[Mode(mode)])] // reused; caller serializes it before the next frame
	ap := st.lspS.lsp(mode, az, azQ, prm)

	st.preBig(mode, 0, az)
	st.preBig(mode, cL_FRAME_BY2, az)
	var Top [2]int16
	if mode == dMR475 || mode == dMR515 {
		Top[0] = pitchOl(mode, st.oldWsp[:], cPIT_MAX, cPIT_MIN, cPIT_MAX, cL_FRAME)
		Top[1] = Top[0]
	} else {
		Top[0] = st.olPitch(mode, 0)
		Top[1] = st.olPitch(mode, cL_FRAME_BY2)
	}

	synth := make([]int16, cL_FRAME)
	if mode == dMR475 {
		st.mr475Loop(az, azQ, Top[:], prm, ap, synth)
		copy(st.oldExc[:cExcOff], st.oldExc[cL_FRAME:cL_FRAME+cExcOff])
		copy(st.oldWsp[:cPIT_MAX], st.oldWsp[cL_FRAME:cL_FRAME+cPIT_MAX])
		copy(st.oldSpeech[:cL_TOTAL-cL_FRAME], st.oldSpeech[cL_FRAME:cL_TOTAL])
		return prm, nil
	}
	for subfrNr := 0; subfrNr < cNB_SUBFR; subfrNr++ {
		iSubfr := subfrNr * cL_SUBFR
		aSub := az[subfrNr*cMP1:]
		aqSub := azQ[subfrNr*cMP1:]
		excBase := cExcOff + iSubfr

		xn := make([]int16, cL_SUBFR)
		res := make([]int16, cL_SUBFR)
		errSig := make([]int16, cL_SUBFR)
		h1 := make([]int16, cL_SUBFR)
		subframePreProc(mode, aSub, aqSub, st.oldSpeech[:], cSpeechOff+iSubfr,
			st.memErr[:], st.memW0[:], st.oldExc[excBase:], h1, xn, res, errSig)

		res2 := make([]int16, cL_SUBFR)
		copy(res2, res)

		xn2 := make([]int16, cL_SUBFR)
		y1 := make([]int16, cL_SUBFR)
		r := st.pit.clLtp(mode, iSubfr, Top[:], h1, st.oldExc[:], excBase, res2, xn, xn2, y1)
		// MR102: feed the first/last closed-loop lag back into the OLP history
		if subfrNr == 0 && st.olGainFlg[0] > 0 {
			st.oldLags[1] = r.T0
		} else if subfrNr == 3 && st.olGainFlg[1] > 0 {
			st.oldLags[0] = r.T0
		}
		prm[ap] = r.pitchIndex
		ap++
		if r.hasGainPitIndex {
			prm[ap] = r.gainPitIndex
			ap++
		}

		code := make([]int16, cL_SUBFR)
		y2 := make([]int16, cL_SUBFR)
		ncb := cbsearch(xn2, h1, r.T0, st.sharp, r.gainPit, res2, code, y2, mode, subfrNr, prm[ap:])
		ap += ncb

		gainPit, gainCode, ng := st.gainQuant(mode, r, xn, xn2, y1, y2, code, res, st.oldExc[excBase:], prm[ap:])
		ap += ng

		subframePostProc(st.oldSpeech[:], cSpeechOff, mode, iSubfr, gainPit, gainCode,
			aqSub, synth, xn, code, y1, y2, st.memSyn[:], st.memErr[:], st.memW0[:],
			st.oldExc[:], cExcOff, &st.sharp)
	}

	// slide the analysis buffers left by one frame
	copy(st.oldExc[:cExcOff], st.oldExc[cL_FRAME:cL_FRAME+cExcOff])
	copy(st.oldWsp[:cPIT_MAX], st.oldWsp[cL_FRAME:cL_FRAME+cPIT_MAX])
	copy(st.oldSpeech[:cL_TOTAL-cL_FRAME], st.oldSpeech[cL_FRAME:cL_TOTAL])

	return prm, nil
}

// mr475Loop runs the four-subframe analysis-by-synthesis loop for MR475, whose
// gains are quantized two subframes at a time. Each even subframe is processed
// tentatively (storing its energy terms and an unquantized synthesis), and at
// the following odd subframe the joint VQ chooses one index for the pair; both
// subframes' final syntheses are then redone with the reconstructed gains.
// Mirrors the MR475 branch of cod_amr.
func (st *coderState) mr475Loop(az, azQ, Top, prm []int16, ap int, synth []int16) {
	const mode = dMR475
	var memSynSave, memW0Save, memErrSave [cM]int16
	var h1Sf0, xnSf0, y2Sf0, codeSf0 [cL_SUBFR]int16
	var sharpSave, gainPitSf0, gainCodeSf0, T0Sf0, T0fracSf0 int16
	var iSubfrSf0, gainIdxPos int

	for subfrNr := 0; subfrNr < cNB_SUBFR; subfrNr++ {
		iSubfr := subfrNr * cL_SUBFR
		evenSubfr := (subfrNr & 1) == 0
		aSub := az[subfrNr*cMP1:]
		aqSub := azQ[subfrNr*cMP1:]
		excBase := cExcOff + iSubfr

		if evenSubfr {
			memSynSave = st.memSyn
			memW0Save = st.memW0
			memErrSave = st.memErr
			sharpSave = st.sharp
		}

		// Both subframes of the pair build their target from memW0Save so the
		// even subframe's tentative synthesis carries into the odd one.
		xn := make([]int16, cL_SUBFR)
		res := make([]int16, cL_SUBFR)
		errSig := make([]int16, cL_SUBFR)
		h1 := make([]int16, cL_SUBFR)
		subframePreProc(mode, aSub, aqSub, st.oldSpeech[:], cSpeechOff+iSubfr,
			st.memErr[:], memW0Save[:], st.oldExc[excBase:], h1, xn, res, errSig)
		if evenSubfr {
			copy(h1Sf0[:], h1)
		}
		res2 := make([]int16, cL_SUBFR)
		copy(res2, res)

		xn2 := make([]int16, cL_SUBFR)
		y1 := make([]int16, cL_SUBFR)
		r := st.pit.clLtp(mode, iSubfr, Top, h1, st.oldExc[:], excBase, res2, xn, xn2, y1)
		prm[ap] = r.pitchIndex
		ap++

		code := make([]int16, cL_SUBFR)
		y2 := make([]int16, cL_SUBFR)
		ap += cbsearch(xn2, h1, r.T0, st.sharp, r.gainPit, res2, code, y2, mode, subfrNr, prm[ap:])

		if evenSubfr {
			gainIdxPos = ap // reserve the joint gain index slot
			ap++

			st.predUnq = st.pred // gc_pred_copy
			gpu := st.predUnq.gcPred(mode, code)
			st.sf0ExpGcode0, st.sf0FracGcode0 = gpu.expGcode0, gpu.fracGcode0
			fc, ec, cgFrac, cgExp := calcFiltEnergies(mode, xn, xn2, y1, y2, r.gCoeff[:])
			st.sf0FracCoeff, st.sf0ExpCoeff = fc, ec
			st.sf0ExpTargetEn, st.sf0FracTargetEn = calcTargetEnergy(xn)
			st.predUnq.mr475UpdateUnqPred(st.sf0ExpGcode0, st.sf0FracGcode0, cgExp, cgFrac)

			// tentative synthesis with the unquantized optimum gains
			gainCode := shl(cgFrac, cgExp+1)
			iSubfrSf0 = iSubfr
			copy(xnSf0[:], xn)
			copy(y2Sf0[:], y2)
			copy(codeSf0[:], code)
			T0Sf0, T0fracSf0 = r.T0, r.T0frac
			subframePostProc(st.oldSpeech[:], cSpeechOff, mode, iSubfr, r.gainPit, gainCode,
				aqSub, synth, xn, code, y1, y2, memSynSave[:], st.memErr[:], memW0Save[:],
				st.oldExc[:], cExcOff, &st.sharp)
			st.sharp = sharpSave
			continue
		}

		// odd subframe: joint gain quantization for the pair
		gpu := st.predUnq.gcPred(mode, code)
		fc, ec, _, _ := calcFiltEnergies(mode, xn, xn2, y1, y2, r.gCoeff[:])
		tEnExp, tEnFrac := calcTargetEnergy(xn)
		idx, sf0gp, sf0gc, sf1gp, sf1gc := st.mr475GainQuant(code, gpu.expGcode0, gpu.fracGcode0,
			ec[:], fc[:], tEnExp, tEnFrac, r.gpLimit)
		prm[gainIdxPos] = idx
		gainPitSf0, gainCodeSf0 = sf0gp, sf0gc

		// redo the even subframe's final synthesis with the quantized gains
		st.memErr = memErrSave
		predLt(st.oldExc[:], cExcOff+iSubfrSf0, T0Sf0, T0fracSf0, cL_SUBFR, 1)
		convolve(st.oldExc[cExcOff+iSubfrSf0:], h1Sf0[:], y1, cL_SUBFR)
		subframePostProc(st.oldSpeech[:], cSpeechOff, mode, iSubfrSf0, gainPitSf0, gainCodeSf0,
			azQ[(subfrNr-1)*cMP1:], synth, xnSf0[:], codeSf0[:], y1, y2Sf0[:],
			st.memSyn[:], st.memErr[:], st.memW0[:], st.oldExc[:], cExcOff, &sharpSave)

		// redo the odd subframe's analysis + synthesis with the final memories
		subframePreProc(mode, aSub, aqSub, st.oldSpeech[:], cSpeechOff+iSubfr,
			st.memErr[:], st.memW0[:], st.oldExc[excBase:], h1, xn, res, errSig)
		predLt(st.oldExc[:], excBase, r.T0, r.T0frac, cL_SUBFR, 1)
		convolve(st.oldExc[excBase:], h1, y1, cL_SUBFR)
		subframePostProc(st.oldSpeech[:], cSpeechOff, mode, iSubfr, sf1gp, sf1gc,
			aqSub, synth, xn, code, y1, y2, st.memSyn[:], st.memErr[:], st.memW0[:],
			st.oldExc[:], cExcOff, &st.sharp)
	}
}

// olPitch runs the open-loop pitch over the weighted-speech buffer for the
// half-frame at frameOffset.
func (st *coderState) olPitch(mode, frameOffset int) int16 {
	idx := 0
	if frameOffset != 0 {
		idx = 1
	}
	if mode == dMR102 {
		// weighted open-loop pitch with median/gain-flag adaptation
		return st.pitWght.pitchOlWgh(st.oldWsp[:], cPIT_MAX+frameOffset, cPIT_MIN, cPIT_MAX,
			cL_FRAME_BY2, st.oldLags[:], st.olGainFlg[:], idx)
	}
	pitMin := int16(cPIT_MIN)
	if mode == dMR122 {
		pitMin = cPIT_MIN_MR122
	}
	return pitchOl(mode, st.oldWsp[:], cPIT_MAX+frameOffset, int(pitMin), cPIT_MAX, cL_FRAME_BY2)
}
