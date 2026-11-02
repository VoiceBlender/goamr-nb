package amrnb

// Voice activity detector (VAD option 1), ported from opencore-amrnb vad1.cpp
// and hp_max.cpp. It runs only with DTX on: vad1 analyses the band energies
// of each frame, while the open-loop pitch search feeds it tone, pitch and
// correlation-complexity hints (vadToneDetection, vadPitchDetection,
// vadComplexDetectionUpdate).

const (
	cVAD_FRAME_LEN    = 160
	cCOMPLEN          = 9
	cINV_COMPLEN      = 3641
	cLOOKAHEAD        = 40
	cUNIRSHFT         = 6
	cTONE_THR         = 21298
	cALPHA_UP1        = 1638
	cALPHA_DOWN1      = 2097
	cALPHA_UP2        = 491
	cALPHA_DOWN2      = 1867
	cALPHA3           = 1638
	cALPHA4           = 3276
	cALPHA5           = 16383
	cVAD_THR_HIGH     = 1260
	cVAD_THR_LOW      = 720
	cVAD_P1           = 0
	cVAD_SLOPE        = -2808
	cSTAT_COUNT       = 20
	cCAD_MIN_STAT_CNT = 5
	cSTAT_THR_LEVEL   = 184
	cSTAT_THR         = 1000
	cNOISE_MIN        = 40
	cNOISE_MAX        = 16000
	cNOISE_INIT       = 150
	cHANG_NOISE_THR   = 100
	cBURST_LEN_HIGH   = 4
	cHANG_LEN_HIGH    = 7
	cBURST_LEN_LOW    = 5
	cHANG_LEN_LOW     = 4
	cVAD_POW_LOW      = 15000
	cPOW_PITCH_THR    = 343040
	cPOW_COMPLEX_THR  = 15000
	cCOEFF3           = 13363
	cCOEFF5_1         = 21955
	cCOEFF5_2         = 6390
	cLTHRESH          = 4
	cNTHRESH          = 4
	cCVAD_ADAPT_HIGH  = 19660
	cCVAD_ADAPT_LOW   = 16383
	cCVAD_IN_NOISE    = 21298
	cCVAD_HANG_THRESH = 22936
	cCVAD_HANG_LIMIT  = 100
	cCVAD_HANG_LENGTH = 250
	cCVAD_LOWPOW_RST  = 13106
	cCVAD_MIN_CORR    = 13106
	cCVAD_SLOW        = 655
	cCVAD_FAST        = 2621
	cCVAD_REALLY_FAST = 6553
)

// vadState is the VAD1 state. Mirrors vadState1.
type vadState struct {
	bckrEst          [cCOMPLEN]int16
	aveLevel         [cCOMPLEN]int16
	oldLevel         [cCOMPLEN]int16
	subLevel         [cCOMPLEN]int16
	aData5           [3][2]int16
	aData3           [5]int16
	burstCount       int16
	hangCount        int16
	statCount        int16
	vadreg           int16
	pitch            int16
	tone             int16
	complexHigh      int16
	complexLow       int16
	complexHangTimer int16
	complexHangCount int16
	oldlagCount      int16
	oldlag           int16
	complexWarning   int16
	bestCorrHp       int16
	corrHpFast       int16
	speechVadDec     int16
}

// reset mirrors vad1_reset.
func (st *vadState) reset() {
	*st = vadState{}
	for i := 0; i < cCOMPLEN; i++ {
		st.bckrEst[i] = cNOISE_INIT
		st.oldLevel[i] = cNOISE_INIT
		st.aveLevel[i] = cNOISE_INIT
	}
	st.bestCorrHp = cCVAD_LOWPOW_RST
	st.corrHpFast = cCVAD_LOWPOW_RST
}

func vadFirstFilterStage(in, out, data []int16) {
	data0, data1 := data[0], data[1]
	for i := 0; i < cVAD_FRAME_LEN/4; i++ {
		temp0 := sub(shr(in[4*i], 2), mult(cCOEFF5_1, data0))
		temp1 := add(data0, mult(cCOEFF5_1, temp0))
		temp3 := sub(shr(in[4*i+1], 2), mult(cCOEFF5_2, data1))
		temp2 := add(data1, mult(cCOEFF5_2, temp3))
		out[4*i] = add(temp1, temp2)
		out[4*i+1] = sub(temp1, temp2)

		data0 = sub(shr(in[4*i+2], 2), mult(cCOEFF5_1, temp0))
		temp1 = add(temp0, mult(cCOEFF5_1, data0))
		data1 = sub(shr(in[4*i+3], 2), mult(cCOEFF5_2, temp3))
		temp2 = add(temp3, mult(cCOEFF5_2, data1))
		out[4*i+2] = add(temp1, temp2)
		out[4*i+3] = sub(temp1, temp2)
	}
	data[0], data[1] = data0, data1
}

func vadFilter5(in0, in1 *int16, data []int16) {
	temp0 := sub(*in0, mult(cCOEFF5_1, data[0]))
	temp1 := add(data[0], mult(cCOEFF5_1, temp0))
	data[0] = temp0
	temp0 = sub(*in1, mult(cCOEFF5_2, data[1]))
	temp2 := add(data[1], mult(cCOEFF5_2, temp0))
	data[1] = temp0
	*in0 = shr(add(temp1, temp2), 1)
	*in1 = shr(sub(temp1, temp2), 1)
}

func vadFilter3(in0, in1, data *int16) {
	temp1 := sub(*in1, mult(cCOEFF3, *data))
	temp2 := add(*data, mult(cCOEFF3, temp1))
	*data = temp1
	*in1 = shr(sub(*in0, temp2), 1)
	*in0 = shr(add(*in0, temp2), 1)
}

func vadLevelCalculation(data []int16, subLevel *int16, count1, count2, indM, indA int, scale int16) int16 {
	var l1 int32
	for i := count1; i < count2; i++ {
		l1 = L_mac(l1, 1, abs_s(data[indM*i+indA]))
	}
	l2 := L_add(l1, L_shl(int32(*subLevel), sub(16, scale)))
	*subLevel = extract_h(L_shl(l1, scale))
	for i := 0; i < count1; i++ {
		l2 = L_mac(l2, 1, abs_s(data[indM*i+indA]))
	}
	return extract_h(L_shl(l2, scale))
}

func (st *vadState) filterBank(in []int16, level []int16) {
	var b [cVAD_FRAME_LEN]int16
	vadFirstFilterStage(in, b[:], st.aData5[0][:])
	for i := 0; i < cVAD_FRAME_LEN/4; i++ {
		vadFilter5(&b[4*i], &b[4*i+2], st.aData5[1][:])
		vadFilter5(&b[4*i+1], &b[4*i+3], st.aData5[2][:])
	}
	for i := 0; i < cVAD_FRAME_LEN/8; i++ {
		vadFilter3(&b[8*i], &b[8*i+4], &st.aData3[0])
		vadFilter3(&b[8*i+2], &b[8*i+6], &st.aData3[1])
		vadFilter3(&b[8*i+3], &b[8*i+7], &st.aData3[4])
	}
	for i := 0; i < cVAD_FRAME_LEN/16; i++ {
		vadFilter3(&b[16*i], &b[16*i+8], &st.aData3[2])
		vadFilter3(&b[16*i+4], &b[16*i+12], &st.aData3[3])
	}
	const n4, n8, n16 = cVAD_FRAME_LEN / 4, cVAD_FRAME_LEN / 8, cVAD_FRAME_LEN / 16
	level[8] = vadLevelCalculation(b[:], &st.subLevel[8], n4-8, n4, 4, 1, 15)
	level[7] = vadLevelCalculation(b[:], &st.subLevel[7], n8-4, n8, 8, 7, 16)
	level[6] = vadLevelCalculation(b[:], &st.subLevel[6], n8-4, n8, 8, 3, 16)
	level[5] = vadLevelCalculation(b[:], &st.subLevel[5], n8-4, n8, 8, 2, 16)
	level[4] = vadLevelCalculation(b[:], &st.subLevel[4], n8-4, n8, 8, 6, 16)
	level[3] = vadLevelCalculation(b[:], &st.subLevel[3], n16-2, n16, 16, 4, 16)
	level[2] = vadLevelCalculation(b[:], &st.subLevel[2], n16-2, n16, 16, 12, 16)
	level[1] = vadLevelCalculation(b[:], &st.subLevel[1], n16-2, n16, 16, 8, 16)
	level[0] = vadLevelCalculation(b[:], &st.subLevel[0], n16-2, n16, 16, 0, 16)
}

func (st *vadState) updateCntrl(level []int16) {
	if st.complexWarning != 0 && st.statCount < cCAD_MIN_STAT_CNT {
		st.statCount = cCAD_MIN_STAT_CNT
	}
	if st.pitch&0x6000 == 0x6000 || st.tone&0x7c00 == 0x7c00 {
		st.statCount = cSTAT_COUNT
	} else if st.vadreg&0x7f80 == 0 {
		st.statCount = cSTAT_COUNT
	} else {
		var statRat int16
		for i := 0; i < cCOMPLEN; i++ {
			num, denom := st.aveLevel[i], level[i]
			if level[i] > st.aveLevel[i] {
				num, denom = level[i], st.aveLevel[i]
			}
			if num < cSTAT_THR_LEVEL {
				num = cSTAT_THR_LEVEL
			}
			if denom < cSTAT_THR_LEVEL {
				denom = cSTAT_THR_LEVEL
			}
			exp := norm_s(denom)
			denom = shl(denom, exp)
			temp := div_s(shr(num, 1), denom)
			statRat = add(statRat, shr(temp, sub(8, exp)))
		}
		if statRat > cSTAT_THR {
			st.statCount = cSTAT_COUNT
		} else if st.vadreg&0x4000 != 0 && st.statCount != 0 {
			st.statCount = sub(st.statCount, 1)
		}
	}
	alpha := int16(cALPHA4)
	if st.statCount == cSTAT_COUNT {
		alpha = 32767
	} else if st.vadreg&0x4000 == 0 {
		alpha = cALPHA5
	}
	for i := 0; i < cCOMPLEN; i++ {
		st.aveLevel[i] = add(st.aveLevel[i], mult_r(alpha, sub(level[i], st.aveLevel[i])))
	}
}

func (st *vadState) hangoverAddition(noiseLevel, lowPower int16) int16 {
	burstLen, hangLen := int16(cBURST_LEN_LOW), int16(cHANG_LEN_LOW)
	if noiseLevel > cHANG_NOISE_THR {
		burstLen, hangLen = cBURST_LEN_HIGH, cHANG_LEN_HIGH
	}
	if lowPower != 0 {
		st.burstCount = 0
		st.hangCount = 0
		st.complexHangCount = 0
		st.complexHangTimer = 0
		return 0
	}
	if st.complexHangTimer > cCVAD_HANG_LIMIT && st.complexHangCount < cCVAD_HANG_LENGTH {
		st.complexHangCount = cCVAD_HANG_LENGTH
	}
	if st.complexHangCount != 0 {
		st.burstCount = cBURST_LEN_HIGH
		st.complexHangCount = sub(st.complexHangCount, 1)
		return 1
	}
	if st.vadreg&0x3ff0 == 0 && st.corrHpFast > cCVAD_IN_NOISE {
		return 1
	}
	if st.vadreg&0x4000 != 0 {
		st.burstCount = add(st.burstCount, 1)
		if st.burstCount >= burstLen {
			st.hangCount = hangLen
		}
		return 1
	}
	st.burstCount = 0
	if st.hangCount > 0 {
		st.hangCount = sub(st.hangCount, 1)
		return 1
	}
	return 0
}

func (st *vadState) noiseEstimateUpdate(level []int16) {
	st.updateCntrl(level)
	bckrAdd := int16(2)
	var alphaUp, alphaDown int16
	switch {
	case st.vadreg&0x7800 == 0 && st.pitch&0x7800 == 0 && st.complexHangCount == 0:
		alphaUp, alphaDown = cALPHA_UP1, cALPHA_DOWN1
	case st.statCount == 0 && st.complexHangCount == 0:
		alphaUp, alphaDown = cALPHA_UP2, cALPHA_DOWN2
	default:
		alphaUp, alphaDown = 0, cALPHA3
		bckrAdd = 0
	}
	for i := 0; i < cCOMPLEN; i++ {
		temp := sub(st.oldLevel[i], st.bckrEst[i])
		if temp < 0 {
			temp = add(st.bckrEst[i], mult_r(alphaDown, temp))
			st.bckrEst[i] = add(-2, temp)
			if st.bckrEst[i] < cNOISE_MIN {
				st.bckrEst[i] = cNOISE_MIN
			}
		} else {
			temp = add(st.bckrEst[i], mult_r(alphaUp, temp))
			st.bckrEst[i] = add(bckrAdd, temp)
			if st.bckrEst[i] > cNOISE_MAX {
				st.bckrEst[i] = cNOISE_MAX
			}
		}
	}
	copy(st.oldLevel[:], level[:cCOMPLEN])
}

func (st *vadState) complexEstimateAdapt(lowPower int16) {
	var alpha int16
	switch {
	case st.corrHpFast < cCVAD_ADAPT_HIGH:
		alpha = cCVAD_FAST
	case st.bestCorrHp < st.corrHpFast:
		alpha = cCVAD_REALLY_FAST
	default:
		alpha = cCVAD_SLOW
	}
	L := L_deposit_h(st.corrHpFast)
	L = L_msu(L, alpha, st.corrHpFast)
	L = L_mac(L, alpha, st.bestCorrHp)
	st.corrHpFast = round_(L)
	if st.corrHpFast < cCVAD_MIN_CORR {
		st.corrHpFast = cCVAD_MIN_CORR
	}
	if lowPower != 0 {
		st.corrHpFast = cCVAD_MIN_CORR
	}
}

func (st *vadState) complexVad(lowPower int16) int16 {
	st.complexHigh = shr(st.complexHigh, 1)
	st.complexLow = shr(st.complexLow, 1)
	if lowPower == 0 {
		if st.corrHpFast > cCVAD_ADAPT_HIGH {
			st.complexHigh |= 0x4000
		}
		if st.corrHpFast > cCVAD_ADAPT_LOW {
			st.complexLow |= 0x4000
		}
	}
	if st.corrHpFast > cCVAD_HANG_THRESH {
		st.complexHangTimer = add(st.complexHangTimer, 1)
	} else {
		st.complexHangTimer = 0
	}
	if st.complexHigh&0x7f80 == 0x7f80 || st.complexLow&0x7fff == 0x7fff {
		return 1
	}
	return 0
}

func (st *vadState) decision(level []int16, powSum int32) int16 {
	var L int32
	for i := 0; i < cCOMPLEN; i++ {
		exp := norm_s(st.bckrEst[i])
		temp := shl(st.bckrEst[i], exp)
		temp = div_s(shr(level[i], 1), temp)
		temp = shl(temp, sub(exp, cUNIRSHFT-1))
		L = L_mac(L, temp, temp)
	}
	snrSum := extract_h(L_shl(L, 6))
	snrSum = mult(snrSum, cINV_COMPLEN)

	L = 0
	for i := 0; i < cCOMPLEN; i++ {
		L = L_add(L, int32(st.bckrEst[i]))
	}
	noiseLevel := extract_h(L_shl(L, 13))
	vadThr := add(mult(cVAD_SLOPE, sub(noiseLevel, cVAD_P1)), cVAD_THR_HIGH)
	if vadThr < cVAD_THR_LOW {
		vadThr = cVAD_THR_LOW
	}
	st.vadreg = shr(st.vadreg, 1)
	if snrSum > vadThr {
		st.vadreg |= 0x4000
	}
	lowPower := int16(0)
	if L_sub(powSum, cVAD_POW_LOW) < 0 {
		lowPower = 1
	}
	st.complexEstimateAdapt(lowPower)
	st.complexWarning = st.complexVad(lowPower)
	st.noiseEstimateUpdate(level)
	st.speechVadDec = st.hangoverAddition(noiseLevel, lowPower)
	return st.speechVadDec
}

// vad1 returns the VAD decision for the frame buf[off:off+160], whose
// preceding 40 samples (the coder's lookahead from the previous frame) are
// buf[off-40:off]. Mirrors vad1.
func (st *vadState) vad1(buf []int16, off int) int16 {
	var powSum int32
	for i := 0; i < cVAD_FRAME_LEN; i++ {
		v := buf[off+i-cLOOKAHEAD]
		powSum = L_mac(powSum, v, v)
	}
	if L_sub(powSum, cPOW_PITCH_THR) < 0 {
		st.pitch &= 0x3fff
	}
	if L_sub(powSum, cPOW_COMPLEX_THR) < 0 {
		st.complexLow &= 0x3fff
	}
	var level [cCOMPLEN]int16
	st.filterBank(buf[off:off+cVAD_FRAME_LEN], level[:])
	return st.decision(level[:], powSum)
}

// toneDetection flags a tone when the open-loop correlation t0 is close to
// the energy t1. Mirrors vad_tone_detection.
func (st *vadState) toneDetection(t0, t1 int32) {
	temp := round_(t1)
	if temp > 0 && L_msu(t0, temp, cTONE_THR) > 0 {
		st.tone |= 0x4000
	}
}

// toneDetectionUpdate shifts the tone history. Mirrors
// vad_tone_detection_update.
func (st *vadState) toneDetectionUpdate(oneLagPerFrame bool) {
	st.tone = shr(st.tone, 1)
	if oneLagPerFrame {
		st.tone = shr(st.tone, 1)
		st.tone |= 0x2000
	}
}

// pitchDetection updates the pitch-stationarity history from the frame's two
// open-loop lags. Mirrors vad_pitch_detection.
func (st *vadState) pitchDetection(Top []int16) {
	var lagcount int16
	for i := 0; i < 2; i++ {
		if abs_s(sub(st.oldlag, Top[i])) < cLTHRESH {
			lagcount = add(lagcount, 1)
		}
		st.oldlag = Top[i]
	}
	st.pitch = shr(st.pitch, 1)
	if add(st.oldlagCount, lagcount) >= cNTHRESH {
		st.pitch |= 0x4000
	}
	st.oldlagCount = lagcount
}

// hpMax returns the maximum high-pass filtered correlation of the open-loop
// search, normalized by the high-pass filtered energy. corr(lag) is the
// correlation at lag; sig[off:] is the scaled current frame. Mirrors hp_max.
func hpMax(corr func(lag int) int32, sig []int16, off, Lframe, lagMax, lagMin int) int16 {
	max := int32(minInt32)
	for i := lagMax - 1; i > lagMin; i-- {
		t0 := L_sub(L_sub(L_shl(corr(i), 1), corr(i+1)), corr(i-1))
		t0 = L_abs(t0)
		if t0 >= max {
			max = t0
		}
	}
	var t0, t1 int32
	for i := 0; i < Lframe; i++ {
		t0 = L_mac(t0, sig[off+i], sig[off+i])
	}
	for i := 0; i < Lframe; i++ {
		t1 = L_mac(t1, sig[off+i], sig[off+i-1])
	}
	t0 = L_abs(L_sub(L_shl(t0, 1), L_shl(t1, 1)))

	shift1 := norm_l(max) - 1
	max16 := int16(L_shl(max, shift1) >> 16)
	shift2 := norm_l(t0)
	t016 := int16(L_shl(t0, shift2) >> 16)
	var corMax int16
	if t016 != 0 {
		corMax = div_s(max16, t016)
	}
	shift := shift1 - shift2
	if shift >= 0 {
		return shr(corMax, shift)
	}
	return shl(corMax, negate(shift))
}
