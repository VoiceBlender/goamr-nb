package amrnb

// Separate pitch/code gain quantizers (MR122; pitch also used by MR795), ported
// from opencore-amrnb q_gain_p.cpp (q_gain_pitch) and q_gain_c.cpp
// (q_gain_code). qGainPitch picks the nearest quantized pitch gain; qGainCode
// picks the code-gain index minimizing the error to the predicted gain. The
// exact encoder counterparts of dGainPitch / dGainCode. Bit-exact with the
// reference.

const (
	cNB_QUA_PITCH = 16
	cNB_QUA_CODE  = 32
)

// qGainPitch quantizes the pitch gain (in place) to the nearest table entry
// within gpLimit, returning the index. For MR795 it also fills three candidate
// gains/indices. Mirrors q_gain_pitch.
func qGainPitch(mode int, gpLimit int16, gain *int16, gainCand, gainCind []int16) int16 {
	errMin := abs_s(sub(*gain, qua_gain_pitch[0]))
	index := int16(0)
	for i := int16(1); i < cNB_QUA_PITCH; i++ {
		if qua_gain_pitch[i] <= gpLimit {
			err := abs_s(sub(*gain, qua_gain_pitch[i]))
			if err < errMin {
				errMin = err
				index = i
			}
		}
	}

	if mode == dMR795 {
		var ii int16
		switch {
		case index == 0:
			ii = index
		case index == cNB_QUA_PITCH-1 || qua_gain_pitch[index+1] > gpLimit:
			ii = index - 2
		default:
			ii = index - 1
		}
		for i := 0; i < 3; i++ {
			gainCind[i] = ii
			gainCand[i] = qua_gain_pitch[ii]
			ii++
		}
		*gain = qua_gain_pitch[index]
	} else if mode == dMR122 {
		*gain = qua_gain_pitch[index] &^ 3
	} else {
		*gain = qua_gain_pitch[index]
	}
	return index
}

// qGainCode quantizes the fixed-codebook gain (in place) against the predicted
// gain, returning the index and the predictor-update energies. Mirrors
// q_gain_code.
func qGainCode(mode int, expGcode0, fracGcode0 int16, gain *int16) (index, quaEnerMR122, quaEner int16) {
	var gQ0 int16
	if mode == dMR122 {
		gQ0 = *gain >> 1 // Q1 -> Q0
	} else {
		gQ0 = *gain
	}

	gcode0 := int16(pow2(expGcode0, fracGcode0))
	if mode == dMR122 {
		gcode0 = shl(gcode0, 4)
	} else {
		gcode0 = shl(gcode0, 5)
	}

	errMin := gQ0 - int16((int32(gcode0)*int32(qua_gain_code[0]))>>15)
	if errMin < 0 {
		errMin = -errMin
	}
	index = 0
	for i := int16(1); i < cNB_QUA_CODE; i++ {
		err := gQ0 - int16((int32(gcode0)*int32(qua_gain_code[i*3]))>>15)
		if err < 0 {
			err = -err
		}
		if err < errMin {
			errMin = err
			index = i
		}
	}

	p := int(index) * 3
	temp := int16((int32(gcode0) * int32(qua_gain_code[p])) >> 15)
	if mode == dMR122 {
		*gain = temp << 1
	} else {
		*gain = temp
	}
	quaEnerMR122 = qua_gain_code[p+1]
	quaEner = qua_gain_code[p+2]
	return
}
