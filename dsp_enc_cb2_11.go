package amrnb

// 2-pulse algebraic codebook search (11 bits, MR59), ported from opencore-amrnb
// c2_11pf.cpp. Like c2_9pf but with a wider track structure (2×4 track pairs)
// and its own index encoding. Reuses corHX/setSign/corH. The exact encoder
// counterpart of decode2i40_11bits. Bit-exact with the reference.

var startPos1 = [2]int16{1, 3}
var startPos2 = [4]int16{0, 1, 2, 4}

// search2i40_11 finds the two pulse positions maximizing ps²/energy across the
// 2×4 track pairs. Mirrors search_2i40 (c2_11pf.cpp).
func search2i40_11(dn, rr, codvec []int16) {
	const L = cL_CODE
	var ipos [2]int16
	psk := int16(-1)
	alpk := int16(1)
	codvec[0] = 0
	codvec[1] = 1

	for track1 := 0; track1 < 2; track1++ {
		for track2 := 0; track2 < 4; track2++ {
			ipos[0] = startPos1[track1]
			ipos[1] = startPos2[track2]

			for i0 := ipos[0]; i0 < L; i0 += cSTEP {
				ps0 := dn[i0]
				alp0 := int32(rr[int(i0)*L+int(i0)]) << 14

				sq := int16(-1)
				alp := int16(1)
				ix := ipos[1]

				for i1 := ipos[1]; i1 < L; i1 += cSTEP {
					ps1 := add(ps0, dn[i1])
					alp1 := alp0 + (int32(rr[int(i1)*L+int(i1)]) << 14)
					alp1 += int32(rr[int(i0)*L+int(i1)]) << 15
					sq1 := int16((int32(ps1) * int32(ps1)) >> 15)
					alp16 := int16((alp1 + 0x8000) >> 16)
					s := (int32(alp)*int32(sq1))<<1 - (int32(sq)*int32(alp16))<<1
					if s > 0 {
						sq = sq1
						alp = alp16
						ix = i1
					}
				}

				s := (int32(alpk)*int32(sq))<<1 - (int32(psk)*int32(alp))<<1
				if s > 0 {
					psk = sq
					alpk = alp
					codvec[0] = i0
					codvec[1] = ix
				}
			}
		}
	}
}

// build2Code11 assembles the innovation, filtered code, signs and the 11-bit
// index. Mirrors build_code (c2_11pf.cpp).
func build2Code11(codvec, dnSign, cod, h, y []int16) (indx, sign int16) {
	const L = cL_CODE
	for i := 0; i < L; i++ {
		cod[i] = 0
	}

	var sgn [2]int16
	rsign := int16(0)
	for k := 0; k < 2; k++ {
		i := codvec[k]
		j := dnSign[i]
		index := int16((int32(i) * 6554) >> 15)
		tw := (index << 3) + (index << 1) // index*10
		tw >>= 1                          // index*5
		track := i - tw                   // i % 5

		switch track {
		case 0:
			track = 1
			index <<= 6
		case 1:
			if k == 0 {
				track = 0
				index <<= 1
			} else {
				track = 1
				index = (index << 6) + 16
			}
		case 2:
			track = 1
			index = (index << 6) + 32
		case 3:
			track = 0
			index = (index << 1) + 1
		case 4:
			track = 1
			index = (index << 6) + 48
		}

		if j > 0 {
			cod[i] = 8191
			sgn[k] = 32767
			rsign = add(rsign, shl(1, track))
		} else {
			cod[i] = -8192
			sgn[k] = -32768
		}
		indx = add(indx, index)
	}
	sign = rsign

	c0, c1 := int(codvec[0]), int(codvec[1])
	for i := 0; i < L; i++ {
		var h0, h1 int16
		if i-c0 >= 0 {
			h0 = h[i-c0]
		}
		if i-c1 >= 0 {
			h1 = h[i-c1]
		}
		s := L_mult(h0, sgn[0])
		s = L_mac(s, h1, sgn[1])
		y[i] = round_(s)
	}
	return indx, sign
}

// code2i40_11bits runs the full MR59 2-pulse search. Mirrors code_2i40_11bits.
func code2i40_11bits(x, h []int16, T0, pitchSharp int16, code, y []int16) (index, sign int16) {
	sharp := shl(pitchSharp, 1)
	if T0 < cL_CODE {
		for i := T0; i < cL_CODE; i++ {
			h[i] = add(h[i], mult(h[i-T0], sharp))
		}
	}

	var dn, dn2, dnSign [cL_CODE]int16
	var rr [cL_CODE * cL_CODE]int16
	var codvec [2]int16

	corHX(h, x, dn[:], 1)
	setSign(dn[:], dnSign[:], dn2[:], 8)
	corH(h, dnSign[:], rr[:])
	search2i40_11(dn[:], rr[:], codvec[:])
	index, sign = build2Code11(codvec[:], dnSign[:], code, h, y)

	if T0 < cL_CODE {
		for i := T0; i < cL_CODE; i++ {
			code[i] = add(code[i], mult(code[i-T0], sharp))
		}
	}
	return index, sign
}
