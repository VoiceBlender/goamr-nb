package amrnb

// Sign-weighted impulse-response autocorrelation matrix, ported from
// opencore-amrnb cor_h.cpp. corH builds rr[i][j] (flattened, row-major, size
// L_CODE*L_CODE): the diagonal holds the truncated energy of the weighted
// impulse response (used to place a pulse at position i), and the off-diagonal
// holds the sign-weighted cross-correlation between candidate pulse positions
// i and j. This matrix drives the per-mode algebraic codebook pulse searches.
// The reference's exact incremental Q15 roundings and pointer walk are
// preserved (bit-exact).

// corH fills the flattened rr (length cL_CODE*cL_CODE) from the weighted impulse
// response h and the per-position signs. Mirrors cor_h.
func corH(h, sign, rr []int16) {
	var h2 [cL_CODE]int16

	// energy of h, and the scaling of h into h2
	s := int32(1)
	for i := 0; i < cL_CODE; i++ {
		s += int32(h[i]) * int32(h[i])
	}
	s <<= 1
	if s < 0 { // overflow -> halve
		for i := 0; i < cL_CODE; i++ {
			h2[i] = h[i] >> 1
		}
	} else {
		s >>= 1
		s = invSqrt(s)
		var dec int16
		if s < 0x00ffffff {
			dec = int16(((s >> 9) * 32440) >> 15)
		} else {
			dec = 32440 // 0.99
		}
		for i := 0; i < cL_CODE; i++ {
			h2[i] = int16((int32(h[i])*int32(dec) + 0x20) >> 6)
		}
	}

	const L = cL_CODE

	// diagonal: rr[L-1-m][L-1-m] = sum_{k=0..m} h2[k]^2
	s = 0
	p := 0
	rr1 := (L-1)*L + (L - 1)
	for i := L >> 1; i != 0; i-- {
		t := int32(h2[p])
		p++
		s += t * t
		rr[rr1] = int16((s + 0x4000) >> 15)
		rr1 -= L + 1
		t = int32(h2[p])
		p++
		s += t * t
		rr[rr1] = int16((s + 0x4000) >> 15)
		rr1 -= L + 1
	}

	// off-diagonal: sign-weighted cross-correlation at each distance dec
	pRrRef1 := (L - 1) * L
	for dec := 1; dec < L; dec += 2 {
		rr1 := pRrRef1 + (L - 1 - dec) // &rr[L-1][L-1-dec]
		rr2 := (L-1-dec)*L + (L - 1)   // &rr[L-1-dec][L-1]
		rr3 := (L-1-(dec+1))*L + (L - 1)

		var s2 int32
		s = 0
		pSign1 := L - 1
		pSign2 := L - 1 - dec
		ph2 := 0
		ph := dec

		for i := L - dec - 1; i != 0; i-- {
			s += int32(h2[ph2]) * int32(h2[ph])
			ph++
			s2 += int32(h2[ph2]) * int32(h2[ph])
			ph2++
			tmp1 := int16((s + 0x4000) >> 15)
			tmp11 := int16((s2 + 0x4000) >> 15)
			tmp2 := int16((int32(sign[pSign1]) * int32(sign[pSign2])) >> 15)
			pSign2--
			tmp22 := int16((int32(sign[pSign1]) * int32(sign[pSign2])) >> 15)
			pSign1--

			rr[rr2] = int16((int32(tmp1) * int32(tmp2)) >> 15)
			rr[rr1] = rr[rr2]
			rr1--
			rr[rr1] = int16((int32(tmp11) * int32(tmp22)) >> 15)
			rr[rr3] = rr[rr1]

			rr1 -= L
			rr2 -= L + 1
			rr3 -= L + 1
		}

		s += int32(h2[ph2]) * int32(h2[ph])
		tmp1 := int16((s + 0x4000) >> 15)
		tmp2 := int16((int32(sign[pSign1]) * int32(sign[pSign2])) >> 15)
		rr[rr1] = int16((int32(tmp1) * int32(tmp2)) >> 15)
		rr[rr2] = rr[rr1]
	}
}
