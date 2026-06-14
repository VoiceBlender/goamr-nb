package amrnb

// Gain quantization (general modes), ported from opencore-amrnb qua_gain.cpp
// (Qua_gain) and calc_en.cpp (calc_filt_energies). calcFiltEnergies forms the
// five quadratic-form energy coefficients of the gain error from the target,
// the adaptive codebook y1 and the filtered innovation y2; quaGain searches the
// gain VQ table for the (pitch, code) gain pair minimizing that error, returning
// the index and the dequantized gains — the exact encoder counterpart of
// decGain. Bit-exact with the reference.

const (
	cVQ_SIZE_HIGHRATES = 128
	cVQ_SIZE_LOWRATES  = 64
)

// calcFiltEnergies computes the five energy coefficients (frac/exp) of the gain
// MSE. gCoeff[0..3] are the <xn,y1>/<y1,y1> correlations from gPitch. For
// MR795/MR475 it also returns the optimum unconstrained code gain. Mirrors
// calc_filt_energies.
func calcFiltEnergies(mode int, xn, xn2, y1, y2, gCoeff []int16) (fracCoeff, expCoeff [5]int16, codGainFrac, codGainExp int16) {
	fracCoeff[0] = gCoeff[0]
	expCoeff[0] = gCoeff[1]
	fracCoeff[1] = negate(gCoeff[2])
	expCoeff[1] = gCoeff[3] + 1

	var s1, s2, s3 int32
	if mode != dMR795 && mode != dMR475 {
		s1, s2, s3 = 1, 1, 1
	}
	var scaledY2 [cL_SUBFR]int16
	for i := 0; i < cL_SUBFR; i++ {
		tmp := y2[i] >> 3
		scaledY2[i] = tmp
		s1 = L_mac(s1, tmp, tmp)
		s2 = L_mac(s2, xn[i], tmp)
		s3 = L_mac(s3, y1[i], tmp)
	}
	exp := norm_l(s1)
	fracCoeff[2] = int16(L_shl(s1, exp) >> 16)
	expCoeff[2] = -3 - exp
	exp = norm_l(s2)
	fracCoeff[3] = negate(int16(L_shl(s2, exp) >> 16))
	expCoeff[3] = 7 - exp
	exp = norm_l(s3)
	fracCoeff[4] = int16(L_shl(s3, exp) >> 16)
	expCoeff[4] = 7 - exp

	if mode == dMR795 || mode == dMR475 {
		s1 = 0
		for i := 0; i < cL_SUBFR; i++ {
			s1 += int32(xn2[i]) * int32(scaledY2[i])
		}
		s1 <<= 1
		exp = norm_l(s1)
		frac := int16(L_shl(s1, exp) >> 16)
		exp = 6 - exp
		if frac <= 0 {
			codGainFrac = 0
			codGainExp = 0
		} else {
			codGainFrac = div_s(shr(frac, 1), fracCoeff[2])
			codGainExp = (exp - expCoeff[2]) - 14
		}
	}
	return
}

// quaGain searches the mode's gain VQ table for the (pitch, code) gain pair
// minimizing the weighted gain MSE, returning the index and dequantized gains
// plus the predictor-update energies. Mirrors Qua_gain.
func quaGain(mode int, expGcode0, fracGcode0 int16, fracCoeff, expCoeff []int16, gpLimit int16) (index, gainPit, gainCod, quaEnerMR122, quaEner int16) {
	var tableGain []int16
	var tableLen int
	if mode == dMR102 || mode == dMR74 || mode == dMR67 {
		tableLen = cVQ_SIZE_HIGHRATES
		tableGain = table_gain_highrates
	} else {
		tableLen = cVQ_SIZE_LOWRATES
		tableGain = table_gain_lowrates
	}

	gcode0 := int16(pow2(14, fracGcode0))
	expCode := expGcode0 - 11

	var expMax [5]int16
	expMax[0] = expCoeff[0] - 13
	expMax[1] = expCoeff[1] - 14
	temp := shl(expCode, 1) + 15
	expMax[2] = add(expCoeff[2], temp)
	expMax[3] = add(expCoeff[3], expCode)
	expMax[4] = add(expCoeff[4], expCode+1)

	eMax := expMax[0]
	for i := 1; i < 5; i++ {
		if expMax[i] > eMax {
			eMax = expMax[i]
		}
	}
	eMax++

	var coeff, coeffLo [5]int16
	for i := 0; i < 5; i++ {
		j := eMax - expMax[i]
		coeff[i], coeffLo[i] = L_Extract(L_shr(int32(fracCoeff[i])<<16, j))
	}

	distMin := int32(maxInt32)
	for i := 0; i < tableLen; i++ {
		p := i << 2
		gPitch := tableGain[p]
		gCode := tableGain[p+1]
		if gPitch <= gpLimit {
			gc := mult(gCode, gcode0)
			g2Pitch := mult(gPitch, gPitch)
			g2Code := mult(gc, gc)
			gPitCod := mult(gc, gPitch)
			L_tmp := mpy32_16(coeff[0], coeffLo[0], g2Pitch)
			L_tmp = L_add(L_tmp, mpy32_16(coeff[1], coeffLo[1], gPitch))
			L_tmp = L_add(L_tmp, mpy32_16(coeff[2], coeffLo[2], g2Code))
			L_tmp = L_add(L_tmp, mpy32_16(coeff[3], coeffLo[3], gc))
			L_tmp = L_add(L_tmp, mpy32_16(coeff[4], coeffLo[4], gPitCod))
			if L_tmp < distMin {
				distMin = L_tmp
				index = int16(i)
			}
		}
	}

	p := int(index) << 2
	gainPit = tableGain[p]
	gCode := tableGain[p+1]
	quaEnerMR122 = tableGain[p+2]
	quaEner = tableGain[p+3]

	L_tmp := L_shr(L_mult(gCode, gcode0), 10-expGcode0)
	gainCod = int16(L_tmp >> 16)
	return
}
