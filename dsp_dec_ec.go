package amrnb

// Decoder error concealment and excitation enhancement, ported from
// opencore-amrnb: ec_gains.cpp (gain concealment), bgnscd.cpp (background
// noise detection), c_g_aver.cpp (code-gain averaging), ph_disp.cpp (adaptive
// phase dispersion), ex_ctrl.cpp (excitation energy control), lsp_avg.cpp,
// int_lsf.cpp and agc2 (agc.cpp). They run for every frame, good or bad.

const (
	cL_ENERGYHIST      = 60
	cFRAMEENERGYLIMIT  = 17578
	cLOWERNOISELIMIT   = 20
	cUPPERNOISELIMIT   = 1953
	cL_CBGAINHIST      = 7
	cPHDGAINMEMSIZE    = 5
	cPHDTHR1LTP        = 9830  // 0.6 Q14
	cPHDTHR2LTP        = 14746 // 0.9 Q14
	cONFACTPLUS1       = 16384 // 2.0 Q13
	cONLENGTH          = 2
	cEXPCONST          = 5243 // 0.16 Q15
	cEXC_ENERGY_HIST   = 9
	cLTP_GAIN_HIST_LEN = 9
)

// ecGainPitchState conceals the pitch gain of bad frames. Mirrors
// ec_gain_pitchState.
type ecGainPitchState struct {
	pbuf        [5]int16
	pastGainPit int16
	prevGp      int16
}

func (st *ecGainPitchState) reset() {
	for i := range st.pbuf {
		st.pbuf[i] = 1640
	}
	st.pastGainPit = 0
	st.prevGp = 16384
}

var ecPdown = [7]int16{32767, 32112, 32112, 26214, 9830, 6553, 6553}

// conceal returns the pitch gain for a bad frame in concealment state s.
// Mirrors ec_gain_pitch.
func (st *ecGainPitchState) conceal(s int16) int16 {
	tmp := gmedN(st.pbuf[:], 5)
	if tmp > st.pastGainPit {
		tmp = st.pastGainPit
	}
	return mult(tmp, ecPdown[s])
}

// update records the pitch gain of a frame, limiting the first good frame
// after a bad one. Mirrors ec_gain_pitch_update.
func (st *ecGainPitchState) update(bfi, prevBF int16, gainPitch *int16) {
	if bfi == 0 {
		if prevBF != 0 && *gainPitch > st.prevGp {
			*gainPitch = st.prevGp
		}
		st.prevGp = *gainPitch
	}
	st.pastGainPit = *gainPitch
	if st.pastGainPit > 16384 {
		st.pastGainPit = 16384
	}
	copy(st.pbuf[:4], st.pbuf[1:])
	st.pbuf[4] = st.pastGainPit
}

// ecGainCodeState conceals the code gain of bad frames. Mirrors
// ec_gain_codeState.
type ecGainCodeState struct {
	gbuf         [5]int16
	pastGainCode int16
	prevGc       int16
}

func (st *ecGainCodeState) reset() {
	for i := range st.gbuf {
		st.gbuf[i] = 1
	}
	st.pastGainCode = 0
	st.prevGc = 1
}

var ecCdown = [7]int16{32767, 32112, 32112, 32112, 32112, 32112, 22937}

// conceal returns the code gain for a bad frame in concealment state s and
// updates the gain predictor with its limited average. Mirrors ec_gain_code.
func (st *ecGainCodeState) conceal(pred *gcPredState, s int16) int16 {
	tmp := gmedN(st.gbuf[:], 5)
	if tmp > st.pastGainCode {
		tmp = st.pastGainCode
	}
	g := mult(tmp, ecCdown[s])
	qe122, qe := pred.averageLimited()
	pred.update(qe122, qe)
	return g
}

// update records the code gain of a frame. Mirrors ec_gain_code_update.
func (st *ecGainCodeState) update(bfi, prevBF int16, gainCode *int16) {
	if bfi == 0 {
		if prevBF != 0 && *gainCode > st.prevGc {
			*gainCode = st.prevGc
		}
		st.prevGc = *gainCode
	}
	st.pastGainCode = *gainCode
	copy(st.gbuf[:4], st.gbuf[1:])
	st.gbuf[4] = *gainCode
}

// averageLimited returns the mean of the predictor memories, floored at the
// minimum energies. Mirrors gc_pred_average_limited.
func (st *gcPredState) averageLimited() (enerAvgMR122, enerAvg int16) {
	var a int16
	for i := 0; i < cNPRED; i++ {
		a = add(a, st.pastQuaEnMR122[i])
	}
	a >>= 2
	if a < cMIN_ENERGY_MR122 {
		a = cMIN_ENERGY_MR122
	}
	enerAvgMR122 = a
	a = 0
	for i := 0; i < cNPRED; i++ {
		a = add(a, st.pastQuaEn[i])
	}
	a >>= 2
	if a < cMIN_ENERGY {
		a = cMIN_ENERGY
	}
	return enerAvgMR122, a
}

// frameEnergy returns the saturated energy sum 2*x[i]^2 of a frame, as
// Bgn_scd and dtx_dec_activity_update compute it.
func frameEnergy(x []int16) int32 {
	var s int32
	for i := len(x) - 1; i >= 0; i-- {
		s = L_add(s, L_mult(x[i], x[i]))
	}
	return s
}

// bgnScdState detects stationary background noise. Mirrors Bgn_scdState.
type bgnScdState struct {
	frameEnergyHist [cL_ENERGYHIST]int16
	bgHangover      int16
}

func (st *bgnScdState) reset() { *st = bgnScdState{} }

// detect classifies the frame speech[0:160] and updates voicedHangover; it
// returns whether the decoder is in background noise. Mirrors Bgn_scd.
func (st *bgnScdState) detect(ltpGainHist []int16, speech []int16, voicedHangover *int16) int16 {
	s := frameEnergy(speech[:cL_FRAME])
	var currEnergy int16
	if s > 0x1fffffff {
		currEnergy = maxInt16
	} else {
		currEnergy = int16(s >> 14)
	}
	frameEnergyMin := int16(32767)
	for _, e := range st.frameEnergyHist {
		if e < frameEnergyMin {
			frameEnergyMin = e
		}
	}
	noiseFloor := shl(frameEnergyMin, 4)
	maxEnergy := st.frameEnergyHist[0]
	for i := cL_ENERGYHIST - 5; i >= 1; i-- {
		if maxEnergy < st.frameEnergyHist[i] {
			maxEnergy = st.frameEnergyHist[i]
		}
	}
	maxEnergyLastPart := st.frameEnergyHist[2*cL_ENERGYHIST/3]
	for i := 2*cL_ENERGYHIST/3 + 1; i < cL_ENERGYHIST; i++ {
		if maxEnergyLastPart < st.frameEnergyHist[i] {
			maxEnergyLastPart = st.frameEnergyHist[i]
		}
	}
	if maxEnergy > cLOWERNOISELIMIT && currEnergy < cFRAMEENERGYLIMIT &&
		currEnergy > cLOWERNOISELIMIT &&
		(currEnergy < noiseFloor || maxEnergyLastPart < cUPPERNOISELIMIT) {
		if st.bgHangover+1 > 30 {
			st.bgHangover = 30
		} else {
			st.bgHangover++
		}
	} else {
		st.bgHangover = 0
	}
	var inbgNoise int16
	if st.bgHangover > 1 {
		inbgNoise = 1
	}
	copy(st.frameEnergyHist[:cL_ENERGYHIST-1], st.frameEnergyHist[1:])
	st.frameEnergyHist[cL_ENERGYHIST-1] = currEnergy

	ltpLimit := int16(13926)
	if st.bgHangover > 15 {
		ltpLimit = 16383
	} else if st.bgHangover > 8 {
		ltpLimit = 15565
	}
	prevVoiced := gmedN(ltpGainHist[4:], 5) > ltpLimit
	if st.bgHangover > 20 {
		prevVoiced = gmedN(ltpGainHist, 9) > ltpLimit
	}
	if prevVoiced {
		*voicedHangover = 0
	} else if *voicedHangover+1 > 10 {
		*voicedHangover = 10
	} else {
		*voicedHangover++
	}
	return inbgNoise
}

// cbGainAverageState smooths the code gain in background noise. Mirrors
// Cb_gain_averageState.
type cbGainAverageState struct {
	cbGainHistory [cL_CBGAINHIST]int16
	hangVar       int16
	hangCount     int16
}

func (st *cbGainAverageState) reset() { *st = cbGainAverageState{} }

// average returns the code gain mixed with its recent mean according to the
// LSF stationarity. Mirrors Cb_gain_average.
func (st *cbGainAverageState) average(mode int, gainCode int16, lsp, lspAver []int16,
	bfi, prevBF, pdfi, prevPDF, inBackgroundNoise, voicedHangover int16) int16 {
	cbGainMix := gainCode
	copy(st.cbGainHistory[:cL_CBGAINHIST-1], st.cbGainHistory[1:])
	st.cbGainHistory[cL_CBGAINHIST-1] = gainCode

	var diff int16
	for i := 0; i < cM; i++ {
		tmp1 := abs_s(sub(lspAver[i], lsp[i]))
		shift1 := norm_s(tmp1) - 1
		tmp1 = shl(tmp1, shift1)
		shift2 := norm_s(lspAver[i])
		tmp2 := shl(lspAver[i], shift2)
		t := div_s(tmp1, tmp2)
		shift := 2 + shift1 - shift2
		if shift >= 0 {
			t = shr(t, shift)
		} else {
			t = shl(t, negate(shift))
		}
		diff = add(diff, t)
	}
	if diff > 5325 {
		st.hangVar++
	} else {
		st.hangVar = 0
	}
	if st.hangVar > 10 {
		st.hangCount = 0
	}

	if mode <= dMR67 || mode == dMR102 {
		lowMode := mode == dMR475 || mode == dMR515 || mode == dMR59
		var tmpDiff int16
		if ((pdfi != 0 && prevPDF != 0) || bfi != 0 || prevBF != 0) &&
			voicedHangover > 1 && inBackgroundNoise != 0 && lowMode {
			tmpDiff = diff - 4506
		} else {
			tmpDiff = diff - 3277
		}
		tmp1 := tmpDiff
		if tmp1 < 0 {
			tmp1 = 0
		}
		var bgMix int16 = 8192
		if tmp1 <= 2048 {
			bgMix = shl(tmp1, 2)
		}
		if st.hangCount < 40 || diff > 5325 {
			bgMix = 8192
		}
		L := L_mult(6554, st.cbGainHistory[2])
		for i := 3; i < cL_CBGAINHIST; i++ {
			L = L_mac(L, 6554, st.cbGainHistory[i])
		}
		cbGainMean := round_(L)
		if (bfi != 0 || prevBF != 0) && inBackgroundNoise != 0 && lowMode {
			L = L_mult(4681, st.cbGainHistory[0])
			for i := 1; i < cL_CBGAINHIST; i++ {
				L = L_mac(L, 4681, st.cbGainHistory[i])
			}
			cbGainMean = round_(L)
		}
		L = L_mult(bgMix, cbGainMix)
		L = L_mac(L, 8192, cbGainMean)
		L = L_msu(L, bgMix, cbGainMean)
		cbGainMix = round_(L_shl(L, 2))
	}
	st.hangCount++
	return cbGainMix
}

// phDispState is the adaptive phase dispersion memory. Mirrors ph_dispState.
type phDispState struct {
	gainMem    [cPHDGAINMEMSIZE]int16
	prevState  int16
	prevCbGain int16
	lockFull   int16
	onset      int16
}

func (st *phDispState) reset() { *st = phDispState{} }

// disperse disperses the innovation inno (in place) and writes the total
// excitation x = pitchFac*x + cbGain*inno, shifted by tmpShift. x holds the
// adaptive excitation on entry. Mirrors ph_disp.
func (st *phDispState) disperse(mode int, x []int16, cbGain, ltpGain int16, inno []int16, pitchFac, tmpShift int16) {
	copy(st.gainMem[1:], st.gainMem[:cPHDGAINMEMSIZE-1])
	st.gainMem[0] = ltpGain

	var impNr int16
	switch {
	case ltpGain >= cPHDTHR2LTP:
		impNr = 2
	case ltpGain > cPHDTHR1LTP:
		impNr = 1
	}

	// onset detection: cbGain > 2 * prevCbGain
	tmp1 := round_(L_shl(int32(st.prevCbGain)*cONFACTPLUS1<<1, 2))
	if cbGain > tmp1 {
		st.onset = cONLENGTH
	} else if st.onset > 0 {
		st.onset--
	}
	if st.onset == 0 {
		var i1 int16
		for _, g := range st.gainMem {
			if g < cPHDTHR1LTP {
				i1++
			}
		}
		if i1 > 2 {
			impNr = 0
		}
	}
	if impNr > st.prevState+1 && st.onset == 0 {
		impNr--
	}
	if impNr < 2 && st.onset > 0 {
		impNr++
	}
	if cbGain < 10 {
		impNr = 2
	}
	if st.lockFull == 1 {
		impNr = 0
	}
	st.prevState = impNr
	st.prevCbGain = cbGain

	if mode != dMR122 && mode != dMR102 && mode != dMR74 && impNr < 2 {
		var innoSav [cL_SUBFR]int16
		var psPoss [cL_SUBFR]int
		nze := 0
		for i := 0; i < cL_SUBFR; i++ {
			if inno[i] != 0 {
				psPoss[nze] = i
				nze++
			}
			innoSav[i] = inno[i]
			inno[i] = 0
		}
		var imp []int16
		switch {
		case mode == dMR795 && impNr == 0:
			imp = ph_imp_low_MR795
		case mode == dMR795:
			imp = ph_imp_mid_MR795
		case impNr == 0:
			imp = ph_imp_low
		default:
			imp = ph_imp_mid
		}
		for n := 0; n < nze; n++ {
			ppos := psPoss[n]
			c := int32(innoSav[ppos])
			k := 0
			for i := ppos; i < cL_SUBFR; i++ {
				inno[i] = add(inno[i], int16((c*int32(imp[k]))>>15))
				k++
			}
			for i := 0; i < ppos; i++ {
				inno[i] = add(inno[i], int16((c*int32(imp[k]))>>15))
				k++
			}
		}
	}

	for i := 0; i < cL_SUBFR; i++ {
		L := L_mult(x[i], pitchFac)
		L = L_add(L, (int32(inno[i])*int32(cbGain))<<1)
		x[i] = round_(L_shl(L, tmpShift))
	}
}

// exCtrl limits the excitation energy after bad frames in background noise.
// Mirrors Ex_ctrl.
func exCtrl(excitation []int16, excEnergy int16, exEnergyHist []int16, voicedHangover, prevBFI, carefulFlag int16) {
	avgEnergy := gmedN(exEnergyHist, 9)
	prevEnergy := int16((int32(exEnergyHist[7]) + int32(exEnergyHist[8])) >> 1)
	if exEnergyHist[8] < prevEnergy {
		prevEnergy = exEnergyHist[8]
	}
	if excEnergy < avgEnergy && excEnergy > 5 {
		testEnergy := shl(prevEnergy, 2)
		if voicedHangover < 7 || prevBFI != 0 {
			testEnergy = sub(testEnergy, prevEnergy)
		}
		if avgEnergy > testEnergy {
			avgEnergy = testEnergy
		}
		exp := norm_s(excEnergy)
		excEnergy = shl(excEnergy, exp)
		excEnergy = div_s(16383, excEnergy)
		t0 := L_mult(avgEnergy, excEnergy)
		t0 = L_shr(t0, sub(20, exp))
		if t0 > 32767 {
			t0 = 32767
		}
		scaleFactor := int16(t0)
		if carefulFlag != 0 && scaleFactor > 3072 {
			scaleFactor = 3072
		}
		for i := 0; i < cL_SUBFR; i++ {
			excitation[i] = int16(L_shr(L_mult(scaleFactor, excitation[i]), 11))
		}
	}
}

// lspAvg updates the running LSF mean. Mirrors lsp_avg.
func lspAvg(meanSave, lsp []int16) {
	for i := 0; i < cM; i++ {
		L := int32(meanSave[i]) << 16
		L = L_msu(L, cEXPCONST, meanSave[i])
		L = L_mac(L, cEXPCONST, lsp[i])
		meanSave[i] = round_(L)
	}
}

// intLsf interpolates the LSFs of subframe iSubfr between the previous and
// current frame. Mirrors Int_lsf.
func intLsf(lsfOld, lsfNew []int16, iSubfr int, lsfOut []int16) {
	switch iSubfr {
	case 0:
		for i := 0; i < cM; i++ {
			lsfOut[i] = add(lsfOld[i]-lsfOld[i]>>2, lsfNew[i]>>2)
		}
	case 40:
		for i := 0; i < cM; i++ {
			lsfOut[i] = lsfOld[i]>>1 + lsfNew[i]>>1
		}
	case 80:
		for i := 0; i < cM; i++ {
			lsfOut[i] = add(lsfOld[i]>>2, lsfNew[i]-lsfNew[i]>>2)
		}
	case 120:
		copy(lsfOut[:cM], lsfNew[:cM])
	}
}

// agc2 scales sigOut to the energy of sigIn. Mirrors agc2.
func agc2(sigIn, sigOut []int16, lTrm int) {
	s := energyNew(sigOut, lTrm)
	if s == 0 {
		return
	}
	exp := norm_l(s) - 1
	gainOut := round_(L_shl(s, exp))
	s = energyNew(sigIn, lTrm)
	var g0 int16
	if s != 0 {
		i := norm_l(s)
		gainIn := round_(L_shl(s, i))
		exp -= i
		s = int32(div_s(gainOut, gainIn)) << 7 // div_s < 2^15: no saturation
		s = L_shr(s, exp)
		s = invSqrt(s)
		var L int32
		switch {
		case s > 0x003FFFFF:
			L = maxInt32
		case s < -4194304:
			L = minInt32
		default:
			L = s << 9
		}
		g0 = round_(L)
	}
	for i := lTrm - 1; i >= 0; i-- {
		L := L_mult(sigOut[i], g0)
		switch {
		case L > 0x0FFFFFFF:
			sigOut[i] = maxInt16
		case L < -268435456:
			sigOut[i] = minInt16
		default:
			sigOut[i] = int16(L >> 13)
		}
	}
}
