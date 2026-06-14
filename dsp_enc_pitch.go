package amrnb

// Pitch-lag encoding, ported from opencore-amrnb enc_lag3.cpp and enc_lag6.cpp.
// encLag3/encLag6 map a (T0, T0_frac) closed-loop pitch lag to the index the
// bitstream carries — the exact inverses of the decoder's decLag3/decLag6.
// Bit-exact with the reference.

// encLag3 encodes a 1/3-resolution pitch lag. deltaFlag==0 selects the 1st/3rd
// subframe (absolute); otherwise the lag is coded relative to T0_min (or, with
// flag4, the 4-bit scheme). Mirrors Enc_lag3.
func encLag3(T0, T0frac, T0prev, T0min, T0max, deltaFlag, flag4 int16) int16 {
	var index, i, tmpInd, uplag, tmpLag, temp1, temp2 int16

	if deltaFlag == 0 { // 1st or 3rd subframe
		if T0-85 <= 0 {
			index = (T0 << 1) + T0 - 58 + T0frac
		} else {
			index = T0 + 112
		}
	} else { // 2nd or 4th subframe
		if flag4 == 0 {
			i = T0 - T0min
			index = i + (i << 1) + 2 + T0frac
		} else {
			tmpLag = T0prev
			temp1 = tmpLag - T0min
			temp2 = temp1 - 5
			if temp2 > 0 {
				tmpLag = T0min + 5
			}
			temp1 = T0max - tmpLag
			temp2 = temp1 - 4
			if temp2 > 0 {
				tmpLag = T0max - 4
			}
			uplag = T0 + (T0 << 1) + T0frac
			i = tmpLag - 2
			tmpInd = i + (i << 1)
			temp1 = tmpInd - uplag
			if temp1 >= 0 {
				index = T0 - tmpLag + 5
			} else {
				i = tmpLag + 1
				i += i << 1
				if i > uplag {
					index = uplag - tmpInd + 3
				} else {
					index = T0 - tmpLag + 11
				}
			}
		}
	}
	return index
}

// gPitch computes the optimal adaptive-codebook (pitch) gain in Q14, the ratio
// <xn,y1>/<y1,y1> saturated to 1.2, where xn is the pitch target and y1 the
// filtered adaptive excitation. It also returns the normalized correlations in
// gCoeff for later gain quantization. Mirrors G_pitch (g_pitch.cpp).
func gPitch(mode int, xn, y1, gCoeff []int16, Lsubfr int) int16 {
	// <y1,y1>
	var s int32
	for i := 0; i < Lsubfr; i++ {
		s += int32(y1[i]) * int32(y1[i])
	}
	var yy, expYy int16
	if s >= 0 && s < 0x40000000 {
		s = (s << 1) + 1
		expYy = norm_l(s)
		yy = round_(s << uint(expYy))
	} else {
		s = 0
		for i := 0; i < Lsubfr; i++ {
			tmp := y1[i] >> 2
			s += int32(tmp) * int32(tmp)
		}
		s = (s << 1) + 1
		expYy = norm_l(s)
		yy = round_(s << uint(expYy))
		expYy -= 4
	}

	// <xn,y1> with overflow detection
	s = 0
	overflow := false
	for i := 0; i < Lsubfr; i++ {
		L := int32(xn[i]) * int32(y1[i])
		s1 := s
		s = s1 + L
		if (s1^L) > 0 && (s1^s) < 0 {
			overflow = true
			break
		}
	}
	var xy, expXy int16
	if !overflow {
		s = (s << 1) + 1
		expXy = norm_l(s)
		xy = round_(s << uint(expXy))
	} else {
		s = 0
		for i := 0; i < Lsubfr; i++ {
			L := int32(y1[i] >> 2)
			s += int32(xn[i]) * L
		}
		s = (s << 1) + 1
		expXy = norm_l(s)
		xy = round_(s << uint(expXy))
		expXy -= 4
	}

	gCoeff[0] = yy
	gCoeff[1] = 15 - expYy
	gCoeff[2] = xy
	gCoeff[3] = 15 - expXy

	if xy < 4 {
		return 0
	}
	xy >>= 1
	gain := div_s(xy, yy)
	gain = shr(gain, expXy-expYy)
	if gain > 19661 {
		gain = 19661
	}
	if mode == dMR122 {
		gain &^= 3
	}
	return gain
}

// encLag6 encodes a 1/6-resolution pitch lag (MR122). deltaFlag==0 selects the
// 1st/3rd subframe. Mirrors Enc_lag6.
func encLag6(T0, T0frac, T0min, deltaFlag int16) int16 {
	var index, i int16
	if deltaFlag == 0 {
		if T0 <= 94 {
			i = (T0 << 3) - (T0 << 1) - 105
			index = i + T0frac
		} else {
			index = T0 + 368
		}
	} else {
		temp := T0 - T0min
		i = (temp << 3) - (temp << 1) + 3
		index = i + T0frac
	}
	return index
}

// gCode computes the optimum innovation (fixed-codebook) gain
// <xn2,y2>/<y2,y2> in Q1, used by the MR122 gain quantizer. Mirrors G_code.
func gCode(xn2, y2 []int16) int16 {
	var s int32
	for i := 0; i < cL_SUBFR; i++ {
		s += int32(xn2[i]) * int32(y2[i]>>1)
	}
	s <<= 1
	expXy := norm_l(s + 1) // avoid the all-zeros case
	var xy int16
	if expXy < 17 {
		xy = int16(s >> uint(17-expXy))
	} else {
		xy = int16(s << uint(expXy-17))
	}
	if xy <= 0 {
		return 0
	}

	s = 0
	for i := 0; i < cL_SUBFR; i++ {
		temp := y2[i] >> 1
		s += (int32(temp) * int32(temp)) >> 2
	}
	s <<= 3
	expYy := norm_l(s)
	var yy int16
	if expYy < 16 {
		yy = int16(s >> uint(16-expYy))
	} else {
		yy = int16(s << uint(expYy-16))
	}

	gain := div_s(xy, yy)
	i := expXy + 5 - expYy // 15-1+9-18 = 5
	if i > 1 {
		gain >>= uint(i - 1)
	} else {
		gain <<= uint(1 - i)
	}
	return gain
}
