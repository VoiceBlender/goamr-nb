package amrnb

// Per-mode algebraic (fixed) codebook decoders, ported from opencore-amrnb
// d2_9pf.cpp, d2_11pf.cpp, d3_14pf.cpp, d4_17pf.cpp, d8_31pf.cpp and
// d1035pf.cpp. Each reconstructs the 40-sample innovation vector cod[] from a
// mode's pulse-position and sign indices (GSM-style tracks, Gray-coded for the
// 4/8/10-pulse modes). Pulse amplitudes are ±8191/∓8192 (or ±4096 for MR122),
// matching the reference exactly.

// pulse amplitude for the ±1.0 codebook entries (MR122 uses ±4096).
const cPOS_CODE = 8191

// decode2i40_9bits decodes the 2-pulse codebook for MR475/MR515. Mirrors
// decode_2i40_9bits (d2_9pf.cpp); subNr is the subframe number.
func decode2i40_9bits(subNr, sign, index int16, cod []int16) {
	var pos [2]int16

	j := index & 64
	j >>= 3
	i := index & 7
	k := shl(subNr, 1)
	k += j
	pos[0] = i*5 + startPos[k]
	k++

	index >>= 3
	i = index & 7
	pos[1] = i*5 + startPos[k]

	for n := 0; n < cL_SUBFR; n++ {
		cod[n] = 0
	}
	for j := 0; j < 2; j++ {
		s := sign & 0x1
		cod[pos[j]] = s*16383 - 8192
		sign >>= 1
	}
}

// decode2i40_11bits decodes the 2-pulse codebook for MR59. Mirrors
// decode_2i40_11bits (d2_11pf.cpp).
func decode2i40_11bits(sign, index int16, cod []int16) {
	var pos [2]int16

	j := index & 0x1
	index >>= 1
	i := index & 0x7
	pos[0] = i*5 + j*2 + 1

	index >>= 3
	j = index & 0x3
	index >>= 2
	i = index & 0x7
	if j == 3 {
		pos[1] = i*5 + 4
	} else {
		pos[1] = i*5 + j
	}

	for n := 0; n < cL_SUBFR; n++ {
		cod[n] = 0
	}
	for j := 0; j < 2; j++ {
		s := sign & 1
		cod[pos[j]] = s*16383 - 8192
		sign >>= 1
	}
}

// decode3i40_14bits decodes the 3-pulse codebook for MR67. Mirrors
// decode_3i40_14bits (d3_14pf.cpp).
func decode3i40_14bits(sign, index int16, cod []int16) {
	var pos [3]int16

	i := index & 0x7
	pos[0] = i * 5

	index >>= 3
	j := index & 0x1
	index >>= 1
	i = index & 0x7
	pos[1] = i*5 + j*2 + 1

	index >>= 3
	j = index & 0x1
	index >>= 1
	i = index & 0x7
	pos[2] = i*5 + j*2 + 2

	for n := 0; n < cL_SUBFR; n++ {
		cod[n] = 0
	}
	for j := 0; j < 3; j++ {
		s := sign & 1
		cod[pos[j]] = s*16383 - 8192
		sign >>= 1
	}
}

// decode4i40_17bits decodes the 4-pulse Gray-coded codebook for MR74/MR795.
// Mirrors decode_4i40_17bits (d4_17pf.cpp).
func decode4i40_17bits(sign, index int16, cod []int16) {
	var pos [4]int16

	i := index & 0x7
	i = dgray[i]
	pos[0] = i * 5

	index >>= 3
	i = index & 0x7
	i = dgray[i]
	pos[1] = i*5 + 1

	index >>= 3
	i = index & 0x7
	i = dgray[i]
	pos[2] = i*5 + 2

	index >>= 3
	j := index & 0x1
	index >>= 1
	i = index & 0x7
	i = dgray[i]
	pos[3] = i*5 + 3 + j

	for n := 0; n < cL_SUBFR; n++ {
		cod[n] = 0
	}
	for j := 0; j < 4; j++ {
		s := sign & 0x1
		cod[pos[j]] = s*16383 - 8192
		sign >>= 1
	}
}

// decompress10 expands one compressed 10x10x10 track index into three pulse
// positions. Mirrors decompress10 (d8_31pf.cpp).
func decompress10(MSBs, LSBs, index1, index2, index3 int16, posIndx []int16) {
	if MSBs > 124 {
		MSBs = 124
	}
	ia := mult(MSBs, 1311)
	ia = int16(int32(MSBs) - (L_mult(ia, 25) >> 1))
	ib := mult(ia, 6554)
	ib = ia - int16(L_mult(ib, 5)>>1)
	ib = shl(ib, 1)
	ic := LSBs - ((LSBs >> 2) << 2)
	posIndx[index1] = ib + (ic & 1)

	ib = mult(ia, 6554)
	ib = shl(ib, 1)
	posIndx[index2] = ib + (ic >> 1)

	ib = LSBs >> 2
	ic = mult(MSBs, 1311)
	ic = shl(ic, 1)
	posIndx[index3] = add(ib, ic)
}

// decompressCode expands the eight compressed MR102 pulse indices into linear
// signs and codeword positions. Mirrors decompress_code (d8_31pf.cpp).
func decompressCode(indx, signIndx, posIndx []int16) {
	const nbTrack = 4
	for i := 0; i < nbTrack; i++ {
		signIndx[i] = indx[i]
	}

	MSBs := indx[nbTrack] >> 3
	LSBs := indx[nbTrack] & 0x7
	decompress10(MSBs, LSBs, 0, 4, 1, posIndx)

	MSBs = indx[nbTrack+1] >> 3
	LSBs = indx[nbTrack+1] & 0x7
	decompress10(MSBs, LSBs, 2, 6, 5, posIndx)

	MSBs = indx[nbTrack+2] >> 2
	LSBs = indx[nbTrack+2] & 0x3
	ia := int16(L_mult(MSBs, 25) >> 1)
	ia += 12
	MSBs0_24 := ia >> 5

	ia = mult(MSBs0_24, 6554)
	ia &= 1
	ib := mult(MSBs0_24, 6554)
	ib = MSBs0_24 - int16(L_mult(ib, 5)>>1)
	if ia == 1 {
		ib = 4 - ib
	}
	ib = shl(ib, 1)
	ia = LSBs & 0x1
	posIndx[3] = add(ib, ia)

	ia = mult(MSBs0_24, 6554)
	ia = shl(ia, 1)
	posIndx[7] = ia + (LSBs >> 1)
}

// dec8i40_31bits decodes the 8-pulse codebook for MR102. Mirrors
// dec_8i40_31bits (d8_31pf.cpp).
func dec8i40_31bits(index, cod []int16) {
	const nbTrack = 4
	var linearSigns [nbTrack]int16
	var linearCodewords [8]int16

	for i := 0; i < cL_CODE; i++ {
		cod[i] = 0
	}
	decompressCode(index, linearSigns[:], linearCodewords[:])

	for j := int16(0); j < nbTrack; j++ {
		pos1 := (linearCodewords[j] << 2) + j
		var sign int16
		if linearSigns[j] == 0 {
			sign = cPOS_CODE // +1.0
		} else {
			sign = -cPOS_CODE // -1.0
		}
		if pos1 < cL_SUBFR {
			cod[pos1] = sign
		}
		pos2 := (linearCodewords[j+4] << 2) + j
		if pos2 < pos1 {
			sign = negate(sign)
		}
		if pos2 < cL_SUBFR {
			cod[pos2] += sign
		}
	}
}

// dec10i40_35bits decodes the 10-pulse Gray-coded codebook for MR122. Mirrors
// dec_10i40_35bits (d1035pf.cpp).
func dec10i40_35bits(index, cod []int16) {
	const nbTrack = 5
	for i := 0; i < cL_CODE; i++ {
		cod[i] = 0
	}
	for j := int16(0); j < nbTrack; j++ {
		tmp := index[j]
		i := tmp & 7
		i = dgray[i]
		i = i * 5
		pos1 := i + j

		var sign int16
		if (tmp>>3)&1 == 0 {
			sign = 4096 // +1.0
		} else {
			sign = -4096 // -1.0
		}
		cod[pos1] = sign

		i = index[j+5] & 7
		i = dgray[i]
		i = i * 5
		pos2 := i + j
		if pos2 < pos1 {
			sign = negate(sign)
		}
		cod[pos2] += sign
	}
}
