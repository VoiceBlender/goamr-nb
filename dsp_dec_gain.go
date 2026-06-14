package amrnb

// Gain dequantization, ported from opencore-amrnb d_gain_p.cpp, d_gain_c.cpp and
// dec_gain.cpp. These turn the received gain indices into the per-subframe pitch
// gain and fixed-codebook gain, driving the gc_pred energy predictor. dGainPitch
// and dGainCode handle MR795/MR122; decGain handles the joint/scalar-VQ modes
// (MR475 joint, MR515/MR59 low-rate, MR67/MR74/MR102 high-rate). Bit-exact with
// the reference.

const cMR475_VQ_SIZE = 256

// dGainPitch returns the quantized pitch gain (Q14) for the given index.
// Mirrors d_gain_pitch.
func dGainPitch(mode int, index int16) int16 {
	gain := qua_gain_pitch[index]
	if mode == dMR122 {
		gain &^= 3 // clear 2 LSBits
	}
	return gain
}

// dGainCode decodes the fixed-codebook gain for MR795/MR122 from the predicted
// gain (gc_pred) and the quantized correction factor, then updates the
// predictor. Mirrors d_gain_code.
func (pred *gcPredState) dGainCode(mode int, index int16, code []int16) (gainCode int16) {
	r := pred.gcPred(mode, code)
	exp, frac := r.expGcode0, r.fracGcode0

	index &= 31 // index < 32, avoid buffer overflow
	tblTmp := index + (index << 1)
	p := qua_gain_code[tblTmp:]

	if mode == dMR122 {
		gcode0 := int16(pow2(exp, frac))
		gcode0 = shl(gcode0, 4)
		gainCode = shl(mult(gcode0, p[0]), 1)
	} else {
		gcode0 := int16(pow2(14, frac))
		lTmp := L_mult(p[0], gcode0)
		lTmp = L_shr(lTmp, sub(9, exp))
		gainCode = int16(lTmp >> 16) // Q1
	}

	quaEnerMR122 := p[1]
	quaEner := p[2]
	pred.update(quaEnerMR122, quaEner)
	return gainCode
}

// decGain decodes both the pitch gain and fixed-codebook gain for the
// scalar/joint-VQ modes (all except MR795/MR122), updating the predictor.
// evenSubfr selects the MR475 joint-gain half. Mirrors Dec_gain.
func (pred *gcPredState) decGain(mode int, index int16, code []int16, evenSubfr int16) (gainPit, gainCod int16) {
	index = shl(index, 2)

	var gCode, quaEnerMR122, quaEner int16

	switch {
	case mode == dMR102 || mode == dMR74 || mode == dMR67:
		p := table_gain_highrates[index:]
		gainPit = p[0]
		gCode = p[1]
		quaEnerMR122 = p[2]
		quaEner = p[3]
	case mode == dMR475:
		index += (1 ^ evenSubfr) << 1 // evenSubfr is 0 or 1
		if index > int16(cMR475_VQ_SIZE*4-2) {
			index = int16(cMR475_VQ_SIZE*4 - 2)
		}
		p := table_gain_MR475[index:]
		gainPit = p[0]
		gCode = p[1]
		// Predictor update values are recomputed (not stored in the 4.75 table):
		//   qua_ener_MR122 = log2(g),  qua_ener = 20*log10(g)
		e, f := log2(int32(gCode)) // Log2(x Q12) = log2(x) + 12
		e -= 12
		t1 := shr_r(f, 5)
		t2 := shl(e, 10)
		quaEnerMR122 = add(t1, t2)
		lTmp := mpy32_16(e, f, 24660) // 24660 Q12 ~= 20*log10(2)
		lTmp = L_shl(lTmp, 13)
		quaEner = round_(lTmp)
	default: // MR515, MR59
		p := table_gain_lowrates[index:]
		gainPit = p[0]
		gCode = p[1]
		quaEnerMR122 = p[2]
		quaEner = p[3]
	}

	r := pred.gcPred(mode, code)
	exp, frac := r.expGcode0, r.fracGcode0
	gcode0 := int16(pow2(14, frac))

	lTmp := L_mult(gCode, gcode0)
	lTmp = L_shr(lTmp, 10-exp)
	gainCod = int16(lTmp >> 16)

	pred.update(quaEnerMR122, quaEner)
	return gainPit, gainCod
}
