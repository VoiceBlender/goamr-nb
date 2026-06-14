package amrnb

// Adaptive-codebook (pitch) reconstruction, ported from opencore-amrnb
// pred_lt.cpp, dec_lag3.cpp and dec_lag6.cpp. predLt interpolates the past
// excitation at a fractional pitch lag to build this subframe's adaptive
// codebook contribution; decLag3/decLag6 turn the received pitch index into an
// integer lag T0 and a fractional part. Bit-exact with the reference.

const (
	cL_INTER10   = 10 // interpolation FIR half-context (L_INTERPOL-1)
	cUP_SAMP_MAX = 6  // fractional pitch upsampling factor
)

// predLt computes the interpolated past excitation for the current subframe in
// place: exc[pos : pos+Lsubfr] is overwritten using exc[pos-T0 ...] convolved
// with the 1/6-resolution FIR selected by frac. For short lags (T0 < Lsubfr)
// the loop deliberately reads samples it just wrote, matching the reference.
// Mirrors Pred_lt_3or6.
func predLt(exc []int16, pos int, T0, frac, Lsubfr, flag3 int16) {
	var coeff [2 * cL_INTER10]int16

	pX0 := pos - int(T0)
	frac = -frac
	if flag3 != 0 {
		frac <<= 1 // inter_3l[k] = inter_6[2k]
	}
	if frac < 0 {
		frac += cUP_SAMP_MAX
		pX0--
	}
	f := int(frac)

	k := 0
	ci := 0
	for i := cL_INTER10 >> 1; i > 0; i-- {
		coeff[ci] = inter_6_pred_lt[f+k]
		coeff[ci+1] = inter_6_pred_lt[(cUP_SAMP_MAX-f)+k]
		ci += 2
		k += cUP_SAMP_MAX
		coeff[ci] = inter_6_pred_lt[f+k]
		coeff[ci+1] = inter_6_pred_lt[(cUP_SAMP_MAX-f)+k]
		ci += 2
		k += cUP_SAMP_MAX
	}

	pExc := pos
	for j := int(Lsubfr) >> 1; j != 0; j-- {
		pX0++
		ix2 := pX0
		ix3 := pX0
		pX0++

		ic := 0
		s1 := int32(0x4000)
		s2 := int32(0x4000)
		for i := cL_INTER10 >> 1; i > 0; i-- {
			s2 += int32(exc[ix3]) * int32(coeff[ic])
			ix3--
			s1 += int32(exc[ix3]) * int32(coeff[ic])
			ic++
			s1 += int32(exc[ix2]) * int32(coeff[ic])
			ix2++
			s2 += int32(exc[ix2]) * int32(coeff[ic])
			ic++
			s2 += int32(exc[ix3]) * int32(coeff[ic])
			ix3--
			s1 += int32(exc[ix3]) * int32(coeff[ic])
			ic++
			s1 += int32(exc[ix2]) * int32(coeff[ic])
			ix2++
			s2 += int32(exc[ix2]) * int32(coeff[ic])
			ic++
		}
		exc[pExc] = int16(s1 >> 15)
		pExc++
		exc[pExc] = int16(s2 >> 15)
		pExc++
	}
}

// decLag3 decodes the 1/3-resolution pitch lag (used by most modes) from the
// received index into an integer lag T0 and fractional part T0frac. i_subfr==0
// marks the 1st/3rd subframe (absolute coding); otherwise the lag is coded
// relative to t0_min (or, with flag4, to T0prev). Mirrors Dec_lag3.
func decLag3(index, t0Min, t0Max, iSubfr, T0prev, flag4 int16) (T0, T0frac int16) {
	var i, tmpLag int16
	if iSubfr == 0 { // 1st or 3rd subframe
		if index < 197 {
			tmpLag = index + 2
			tmpLag = mult(tmpLag, 10923)
			i = tmpLag + 19
			T0 = i
			i <<= 1
			i += T0
			tmpLag = index - i
			T0frac = tmpLag + 58
		} else {
			T0 = index - 112
			T0frac = 0
		}
	} else { // 2nd or 4th subframe
		if flag4 == 0 {
			i = index + 2
			i = int16((int32(i) * 10923) >> 15)
			i -= 1
			T0 = i + t0Min
			i = i + (i << 1)
			tmpLag = index - 2
			T0frac = tmpLag - i
		} else {
			tmpLag = T0prev
			i = sub(tmpLag, t0Min)
			if i > 5 {
				tmpLag = t0Min + 5
			}
			i = t0Max - tmpLag
			if i > 4 {
				tmpLag = t0Max - 4
			}
			if index < 4 {
				i = tmpLag - 5
				T0 = i + index
				T0frac = 0
			} else if index < 12 {
				i = index - 5
				i = int16((int32(i) * 10923) >> 15)
				i--
				T0 = i + tmpLag
				i = i + (i << 1)
				tmpLag = index - 9
				T0frac = tmpLag - i
			} else {
				i = index - 12
				i = i + tmpLag
				T0 = i + 1
				T0frac = 0
			}
		}
	}
	return T0, T0frac
}

// decLag6 decodes the 1/6-resolution pitch lag (MR122) from the received index.
// T0in supplies the previous subframe's integer lag for the relative (2nd/4th)
// subframe path. Mirrors Dec_lag6.
func decLag6(index, pitMin, pitMax, iSubfr, T0in int16) (T0, T0frac int16) {
	var i, T0min, T0max, k int16
	if iSubfr == 0 { // 1st or 3rd subframe
		if index < 463 {
			i = index + 5
			i = int16((int32(i) * 5462) >> 15)
			i += 17
			T0 = i
			i <<= 1
			i += T0
			i <<= 1
			i = index - i
			T0frac = i + 105
		} else {
			T0 = index - 368
			T0frac = 0
		}
	} else { // 2nd or 4th subframe
		T0min = T0in - 5
		if T0min < pitMin {
			T0min = pitMin
		}
		T0max = T0min + 9
		if T0max > pitMax {
			T0max = pitMax
			T0min = T0max - 9
		}
		i = index + 5
		i = int16((int32(i) * 5462) >> 15)
		i -= 1
		T0 = i + T0min
		i = i + (i << 1)
		i <<= 1
		k = index - 3
		T0frac = k - i
	}
	return T0, T0frac
}
