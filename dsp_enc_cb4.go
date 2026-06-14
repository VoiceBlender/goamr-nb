package amrnb

// 4-pulse algebraic codebook search (17 bits, MR74/MR795), ported from
// opencore-amrnb c4_17pf.cpp. A depth-first search over four pulses with dn2
// pruning of the first pulse and cyclic track rotation; the index is Gray-coded
// per position. The exact encoder counterpart of decode4i40_17bits. Bit-exact
// with the reference.

// search4i40 places four pulses maximizing the codebook criterion. Mirrors
// search_4i40 (c4_17pf.cpp).
func search4i40(dn, dn2, rr, codvec []int16) {
	const L = cL_CODE
	var ipos [4]int16
	psk := int16(-1)
	alpk := int16(1)
	codvec[0], codvec[1], codvec[2], codvec[3] = 0, 1, 2, 3

	for track := int16(3); track < 5; track++ {
		ipos[0], ipos[1], ipos[2], ipos[3] = 0, 1, 2, track

		for rot := 0; rot < 4; rot++ {
			for i0 := ipos[0]; i0 < L; i0 += cSTEP {
				if dn2[i0] < 0 {
					continue
				}
				ps0 := dn[i0]
				alp0 := int32(rr[int(i0)*L+int(i0)]) << 14

				sq := int16(-1)
				alp := int16(1)
				ps := int16(0)
				ix := ipos[1]
				for i1 := ipos[1]; i1 < L; i1 += cSTEP {
					ps1 := ps0 + dn[i1]
					alp1 := alp0 + (int32(rr[int(i1)*L+int(i1)]) << 14)
					alp1 += int32(rr[int(i0)*L+int(i1)]) << 15
					sq1 := int16((int32(ps1) * int32(ps1)) >> 15)
					alp16 := int16((alp1 + 0x8000) >> 16)
					s := (int32(alp)*int32(sq1))<<1 - (int32(sq)*int32(alp16))<<1
					if s > 0 {
						sq, ps, alp, ix = sq1, ps1, alp16, i1
					}
				}
				i1 := ix

				ps0 = ps
				alp0 = int32(alp) << 14
				sq, alp, ps, ix = -1, 1, 0, ipos[2]
				for i2 := ipos[2]; i2 < L; i2 += cSTEP {
					ps1 := ps0 + dn[i2]
					alp1 := alp0 + (int32(rr[int(i2)*L+int(i2)]) << 12)
					alp1 += int32(rr[int(i1)*L+int(i2)]) << 13
					alp1 += int32(rr[int(i0)*L+int(i2)]) << 13
					sq1 := int16((int32(ps1) * int32(ps1)) >> 15)
					alp16 := int16((alp1 + 0x8000) >> 16)
					s := (int32(alp)*int32(sq1))<<1 - (int32(sq)*int32(alp16))<<1
					if s > 0 {
						sq, ps, alp, ix = sq1, ps1, alp16, i2
					}
				}
				i2 := ix

				ps0 = ps
				alp0 = int32(alp) << 16
				sq, alp, ps, ix = -1, 1, 0, ipos[3]
				for i3 := ipos[3]; i3 < L; i3 += cSTEP {
					ps1 := ps0 + dn[i3]
					alp1 := alp0 + (int32(rr[int(i3)*L+int(i3)]) << 12)
					alp1 += int32(rr[int(i2)*L+int(i3)]) << 13
					alp1 += int32(rr[int(i1)*L+int(i3)]) << 13
					alp1 += int32(rr[int(i0)*L+int(i3)]) << 13
					sq1 := int16((int32(ps1) * int32(ps1)) >> 15)
					alp16 := int16((alp1 + 0x8000) >> 16)
					s := (int32(alp)*int32(sq1))<<1 - (int32(sq)*int32(alp16))<<1
					if s > 0 {
						sq, ps, alp, ix = sq1, ps1, alp16, i3
					}
				}

				s := (int32(alpk)*int32(sq))<<1 - (int32(psk)*int32(alp))<<1
				if s > 0 {
					psk = sq
					alpk = alp
					codvec[0], codvec[1], codvec[2], codvec[3] = i0, i1, i2, ix
				}
			}
			pos := ipos[3]
			ipos[3] = ipos[2]
			ipos[2] = ipos[1]
			ipos[1] = ipos[0]
			ipos[0] = pos
		}
	}
}

// build4Code assembles the 4-pulse innovation, filtered code, signs and the
// Gray-coded 17-bit index. Mirrors build_code (c4_17pf.cpp).
func build4Code(codvec, dnSign, cod, h, y []int16) (indx, sign int16) {
	const L = cL_CODE
	for i := 0; i < L; i++ {
		cod[i] = 0
	}

	var sgn [4]int16
	rsign := int16(0)
	for k := 0; k < 4; k++ {
		i := codvec[k]
		j := dnSign[i]
		index := int16((int32(i) * 6554) >> 15)
		track := i - index*5
		index = gray[index]
		switch track {
		case 1:
			index <<= 3
		case 2:
			index <<= 6
		case 3:
			index <<= 10
		case 4:
			track = 3
			index = (index << 10) + 512
		}
		if j > 0 {
			cod[i] = 8191
			sgn[k] = 32767
			track = 1 << uint(track)
			rsign += track
		} else {
			cod[i] = -8192
			sgn[k] = -32768
		}
		indx += index
	}
	sign = rsign

	c := [4]int{int(codvec[0]), int(codvec[1]), int(codvec[2]), int(codvec[3])}
	for i := 0; i < L; i++ {
		var s int32
		for k := 0; k < 4; k++ {
			if i-c[k] >= 0 {
				s = L_mac(s, h[i-c[k]], sgn[k])
			}
		}
		y[i] = round_(s)
	}
	return indx, sign
}

// code4i40_17bits runs the full MR74/MR795 4-pulse search. Mirrors
// code_4i40_17bits.
func code4i40_17bits(x, h []int16, T0, pitchSharp int16, code, y []int16) (index, sign int16) {
	sharp := shl(pitchSharp, 1)
	if T0 < cL_CODE {
		for i := T0; i < cL_CODE; i++ {
			h[i] = add(h[i], mult(h[i-T0], sharp))
		}
	}

	var dn, dn2, dnSign [cL_CODE]int16
	var rr [cL_CODE * cL_CODE]int16
	var codvec [4]int16

	corHX(h, x, dn[:], 1)
	setSign(dn[:], dnSign[:], dn2[:], 4)
	corH(h, dnSign[:], rr[:])
	search4i40(dn[:], dn2[:], rr[:], codvec[:])
	index, sign = build4Code(codvec[:], dnSign[:], code, h, y)

	if T0 < cL_CODE {
		for i := T0; i < cL_CODE; i++ {
			code[i] = add(code[i], mult(code[i-T0], sharp))
		}
	}
	return index, sign
}
