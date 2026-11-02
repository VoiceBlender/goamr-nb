package amrnb

// Fixed-point basic operators, ported from the ITU-T / ETSI "basicop2"
// reference (the same primitives used by opencore-amrnb).
//
// Word16 maps to int16, Word32 to int32. All operators saturate to the 16- or
// 32-bit range exactly as the reference does; Go has no implicit integer
// saturation, so every operation that can overflow clamps explicitly. The
// reference's global Overflow/Carry flags are not modelled — no AMR-NB code
// path depends on reading them back.

const (
	maxInt16 = 32767
	minInt16 = -32768
	maxInt32 = 2147483647
	minInt32 = -2147483648
)

// satWord16 clamps a 32-bit value to the int16 range.
func satWord16(v int32) int16 {
	if v > maxInt16 {
		return maxInt16
	}
	if v < minInt16 {
		return minInt16
	}
	return int16(v)
}

// satWord32 clamps a 64-bit value to the int32 range.
func satWord32(v int64) int32 {
	if v > maxInt32 {
		return maxInt32
	}
	if v < minInt32 {
		return minInt32
	}
	return int32(v)
}

// add returns the saturated 16-bit sum a+b.
func add(a, b int16) int16 { return satWord16(int32(a) + int32(b)) }

// sub returns the saturated 16-bit difference a-b.
func sub(a, b int16) int16 { return satWord16(int32(a) - int32(b)) }

// abs_s returns the saturated 16-bit absolute value (|-32768| -> 32767).
func abs_s(a int16) int16 {
	if a == minInt16 {
		return maxInt16
	}
	if a < 0 {
		return -a
	}
	return a
}

// shl returns a shifted left by b bits with saturation; negative b shifts
// right. Mirrors the opencore-amrnb inline shl, which differs from ETSI shl for
// negative shifts: a right shift by 15 or more gives 0, also for negative a.
// The C shifts an int by the count as given, which x86 and arm64 take modulo
// 32, so this does too.
func shl(a, b int16) int16 {
	if b < 0 {
		if -int32(b) < 15 {
			return a >> uint(-b)
		}
		return 0
	}
	s := uint(b) & 31
	out := int16(int32(a) << s)
	if int32(out)>>s != int32(a) {
		return (a >> 15) ^ maxInt16
	}
	return out
}

// shr returns a shifted right by b bits (arithmetic, counts above 15 act as
// 15); negative b shifts left with saturation. Mirrors opencore-amrnb shr.cpp.
func shr(a, b int16) int16 {
	if b == 0 {
		return a
	}
	if b > 0 {
		if b > 15 {
			b = 15
		}
		return a >> uint(b)
	}
	n := -int32(b)
	if n > 15 || n < 0 {
		n = 15
	}
	out := int16(int32(a) << uint(n))
	if out>>uint(n) != a {
		if a > 0 {
			return maxInt16
		}
		return minInt16
	}
	return out
}

// L_shl returns the saturated 32-bit left shift; negative b shifts right
// (by 31 or more gives 0, also for negative a). Mirrors the opencore-amrnb
// inline L_shl; left shift counts are taken modulo 32, as on x86 and arm64.
func L_shl(a int32, b int16) int32 {
	if b > 0 {
		s := uint(b) & 31
		out := a << s
		if out>>s != a {
			return (a >> 31) ^ maxInt32
		}
		return out
	}
	if -int32(b) < 31 {
		return a >> uint(-b)
	}
	return 0
}

// L_shr returns the 32-bit arithmetic right shift (by 31 or more gives 0, also
// for negative a); negative b shifts left with saturation. Mirrors the
// opencore-amrnb inline L_shr.
func L_shr(a int32, b int16) int32 {
	if b > 0 {
		if b < 31 {
			return a >> uint(b)
		}
		return 0
	}
	s := uint(-int32(b)) & 31
	out := a << s
	if out>>s != a {
		return (a >> 31) ^ maxInt32
	}
	return out
}

// mult returns satWord16((a*b)>>15).
func mult(a, b int16) int16 {
	p := (int32(a) * int32(b)) >> 15
	return satWord16(p)
}

// L_mult returns satWord32(a*b<<1) — the 32-bit product with the fractional
// left shift.
func L_mult(a, b int16) int32 {
	p := int32(a) * int32(b)
	if p != 0x40000000 {
		return p << 1
	}
	return maxInt32
}

// L_add returns the saturated 32-bit sum.
func L_add(a, b int32) int32 { return satWord32(int64(a) + int64(b)) }

// L_sub returns the saturated 32-bit difference.
func L_sub(a, b int32) int32 { return satWord32(int64(a) - int64(b)) }

// L_mac returns acc + (a*b<<1), saturated. Mirrors the opencore-amrnb inline
// L_mac, including its two departures from L_add(acc, L_mult(a, b)): a
// -32768*-32768 product gives MAX_32 whatever acc is, and the overflow check is
// skipped when acc equals a*b (so the sum may wrap).
func L_mac(acc int32, a, b int16) int32 {
	p := int32(a) * int32(b)
	if p == 0x40000000 {
		return maxInt32
	}
	sum := p<<1 + acc
	if acc^p > 0 && (sum^acc)>>31 != 0 {
		if acc>>31 != 0 {
			return minInt32
		}
		return maxInt32
	}
	return sum
}

// L_msu returns acc - (a*b<<1), saturated.
func L_msu(acc int32, a, b int16) int32 { return L_sub(acc, L_mult(a, b)) }

// negate returns the saturated 16-bit negation.
func negate(a int16) int16 {
	if a == minInt16 {
		return maxInt16
	}
	return -a
}

// L_negate returns the saturated 32-bit negation.
func L_negate(a int32) int32 {
	if a == minInt32 {
		return maxInt32
	}
	return -a
}

// extract_h returns the high 16 bits of a 32-bit value.
func extract_h(a int32) int16 { return int16(a >> 16) }

// extract_l returns the low 16 bits of a 32-bit value.
func extract_l(a int32) int16 { return int16(a & 0xFFFF) }

// round_ returns the rounded high word: extract_h(L_add(a, 0x8000)).
func round_(a int32) int16 { return extract_h(L_add(a, 0x8000)) }

// mult_r returns the rounded multiply: satWord16(((a*b)+0x4000)>>15).
func mult_r(a, b int16) int16 {
	p := (int32(a)*int32(b) + 0x4000) >> 15
	return satWord16(p)
}

// L_deposit_h places a 16-bit value in the high half of a 32-bit word.
func L_deposit_h(a int16) int32 { return int32(a) << 16 }

// L_deposit_l sign-extends a 16-bit value into a 32-bit word.
func L_deposit_l(a int16) int32 { return int32(a) }

// norm_l returns the number of left shifts that normalize a (0 for 0).
// Mirrors opencore-amrnb norm_l.cpp, which normalizes |a| rather than ^a for
// negative a, and so returns one less than ETSI norm_l for a = -2^k.
func norm_l(a int32) int16 {
	if a == 0 {
		return 0
	}
	y := a - b2i32(a < 0)
	y ^= y >> 31
	var n int16
	for y&0x40000000 == 0 {
		y <<= 1
		n++
	}
	return n
}

// norm_s returns the number of left shifts that normalize a 16-bit value (0
// for 0). Mirrors opencore-amrnb norm_s.cpp (see norm_l).
func norm_s(a int16) int16 {
	if a == 0 {
		return 0
	}
	y := a - int16(b2i32(a < 0))
	y ^= y >> 15
	var n int16
	for y&0x4000 == 0 {
		y <<= 1
		n++
	}
	return n
}

func b2i32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// div_s returns the fractional division (a/b)<<15 for 0 <= a <= b, b != 0.
func div_s(a, b int16) int16 {
	if a > b || a <= 0 {
		return 0 // opencore-amrnb div_s.cpp: no division for a > b or a < 0
	}
	if a == b {
		return maxInt16
	}
	// The reference's 15-step restoring division computes floor(a*2^15 / b).
	return int16((int32(a) << 15) / int32(b))
}
