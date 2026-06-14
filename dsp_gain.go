package amrnb

// Fixed-codebook gain prediction, ported from opencore-amrnb gc_pred.cpp plus
// the 32-bit composite operators it needs (L_Extract, L_Comp, Mpy_32_16 from
// the ETSI basicop set). gc_pred runs the MA energy predictor that yields the
// predicted code gain (gcode0) used by the gain dequantizers; gc_pred_update
// feeds back the quantized energy each subframe. Bit-exact with the reference.

const (
	cNPRED            = 4
	cMEAN_ENER_MR122  = 783741 // Q17
	cMIN_ENERGY       = -14336 // Q10
	cMIN_ENERGY_MR122 = -2381  // Q10
)

// MA prediction coefficients (gc_pred.cpp).
var gcPred = [cNPRED]int16{5571, 4751, 2785, 1556}
var gcPredMR122 = [cNPRED]int16{44, 37, 22, 12}

// L_Extract splits a 32-bit value into a 16-bit high part and a 15-bit low
// part such that L ≈ (hi<<16) + (lo<<1). Mirrors L_Extract.
func L_Extract(L int32) (hi, lo int16) {
	hi = int16(L >> 16)
	lo = int16((L >> 1) - (int32(hi) << 15))
	return hi, lo
}

// L_Comp recombines a (hi,lo) pair back into a 32-bit value. Mirrors L_Comp.
func L_Comp(hi, lo int16) int32 {
	return L_mac(L_deposit_h(hi), lo, 1)
}

// mpy32_16 multiplies a 32-bit value (given as hi,lo) by a 16-bit value,
// matching the optimized opencore-amrnb Mpy_32_16 (truncated low-part shift).
func mpy32_16(hi, lo, n int16) int32 {
	return L_add(L_mult(hi, n), ((int32(lo)*int32(n))>>15)<<1)
}

// gcPredState is the MA energy-predictor memory (two parallel histories: the
// 20·log10 form for most modes and the log2 form for MR122). Mirrors
// gc_predState.
type gcPredState struct {
	pastQuaEn      [cNPRED]int16 // Q10, 20*log10(qua_err)
	pastQuaEnMR122 [cNPRED]int16 // Q10, log2(qua_err)
}

// reset initializes both predictor histories to their energy floor. Mirrors
// gc_pred_reset.
func (st *gcPredState) reset() {
	for i := 0; i < cNPRED; i++ {
		st.pastQuaEn[i] = cMIN_ENERGY
		st.pastQuaEnMR122[i] = cMIN_ENERGY_MR122
	}
}

// update shifts the new quantized energies into the predictor memory. Mirrors
// gc_pred_update.
func (st *gcPredState) update(quaEnerMR122, quaEner int16) {
	st.pastQuaEn[3] = st.pastQuaEn[2]
	st.pastQuaEnMR122[3] = st.pastQuaEnMR122[2]
	st.pastQuaEn[2] = st.pastQuaEn[1]
	st.pastQuaEnMR122[2] = st.pastQuaEnMR122[1]
	st.pastQuaEn[1] = st.pastQuaEn[0]
	st.pastQuaEnMR122[1] = st.pastQuaEnMR122[0]
	st.pastQuaEnMR122[0] = quaEnerMR122
	st.pastQuaEn[0] = quaEner
}

// gcPredResult carries gc_pred's outputs: the exponent/fraction of the
// predicted gain factor gcode0, and (MR795 only) of the innovation energy.
type gcPredResult struct {
	expGcode0, fracGcode0 int16
	expEn, fracEn         int16
}

// gcPred computes the predicted fixed-codebook gain factor from the innovation
// energy and the predictor memory. Mirrors gc_pred (gc_pred.cpp).
func (st *gcPredState) gcPred(mode int, code []int16) gcPredResult {
	var r gcPredResult

	var enerCode int32
	for i := cL_SUBFR >> 2; i != 0; i-- {
		base := (cL_SUBFR>>2 - i) * 4
		for k := 0; k < 4; k++ {
			tmp := code[base+k]
			enerCode += (int32(tmp) * int32(tmp)) >> 3
		}
	}
	enerCode <<= 4
	if enerCode>>31 != 0 { // saturation
		enerCode = maxInt32
	}

	if mode == dMR122 {
		enerCode = int32(round_(enerCode)) * 26214 << 1
		exp, frac := log2(enerCode)
		lTemp1 := int32(exp-30) << 16
		enerCode = lTemp1 + (int32(frac) << 1)

		ener := int32(cMEAN_ENER_MR122)
		for i := 0; i < cNPRED; i++ {
			lt := (int32(st.pastQuaEnMR122[i]) * int32(gcPredMR122[i])) << 1
			ener = L_add(ener, lt)
		}

		lTemp1 = L_sub(ener, enerCode)
		r.expGcode0 = int16(lTemp1 >> 17)
		lTemp2 := int32(r.expGcode0) << 15
		lTemp1 >>= 2
		r.fracGcode0 = int16(lTemp1 - lTemp2)
		return r
	}

	// all modes except 12.2
	expCode := norm_l(enerCode)
	enerCode = L_shl(enerCode, expCode)
	exp, frac := log2Norm(enerCode, expCode)

	lTemp2 := (int32(exp) * -24660) << 1
	lTmp := (int32(frac) * -24660) >> 15
	if lTmp&0x00010000 != 0 { // sign-extend
		lTmp |= int32(-0x10000) // 0xffff0000
	}
	lTmp <<= 1
	lTmp = L_add(lTmp, lTemp2)

	switch mode {
	case dMR102:
		lTemp2 = int32(16678) << 7
		lTmp = L_add(lTmp, lTemp2)
	case dMR795:
		r.fracEn = int16(enerCode >> 16)
		r.expEn = int16(-11 - expCode)
		lTemp2 = int32(17062) << 7
		lTmp = L_add(lTmp, lTemp2)
	case dMR74:
		lTemp2 = int32(32588) << 6
		lTmp = L_add(lTmp, lTemp2)
	case dMR67:
		lTemp2 = int32(32268) << 6
		lTmp = L_add(lTmp, lTemp2)
	default: // MR59, MR515, MR475
		lTemp2 = int32(16678) << 7
		lTmp = L_add(lTmp, lTemp2)
	}

	// gcode0 = sum(pred[i]*past_qua_en[i]) - ener_code + mean_ener (Q24)
	if lTmp > 0x001fffff {
		lTmp = maxInt32
	} else if lTmp < -2097152 {
		lTmp = minInt32
	} else {
		lTmp <<= 10
	}
	for i := 0; i < 4; i++ {
		lTemp2 = (int32(gcPred[i]) * int32(st.pastQuaEn[i])) << 1
		lTmp = L_add(lTmp, lTemp2)
	}
	gcode0 := int16(lTmp >> 16) // Q8

	// gcode0 = pow(2, 0.166*gcode0)
	if mode == dMR74 { // IS641 bit-exactness
		lTmp = int32(gcode0) * 5439 << 1
	} else {
		lTmp = int32(gcode0) * 5443 << 1
	}
	if lTmp < 0 {
		lTmp = ^((^lTmp) >> 8)
	} else {
		lTmp >>= 8 // -> Q16
	}
	r.expGcode0 = int16(lTmp >> 16)
	var lTemp1 int32
	if lTmp < 0 {
		lTemp1 = ^((^lTmp) >> 1)
	} else {
		lTemp1 = lTmp >> 1
	}
	lTemp2 = int32(r.expGcode0) << 15
	r.fracGcode0 = int16(L_sub(lTemp1, lTemp2))
	return r
}
