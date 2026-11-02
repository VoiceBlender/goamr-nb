package amrnb

// Encoder analysis-by-synthesis filter kernels, ported from opencore-amrnb
// convolve.cpp. convolve computes the (truncated) lower-triangular convolution
// y[n] = sum_{i=0..n} x[i]·h[n-i] >> 12, used throughout the closed-loop pitch
// and algebraic codebook searches to filter an excitation by the weighted
// synthesis impulse response. The reference accumulates in non-saturating int32,
// so a straightforward double loop is bit-exact.
func convolve(x, h, y []int16, L int) {
	if useAVX2 && L == cL_SUBFR {
		convolveFIR(x, h, y)
		return
	}
	convolveGeneric(x, h, y, L)
}

// convolveFIR is convolve for a subframe as one 40-tap FIR (firRaw) over x
// with 39 leading zeros and h reversed: dst[n] = sum_t xp[n+t]*h[39-t] =
// sum_{j<=n} x[j]*h[n-j]. It does twice the multiplies of the triangular
// loop, so it only pays where firRaw is vectorized. The int32 sums wrap, as
// in the reference, so their order does not matter.
func convolveFIR(x, h, y []int16) {
	var xp [2*cL_SUBFR - 1]int16
	var hr [cL_SUBFR]int16
	var dst [cL_SUBFR]int32
	copy(xp[cL_SUBFR-1:], x[:cL_SUBFR])
	h = h[:cL_SUBFR]
	for t := range hr {
		hr[t] = h[cL_SUBFR-1-t]
	}
	firRaw(dst[:], xp[:], hr[:])
	y = y[:cL_SUBFR]
	for n, s := range dst {
		y[n] = int16(s >> 12)
	}
}

// convolveGeneric is the scalar convolve, and the oracle convolveFIR is
// tested against.
func convolveGeneric(x, h, y []int16, L int) {
	xf := x[:L] // hoist bounds: i<=n<L and n-i in [0,n]
	hf := h[:L]
	for n := 0; n < L; n++ {
		var s int32
		hn := hf[:n+1] // h[n-i] == hn[n-i], index in [0,n]
		for i := 0; i <= n; i++ {
			s += int32(xf[i]) * int32(hn[n-i])
		}
		y[n] = int16(s >> 12)
	}
}
