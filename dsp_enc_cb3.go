package amrnb

// 3-pulse algebraic codebook search (14 bits, MR67), ported from opencore-amrnb
// c3_14pf.cpp. A depth-first search: the first pulse ranges only over the
// pruned candidate positions (dn2), then nested searches place the 2nd and 3rd
// pulses; the track triple is cyclically rotated so each track gets the pruned
// role. The exact encoder counterpart of decode3i40_14bits. Bit-exact with the
// reference.

// search3i40 places three pulses maximizing the codebook criterion. Mirrors
// search_3i40 (c3_14pf.cpp).
func search3i40(dn, dn2, rr, codvec []int16) {
	const L = cL_CODE
	var ipos [3]int16
	psk := int16(-1)
	alpk := int16(1)
	codvec[0], codvec[1], codvec[2] = 0, 1, 2

	for track1 := int16(1); track1 < 4; track1 += 2 {
		for track2 := int16(2); track2 < 5; track2 += 2 {
			ipos[0], ipos[1], ipos[2] = 0, track1, track2

			for rot := 0; rot < 3; rot++ {
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
							sq = sq1
							ps = ps1
							alp = alp16
							ix = i1
						}
					}
					i1 := ix

					ps0 = ps
					alp0 = int32(alp) << 14
					sq = -1
					alp = 1
					ps = 0
					ix = ipos[2]
					for i2 := ipos[2]; i2 < L; i2 += cSTEP {
						ps1 := ps0 + dn[i2]
						alp1 := alp0 + (int32(rr[int(i2)*L+int(i2)]) << 12)
						alp1 += int32(rr[int(i1)*L+int(i2)]) << 13
						alp1 += int32(rr[int(i0)*L+int(i2)]) << 13
						sq1 := int16((int32(ps1) * int32(ps1)) >> 15)
						alp16 := int16((alp1 + 0x8000) >> 16)
						s := (int32(alp)*int32(sq1))<<1 - (int32(sq)*int32(alp16))<<1
						if s > 0 {
							sq = sq1
							ps = ps1
							alp = alp16
							ix = i2
						}
					}
					i2 := ix

					s := L_msu(L_mult(alpk, sq), psk, alp)
					if s > 0 {
						psk = sq
						alpk = alp
						codvec[0], codvec[1], codvec[2] = i0, i1, i2
					}
				}
				pos := ipos[2]
				ipos[2] = ipos[1]
				ipos[1] = ipos[0]
				ipos[0] = pos
			}
		}
	}
}

// build3Code assembles the 3-pulse innovation, filtered code, signs and 14-bit
// index. Mirrors build_code (c3_14pf.cpp).
func build3Code(codvec, dnSign, cod, h, y []int16) (indx, sign int16) {
	const L = cL_CODE
	for i := 0; i < L; i++ {
		cod[i] = 0
	}

	var sgn [3]int16
	rsign := int16(0)
	for k := 0; k < 3; k++ {
		i := codvec[k]
		j := dnSign[i]
		index := int16((int32(i) * 6554) >> 15)
		track := i - index*5
		switch track {
		case 1:
			index <<= 4
		case 2:
			track = 2
			index <<= 8
		case 3:
			track = 1
			index = (index << 4) + 8
		case 4:
			track = 2
			index = (index << 8) + 128
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

	c0, c1, c2 := int(codvec[0]), int(codvec[1]), int(codvec[2])
	for i := 0; i < L; i++ {
		var h0, h1, h2 int16
		if i-c0 >= 0 {
			h0 = h[i-c0]
		}
		if i-c1 >= 0 {
			h1 = h[i-c1]
		}
		if i-c2 >= 0 {
			h2 = h[i-c2]
		}
		s := L_mac(L_mac(L_mult(h0, sgn[0]), h1, sgn[1]), h2, sgn[2])
		y[i] = round_(s)
	}
	return indx, sign
}

// code3i40_14bits runs the full MR67 3-pulse search. Mirrors code_3i40_14bits.
func code3i40_14bits(x, h []int16, T0, pitchSharp int16, code, y []int16) (index, sign int16) {
	sharp := shl(pitchSharp, 1)
	if T0 < cL_CODE {
		for i := T0; i < cL_CODE; i++ {
			h[i] = add(h[i], mult(h[i-T0], sharp))
		}
	}

	var dn, dn2, dnSign [cL_CODE]int16
	var rr [cL_CODE * cL_CODE]int16
	var codvec [3]int16

	corHX(h, x, dn[:], 1)
	setSign(dn[:], dnSign[:], dn2[:], 6)
	corH(h, dnSign[:], rr[:])
	search3i40(dn[:], dn2[:], rr[:], codvec[:])
	index, sign = build3Code(codvec[:], dnSign[:], code, h, y)

	if T0 < cL_CODE {
		for i := T0; i < cL_CODE; i++ {
			code[i] = add(code[i], mult(code[i-T0], sharp))
		}
	}
	return index, sign
}
