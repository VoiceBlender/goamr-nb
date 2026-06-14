package amrnb

// Algebraic-codebook search infrastructure, ported from opencore-amrnb
// cor_h_x.cpp (cor_h_x) and set_sign.cpp (set_sign). corHX computes the
// backward correlation dn[] between the search target and the weighted impulse
// response (the gradient that drives pulse placement); setSign extracts the
// per-position pulse signs and prunes each track to its strongest candidate
// positions. These feed the per-mode pulse searches. Bit-exact with the
// reference.

const (
	cNB_TRACK = 5 // number of pulse tracks
	cSTEP     = 5 // codebook track step
)

// corHX computes dn[i] = normalized 2·sum_j x[i+j]·h[j], the correlation between
// the target x and the impulse response h, scaled so it fits 16 bits (sf is the
// scaling factor: 2 for 12.2k, 1 otherwise). Mirrors cor_h_x.
func corHX(h, x, dn []int16, sf int16) {
	// y32[i] = 2·sum_{j=0}^{L-1-i} x[i+j]·h[j]. Zero-padding x past L makes the
	// out-of-range taps contribute 0, so the whole set of positions is one L-tap
	// FIR over the padded input (firRaw). The per-term <<1 distributes over the
	// wrapping sum, so dst[i]<<1 is bit-identical to the per-term shift.
	var xpad [2 * cL_CODE]int16
	copy(xpad[:cL_CODE], x[:cL_CODE])
	var dst [cL_CODE]int32
	firRaw(dst[:], xpad[:], h[:cL_CODE])

	var y32 [cL_CODE]int32
	for i := 0; i < cL_CODE; i++ {
		y32[i] = dst[i] << 1
	}

	tot := int32(5)
	for k := 0; k < cNB_TRACK; k++ {
		var max int32
		for i := k; i < cL_CODE; i += cSTEP {
			s := y32[i]
			if s < 0 {
				s = -s
			}
			if s > max {
				max = s
			}
		}
		tot += max >> 1
	}

	j := norm_l(tot) - sf
	for i := 0; i < cL_CODE; i++ {
		s := L_shl(y32[i], j)
		dn[i] = int16((s + 0x8000) >> 16)
	}
}

// setSign records the sign of each dn[] entry (±32767), takes |dn|, and prunes
// dn2[] to keep only the n strongest positions per track (the rest set to -1).
// Mirrors set_sign.
func setSign(dn, sign, dn2 []int16, n int16) {
	for i := 0; i < cL_CODE; i++ {
		val := dn[i]
		if val >= 0 {
			sign[i] = 32767
		} else {
			sign[i] = -32767
			val = negate(val)
		}
		dn[i] = val
		dn2[i] = val
	}

	for i := 0; i < cNB_TRACK; i++ {
		for k := int16(0); k < 8-n; k++ {
			min := int16(0x7fff)
			pos := i
			for j := i; j < cL_CODE; j += cSTEP {
				if dn2[j] >= 0 && dn2[j] < min {
					min = dn2[j]
					pos = j
				}
			}
			dn2[pos] = -1
		}
	}
}
