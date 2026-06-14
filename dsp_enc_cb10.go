package amrnb

// 10-pulse algebraic codebook search (35 bits, MR122), ported from
// opencore-amrnb c1035pf.cpp. Reuses search10and8i40 (10 pulses, step 5), then
// builds the codeword (±4096 pulses) and Gray-encodes the per-track position
// indices (q_p). The exact encoder counterpart of dec10i40_35bits. Bit-exact
// with the reference.

// build10Code assembles the 10-pulse innovation cod, filtered code y, and the
// raw per-track position+sign indices (with the same-track ordering/swap logic
// that the decoder inverts). Mirrors build_code (c1035pf.cpp).
func build10Code(codvec, sign, cod, h, y, indx []int16) {
	const L = cL_CODE
	for i := 0; i < L; i++ {
		cod[i] = 0
	}
	for t := 0; t < 5; t++ {
		indx[t] = -1
	}

	var sgn [10]int16
	for k := 0; k < 10; k++ {
		i := codvec[k]
		index := int16((int32(i) * 6554) >> 15)
		track := i - (index + (index << 2)) // i - index*5

		if sign[i] > 0 {
			cod[i] += 4096
			sgn[k] = 8192
		} else {
			cod[i] -= 4096
			sgn[k] = -8192
			index += 8
		}

		temp := indx[track]
		switch {
		case temp < 0:
			indx[track] = index
		case (index^temp)&8 == 0:
			if temp <= index {
				indx[track+5] = index
			} else {
				indx[track+5] = temp
				indx[track] = index
			}
		default:
			if (temp & 7) <= (index & 7) {
				indx[track+5] = temp
				indx[track] = index
			} else {
				indx[track+5] = index
			}
		}
	}

	for i := 0; i < L; i++ {
		var s int32
		for k := 0; k < 10; k++ {
			ci := int(codvec[k])
			var hv int16
			if i-ci >= 0 {
				hv = h[i-ci]
			}
			s += (int32(hv) * int32(sgn[k])) >> 7
		}
		y[i] = int16((s + 0x80) >> 8)
	}
}

// code10i40_35bits runs the full MR122 10-pulse search, writing the innovation
// cod, filtered code y, and the 10-value Gray-coded index. Mirrors
// code_10i40_35bits.
func code10i40_35bits(x, cn, h, cod, y, indx []int16) {
	var dn, sign [cL_CODE]int16
	var rr [cL_CODE * cL_CODE]int16
	var ipos, codvec [10]int16
	var posMax [5]int16

	corHX(h, x, dn[:], 2)
	setSign12k2(dn[:], cn, sign[:], posMax[:], 5, ipos[:], 5)
	corH(h, sign[:], rr[:])
	search10and8i40(10, 5, 5, dn[:], rr[:], ipos[:], posMax[:], codvec[:])
	build10Code(codvec[:], sign[:], cod, h, y, indx)

	// q_p: Gray-encode each track's position index (keeping the sign bit for the
	// first five).
	for n := 0; n < 10; n++ {
		tmp := indx[n]
		if n < 5 {
			indx[n] = (tmp & 0x8) | gray[tmp&0x7]
		} else {
			indx[n] = gray[tmp&0x7]
		}
	}
}
