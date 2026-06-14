package amrnb

// 2-pulse algebraic codebook search (9 bits, MR475/MR515), ported from
// opencore-amrnb c2_9pf.cpp. code2i40_9bits sharpens the impulse response with
// the pitch gain, builds the correlation gradient (corHX) and the rr criterion
// matrix (corH), exhaustively searches the two pulse positions maximizing
// ps²/energy (search2i40), then assembles the innovation vector, its filtered
// version, and the codebook index (build2Code). The exact encoder counterpart
// of the decoder's decode2i40_9bits. Bit-exact with the reference.

// per-subframe track table used to encode the pulse track-pair (c2_9pf.cpp).
var trackTable = [20]int16{
	0, 1, 0, 1, -1, // subframe 1
	0, -1, 1, 0, 1, // subframe 2
	0, 1, 0, -1, 1, // subframe 3
	0, 1, -1, 0, 1, // subframe 4
}

// search2i40 finds the two pulse positions (one per track of a track pair) that
// maximize the codebook criterion ps²/energy. Mirrors search_2i40.
func search2i40(subNr int16, dn, rr, codvec []int16) {
	const L = cL_CODE
	var ipos [2]int16
	psk := int16(-1)
	alpk := int16(1)
	codvec[0] = 0
	codvec[1] = 1

	for track1 := int16(0); track1 < 2; track1++ {
		i := (subNr << 1) + (track1 << 3)
		ipos[0] = startPos[i]
		ipos[1] = startPos[i+1]

		for i0 := ipos[0]; i0 < L; i0 += cSTEP {
			ps0 := dn[i0]
			alp0 := int32(rr[int(i0)*L+int(i0)]) << 14

			sq := int16(-1)
			alp := int16(1)
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

// build2Code assembles the innovation vector cod (two ±8191/∓8192 pulses), its
// filtered version y, the pulse signs, and the codebook index. Mirrors
// build_code (c2_9pf.cpp).
func build2Code(subNr int16, codvec, dnSign, cod, h, y []int16) (indx, sign int16) {
	const L = cL_CODE
	ptBase := int(subNr) + (int(subNr) << 2) // subNr*5
	for i := 0; i < L; i++ {
		cod[i] = 0
	}

	var sgn [2]int16
	rsign := int16(0)
	for k := 0; k < 2; k++ {
		i := codvec[k]
		j := dnSign[i]
		index := int16((int32(i) * 6554) >> 15) // pos/5
		track := i - 5*index
		first := trackTable[ptBase+int(track)]
		if k == 0 {
			track = 0
			if first != 0 {
				index += 64
			}
		} else {
			track = 1
			index <<= 3
		}
		if j > 0 {
			cod[i] = 8191
			sgn[k] = 32767
			rsign += 1 << uint(track)
		} else {
			cod[i] = -8192
			sgn[k] = -32768
		}
		indx += index
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

// code2i40_9bits runs the full 2-pulse search for one subframe, returning the
// codebook index and pulse signs and writing the innovation code and its
// filtered version y. h is sharpened in place by the pitch gain. Mirrors
// code_2i40_9bits.
func code2i40_9bits(subNr int16, x, h []int16, T0, pitchSharp int16, code, y []int16) (index, sign int16) {
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
	search2i40(subNr, dn[:], rr[:], codvec[:])
	index, sign = build2Code(subNr, codvec[:], dnSign[:], code, h, y)

	if T0 < cL_CODE {
		for i := T0; i < cL_CODE; i++ {
			code[i] = add(code[i], mult(code[i-T0], sharp))
		}
	}
	return index, sign
}
