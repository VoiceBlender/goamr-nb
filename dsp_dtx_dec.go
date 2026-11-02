package amrnb

// Decoder side of DTX (comfort noise generation), ported from opencore-amrnb
// dtx_dec.cpp (dtx_dec, dtx_dec_activity_update, rx_dtx_handler), b_cn_cod.cpp
// (pseudonoise, build_CN_code, build_CN_param) and a_refl.cpp.

const (
	cDTX_HIST_SIZE             = 8
	cDTX_MAX_EMPTY_THRESH      = 50
	cDTX_ELAPSED_FRAMES_THRESH = 24 + 7 - 1
	cDTX_HANG_CONST            = 7
	cPN_INITIAL_SEED           = 0x70816958
)

// DTX states of the decoder (enum DTXStateType).
const (
	dtxSpeech = iota
	dtxActive
	dtxMute
)

var lsfHistMeanScale = [cM]int16{20000, 20000, 20000, 20000, 20000, 18000, 16384, 8192, 0, 0}

var dtxLogEnAdjust = [9]int16{-1023, -878, -732, -586, -440, -294, -148, 0, 0}

// dtxDecState is the comfort-noise decoder state. Mirrors dtx_decState.
type dtxDecState struct {
	sinceLastSid       int16
	trueSidPeriodInv   int16
	logEn              int16
	oldLogEn           int16
	pnSeedRx           int32
	lsp                [cM]int16
	lspOld             [cM]int16
	lsfHist            [cM * cDTX_HIST_SIZE]int16
	lsfHistPtr         int16
	lsfHistMean        [cM * cDTX_HIST_SIZE]int16
	logPgMean          int16
	logEnHist          [cDTX_HIST_SIZE]int16
	logEnHistPtr       int16
	logEnAdjust        int16
	dtxHangoverCount   int16
	decAnaElapsedCount int16
	sidFrame           int16
	validData          int16
	dtxHangoverAdded   int16
	dtxGlobalState     int
	dataUpdated        int16
}

var dtxInitLsp = [cM]int16{30000, 26000, 21000, 15000, 8000, 0, -8000, -15000, -21000, -26000}

// reset mirrors dtx_dec_reset.
func (st *dtxDecState) reset() {
	*st = dtxDecState{}
	st.trueSidPeriodInv = 1 << 13
	st.logEn = 3500
	st.oldLogEn = 3500
	st.pnSeedRx = cPN_INITIAL_SEED
	st.lsp = dtxInitLsp
	st.lspOld = dtxInitLsp
	hist := [cM]int16{1384, 2077, 3420, 5108, 6742, 8122, 9863, 11092, 12714, 13701}
	for i := 0; i < cDTX_HIST_SIZE; i++ {
		copy(st.lsfHist[i*cM:], hist[:])
	}
	for i := range st.logEnHist {
		st.logEnHist[i] = st.logEn
	}
	st.dtxHangoverCount = cDTX_HANG_CONST
	st.decAnaElapsedCount = 32767
	st.dtxGlobalState = dtxActive
}

// rxDtxHandler returns the DTX state for a received frame type. Mirrors
// rx_dtx_handler.
func (st *dtxDecState) rxDtxHandler(frameType int) int {
	var newState int
	isSID := frameType == cRX_SID_FIRST || frameType == cRX_SID_UPDATE || frameType == cRX_SID_BAD
	inDTX := st.dtxGlobalState == dtxActive || st.dtxGlobalState == dtxMute
	if isSID || (inDTX && (frameType == cRX_NO_DATA || frameType == cRX_SPEECH_BAD || frameType == cRX_ONSET)) {
		newState = dtxActive
		if st.dtxGlobalState == dtxMute && (frameType == cRX_SID_BAD || frameType == cRX_SID_FIRST ||
			frameType == cRX_ONSET || frameType == cRX_NO_DATA) {
			newState = dtxMute
		}
		st.sinceLastSid++
		if frameType != cRX_SID_UPDATE && st.sinceLastSid > cDTX_MAX_EMPTY_THRESH {
			newState = dtxMute
		}
	} else {
		newState = dtxSpeech
		st.sinceLastSid = 0
	}

	if st.dataUpdated == 0 && frameType == cRX_SID_UPDATE {
		st.decAnaElapsedCount = 0
	}
	st.decAnaElapsedCount = add(st.decAnaElapsedCount, 1)
	st.dtxHangoverAdded = 0

	encState := dtxSpeech
	if isSID || frameType == cRX_ONSET || frameType == cRX_NO_DATA {
		encState = dtxActive
		if frameType == cRX_NO_DATA && newState == dtxSpeech {
			encState = dtxSpeech
		}
	}
	if encState == dtxSpeech {
		st.dtxHangoverCount = cDTX_HANG_CONST
	} else {
		switch {
		case st.decAnaElapsedCount > cDTX_ELAPSED_FRAMES_THRESH:
			st.dtxHangoverAdded = 1
			st.decAnaElapsedCount = 0
			st.dtxHangoverCount = 0
		case st.dtxHangoverCount == 0:
			st.decAnaElapsedCount = 0
		default:
			st.dtxHangoverCount--
		}
	}

	if newState != dtxSpeech {
		st.sidFrame = 0
		st.validData = 0
		switch frameType {
		case cRX_SID_FIRST:
			st.sidFrame = 1
		case cRX_SID_UPDATE:
			st.sidFrame = 1
			st.validData = 1
		case cRX_SID_BAD:
			st.sidFrame = 1
			st.dtxHangoverAdded = 0
		}
	}
	return newState
}

// decode generates one frame of comfort noise into synth[0:160] and writes the
// LP filters used to at (4 x 11). Mirrors dtx_dec.
func (st *dtxDecState) decode(memSyn []int16, lsf *dPlsfState, pred *gcPredState, aver *cbGainAverageState,
	newState, mode int, parm []int16, synth, at []int16) {
	if st.dtxHangoverAdded != 0 && st.sidFrame != 0 {
		// The hangover period was a speech period: use its LSFs and energy as
		// the comfort-noise parameters.
		st.logEnAdjust = dtxLogEnAdjust[mode]
		ptr := st.lsfHistPtr + cM
		if ptr == 80 {
			ptr = 0
		}
		copy(st.lsfHist[ptr:ptr+cM], st.lsfHist[st.lsfHistPtr:st.lsfHistPtr+cM])
		ptr = st.logEnHistPtr + 1
		if ptr == cDTX_HIST_SIZE {
			ptr = 0
		}
		st.logEnHist[ptr] = st.logEnHist[st.logEnHistPtr]

		st.logEn = 0
		var Llsf [cM]int32
		for i := cDTX_HIST_SIZE - 1; i >= 0; i-- {
			st.logEn = add(st.logEn, st.logEnHist[i]>>3)
			for j := cM - 1; j >= 0; j-- {
				Llsf[j] = L_add(Llsf[j], int32(st.lsfHist[i*cM+j]))
			}
		}
		var lsfv [cM]int16
		for j := range lsfv {
			lsfv[j] = int16(Llsf[j] >> 3)
		}
		lsfLsp(lsfv[:], st.lsp[:], cM)
		st.logEn = sub(st.logEn, st.logEnAdjust)

		st.lsfHistMean = st.lsfHist
		for i := cM - 1; i >= 0; i-- {
			var Lmean int32
			for j := 7; j >= 0; j-- {
				Lmean = L_add(Lmean, int32(st.lsfHistMean[i+j*cM]))
			}
			lsfMean := int16(Lmean >> 3)
			for j := 7; j >= 0; j-- {
				v := sub(st.lsfHistMean[i+j*cM], lsfMean)
				v = mult(v, lsfHistMeanScale[i])
				negative := v < 0
				v = abs_s(v)
				if v > 655 {
					v = 655 + (v-655)>>2
				}
				if v > 1310 {
					v = 1310
				}
				if negative {
					v = -v
				}
				st.lsfHistMean[i+j*cM] = v
			}
		}
	}

	if st.sidFrame != 0 {
		st.lspOld = st.lsp
		st.oldLogEn = st.logEn
		if st.validData != 0 { // a SID_UPDATE: decode its parameters
			n := st.sinceLastSid
			st.sinceLastSid = 0
			if n >= 32 {
				n = 32
			}
			if n >= 2 {
				st.trueSidPeriodInv = div_s(1<<10, shl(n, 10))
			} else {
				st.trueSidPeriodInv = 1 << 14
			}
			copy(lsf.pastRQ[:], past_rq_init[int(parm[0])*cM:int(parm[0])*cM+cM]) // Init_D_plsf_3
			lsf.dPlsf3(dMRDTX, 0, parm[1:4], st.lsp[:])
			lsf.pastRQ = [cM]int16{}

			logEnIndex := parm[4] // 6 bits: 0..63
			st.logEn = logEnIndex<<(11-2) - 2560*2
			if logEnIndex == 0 {
				st.logEn = minInt16
			}
			if st.dataUpdated == 0 || st.dtxGlobalState == dtxSpeech {
				st.lspOld = st.lsp
				st.oldLogEn = st.logEn
			}
		}

		// Initialize the gain predictor from the comfort-noise energy.
		maPredInit := st.logEn>>1 - 9000
		if maPredInit > 0 {
			maPredInit = 0
		} else if maPredInit < -14436 {
			maPredInit = -14436
		}
		for i := range pred.pastQuaEn {
			pred.pastQuaEn[i] = maPredInit
		}
		maPredInit = int16((int32(maPredInit) * 5443) >> 15)
		for i := range pred.pastQuaEnMR122 {
			pred.pastQuaEnMR122[i] = maPredInit
		}
	}

	// Smooth the energy adjustment toward this mode's value.
	temp := int16(((int32(dtxLogEnAdjust[mode]) << 5) * 3277) >> 15)
	temp >>= 5
	st.logEnAdjust = add(int16((int32(st.logEnAdjust)*29491)>>15), temp)

	// Interpolate the SID parameters over the SID period.
	intFac := shl(st.sinceLastSid+1, 10)
	intFac = mult(intFac, st.trueSidPeriodInv)
	switch {
	case intFac > 1024:
		intFac = 16384
	case intFac < -2048:
		intFac = minInt16
	default:
		intFac <<= 4
	}
	LlogEnInt := L_mult(intFac, st.logEn)
	var lspInt [cM]int16
	for i := cM - 1; i >= 0; i-- {
		lspInt[i] = mult(intFac, st.lsp[i])
	}
	intFac = 16384 - intFac
	LlogEnInt = L_mac(LlogEnInt, intFac, st.oldLogEn)
	for i := cM - 1; i >= 0; i-- {
		lspInt[i] = shl(add(lspInt[i], mult(intFac, st.lspOld[i])), 1)
	}

	// LSF variability from the hangover-period history.
	lsfVariabFactor := st.logPgMean - 2457
	lsfVariabFactor = 4096 - mult(lsfVariabFactor, 9830)
	switch {
	case lsfVariabFactor > 4095:
		lsfVariabFactor = maxInt16
	case lsfVariabFactor < 0:
		lsfVariabFactor = 0
	default:
		lsfVariabFactor <<= 3
	}
	lsfVariabIndex := int(pseudonoise(&st.pnSeedRx, 3))

	var lsfInt, lsfIntVariab, lspIntVariab [cM]int16
	lspLsf(lspInt[:], lsfInt[:], cM)
	lsfIntVariab = lsfInt
	for i := cM - 1; i >= 0; i-- {
		lsfIntVariab[i] = add(lsfIntVariab[i], mult(lsfVariabFactor, st.lsfHistMean[i+lsfVariabIndex*cM]))
	}
	reorderLsf(lsfInt[:], cLSF_GAP, cM)
	reorderLsf(lsfIntVariab[:], cLSF_GAP, cM)
	lsf.pastLsfQ = lsfInt // update the LSF predictor for the next speech frame
	lsfLsp(lsfInt[:], lspInt[:], cM)
	lsfLsp(lsfIntVariab[:], lspIntVariab[:], cM)

	var acoeff, acoeffVariab [cMP1]int16
	lspAz(lspInt[:], acoeff[:])
	lspAz(lspIntVariab[:], acoeffVariab[:])
	for i := 0; i < 4; i++ {
		copy(at[i*cMP1:(i+1)*cMP1], acoeff[:])
	}

	// Prediction gain of the comfort-noise filter.
	var refl [cM]int16
	aRefl(acoeff[1:], refl[:])
	predErr := int16(maxInt16)
	for i := 0; i < cM; i++ {
		L := (int32(refl[i]) * int32(refl[i])) >> 15
		t := int16(0)
		if L <= 0x7fff {
			t = maxInt16 - int16(L)
		}
		predErr = mult(predErr, t)
	}
	logPgE, logPgM := log2(int32(predErr))
	logPg := shl(logPgE-15, 12)
	logPg = shr(sub(0, add(logPg, shr(logPgM, 15-12))), 1)
	st.logPgMean = add(mult(29491, st.logPgMean), mult(3277, logPg))

	// Excitation level: interpolated energy, less the prediction gain.
	LlogEnInt = L_shr(LlogEnInt, 10)
	LlogEnInt = L_add(LlogEnInt, 4*65536)
	LlogEnInt = L_sub(LlogEnInt, L_shl(int32(logPg), 4))
	LlogEnInt = L_add(LlogEnInt, L_shl(int32(st.logEnAdjust), 5))
	logEnIntE := int16(LlogEnInt >> 16)
	logEnIntM := int16(L_shr(L_sub(LlogEnInt, int32(logEnIntE)<<16), 1))
	level := int16(pow2(logEnIntE, logEnIntM))

	var ex [cL_SUBFR]int16
	for i := 0; i < 4; i++ {
		buildCNCode(&st.pnSeedRx, ex[:])
		for j := range ex {
			ex[j] = mult(level, ex[j])
		}
		synFilt(acoeffVariab[:], ex[:], synth[i*cL_SUBFR:], cL_SUBFR, memSyn, true)
	}

	aver.hangVar = 20
	aver.hangCount = 0

	if newState == dtxMute {
		n := st.sinceLastSid
		if n > 32 {
			n = 32
		} else if n <= 0 {
			n = 8
		}
		st.trueSidPeriodInv = div_s(1<<10, shl(n, 10))
		st.sinceLastSid = 0
		st.lspOld = st.lsp
		st.oldLogEn = st.logEn
		st.logEn -= 256 // fade out
		if st.logEn < 0 {
			st.logEn = 0
		}
	}

	if st.sidFrame != 0 && (st.validData != 0 || st.dtxHangoverAdded != 0) {
		st.sinceLastSid = 0
		st.dataUpdated = 1
	}
}

// activityUpdate records the LSFs and energy of a decoded speech frame for
// later comfort noise. Mirrors dtx_dec_activity_update.
func (st *dtxDecState) activityUpdate(lsf, frame []int16) {
	st.lsfHistPtr += cM
	if st.lsfHistPtr == 80 {
		st.lsfHistPtr = 0
	}
	copy(st.lsfHist[st.lsfHistPtr:st.lsfHistPtr+cM], lsf[:cM])

	e, m := log2(frameEnergy(frame[:cL_FRAME]))
	logEn := shl(e, 10) + m>>5
	logEn -= 7497 + 1024
	st.logEnHistPtr++
	if st.logEnHistPtr == cDTX_HIST_SIZE {
		st.logEnHistPtr = 0
	}
	st.logEnHist[st.logEnHistPtr] = logEn
}

// pseudonoise returns nBits bits of the 31-bit pseudo-random sequence in
// *shiftReg. Mirrors pseudonoise.
func pseudonoise(shiftReg *int32, nBits int) int16 {
	var bits int16
	for i := 0; i < nBits; i++ {
		sn := *shiftReg & 1
		if *shiftReg&0x10000000 != 0 {
			sn ^= 1
		}
		bits = bits<<1 | int16(*shiftReg&1)
		*shiftReg >>= 1
		if sn != 0 {
			*shiftReg |= 0x40000000
		}
	}
	return bits
}

// buildCNCode builds a random ten-pulse comfort-noise innovation. Mirrors
// build_CN_code.
func buildCNCode(seed *int32, cod []int16) {
	for i := range cod[:cL_SUBFR] {
		cod[i] = 0
	}
	for k := int16(0); k < 10; k++ {
		i := pseudonoise(seed, 2)*10 + k
		if pseudonoise(seed, 1) > 0 {
			cod[i] = 4096
		} else {
			cod[i] = -4096
		}
	}
}

// buildCNParam fills parm with random parameters for a frame received
// without data. Mirrors build_CN_param.
func buildCNParam(seed *int16, sizes []int16, parm []int16) {
	*seed = int16(L_add(L_mult(*seed, 31821)>>1, 13849))
	w := window_200_40[*seed&0x7F:]
	for i, n := range sizes {
		parm[i] = w[i] & ^(-1 << uint(n))
	}
}

// aRefl converts the LP coefficients a[0:M] (without a[0]) into reflection
// coefficients. Mirrors A_Refl, including what it does when a stage
// overflows: refl is zeroed, the stages computed so far are kept, and the
// recursion goes on.
func aRefl(a, refl []int16) {
	var aState, bState [cM]int16
	copy(aState[:], a[:cM])
	for i := cM - 1; i >= 0; i-- {
		if abs_s(aState[i]) >= 4096 {
			for j := range refl[:cM] {
				refl[j] = 0
			}
			return
		}
		refl[i] = shl(aState[i], 3)
		L := L_sub(maxInt32, L_mult(refl[i], refl[i]))
		normShift := norm_l(L)
		scale := 15 - normShift
		// The reference rounds with pv_round, whose saturation the Ubuntu and
		// Debian builds of opencore-amrnb (GCC, LTO) drop when it is inlined
		// here: L_add is written as a plain signed addition, which the
		// compiler may assume does not overflow. So a value within 0x8000 of
		// MAX_32 wraps to -32768 instead of saturating to 32767.
		normProd := int16((L_shl(L, normShift) + 0x8000) >> 16)
		m := div_s(16384, normProd)
		bState = aState // stages not recomputed keep their values
		for j := 0; j < i; j++ {
			L = int32(aState[j]) << 16
			L = L_msu(L, refl[i], aState[i-j-1])
			t := round_(L)
			L = L_shr_r(L_mult(m, t), scale)
			abs := L - b2i32(L < 0)
			abs ^= abs >> 31
			if abs > 32767 {
				for k := range refl[:cM] {
					refl[k] = 0
				}
				break
			}
			bState[j] = int16(L)
		}
		copy(aState[:i], bState[:i])
	}
}
