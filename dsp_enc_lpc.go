package amrnb

// Encoder LP analysis chain, ported from opencore-amrnb pre_proc.cpp
// (Pre_Process), autocorr.cpp (Autocorr), lag_wind.cpp (Lag_window) and
// levinson.cpp (Levinson), plus the 32-bit operators they need (Mpy_32, Div_32,
// L_abs). Together these turn 8 kHz PCM into the order-10 LP coefficients A(z):
// input high-pass + scaling, windowed autocorrelation, lag-window bandwidth
// expansion, then the Levinson-Durbin recursion. Bit-exact with the reference.

// L_abs returns the saturated 32-bit absolute value (|MIN_32| -> MAX_32).
// Mirrors L_abs.
func L_abs(x int32) int32 {
	if x == minInt32 {
		return maxInt32
	}
	if x < 0 {
		return -x
	}
	return x
}

// mpy_32 multiplies two 32-bit values given in (hi,lo) double-precision form.
// Mirrors the generic Mpy_32.
func mpy_32(hi1, lo1, hi2, lo2 int16) int32 {
	L := L_mult(hi1, hi2)
	L = L_add(L, ((int32(hi1)*int32(lo2))>>15)<<1)
	L = L_add(L, ((int32(lo1)*int32(hi2))>>15)<<1)
	return L
}

// div_32 divides a 32-bit numerator by a 32-bit denominator (given as hi,lo).
// Mirrors Div_32.
func div_32(num int32, denomHi, denomLo int16) int32 {
	approx := div_s(0x3fff, denomHi)
	result := mpy32_16(denomHi, denomLo, approx)
	result = 0x7fffffff - result
	hi, lo := L_Extract(result)
	result = mpy32_16(hi, lo, approx)
	hi, lo = L_Extract(result)
	nHi, nLo := L_Extract(num)
	result = mpy_32(nHi, nLo, hi, lo)
	return L_shl(result, 2)
}

// preProcessState holds the encoder input high-pass biquad memory.
type preProcessState struct {
	x0, x1                 int16
	y1hi, y1lo, y2hi, y2lo int16
}

// process applies the input high-pass filter (with 0.5 scaling) in place over
// lg samples. Mirrors Pre_Process.
func (st *preProcessState) process(signal []int16, lg int) {
	xn2 := st.x1
	xn1 := st.x0
	for i := 0; i < lg; i++ {
		L := int32(st.y1hi) * 7807
		L += (int32(st.y1lo) * 7807) >> 15
		L += int32(st.y2hi) * -3733
		st.y2hi = st.y1hi
		L += (int32(st.y2lo) * -3733) >> 15
		st.y2lo = st.y1lo
		L += int32(xn2) * 1899
		xn2 = xn1
		L += int32(xn1) * -3798
		xn1 = signal[i]
		L += int32(xn1) * 1899

		signal[i] = int16((L + 0x800) >> 12)
		st.y1hi = int16(L >> 12)
		st.y1lo = int16((L << 3) - (int32(st.y1hi) << 15))
	}
	st.x1 = xn2
	st.x0 = xn1
}

// autocorr windows x with wind and computes the order-m autocorrelation into
// (rh,rl) double precision, returning the normalization shift applied. Mirrors
// Autocorr.
func autocorr(x []int16, m int, rh, rl, wind []int16) int16 {
	var y [cL_WINDOW]int16

	var sum int32
	overflow := false
	b := 0
	for ; b < cL_WINDOW; b++ {
		temp := int16((int32(x[b])*int32(wind[b]) + 0x4000) >> 15)
		y[b] = temp
		sum += (int32(temp) * int32(temp)) << 1
		if sum < 0 {
			overflow = true
			break
		}
	}

	overflShft := int16(0)
	if overflow {
		for k := b + 1; k < cL_WINDOW; k++ {
			y[k] = int16((int32(x[k])*int32(wind[k]) + 0x4000) >> 15)
		}
		for {
			overflShft += 4
			sum = 0
			for k := 0; k < cL_WINDOW; k++ {
				t := y[k] >> 2
				y[k] = t
				sum += (int32(t) * int32(t)) << 1
			}
			if sum > 0 {
				break
			}
		}
	}

	sum++ // avoid all-zeros
	norm := norm_l(sum)
	sum <<= uint(norm)
	rh[0] = int16(sum >> 16)
	rl[0] = int16((sum >> 1) - (int32(rh[0]) << 15))

	for i := 1; i <= m; i++ {
		// sum_{n=i}^{W-1} y[n]·y[n-i] = sum_k y[i+k]·y[k] (wrapping int32) via firDot
		s := firDot(y[i:cL_WINDOW], y[:cL_WINDOW-i])
		s <<= uint(norm + 1)
		rh[i] = int16(s >> 16)
		rl[i] = int16((s >> 1) - (int32(rh[i]) << 15))
	}

	return norm - overflShft
}

// lagWindow applies the lag-window bandwidth expansion to the autocorrelation
// in place. Mirrors Lag_window.
func lagWindow(m int, rh, rl []int16) {
	for i := 1; i <= m; i++ {
		x := mpy_32(rh[i], rl[i], lag_h[i-1], lag_l[i-1])
		rh[i] = int16(x >> 16)
		rl[i] = int16((x >> 1) - (int32(rh[i]) << 15))
	}
}

// levinsonState carries the last stable A(z) for the unstable-filter fallback.
type levinsonState struct {
	oldA [cMP1]int16
}

// levinson runs the Levinson-Durbin recursion on the (rh,rl) autocorrelation,
// producing order-10 LP coefficients A[0:M+1] and the first 4 reflection
// coefficients rc[0:4]. Mirrors Levinson.
func (st *levinsonState) levinson(rh, rl, a, rc []int16) {
	var ah, al, anh, anl [cMP1]int16

	t1 := (int32(rh[1]) << 16) + (int32(rl[1]) << 1)
	t2 := L_abs(t1)
	t0 := div_32(t2, rh[0], rl[0]) // R[1]/R[0]
	if t1 > 0 {
		t0 = L_negate(t0)
	}
	kh, kl := L_Extract(t0)
	rc[0] = round_(t0)

	t0 >>= 4
	ah[1] = int16(t0 >> 16)
	al[1] = int16((t0 >> 1) - (int32(ah[1]) << 15))

	t0 = mpy_32(kh, kl, kh, kl) // K*K
	t0 = L_abs(t0)
	t0 = 0x7fffffff - t0 // 1 - K*K
	hi, lo := L_Extract(t0)
	t0 = mpy_32(rh[0], rl[0], hi, lo) // Alpha
	alpExp := norm_l(t0)
	t0 = t0 << uint(alpExp)
	alpH := int16(t0 >> 16)
	alpL := int16((t0 >> 1) - (int32(alpH) << 15))

	for i := 2; i <= cM; i++ {
		t0 = 0
		for j := 1; j < i; j++ {
			t0 += (int32(rh[j]) * int32(al[i-j])) >> 15
			t0 += (int32(rl[j]) * int32(ah[i-j])) >> 15
			t0 += int32(rh[j]) * int32(ah[i-j])
		}
		t0 <<= 5
		t1 = (int32(rh[i]) << 16) + (int32(rl[i]) << 1)
		t0 += t1

		t1 = L_abs(t0)
		t2 = div_32(t1, alpH, alpL)
		if t0 > 0 {
			t2 = L_negate(t2)
		}
		t2 = L_shl(t2, alpExp)
		kh = int16(t2 >> 16)
		kl = int16((t2 >> 1) - (int32(kh) << 15))

		if i < 5 {
			rc[i-1] = int16((t2 + 0x8000) >> 16)
		}
		if abs_s(kh) > 32750 { // unstable filter -> reuse last A(z)
			copy(a[:cMP1], st.oldA[:])
			for k := 0; k < 4; k++ {
				rc[k] = 0
			}
			return
		}

		for j := 1; j < i; j++ {
			t0 = (int32(kh) * int32(al[i-j])) >> 15
			t0 += (int32(kl) * int32(ah[i-j])) >> 15
			t0 += int32(kh) * int32(ah[i-j])
			t0 += (int32(ah[j]) << 15) + int32(al[j])
			anh[j] = int16(t0 >> 15)
			anl[j] = int16(t0 - (int32(anh[j]) << 15))
		}
		anh[i] = int16(t2 >> 20)
		anl[i] = int16((t2 >> 5) - (int32(anh[i]) << 15))

		t0 = mpy_32(kh, kl, kh, kl) // K*K
		t0 = L_abs(t0)
		t0 = 0x7fffffff - t0 // 1 - K*K
		hi = int16(t0 >> 16)
		lo = int16((t0 >> 1) - (int32(hi) << 15))

		t0 = (int32(alpH) * int32(lo)) >> 15
		t0 += (int32(alpL) * int32(hi)) >> 15
		t0 += int32(alpH) * int32(hi)
		t0 <<= 1
		jn := norm_l(t0)
		t0 = t0 << uint(jn)
		alpH = int16(t0 >> 16)
		alpL = int16((t0 >> 1) - (int32(alpH) << 15))
		alpExp += jn

		copy(ah[1:1+i], anh[1:1+i])
		copy(al[1:1+i], anl[1:1+i])
	}

	a[0] = 4096
	for i := 1; i <= cM; i++ {
		t0 = (int32(ah[i]) << 15) + int32(al[i])
		a[i] = int16((t0 + 0x2000) >> 14)
		st.oldA[i] = a[i]
	}
}
