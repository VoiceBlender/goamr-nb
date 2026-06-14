package amrnb

// MR475 joint two-subframe gain quantization, ported from opencore-amrnb
// qgain475.cpp (MR475_gain_quant, MR475_quant_store_results,
// MR475_update_unq_pred) and the target-energy helper calc_en.cpp
// (calc_target_energy). The 4.75 kbit/s mode quantizes the pitch and code gains
// of two consecutive subframes jointly with a single 8-bit index into
// table_gain_MR475, using an "unquantized" gain predictor (predUnq) to drive the
// search and the real predictor (pred) for the reconstructed gains. Bit-exact
// with the reference.

const (
	cMIN_QUA_ENER       = -5443
	cMIN_QUA_ENER_MR122 = -32768
	cMAX_QUA_ENER       = 3037
	cMAX_QUA_ENER_MR122 = 18284
)

// calcTargetEnergy returns the energy of the LTP target xn as a normalized
// (exponent, fraction) pair. Mirrors calc_target_energy.
func calcTargetEnergy(xn []int16) (enExp, enFrac int16) {
	var s int32
	for i := 0; i < cL_SUBFR; i++ {
		s += int32(xn[i]) * int32(xn[i]) // 32-bit MAC, wraps like the reference
	}
	if s < 0 { // overflow wrapped negative
		s = maxInt32
	}
	exp := norm_l(s)
	enFrac = int16(L_shl(s, exp) >> 16)
	enExp = 16 - exp
	return
}

// mr475StoreResults dequantizes one subframe's gains from a table entry p
// (p[0]=gain_pit, p[1]=gain_code) given the predicted code gain gcode0, and
// advances the predictor with the resulting quantized energy error. Mirrors
// MR475_quant_store_results.
func (pred *gcPredState) mr475StoreResults(p []int16, gcode0, expGcode0 int16) (gainPit, gainCod int16) {
	gainPit = p[0]
	gCode := p[1]

	L := (int32(gCode) * int32(gcode0)) << 1
	L = L_shr(L, 10-expGcode0)
	gainCod = int16(L >> 16)

	exp, frac := log2(int32(gCode))
	exp -= 12
	tmp := shr_r(frac, 5)
	quaEnerMR122 := (exp << 10) + tmp
	L2 := mpy32_16(exp, frac, 24660) << 13
	quaEner := int16((L2 + 0x00008000) >> 16)

	pred.update(quaEnerMR122, quaEner)
	return
}

// mr475UpdateUnqPred advances the "unquantized" predictor with the optimum
// (unquantized) code gain of a subframe. Mirrors MR475_update_unq_pred.
func (pred *gcPredState) mr475UpdateUnqPred(expGcode0, fracGcode0, codGainExp, codGainFrac int16) {
	var quaEner, quaEnerMR122 int16
	if codGainFrac <= 0 {
		quaEner = cMIN_QUA_ENER
		quaEnerMR122 = cMIN_QUA_ENER_MR122
	} else {
		fg := int16(pow2(14, fracGcode0))
		if codGainFrac >= fg {
			codGainFrac >>= 1
			codGainExp++
		}
		frac := div_s(codGainFrac, fg)
		tmp := codGainExp - expGcode0 - 1
		exp, fr := log2(int32(frac))
		exp += tmp
		quaEnerMR122 = shr_r(fr, 5) + (exp << 10)
		if quaEnerMR122 > cMAX_QUA_ENER_MR122 {
			quaEner = cMAX_QUA_ENER
			quaEnerMR122 = cMAX_QUA_ENER_MR122
		} else {
			L := L_shl(mpy32_16(exp, fr, 24660), 13)
			quaEner = round_(L)
		}
	}
	pred.update(quaEnerMR122, quaEner)
}

// mr475GainQuant runs the joint pitch+code gain VQ for a subframe pair: it
// searches table_gain_MR475 for the entry minimizing the combined weighted
// error of both subframes (respecting the pitch-gain limit), stores the
// reconstructed gains for both subframes, and returns the table index. The sf0
// energy terms were stashed in st by the even-subframe pass; the sf1 terms are
// passed in. Mirrors MR475_gain_quant.
func (st *coderState) mr475GainQuant(sf1Code []int16, sf1ExpGcode0, sf1FracGcode0 int16,
	sf1ExpCoeff, sf1FracCoeff []int16, sf1ExpTargetEn, sf1FracTargetEn, gpLimit int16) (index, sf0GainPit, sf0GainCod, sf1GainPit, sf1GainCod int16) {

	sf0Gcode0 := int16(pow2(14, st.sf0FracGcode0))
	sf1Gcode0 := int16(pow2(14, sf1FracGcode0))

	// per-coefficient exponents (0..4: sf0, 5..9: sf1)
	var expMax [10]int16
	exp := st.sf0ExpGcode0 - 11
	expMax[0] = st.sf0ExpCoeff[0] - 13
	expMax[1] = st.sf0ExpCoeff[1] - 14
	expMax[2] = st.sf0ExpCoeff[2] + (15 + (exp << 1))
	expMax[3] = st.sf0ExpCoeff[3] + exp
	expMax[4] = st.sf0ExpCoeff[4] + (1 + exp)
	exp = sf1ExpGcode0 - 11
	expMax[5] = sf1ExpCoeff[0] - 13
	expMax[6] = sf1ExpCoeff[1] - 14
	expMax[7] = sf1ExpCoeff[2] + (15 + (exp << 1))
	expMax[8] = sf1ExpCoeff[3] + exp
	expMax[9] = sf1ExpCoeff[4] + (1 + exp)

	// scale the two target energies to a common exponent, then nudge the sf0
	// coefficient exponents so the two subframes' errors are comparable.
	sf0FracTargetEn := st.sf0FracTargetEn
	if e := st.sf0ExpTargetEn - sf1ExpTargetEn; e > 0 {
		sf1FracTargetEn >>= uint(e)
	} else {
		sf0FracTargetEn >>= uint(-e)
	}
	adj := int16(0)
	if tmp := shr_r(sf1FracTargetEn, 1); tmp > sf0FracTargetEn {
		adj = 1
	} else if (sf0FracTargetEn+3)>>2 > sf1FracTargetEn {
		adj = -1
	}
	for i := 0; i < 5; i++ {
		expMax[i] += adj
	}

	// common exponent for all 10 coefficients
	mx := expMax[0]
	for i := 9; i > 0; i-- {
		if expMax[i] > mx {
			mx = expMax[i]
		}
	}
	mx++ // avoid overflow

	var coeff, coeffLo [10]int16
	for i := 0; i < 10; i++ {
		var fc int16
		if i < 5 {
			fc = st.sf0FracCoeff[i]
		} else {
			fc = sf1FracCoeff[i-5]
		}
		L := L_shr(int32(fc)<<16, mx-expMax[i])
		coeff[i] = int16(L >> 16)
		coeffLo[i] = int16((L >> 1) - ((L >> 16) << 15))
	}

	// exhaustive search over the 256-entry joint codebook
	distMin := int32(maxInt32)
	index = 0
	for i := 0; i < cMR475_VQ_SIZE; i++ {
		b := i * 4
		gPitch := table_gain_MR475[b]
		gCode := int16((int32(table_gain_MR475[b+1]) * int32(sf0Gcode0)) >> 15)
		g2Pitch := int16((int32(gPitch) * int32(gPitch)) >> 15)
		g2Code := int16((int32(gCode) * int32(gCode)) >> 15)
		gPitCod := int16((int32(gCode) * int32(gPitch)) >> 15)
		L := mpy32_16(coeff[0], coeffLo[0], g2Pitch) +
			mpy32_16(coeff[1], coeffLo[1], gPitch) +
			mpy32_16(coeff[2], coeffLo[2], g2Code) +
			mpy32_16(coeff[3], coeffLo[3], gCode) +
			mpy32_16(coeff[4], coeffLo[4], gPitCod)

		tmp := gPitch - gpLimit
		gPitch1 := table_gain_MR475[b+2]
		if tmp <= 0 && gPitch1 <= gpLimit {
			gCode1 := int16((int32(table_gain_MR475[b+3]) * int32(sf1Gcode0)) >> 15)
			g2Pitch = int16((int32(gPitch1) * int32(gPitch1)) >> 15)
			g2Code = int16((int32(gCode1) * int32(gCode1)) >> 15)
			gPitCod = int16((int32(gCode1) * int32(gPitch1)) >> 15)
			L += mpy32_16(coeff[5], coeffLo[5], g2Pitch) +
				mpy32_16(coeff[6], coeffLo[6], gPitch1) +
				mpy32_16(coeff[7], coeffLo[7], g2Code) +
				mpy32_16(coeff[8], coeffLo[8], gCode1) +
				mpy32_16(coeff[9], coeffLo[9], gPitCod)
			if L < distMin {
				distMin = L
				index = int16(i)
			}
		}
	}

	// reconstruct sf0 gains (updates the real predictor), re-predict, then sf1
	base := int(index) << 2
	sf0GainPit, sf0GainCod = st.pred.mr475StoreResults(table_gain_MR475[base:], sf0Gcode0, st.sf0ExpGcode0)
	gp := st.pred.gcPred(dMR475, sf1Code)
	sf1Gcode0 = int16(pow2(14, gp.fracGcode0))
	sf1GainPit, sf1GainCod = st.pred.mr475StoreResults(table_gain_MR475[base+2:], sf1Gcode0, gp.expGcode0)
	return
}
