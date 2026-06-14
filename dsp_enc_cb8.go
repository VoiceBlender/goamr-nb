package amrnb

// 8-pulse algebraic codebook search (31 bits, MR102), ported from opencore-amrnb
// c8_31pf.cpp. Drives search10and8i40 (8 pulses, step 4), then builds the
// codeword and compresses the eight pulse positions / four track signs into the
// 7-value transmitted index. The exact encoder counterpart of dec8i40_31bits
// (decompressCode). Bit-exact with the reference.

// build8Code assembles the 8-pulse innovation cod and filtered code y, and the
// linear track signs / pulse positions (with the same-track ordering/swap logic
// that decompressCode inverts). Mirrors build_code (c8_31pf.cpp).
func build8Code(codvec, sign, cod, h, y []int16) (signIndx [4]int16, posIndx [8]int16) {
	const L = cL_CODE
	for i := 0; i < L; i++ {
		cod[i] = 0
	}
	for t := 0; t < 4; t++ {
		posIndx[t] = -1
		signIndx[t] = -1
	}

	var sgn [8]int16
	for k := 0; k < 8; k++ {
		i := codvec[k]
		j := sign[i]
		posIndex := i >> 2
		track := i & 3

		var signIndex int16
		if j > 0 {
			cod[i] = cod[i] + 8191
			sgn[k] = 32767
			signIndex = 0
		} else {
			cod[i] = cod[i] - 8191
			sgn[k] = -32768
			signIndex = 1
		}

		if posIndx[track] < 0 {
			posIndx[track] = posIndex
			signIndx[track] = signIndex
		} else if (signIndex^signIndx[track])&1 == 0 {
			if posIndx[track] <= posIndex {
				posIndx[track+4] = posIndex
			} else {
				posIndx[track+4] = posIndx[track]
				posIndx[track] = posIndex
				signIndx[track] = signIndex
			}
		} else {
			if posIndx[track] <= posIndex {
				posIndx[track+4] = posIndx[track]
				posIndx[track] = posIndex
				signIndx[track] = signIndex
			} else {
				posIndx[track+4] = posIndex
			}
		}
	}

	for i := 0; i < L; i++ {
		var s int32
		for k := 0; k < 8; k++ {
			ci := int(codvec[k])
			if i-ci >= 0 {
				s = L_mac(s, h[i-ci], sgn[k])
			}
		}
		y[i] = round_(s)
	}
	return signIndx, posIndx
}

// compress10 packs three position indices into one 10-bit value. Mirrors
// compress10 (the inverse of decompress10).
func compress10(A, B, C int16) int16 {
	ia := A >> 1
	ib := (B >> 1) * 5
	ic := (C >> 1) * 25
	indx := (ib + ic + ia) << 3
	indx += ((B & 1) << 1) + ((C & 1) << 2) + (A & 1)
	return indx
}

// compressCode packs the four track signs and eight pulse positions into the
// seven transmitted index values. Mirrors compress_code.
func compressCode(signIndx, posIndx, indx []int16) {
	for i := 0; i < 4; i++ {
		indx[i] = signIndx[i]
	}
	indx[4] = compress10(posIndx[0], posIndx[4], posIndx[1])
	indx[5] = compress10(posIndx[2], posIndx[6], posIndx[5])

	ib := (posIndx[7] >> 1) & 1
	ia := posIndx[3] >> 1
	if ib == 1 {
		ia = 4 - ia
	}
	ib = (posIndx[7] >> 1) * 5
	ib += ia
	ib <<= 5
	ib += 12
	ic := int16((int32(ib) * 1311) >> 15)
	ic <<= 2
	ia = posIndx[3] & 1
	ib = (posIndx[7] & 1) << 1
	indx[6] = ib + ic + ia
}

// code8i40_31bits runs the full MR102 8-pulse search, writing the innovation
// cod, filtered code y, and the 7-value index. Mirrors code_8i40_31bits.
func code8i40_31bits(x, cn, h, cod, y, indx []int16) {
	var dn, sign [cL_CODE]int16
	var rr [cL_CODE * cL_CODE]int16
	var ipos, codvec [8]int16
	var posMax [4]int16

	corHX2(h, x, dn[:], 2, 4, 4)
	setSign12k2(dn[:], cn, sign[:], posMax[:], 4, ipos[:], 4)
	corH(h, sign[:], rr[:])
	search10and8i40(8, 4, 4, dn[:], rr[:], ipos[:], posMax[:], codvec[:])
	signIndx, posIndx := build8Code(codvec[:], sign[:], cod, h, y)
	compressCode(signIndx[:], posIndx[:], indx)
}
