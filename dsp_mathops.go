package amrnb

// Higher-level fixed-point math operators, ported function-by-function from the
// opencore-amrnb common sources (log2_norm.cpp, log2.cpp, pow2.cpp,
// inv_sqrt.cpp, sqrt_l.cpp, shr_r.cpp, l_shr_r.cpp). Each uses the ROM tables
// in tables_math.go and is bit-exact with the reference by construction.

// shr_r is shr with rounding: shr(var1, var2) plus 1 if the most significant
// discarded bit is set. Mirrors shr_r.cpp.
func shr_r(var1, var2 int16) int16 {
	if var2 > 15 {
		return 0
	}
	out := shr(var1, var2)
	if var2 > 0 {
		if var1&(int16(1)<<uint(var2-1)) != 0 {
			out = int16(out + 1) // Word16 wraparound, matching the reference
		}
	}
	return out
}

// L_shr_r is L_shr with rounding. Mirrors l_shr_r.cpp.
func L_shr_r(L_var1 int32, var2 int16) int32 {
	if var2 > 31 {
		return 0
	}
	result := L_shr(L_var1, var2)
	if var2 > 0 {
		if L_var1&(int32(1)<<uint(var2-1)) != 0 {
			result = result + 1 // Word32 wraparound
		}
	}
	return result
}

// log2Norm computes Log2 of a pre-normalized value. exp is norm_l of the
// original input; it returns the integer and fractional parts of the base-2
// log. Mirrors Log2_norm (log2_norm.cpp).
func log2Norm(L_x int32, exp int16) (exponent, fraction int16) {
	if L_x <= 0 {
		return 0, 0
	}
	exponent = sub(30, exp)
	L_x = L_shr(L_x, 9)
	i := extract_h(L_x) // b25-b31
	L_x = L_shr(L_x, 1)
	a := extract_l(L_x) & 0x7fff // b10-b24
	i = sub(i, 32)
	L_y := L_deposit_h(log2_tbl[i])
	tmp := sub(log2_tbl[i], log2_tbl[i+1])
	L_y = L_msu(L_y, tmp, a)
	fraction = extract_h(L_y)
	return exponent, fraction
}

// log2 normalizes L_x and returns the integer and fractional parts of its
// base-2 logarithm. Mirrors Log2 (log2.cpp).
func log2(L_x int32) (exponent, fraction int16) {
	exp := norm_l(L_x)
	return log2Norm(L_shl(L_x, exp), exp)
}

// pow2 computes 2^(exponent + fraction) in Q0..Q31. Mirrors Pow2 (pow2.cpp).
func pow2(exponent, fraction int16) int32 {
	L_x := L_mult(fraction, 32) // fraction << 6
	i := extract_h(L_x)
	L_x = L_shr(L_x, 1)
	a := extract_l(L_x) & 0x7fff
	L_x = L_deposit_h(pow2_tbl[i])
	tmp := sub(pow2_tbl[i], pow2_tbl[i+1])
	L_x = L_msu(L_x, tmp, a)
	exp := sub(30, exponent)
	return L_shr_r(L_x, exp)
}

// invSqrt returns an approximation of 1/sqrt(L_x) by table lookup and linear
// interpolation. Mirrors Inv_sqrt (inv_sqrt.cpp).
func invSqrt(L_x int32) int32 {
	if L_x <= 0 {
		return 0x3fffffff
	}
	exp := norm_l(L_x)
	L_x = L_shl(L_x, exp)
	exp = sub(30, exp)
	if exp&1 == 0 { // even exponent -> shift right
		L_x = L_shr(L_x, 1)
	}
	exp = shr(exp, 1)
	exp = add(exp, 1)
	L_x = L_shr(L_x, 9)
	i := extract_h(L_x)
	L_x = L_shr(L_x, 1)
	a := extract_l(L_x) & 0x7fff
	i = sub(i, 16)
	L_y := L_deposit_h(inv_sqrt_tbl[i])
	tmp := sub(inv_sqrt_tbl[i], inv_sqrt_tbl[i+1])
	L_y = L_msu(L_y, tmp, a)
	return L_shr(L_y, exp) // denormalization
}

// sqrtLExp returns sqrt(L_x) together with the right shift (exp) that must be
// applied to the result to denormalize it: the true value is the return value
// shifted right by exp/2. Mirrors sqrt_l_exp (sqrt_l.cpp).
func sqrtLExp(L_x int32) (result int32, exp int16) {
	if L_x <= 0 {
		return 0, 0
	}
	e := norm_l(L_x) &^ 1 // next lower even normalization exponent (clear bit 0)
	L_x = L_shl(L_x, e)
	L_x = L_shr(L_x, 9)
	i := extract_h(L_x) // 16 <= i <= 63
	L_x = L_shr(L_x, 1)
	a := extract_l(L_x) & 0x7fff
	i = sub(i, 16) // 0 <= i <= 47
	L_y := L_deposit_h(sqrt_l_tbl[i])
	tmp := sub(sqrt_l_tbl[i], sqrt_l_tbl[i+1])
	L_y = L_msu(L_y, tmp, a)
	return L_y, e
}
