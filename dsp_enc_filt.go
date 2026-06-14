package amrnb

// Encoder analysis-by-synthesis filter kernels, ported from opencore-amrnb
// convolve.cpp. convolve computes the (truncated) lower-triangular convolution
// y[n] = sum_{i=0..n} x[i]·h[n-i] >> 12, used throughout the closed-loop pitch
// and algebraic codebook searches to filter an excitation by the weighted
// synthesis impulse response. The reference accumulates in non-saturating int32,
// so a straightforward double loop is bit-exact.
func convolve(x, h, y []int16, L int) {
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
