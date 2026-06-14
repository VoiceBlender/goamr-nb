package amrnb

// MR795 gain quantization, ported from opencore-amrnb qgain795.cpp
// (MR795_gain_quant, MR795_gain_code_quant3, MR795_gain_code_quant_mod), the
// gain adaptor g_adapt.cpp (gain_adapt) + gmed_n.cpp (gmed_n), and the unfiltered
// energies calc_en.cpp (calc_unfilt_energies). MR795 jointly searches the three
// nearest pitch-gain candidates against the code-gain VQ, then — when the LTP is
// strong enough (alpha>0) — re-quantizes the code gain with a perceptually
// adapted criterion. It emits two indices per subframe. Bit-exact with the
// reference.

const (
	cLTP_GAIN_THR1 = 2721 // Q13
	cLTP_GAIN_THR2 = 5443 // Q13
	cLTPG_MEM_SIZE = 5
)

// gmedN returns the median of the first n values of ind, matching the
// index-based tie-breaking of the reference. Mirrors gmed_n.
func gmedN(ind []int16, n int) int16 {
	tmp2 := make([]int16, n)
	copy(tmp2, ind[:n])
	tmp := make([]int16, n)
	ix := 0
	for i := 0; i < n; i++ {
		max := int16(-32767)
		for j := 0; j < n; j++ {
			if tmp2[j] >= max {
				max = tmp2[j]
				ix = j
			}
		}
		tmp2[ix] = -32768
		tmp[i] = int16(ix)
	}
	return ind[tmp[n>>1]]
}

// gainAdaptState is the gain adaptor's persistent state. Mirrors GainAdaptState.
type gainAdaptState struct {
	onset     int16
	prevAlpha int16
	prevGc    int16
	ltpgMem   [cLTPG_MEM_SIZE]int16
}

// gainAdapt computes the gain adaptation factor alpha (Q15) from the LTP coding
// gain ltpg and the code gain, updating the onset/history state. Mirrors
// gain_adapt.
func (st *gainAdaptState) gainAdapt(ltpg, gainCod int16) int16 {
	var adapt int16
	switch {
	case ltpg <= cLTP_GAIN_THR1:
		adapt = 0
	case ltpg <= cLTP_GAIN_THR2:
		adapt = 1
	default:
		adapt = 2
	}

	if tmp := shr_r(gainCod, 1); tmp > st.prevGc && gainCod > 200 {
		st.onset = 8
	} else if st.onset != 0 {
		st.onset--
	}
	if st.onset != 0 && adapt < 2 {
		adapt++
	}

	st.ltpgMem[0] = ltpg
	filt := gmedN(st.ltpgMem[:], 5)
	var result int16
	if adapt == 0 {
		switch {
		case filt > 5443:
			result = 0
		case filt < 0:
			result = 16384
		default:
			filt = shl(filt, 2)
			result = 16384 - mult(24660, filt)
		}
	}
	if st.prevAlpha == 0 {
		result = shr(result, 1)
	}

	st.prevAlpha = result
	st.prevGc = gainCod
	for i := cLTPG_MEM_SIZE - 1; i > 0; i-- {
		st.ltpgMem[i] = st.ltpgMem[i-1]
	}
	return result
}

// calcUnfiltEnergies computes the unfiltered residual/excitation/cross/LTP-error
// energies (as 4 normalized exp/frac pairs) and the LTP coding gain ltpg.
// Mirrors calc_unfilt_energies.
func calcUnfiltEnergies(res, exc, code []int16, gainPit int16, Lsubfr int) (fracEn, expEn [4]int16, ltpg int16) {
	var s1, s2, s3, s4 int32
	for i := 0; i < Lsubfr; i++ {
		tmp1 := res[i]
		tmp2 := exc[i]
		s1 += int32(tmp1) * int32(tmp1) // wrapping MAC, like the reference
		s2 += int32(tmp2) * int32(tmp2)
		s3 += int32(tmp2) * int32(code[i])
		Ltemp := L_shl(L_mult(tmp2, gainPit), 1)
		d := sub(tmp1, round_(Ltemp))
		s4 = L_mac(s4, d, d)
	}
	s1 <<= 1
	s2 <<= 1
	s3 <<= 1

	if s1 < 0 { // overflow (sign bit set)
		s1 = maxInt32
	}
	if s1 < 400 {
		fracEn[0] = 0
		expEn[0] = -15
	} else {
		exp := norm_l(s1)
		fracEn[0] = int16(L_shl(s1, exp) >> 16)
		expEn[0] = 15 - exp
	}

	if s2 < 0 {
		s2 = maxInt32
	}
	exp := norm_l(s2)
	fracEn[1] = int16(L_shl(s2, exp) >> 16)
	expEn[1] = 15 - exp

	exp = norm_l(s3)
	fracEn[2] = int16(L_shl(s3, exp) >> 16)
	expEn[2] = 2 - exp

	exp = norm_l(s4)
	ltpResEn := int16(L_shl(s4, exp) >> 16)
	expS4 := 15 - exp
	fracEn[3] = ltpResEn
	expEn[3] = expS4

	if ltpResEn > 0 && fracEn[0] != 0 {
		predGain := div_s(shr(fracEn[0], 1), ltpResEn)
		e := sub(expS4, expEn[0])
		Ltemp := L_shr(int32(predGain)<<16, e+3)
		ltpgExp, ltpgFrac := log2(Ltemp)
		Ltemp = L_Comp(ltpgExp-27, ltpgFrac)
		ltpg = round_(L_shl(Ltemp, 13)) // Q13
	}
	return
}

// mr795GainCodeQuant3 jointly searches the three pitch-gain candidates against
// the code-gain VQ for the minimum weighted error, returning the selected pitch
// and code gains, their indices, and the predictor-update energies. Mirrors
// MR795_gain_code_quant3.
func mr795GainCodeQuant3(expGcode0, gcode0 int16, gpCand, gpCind, fracCoeff, expCoeff []int16) (gainPit, gainPitInd, gainCod, gainCodInd, quaEnerMR122, quaEner int16) {
	var coeff, coeffLo, expMax [5]int16
	expCode := expGcode0 - 10
	expMax[0] = expCoeff[0] - 13
	expMax[1] = expCoeff[1] - 14
	expMax[2] = expCoeff[2] + shl(expCode, 1) + 15
	expMax[3] = expCoeff[3] + expCode
	expMax[4] = expCoeff[4] + (expCode + 1)

	eMax := expMax[0]
	for i := 1; i < 5; i++ {
		if expMax[i] > eMax {
			eMax = expMax[i]
		}
	}
	eMax = add(eMax, 1)
	for i := 0; i < 5; i++ {
		coeff[i], coeffLo[i] = L_Extract(L_shr(int32(fracCoeff[i])<<16, eMax-expMax[i]))
	}

	distMin := int32(maxInt32)
	var codInd, pitInd int16
	for j := 0; j < 3; j++ {
		gPitch := gpCand[j]
		g2Pitch := mult(gPitch, gPitch)
		Ltmp0 := mpy32_16(coeff[0], coeffLo[0], g2Pitch)
		Ltmp0 = L_add(Ltmp0, mpy32_16(coeff[1], coeffLo[1], gPitch))
		for i := 0; i < cNB_QUA_CODE; i++ {
			gCode := mult(qua_gain_code[i*3], gcode0)
			g2h, g2l := L_Extract(L_mult(gCode, gCode))
			gpch, gpcl := L_Extract(L_mult(gCode, gPitch))
			Ltmp := L_add(Ltmp0, mpy_32(coeff[2], coeffLo[2], g2h, g2l))
			Ltmp = L_add(Ltmp, mpy32_16(coeff[3], coeffLo[3], gCode))
			Ltmp = L_add(Ltmp, mpy_32(coeff[4], coeffLo[4], gpch, gpcl))
			if Ltmp < distMin {
				distMin = Ltmp
				codInd = int16(i)
				pitInd = int16(j)
			}
		}
	}

	base := int(codInd) * 3
	gCode := qua_gain_code[base]
	quaEnerMR122 = qua_gain_code[base+1]
	quaEner = qua_gain_code[base+2]
	gainCod = int16(L_shr(L_mult(gCode, gcode0), 9-expGcode0) >> 16)
	gainCodInd = codInd
	gainPit = gpCand[pitInd]
	gainPitInd = gpCind[pitInd]
	return
}

// mr795GainCodeQuantMod re-quantizes the code gain with the gain-adapted
// criterion once the LTP is strong (alpha>0), returning the new index and
// predictor energies and updating gainCod in place. Mirrors
// MR795_gain_code_quant_mod.
func mr795GainCodeQuantMod(gainPit, expGcode0, gcode0 int16, fracEn, expEn []int16, alpha, gainCodUnq int16, gainCod *int16) (index, quaEnerMR122, quaEner int16) {
	var coeff, coeffLo, expCoeff [5]int16
	gainCode := shl(*gainCod, 10-expGcode0) // Q1 -> Q11(-ec0)
	g2Pitch := mult(gainPit, gainPit)
	oneAlpha := add(32767-alpha, 1) // 32768 - alpha

	Lt1 := L_shl(L_mult(alpha, fracEn[1]), 1)
	Lt1 = L_mult(int16(Lt1>>16), g2Pitch)
	expCoeff[1] = expEn[1] - 15

	coeff[2] = mult(int16(L_shl(L_mult(alpha, fracEn[2]), 1)>>16), gainPit)
	expCoeff[2] = add(expEn[2], expGcode0-10)

	coeff[3] = int16(L_shl(L_mult(alpha, fracEn[3]), 1) >> 16)
	expCoeff[3] = add(expEn[3], shl(expGcode0, 1)-7)

	coeff[4] = mult(oneAlpha, fracEn[3])
	expCoeff[4] = add(expCoeff[3], 1)

	Lt0, sqExp := sqrtLExp(L_mult(alpha, fracEn[0]))
	sqExp += 47
	expCoeff[0] = expEn[0] - sqExp

	eMax := expCoeff[0] + 31
	for i := 1; i <= 4; i++ {
		if expCoeff[i] > eMax {
			eMax = expCoeff[i]
		}
	}
	Lt1 = L_shr(Lt1, eMax-expCoeff[1])
	for i := 2; i <= 4; i++ {
		coeff[i], coeffLo[i] = L_Extract(L_shr(int32(coeff[i])<<16, eMax-expCoeff[i]))
	}
	newExp := eMax - 31
	tmp := newExp - expCoeff[0]
	Lt0 = L_shr(Lt0, shr(tmp, 1))
	if tmp&0x1 != 0 {
		coeff[0], coeffLo[0] = L_Extract(Lt0)
		Lt0 = mpy32_16(coeff[0], coeffLo[0], 23170) // 1/sqrt(2) Q15
	}

	distMin := int32(maxInt32)
	index = 0
	for i := 0; i < cNB_QUA_CODE; i++ {
		gCode := mult(qua_gain_code[i*3], gcode0)
		if gCode >= gainCode {
			break
		}
		g2h, g2l := L_Extract(L_mult(gCode, gCode))
		d := sub(gCode, gainCodUnq)
		d2h, d2l := L_Extract(L_mult(d, d))
		Ltmp := L_add(Lt1, mpy32_16(coeff[2], coeffLo[2], gCode))
		Ltmp = L_add(Ltmp, mpy_32(coeff[3], coeffLo[3], g2h, g2l))
		sq, e := sqrtLExp(Ltmp)
		sq = L_shr(sq, shr(e, 1))
		t := round_(L_sub(sq, Lt0))
		Ltmp = L_mult(t, t)
		Ltmp = L_add(Ltmp, mpy_32(coeff[4], coeffLo[4], d2h, d2l))
		if Ltmp < distMin {
			distMin = Ltmp
			index = int16(i)
		}
	}

	base := int(index) * 3
	gCode := qua_gain_code[base]
	quaEnerMR122 = qua_gain_code[base+1]
	quaEner = qua_gain_code[base+2]
	*gainCod = int16(L_shr(L_mult(gCode, gcode0), 9-expGcode0) >> 16)
	return
}

// mr795GainQuant orchestrates the MR795 gain quantization for one subframe,
// updating *gainPit and returning the quantized gains, predictor energies and
// the two analysis indices. Mirrors MR795_gain_quant.
func (st *coderState) mr795GainQuant(res, exc, code, fracCoeff, expCoeff []int16,
	expCodeEn, fracCodeEn, expGcode0, fracGcode0, codGainFrac, codGainExp, gpLimit int16,
	gainPit *int16) (gainCod, quaEnerMR122, quaEner, gainPitIndex, gainCodIndex int16) {

	var gpCand, gpCind [3]int16
	qGainPitch(dMR795, gpLimit, gainPit, gpCand[:], gpCind[:])
	gcode0 := int16(pow2(14, fracGcode0))

	gp, gpInd, gc, gcInd, qe122, qe := mr795GainCodeQuant3(expGcode0, gcode0, gpCand[:], gpCind[:], fracCoeff, expCoeff)
	*gainPit = gp
	gainPitIndex, gainCod = gpInd, gc
	gainCodIndex, quaEnerMR122, quaEner = gcInd, qe122, qe

	fracEn, expEn, ltpg := calcUnfiltEnergies(res, exc, code, *gainPit, cL_SUBFR)
	alpha := st.adapt.gainAdapt(ltpg, gainCod)
	if fracEn[0] != 0 && alpha > 0 {
		fracEn[3] = fracCodeEn
		expEn[3] = expCodeEn
		gainCodUnq := shl(codGainFrac, sub(codGainExp, expGcode0)+10)
		gainCodIndex, quaEnerMR122, quaEner = mr795GainCodeQuantMod(*gainPit, expGcode0, gcode0,
			fracEn[:], expEn[:], alpha, gainCodUnq, &gainCod)
	}
	return
}
