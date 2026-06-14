package amrnb

// Parameterized codebook-search correlation, ported from opencore-amrnb
// cor_h_x2.cpp. corHX2 is the generalized form of corHX used by the multi-pulse
// searches (MR102: 4 tracks, MR122: 5 tracks): dn[i] = normalized 2·Σ x[j]·h[j-i]
// with a configurable track count and step. Bit-exact with the reference.

const cLOG2_OF_32 = 5

// corHX2 computes the normalized target↔impulse correlation dn[] over nbTrack
// tracks with the given step, scaled by sf. Mirrors cor_h_x2.
func corHX2(h, x, dn []int16, sf, nbTrack, step int16) {
	// y32[i] = 2·sum_{j=i}^{L-1} x[j]·h[j-i] = 2·sum_{j'=0}^{L-1-i} x[i+j']·h[j'].
	// Zero-padding x past L turns this into one L-tap FIR over the padded input
	// (firRaw); s<<1 == dst[i]<<1 holds mod 2^32 since <<1 distributes over the
	// wrapping sum.
	var xpad [2 * cL_CODE]int16
	copy(xpad[:cL_CODE], x[:cL_CODE])
	var dst [cL_CODE]int32
	firRaw(dst[:], xpad[:], h[:cL_CODE])

	var y32 [cL_CODE]int32
	for i := 0; i < cL_CODE; i++ {
		y32[i] = dst[i] << 1
	}

	tot := int32(cLOG2_OF_32)
	for k := int16(0); k < nbTrack; k++ {
		var max int32
		for i := k; i < cL_CODE; i += step {
			if a := L_abs(y32[i]); a > max {
				max = a
			}
		}
		tot += max >> 1
	}

	j := norm_l(tot) - sf
	for i := 0; i < cL_CODE; i++ {
		dn[i] = round_(L_shl(y32[i], j))
	}
}

// setSign12k2 combines the residual cn and the correlation dn to determine each
// position's pulse sign, the per-track maximum positions (posMax), and a cyclic
// pulse-track ordering (ipos, length 2*nbTrack) starting from the strongest
// track — the prep for the multi-pulse search_10and8i40. dn is updated in place
// (negated where the combined sign is negative). Mirrors set_sign12k2.
func setSign12k2(dn, cn, sign, posMax []int16, nbTrack int16, ipos []int16, step int16) {
	const L = cL_CODE
	var en [cL_CODE]int16

	s := int32(256)
	t := int32(256)
	for i := 0; i < L; i++ {
		s = L_mac(s, cn[i], cn[i])
		t += (int32(dn[i]) * int32(dn[i])) << 1
	}
	s = invSqrt(s)
	kCn := int16(L_shl(s, 5) >> 16)
	t = invSqrt(t)
	kDn := int16(t >> 11)

	for i := L - 1; i >= 0; i-- {
		lTemp := (int32(kCn) * int32(cn[i])) << 1
		val := dn[i]
		ss := L_mac(lTemp, kDn, val)
		cor := round_(L_shl(ss, 10))
		if cor >= 0 {
			sign[i] = 32767
		} else {
			sign[i] = -32767
			cor = negate(cor)
			dn[i] = negate(val)
		}
		en[i] = cor
	}

	maxOfAll := int16(-1)
	var pos int16
	for i := int16(0); i < nbTrack; i++ {
		max := int16(-1)
		for j := i; j < L; j += step {
			if en[j] > max {
				max = en[j]
				pos = j
			}
		}
		posMax[i] = pos
		if max > maxOfAll {
			maxOfAll = max
			ipos[0] = i
		}
	}

	pos = ipos[0]
	ipos[nbTrack] = pos
	for i := int16(1); i < nbTrack; i++ {
		pos++
		if pos >= nbTrack {
			pos = 0
		}
		ipos[i] = pos
		ipos[i+nbTrack] = pos
	}
}
